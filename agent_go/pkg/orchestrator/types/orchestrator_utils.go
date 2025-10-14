package types

import (
	"context"
	"fmt"

	"mcp-agent/agent_go/internal/utils"
	"mcp-agent/agent_go/pkg/mcpagent"
	"mcp-agent/agent_go/pkg/orchestrator/agents"

	"github.com/tmc/langchaingo/llms"
)

// EventBridge defines the interface for event bridges
type EventBridge interface {
	mcpagent.AgentEventListener
}

// OrchestratorConfig holds common configuration for both orchestrators
type OrchestratorConfig struct {
	Provider        string
	Model           string
	MCPConfigPath   string
	Temperature     float64
	SelectedServers []string
	AgentMode       string
	Logger          utils.ExtendedLogger
}

// AgentSetupConfig holds configuration for agent setup
type AgentSetupConfig struct {
	AgentType    string
	AgentName    string
	MaxTurns     int
	AgentMode    string
	OutputFormat agents.OutputFormat
}

// OrchestratorUtils provides shared utilities for both orchestrators
type OrchestratorUtils struct {
	config *OrchestratorConfig
	// Map of agent names to their context-aware event bridges
	agentBridges map[string]*ContextAwareEventBridge
}

// newOrchestratorUtils creates a new orchestrator utilities instance
func newOrchestratorUtils(config *OrchestratorConfig) *OrchestratorUtils {
	return &OrchestratorUtils{
		config:       config,
		agentBridges: make(map[string]*ContextAwareEventBridge),
	}
}

// SharedLLMConfig represents detailed LLM configuration from frontend
type SharedLLMConfig struct {
	Provider              string
	ModelID               string
	FallbackModels        []string
	CrossProviderFallback *agents.CrossProviderFallback
}

// createAgentConfig creates a generic agent configuration
func (ou *OrchestratorUtils) createAgentConfig(setupConfig *AgentSetupConfig) *agents.OrchestratorAgentConfig {
	config := agents.NewOrchestratorAgentConfig(setupConfig.AgentType, setupConfig.AgentName)
	config.Provider = ou.config.Provider
	config.Model = ou.config.Model
	config.MCPConfigPath = ou.config.MCPConfigPath
	config.Temperature = 0.0 // Fixed temperature for all agents
	config.MaxTurns = setupConfig.MaxTurns
	config.ToolChoice = "auto"
	config.CacheOnly = true // Use cache-only mode for smart routing optimization
	config.ServerNames = ou.config.SelectedServers
	config.Mode = agents.AgentMode(setupConfig.AgentMode)
	config.OutputFormat = setupConfig.OutputFormat
	config.MaxRetries = 3
	config.Timeout = 300 // Same timeout for all agents
	config.RateLimit = 60
	return config
}

// createAgentConfigWithLLM creates a generic agent configuration with detailed LLM config
func (ou *OrchestratorUtils) createAgentConfigWithLLM(setupConfig *AgentSetupConfig, llmConfig *SharedLLMConfig) *agents.OrchestratorAgentConfig {
	config := agents.NewOrchestratorAgentConfig(setupConfig.AgentType, setupConfig.AgentName)

	// Use detailed LLM configuration from frontend if available
	llmProvider := ou.config.Provider
	llmModel := ou.config.Model
	// Temperature is always 0.0 for all agents
	llmTemp := 0.0

	if llmConfig != nil {
		llmProvider = llmConfig.Provider
		llmModel = llmConfig.ModelID
		ou.config.Logger.Infof("🔧 Using detailed LLM config for %s agent - Provider: %s, Model: %s",
			setupConfig.AgentType, llmProvider, llmModel)
	}

	config.Provider = llmProvider
	config.Model = llmModel
	config.Temperature = llmTemp // Always 0.0
	config.MCPConfigPath = ou.config.MCPConfigPath
	config.MaxTurns = setupConfig.MaxTurns
	config.ToolChoice = "auto"
	config.CacheOnly = true // Use cache-only mode for smart routing optimization
	config.ServerNames = ou.config.SelectedServers
	config.Mode = agents.AgentMode(setupConfig.AgentMode)
	config.OutputFormat = setupConfig.OutputFormat
	config.MaxRetries = 3
	config.Timeout = 300 // Same timeout for all agents
	config.RateLimit = 60

	// Detailed LLM configuration from frontend
	if llmConfig != nil {
		config.FallbackModels = llmConfig.FallbackModels
		config.CrossProviderFallback = llmConfig.CrossProviderFallback
	}

	return config
}

// setupAgent performs common agent setup tasks
func (ou *OrchestratorUtils) setupAgent(
	agent agents.OrchestratorAgent,
	agentType, agentName string,
	customTools []llms.Tool,
	customToolExecutors map[string]interface{},
	eventBridge EventBridge,
	setContextFunc func(string, int, string), // Function to set orchestrator context
) error {
	ctx := context.Background()
	if err := agent.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize %s: %w", agentName, err)
	}

	// Connect event bridge and emit actual agent events
	if eventBridge != nil {
		ou.config.Logger.Infof("🔍 Checking agent structure for %s", agentName)

		baseAgent := agent.GetBaseAgent()
		if baseAgent == nil {
			ou.config.Logger.Infof("ℹ️ Agent %s is a pure orchestrator (no BaseAgent) - skipping agent event connection", agentName)
		} else {
			ou.config.Logger.Infof("✅ GetBaseAgent() returned non-nil for %s", agentName)

			mcpAgent := baseAgent.Agent()
			if mcpAgent == nil {
				ou.config.Logger.Warnf("⚠️ baseAgent.Agent() returned nil for %s", agentName)
			} else {
				ou.config.Logger.Infof("✅ baseAgent.Agent() returned non-nil for %s", agentName)

				// Create a context-aware event bridge for this sub-agent
				// We need to create a wrapper that implements mcpagent.AgentEventListener
				contextAwareBridge := NewContextAwareEventBridge(eventBridge, ou.config.Logger)
				contextAwareBridge.SetOrchestratorContext(agentType, 0, 0, agentName)

				// Connect the event bridge to receive detailed agent events
				mcpAgent.AddEventListener(contextAwareBridge)
				ou.config.Logger.Infof("✅ Context-aware bridge connected to %s", agentName)

				// Store the bridge reference for later context updates
				ou.agentBridges[agentName] = contextAwareBridge
				ou.config.Logger.Infof("✅ Context-aware bridge stored for %s", agentName)

				// CRITICAL FIX: Update the agent's event bridge to use the context-aware bridge
				// This ensures orchestrator-level events also get context metadata
				if baseOrchestratorAgent, ok := agent.(interface {
					SetEventBridge(bridge interface{})
				}); ok {
					baseOrchestratorAgent.SetEventBridge(contextAwareBridge)
					ou.config.Logger.Infof("✅ Updated agent event bridge to context-aware bridge for %s", agentName)
				} else {
					ou.config.Logger.Warnf("⚠️ Agent %s does not support SetEventBridge method", agentName)
				}

				// Note: StartAgentSession is now handled at orchestrator level to avoid duplicate events
				ou.config.Logger.Infof("ℹ️ Skipping StartAgentSession for %s - handled at orchestrator level", agentName)

				// Check if the agent has streaming capability
				if mcpAgent.HasStreamingCapability() {
					ou.config.Logger.Infof("✅ Agent %s has streaming capability", agentName)
				} else {
					ou.config.Logger.Warnf("⚠️ Agent %s does not have streaming capability", agentName)
				}
			}
		}
	}

	// Set workflow context if function provided
	if setContextFunc != nil {
		setContextFunc(agentType, 0, agentName) // Step 0 as default
	}

	// Register custom tools
	if customTools != nil && customToolExecutors != nil {
		if baseAgent := agent.GetBaseAgent(); baseAgent != nil {
			if mcpAgent := baseAgent.Agent(); mcpAgent != nil {
				ou.config.Logger.Infof("🔧 Registering %d custom tools for %s agent (%s mode)", len(customTools), agentName, baseAgent.GetMode())

				for _, tool := range customTools {
					if executor, exists := customToolExecutors[tool.Function.Name]; exists {
						// Type assert parameters to map[string]interface{}
						params, ok := tool.Function.Parameters.(map[string]interface{})
						if !ok {
							ou.config.Logger.Warnf("Warning: Failed to convert parameters for tool %s", tool.Function.Name)
							continue
						}

						// Type assert executor to function type
						if toolExecutor, ok := executor.(func(ctx context.Context, args map[string]interface{}) (string, error)); ok {
							mcpAgent.RegisterCustomTool(
								tool.Function.Name,
								tool.Function.Description,
								params,
								toolExecutor,
							)
						} else {
							ou.config.Logger.Warnf("Warning: Failed to convert executor for tool %s", tool.Function.Name)
						}
					}
				}

				ou.config.Logger.Infof("✅ All custom tools registered for %s agent (%s mode)", agentName, baseAgent.GetMode())
			} else {
				ou.config.Logger.Warnf("⚠️ %s base agent has no MCP agent", agentName)
			}
		} else {
			ou.config.Logger.Warnf("⚠️ %s has no base agent", agentName)
		}
	}

	return nil
}

// UpdateAgentContext updates the context-aware event bridge for a specific agent
func (ou *OrchestratorUtils) UpdateAgentContext(agentName, phase string, stepIndex, iteration int) {
	if bridge, exists := ou.agentBridges[agentName]; exists {
		bridge.SetOrchestratorContext(phase, stepIndex, iteration, agentName)
		ou.config.Logger.Infof("🎯 Updated context for %s: %s (step %d, iteration %d)", agentName, phase, stepIndex+1, iteration+1)
	} else {
		ou.config.Logger.Warnf("⚠️ No context-aware bridge found for agent: %s", agentName)
	}
}

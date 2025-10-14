package agents

import (
	"context"
	"mcp-agent/agent_go/internal/utils"

	"github.com/tmc/langchaingo/llms"
)

// AgentTemplate enforces consistent execution and event patterns for all orchestrator agents
// All agents MUST embed this struct to ensure they follow the same execution pattern
type AgentTemplate struct {
	baseAgent *BaseAgent
	config    *OrchestratorAgentConfig
	logger    utils.ExtendedLogger
}

// NewAgentTemplate creates a new agent template with the required components
func NewAgentTemplate(
	baseAgent *BaseAgent,
	config *OrchestratorAgentConfig,
	logger utils.ExtendedLogger,
) *AgentTemplate {
	return &AgentTemplate{
		baseAgent: baseAgent,
		config:    config,
		logger:    logger,
	}
}

// Execute is the ONLY allowed execution path for all agents
// This method enforces consistent event emission and error handling
// Agents CANNOT override this method - they must use it
func (at *AgentTemplate) Execute(ctx context.Context, templateVars map[string]string, conversationHistory []llms.MessageContent) (string, error) {
	// ALWAYS goes through BaseAgent.Execute() which handles:
	// ✅ Automatic event emission (start, end, error)
	// ✅ Consistent error handling
	// ✅ Performance monitoring
	// ✅ LLM execution
	// ✅ Agent type-specific event emission
	return at.baseAgent.Execute(ctx, templateVars, conversationHistory)
}

// GetType returns the agent type from the base agent
func (at *AgentTemplate) GetType() string {
	return string(at.baseAgent.GetType())
}

// Initialize initializes the base agent
func (at *AgentTemplate) Initialize(ctx context.Context) error {
	// Base agent is already initialized during creation
	return nil
}

// Close closes the base agent and cleans up resources
func (at *AgentTemplate) Close() error {
	if at.baseAgent != nil {
		return at.baseAgent.Close()
	}
	return nil
}

// Event system - now handled by unified events system

// GetBaseAgent returns the base agent for event listener attachment
func (at *AgentTemplate) GetBaseAgent() *BaseAgent {
	return at.baseAgent
}

// GetConfig returns the agent configuration
func (at *AgentTemplate) GetConfig() *OrchestratorAgentConfig {
	return at.config
}

// GetLogger returns the logger
func (at *AgentTemplate) GetLogger() utils.ExtendedLogger {
	return at.logger
}

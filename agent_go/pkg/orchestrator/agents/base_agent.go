package agents

import (
	"context"
	"fmt"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	mcpagent "github.com/manishiitg/mcpagent/agent"
	internalLLM "github.com/manishiitg/mcpagent/llm"
	"github.com/manishiitg/mcpagent/mcpclient"
	"github.com/manishiitg/mcpagent/observability"

	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const orchestratorIDKey contextKey = "orchestrator_id"

// AgentMode represents the mode of operation for an agent
type AgentMode string

const (
	SimpleAgent AgentMode = "simple"
)

// AgentType represents the type of agent
type AgentType string

const (
	// Multi-agent TodoPlanner sub-agents (actively used)
	TodoPlannerEvaluationDebuggerAgentType  AgentType = "todo_planner_evaluation_debugger"  // Analyzes evaluation execution and provides feedback for evaluation plan improvement
	TodoPlannerExecutionQAAgentType         AgentType = "todo_planner_execution_qa"         // Answers questions about execution results (read-only)
	TodoPlannerPlanningAgentType            AgentType = "todo_planner_planning"             // Creates step-wise plan from objective
	TodoPlannerExecutionAgentType           AgentType = "todo_planner_execution"            // Executes first step of plan
	TodoPlannerSuccessLearningAgentType     AgentType = "todo_planner_success_learning"     // Analyzes successful executions to capture best practices
	TodoPlannerLearningDetectionAgentType   AgentType = "todo_planner_learning_detection"   // Detects if new learnings were generated after learning phase
	ConditionalAgentType                    AgentType = "conditional"                       // Conditional decision agent for evaluating step conditions
	EvaluationScoringAgentType              AgentType = "evaluation_scoring"                // Calculates scores for evaluation steps based on success criteria
	TodoTaskOrchestratorAgentType           AgentType = "todo_task_orchestrator"            // TodoTask orchestrator agent that manages todo lists and delegates to sub-agents
	GenericExecutionAgentType               AgentType = "generic_execution"                 // Generic execution agent for todo task steps (no learning, no prevalidation)
	TodoPlannerInteractiveWorkshopAgentType AgentType = "todo_planner_interactive_workshop" // Interactive workshop: execute steps, edit plan, update step config in one session
)

// BaseAgentInterface defines the interface for base agent operations
type BaseAgentInterface interface {
	// Core execution
	Execute(ctx context.Context, userMessage string, conversationHistory []llmtypes.MessageContent, systemPrompt string, overwriteSystemPrompt bool) (string, []llmtypes.MessageContent, error)

	// Agent information
	GetType() AgentType
	GetName() string
	GetInstructions() string
	GetMode() AgentMode
	GetServerNames() []string

	// Resource management
	Close() error

	// Event system - now handled by unified events system

	// Workflow support
	GetWorkflowContext() map[string]interface{}
	SetWorkflowContext(context map[string]interface{})
	GetPreviousAgentOutput() string
	SetPreviousAgentOutput(output string)

	// MCP agent access
	Agent() *mcpagent.Agent
}

// BaseAgent provides comprehensive functionality for all orchestrator agents
type BaseAgent struct {
	// Core identity
	agentType AgentType
	name      string

	// Core functionality
	agent        *mcpagent.Agent
	instructions string
	mode         AgentMode
	serverNames  []string
	llm          llmtypes.Model

	// Observability
	tracer  observability.Tracer
	traceID observability.TraceID
	logger  loggerv2.Logger

	// Event system - now handled by unified events system

	// Workflow context
	workflowContext     map[string]interface{}
	previousAgentOutput string

	// Configuration
	configPath   string
	modelID      string
	temperature  float64
	toolChoice   string
	maxTurns     int
	provider     string
	mcpSessionID string
	toolPolicy   mcpagent.ToolPolicy
	definition   mcpagent.AgentDefinition
	runtime      mcpagent.RuntimeConfig
	lastHandle   *mcpagent.AgentSessionHandle
}

// NewBaseAgent creates a new BaseAgent instance with comprehensive functionality
func NewBaseAgent(
	ctx context.Context,
	agentType AgentType,
	name string,
	llm llmtypes.Model,
	instructions string,
	serverNames []string,
	directTools []mcpagent.ToolDefinition,
	selectedTools []string, // NEW parameter
	useCodeExecutionMode bool, // NEW parameter
	mode AgentMode,
	tracer observability.Tracer,
	traceID observability.TraceID,
	configPath string,
	modelID string,
	temperature float64,
	toolChoice string,
	maxTurns int,
	provider string,
	logger loggerv2.Logger,
	cacheOnly bool,
	enableContextOffloading *bool, // Context offloading configuration
	largeOutputThreshold int, // Token threshold for context offloading (0 = use default: 10000)
	enableContextSummarization bool, // Context summarization configuration
	summarizeOnTokenThreshold bool, // Enable token-based summarization trigger
	tokenThresholdPercent float64, // Percentage of context window to trigger summarization
	summarizeOnFixedTokenThreshold bool, // Enable fixed token-based summarization trigger
	fixedTokenThreshold int, // Fixed token threshold to trigger summarization
	summaryKeepLastMessages int, // Number of recent messages to keep when summarizing
	enableContextEditing bool, // Context editing configuration
	contextEditingThreshold int, // Token threshold for context editing (0 = use default)
	contextEditingTurnThreshold int, // Turn age threshold for context editing (0 = use default)
	enableParallelToolExecution bool, // Parallel tool execution configuration
	llmConfig *LLMConfig, // NEW: Full LLM configuration
	apiKeys *AgentAPIKeys, // API keys for providers
	mcpSessionID string, // MCP session ID for connection sharing across agents
	codingAgentWorkingDir string, // CLI coding-agent process working directory
	codingAgentKeepAlive bool, // Keep tmux-backed coding-agent sessions alive after this agent completes
	forceStructuredCodingAgent bool, // Force structured JSON transport for coding-agent CLIs (overrides tmux default)
	isolateCodingAgentWorkspace bool, // Run the coding-CLI session in a fresh tmp dir (workflow steps only; chat keeps user dir)
	cliSecurityPolicy *llmtypes.CLISecurityPolicy, // Server-resolved immutable CLI security policy
	runtimeOverrides mcpclient.RuntimeOverrides, // Runtime config overrides for MCP servers (e.g., output directories)
) (*BaseAgent, error) {
	// Convert AgentMode to mcpagent.AgentMode
	// All agents use Simple mode
	var mcpMode mcpagent.AgentMode = mcpagent.SimpleAgent

	// Prepare agent options
	agentOptions := []mcpagent.AgentOption{
		mcpagent.WithMode(mcpMode),
		mcpagent.WithTemperature(temperature),
		mcpagent.WithToolChoice(toolChoice),
		mcpagent.WithMaxTurns(maxTurns),
		mcpagent.WithProvider(internalLLM.Provider(provider)),
	}

	// Add LLM config if provided
	if llmConfig != nil {
		// Convert orchestrator LLMConfig to mcpagent AgentLLMConfiguration
		mcpConfig := mcpagent.AgentLLMConfiguration{
			Primary: mcpagent.LLMModel{
				Provider: llmConfig.Primary.Provider,
				ModelID:  llmConfig.Primary.ModelID,
				APIKey:   llmConfig.Primary.APIKey,
				Region:   llmConfig.Primary.Region,
				Options:  llmConfig.Primary.Options,
			},
			Fallbacks: make([]mcpagent.LLMModel, len(llmConfig.Fallbacks)),
		}
		for i, fb := range llmConfig.Fallbacks {
			mcpConfig.Fallbacks[i] = mcpagent.LLMModel{
				Provider: fb.Provider,
				ModelID:  fb.ModelID,
				APIKey:   fb.APIKey,
				Region:   fb.Region,
				Options:  fb.Options,
			}
		}
		agentOptions = append(agentOptions, mcpagent.WithLLMConfig(mcpConfig))
	}

	// Note: API keys are now extracted directly from the LLM instance
	// via extractAPIKeysFromLLM() in mcpagent, so no need to pass them explicitly

	// Add selected servers for "all tools" mode determination
	if len(serverNames) > 0 {
		agentOptions = append(agentOptions, mcpagent.WithSelectedServers(serverNames))
	}

	// Add selected tools if provided
	if len(selectedTools) > 0 {
		agentOptions = append(agentOptions, mcpagent.WithSelectedTools(selectedTools))
	}

	if useCodeExecutionMode {
		agentOptions = append(agentOptions, mcpagent.WithCodeExecutionMode(true))
	}

	// Add context offloading option if specified
	// Default to true if nil (backward compatible)
	contextOffloadingEnabled := true
	if enableContextOffloading != nil {
		contextOffloadingEnabled = *enableContextOffloading
	}
	agentOptions = append(agentOptions, mcpagent.WithContextOffloading(contextOffloadingEnabled))

	// Add large output threshold if specified (0 = use default: 10000 tokens)
	if largeOutputThreshold > 0 {
		agentOptions = append(agentOptions, mcpagent.WithLargeOutputThreshold(largeOutputThreshold))
	}

	// Add context summarization configuration
	if enableContextSummarization {
		agentOptions = append(agentOptions, mcpagent.WithContextSummarization(true))
		if summarizeOnTokenThreshold {
			agentOptions = append(agentOptions, mcpagent.WithSummarizeOnTokenThreshold(true, tokenThresholdPercent))
		}
		if summarizeOnFixedTokenThreshold && fixedTokenThreshold > 0 {
			agentOptions = append(agentOptions, mcpagent.WithSummarizeOnFixedTokenThreshold(true, fixedTokenThreshold))
		}
		if summaryKeepLastMessages > 0 {
			agentOptions = append(agentOptions, mcpagent.WithSummaryKeepLastMessages(summaryKeepLastMessages))
		}
	}

	// Add context editing configuration
	if enableContextEditing {
		agentOptions = append(agentOptions, mcpagent.WithContextEditing(true))
		if contextEditingThreshold > 0 {
			agentOptions = append(agentOptions, mcpagent.WithContextEditingThreshold(contextEditingThreshold))
		}
		if contextEditingTurnThreshold > 0 {
			agentOptions = append(agentOptions, mcpagent.WithContextEditingTurnThreshold(contextEditingTurnThreshold))
		}
	}

	// Add parallel tool execution if enabled
	if enableParallelToolExecution {
		agentOptions = append(agentOptions, mcpagent.WithParallelToolExecution(true))
	}

	// Removed verbose logging

	// Use logger directly (already loggerv2.Logger)
	v2Logger := logger

	// Determine server name (join multiple servers with comma, or use first server, or AllServers)
	// NewAgentConnection supports comma-separated server names to connect to multiple servers
	serverName := mcpclient.AllServers
	if len(serverNames) > 0 {
		if len(serverNames) == 1 {
			serverName = serverNames[0]
		} else {
			// Multiple servers: join with comma for NewAgentConnection
			serverName = strings.Join(serverNames, ",")
		}
	}

	// Build options from parameters
	options := agentOptions
	if serverName != "" && serverName != mcpclient.AllServers {
		options = append(options, mcpagent.WithServerName(serverName))
	}
	if tracer != nil {
		options = append(options, mcpagent.WithTracer(tracer))
	}
	if traceID != "" {
		options = append(options, mcpagent.WithTraceID(traceID))
	}
	if v2Logger != nil {
		options = append(options, mcpagent.WithLogger(v2Logger))
	}

	// Add MCP session ID for connection sharing across agents in the same workflow
	// When set, connections are stored in a session registry and reused
	if mcpSessionID != "" {
		options = append(options, mcpagent.WithSessionID(mcpSessionID))
		logger.Info("🔗 Using MCP session for connection sharing",
			loggerv2.String("session_id", mcpSessionID),
			loggerv2.String("agent_name", name))
	}

	if workingDir := strings.TrimSpace(codingAgentWorkingDir); workingDir != "" {
		options = append(options, mcpagent.WithCodingAgentWorkingDir(workingDir))
		logger.Info("🔗 Using coding-agent working directory",
			loggerv2.String("working_dir", workingDir),
			loggerv2.String("agent_name", name))
	}
	if codingAgentKeepAlive {
		options = append(options,
			mcpagent.WithClaudeCodePersistentInteractiveSession(true),
			mcpagent.WithCodexPersistentInteractiveSession(true),
			mcpagent.WithCursorPersistentInteractiveSession(true),
			mcpagent.WithPiPersistentInteractiveSession(true),
		)
		logger.Info("🔗 Keeping tmux-backed coding-agent session alive after completion",
			loggerv2.String("agent_name", name))
	}
	if isolateCodingAgentWorkspace {
		options = append(options, mcpagent.WithIsolatedSessionWorkspace(true))
		logger.Info("🔒 Isolating coding-agent session in a fresh tmp dir (workflow-step isolation)",
			loggerv2.String("agent_name", name))
	}
	if cliSecurityPolicy != nil {
		options = append(options, mcpagent.WithCLISecurityPolicy(*cliSecurityPolicy))
		logger.Info("🔒 Using server-resolved coding-agent CLI security policy",
			loggerv2.String("mode", string(cliSecurityPolicy.Mode)),
			loggerv2.String("agent_name", name))
	}
	if forceStructuredCodingAgent {
		options = append(options, mcpagent.WithCodingAgentTransport(internalLLM.CodingAgentTransportStructured))
		logger.Info("🔧 Structured JSON transport selected for coding-agent CLIs (step transport=structured)",
			loggerv2.String("agent_name", name))
	}

	// Enable provider streaming for workflow-step agents so the
	// synthetic terminal (driven by opts.StreamChan) can emit
	// terminal pane snapshots for API-provider steps. Without this
	// the agent runs in non-streaming mode and the StreamChan is
	// never attached — the terminal pane stays empty for every
	// non-tmux step. WithGenerationStreamingEvents(false) keeps
	// per-token chat events out of the workflow event store; the
	// terminal-chunk carve-out in mcpagent's processChunks still
	// emits the terminal snapshots regardless.
	options = append(options,
		mcpagent.WithStreaming(true),
		mcpagent.WithGenerationStreamingEvents(false),
	)

	// Add runtime overrides for workflow-specific MCP server configuration
	// e.g., setting unique output directories per workflow run
	if runtimeOverrides != nil {
		options = append(options, mcpagent.WithRuntimeOverrides(runtimeOverrides))
		logger.Info("🔧 Using runtime overrides for MCP servers",
			loggerv2.String("agent_name", name),
			loggerv2.Int("overrides_count", len(runtimeOverrides)))
	}

	mcpSources := make([]mcpagent.MCPToolSource, 0, len(serverNames))
	for _, serverName := range serverNames {
		if name := strings.TrimSpace(serverName); name != "" {
			mcpSources = append(mcpSources, mcpagent.MCPToolSource{Name: name})
		}
	}

	// Create the agent from one identity value. Runtime options remain on the
	// compatibility path while sessions/events are migrated, but instructions,
	// direct tools, and MCP sources are now fixed before construction.
	definition := mcpagent.AgentDefinition{
		Instructions: instructions,
		Tools: mcpagent.ToolSet{
			Direct: directTools,
			MCP:    mcpSources,
		},
	}
	runtime := mcpagent.RuntimeConfig{
		Model:         llm,
		MCPConfigPath: configPath,
		LegacyOptions: options,
	}
	agent, err := mcpagent.NewAgentFromDefinition(ctx, definition, runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	return &BaseAgent{
		agent:        agent,
		name:         name,
		agentType:    agentType,
		logger:       logger,
		tracer:       tracer,
		traceID:      traceID,
		instructions: instructions,
		mode:         mode,
		serverNames:  serverNames,
		llm:          llm,
		configPath:   configPath,
		modelID:      modelID,
		temperature:  temperature,
		toolChoice:   toolChoice,
		maxTurns:     maxTurns,
		provider:     provider,
		mcpSessionID: mcpSessionID,
		definition:   definition,
		runtime:      runtime,
	}, nil
}

// Execute executes the agent with user message and conversation history
func (ba *BaseAgent) Execute(ctx context.Context, userMessage string, conversationHistory []llmtypes.MessageContent, systemPrompt string, overwriteSystemPrompt bool) (string, []llmtypes.MessageContent, error) {
	// Removed verbose logging

	// Set or append system prompt if provided
	if systemPrompt != "" {
		if err := ba.ApplyInstructions(ctx, systemPrompt, overwriteSystemPrompt); err != nil {
			return "", nil, err
		}
	}

	startTime := time.Now()

	// Execute the agent with orchestrator context and conversation history
	orchestratorCtx := context.WithValue(ctx, orchestratorIDKey, fmt.Sprintf("%s_%s_%d", ba.agentType, ba.name, time.Now().UnixNano()))
	if ba.mcpSessionID != "" {
		// Continuation turns that call BaseAgent.Execute directly must still preserve
		// the per-agent MCP session so workspace tools resolve the correct session-
		// scoped folder guard instead of falling back to the parent workflow guard.
		orchestratorCtx = context.WithValue(orchestratorCtx, common.ChatSessionIDKey, ba.mcpSessionID)
	}
	result, err := ba.agent.Run(orchestratorCtx, mcpagent.Turn{
		Input:      userMessage,
		History:    conversationHistory,
		ToolPolicy: ba.toolPolicy,
	})
	ba.lastHandle = result.Handle

	executionTime := time.Since(startTime)

	if err != nil {
		return "", nil, fmt.Errorf("agent execution failed: %w", err)
	}

	// Removed verbose logging
	_ = executionTime

	return result.Text, result.History, nil
}

// ApplyInstructions creates a new immutable agent identity when a caller needs
// a different prompt. Provider continuation is carried through the opaque
// handle; the existing Agent is never mutated in place.
func (ba *BaseAgent) ApplyInstructions(ctx context.Context, systemPrompt string, overwrite bool) error {
	if strings.TrimSpace(systemPrompt) == "" {
		return nil
	}
	nextInstructions := systemPrompt
	if !overwrite && strings.TrimSpace(ba.definition.Instructions) != "" {
		nextInstructions = ba.definition.Instructions + "\n\n" + systemPrompt
	}
	if nextInstructions == ba.definition.Instructions {
		return nil
	}

	nextDefinition := ba.definition
	nextDefinition.Instructions = nextInstructions
	return ba.replaceDefinition(ctx, nextDefinition, false)
}

// ApplyIdentity rebuilds the agent with additional construction-time skills
// and prompt supplements. It exists for workflow factories that resolve these
// inputs after their base configuration is loaded but before the first turn.
func (ba *BaseAgent) ApplyIdentity(ctx context.Context, skills []*llmtypes.Skill, supplements ...string) error {
	nextDefinition := ba.definition
	nextDefinition.Skills = append(append([]*llmtypes.Skill(nil), nextDefinition.Skills...), skills...)
	for _, supplement := range supplements {
		if strings.TrimSpace(supplement) == "" {
			continue
		}
		if strings.TrimSpace(nextDefinition.Instructions) == "" {
			nextDefinition.Instructions = supplement
		} else {
			nextDefinition.Instructions += "\n\n" + supplement
		}
	}
	return ba.replaceDefinition(ctx, nextDefinition, false)
}

func (ba *BaseAgent) replaceDefinition(ctx context.Context, nextDefinition mcpagent.AgentDefinition, force bool) error {
	if !force && nextDefinition.Instructions == ba.definition.Instructions && len(nextDefinition.Skills) == len(ba.definition.Skills) {
		return nil
	}
	nextRuntime := ba.runtime
	nextRuntime.ResumeHandle = ba.lastHandle
	nextAgent, err := mcpagent.NewAgentFromDefinition(ctx, nextDefinition, nextRuntime)
	if err != nil {
		return fmt.Errorf("rebuild agent identity: %w", err)
	}
	nextAgent.PromptLogLabel = ba.agent.PromptLogLabel
	nextAgent.APIKeys = ba.agent.APIKeys
	ba.agent.Close()
	ba.agent = nextAgent
	ba.definition = nextDefinition
	ba.instructions = nextDefinition.Instructions
	return nil
}

// Resume reconstructs the runtime around persisted continuation state without
// mutating the existing Agent instance.
func (ba *BaseAgent) Resume(ctx context.Context, handle *mcpagent.AgentSessionHandle) error {
	if handle == nil || handle.Empty() {
		return nil
	}
	ba.lastHandle = handle
	return ba.replaceDefinition(ctx, ba.definition, true)
}

// SessionHandle returns the latest opaque continuation state produced by Run.
func (ba *BaseAgent) SessionHandle() *mcpagent.AgentSessionHandle {
	return ba.lastHandle
}

// SetToolPolicy applies a runtime authorization view to subsequent turns. It
// does not mutate the agent definition or its request-time tool registry.
func (ba *BaseAgent) SetToolPolicy(toolNames []string) {
	ba.toolPolicy = mcpagent.ToolPolicy{AllowedTools: append([]string(nil), toolNames...)}
}

// Agent returns the underlying MCP agent
func (ba *BaseAgent) Agent() *mcpagent.Agent {
	return ba.agent
}

// GetName returns the agent name
func (ba *BaseAgent) GetName() string {
	return ba.name
}

// GetType returns the agent type
func (ba *BaseAgent) GetType() AgentType {
	return ba.agentType
}

// GetInstructions returns the agent instructions
func (ba *BaseAgent) GetInstructions() string {
	return ba.instructions
}

// GetMode returns the agent mode
func (ba *BaseAgent) GetMode() AgentMode {
	return ba.mode
}

// GetServerNames returns the server names
func (ba *BaseAgent) GetServerNames() []string {
	return ba.serverNames
}

// Close closes the agent
func (ba *BaseAgent) Close() error {
	if ba.agent != nil {
		ba.agent.Close()
	}
	return nil
}

// GetWorkflowContext returns the workflow context
func (ba *BaseAgent) GetWorkflowContext() map[string]interface{} {
	return ba.workflowContext
}

// SetWorkflowContext sets the workflow context
func (ba *BaseAgent) SetWorkflowContext(context map[string]interface{}) {
	ba.workflowContext = context
}

// GetPreviousAgentOutput returns the previous agent output
func (ba *BaseAgent) GetPreviousAgentOutput() string {
	return ba.previousAgentOutput
}

// SetPreviousAgentOutput sets the previous agent output
func (ba *BaseAgent) SetPreviousAgentOutput(output string) {
	ba.previousAgentOutput = output
}

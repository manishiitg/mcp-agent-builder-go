package config

import (
	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/pkg/orchestrator/agents"
)

// OrchestratorConfig contains all configuration for the orchestrator
type OrchestratorConfig struct {
	// LLM Configuration
	ModelID     string  `json:"model_id"`
	Temperature float64 `json:"temperature"`
	ToolChoice  string  `json:"tool_choice"`
	MaxTurns    int     `json:"max_turns"`
	Provider    string  `json:"provider"` // Add provider field

	// MCP Server Configuration
	ConfigPath  string   `json:"config_path"`
	ServerNames []string `json:"server_names"`

	// Agent Configuration
	AgentMode agents.AgentMode `json:"agent_mode"`

	// Observability Configuration
	Tracer  observability.Tracer  `json:"-"`
	TraceID observability.TraceID `json:"-"`

	// Planning Agent Specific Configuration
	PlanningInstructions string `json:"planning_instructions"`
}

// NewOrchestratorConfig creates a new orchestrator configuration with defaults
func NewOrchestratorConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		ModelID:              "claude-3-sonnet-20240229",
		Temperature:          0.1,
		ToolChoice:           "auto",
		MaxTurns:             100,               // Planning agent uses 100 turns
		AgentMode:            agents.ReActAgent, // Planning agent uses ReAct mode for better reasoning
		PlanningInstructions: "You are a planning agent responsible for creating detailed execution plans. Analyze objectives and create comprehensive plans with clear steps, dependencies, and success criteria.",
	}
}

// SetModelID sets the model ID
func (oc *OrchestratorConfig) SetModelID(modelID string) *OrchestratorConfig {
	oc.ModelID = modelID
	return oc
}

// SetTemperature sets the temperature
func (oc *OrchestratorConfig) SetTemperature(temperature float64) *OrchestratorConfig {
	oc.Temperature = temperature
	return oc
}

// SetToolChoice sets the tool choice
func (oc *OrchestratorConfig) SetToolChoice(toolChoice string) *OrchestratorConfig {
	oc.ToolChoice = toolChoice
	return oc
}

// SetMaxTurns sets the max turns
func (oc *OrchestratorConfig) SetMaxTurns(maxTurns int) *OrchestratorConfig {
	oc.MaxTurns = maxTurns
	return oc
}

// SetProvider sets the provider
func (oc *OrchestratorConfig) SetProvider(provider string) *OrchestratorConfig {
	oc.Provider = provider
	return oc
}

// SetConfigPath sets the MCP server config path
func (oc *OrchestratorConfig) SetConfigPath(configPath string) *OrchestratorConfig {
	oc.ConfigPath = configPath
	return oc
}

// SetServerNames sets the available server names
func (oc *OrchestratorConfig) SetServerNames(serverNames []string) *OrchestratorConfig {
	oc.ServerNames = serverNames
	return oc
}

// SetAgentMode sets the agent mode
func (oc *OrchestratorConfig) SetAgentMode(mode agents.AgentMode) *OrchestratorConfig {
	oc.AgentMode = mode
	return oc
}

// SetPlanningInstructions sets the planning agent instructions
func (oc *OrchestratorConfig) SetPlanningInstructions(instructions string) *OrchestratorConfig {
	oc.PlanningInstructions = instructions
	return oc
}

// SetTracer sets the observability tracer
func (oc *OrchestratorConfig) SetTracer(tracer observability.Tracer) *OrchestratorConfig {
	oc.Tracer = tracer
	return oc
}

// SetTraceID sets the trace ID
func (oc *OrchestratorConfig) SetTraceID(traceID observability.TraceID) *OrchestratorConfig {
	oc.TraceID = traceID
	return oc
}

// Validate validates the configuration
func (oc *OrchestratorConfig) Validate() error {
	if oc.ModelID == "" {
		return ErrMissingModelID
	}
	if oc.ConfigPath == "" {
		return ErrMissingConfigPath
	}
	if len(oc.ServerNames) == 0 {
		return ErrMissingServerNames
	}
	if oc.Tracer == nil {
		return ErrMissingTracer
	}
	return nil
}

// Common configuration errors
var (
	ErrMissingModelID     = &ConfigError{Field: "model_id", Message: "Model ID is required"}
	ErrMissingConfigPath  = &ConfigError{Field: "config_path", Message: "Config path is required"}
	ErrMissingServerNames = &ConfigError{Field: "server_names", Message: "At least one server name is required"}
	ErrMissingTracer      = &ConfigError{Field: "tracer", Message: "Tracer is required"}
)

// ConfigError represents a configuration error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}

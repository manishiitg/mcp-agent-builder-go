package external

import (
	"time"
)

// Structured event types that mirror the internal MCP agent events
// These provide type-safe access to event data without requiring internal imports

// BaseEventData contains common fields for all events
type BaseEventData struct {
	Timestamp     time.Time `json:"timestamp"`
	TraceID       string    `json:"trace_id,omitempty"`
	SpanID        string    `json:"span_id,omitempty"`
	EventID       string    `json:"event_id,omitempty"`
	ParentID      string    `json:"parent_id,omitempty"`
	IsEndEvent    bool      `json:"is_end_event,omitempty"`
	CorrelationID string    `json:"correlation_id,omitempty"`
}

// ToolCallStartEvent represents the start of a tool execution
type ToolCallStartEvent struct {
	BaseEventData
	ToolName   string `json:"tool_name"`
	Turn       int    `json:"turn"`
	ServerName string `json:"server_name"`
	Arguments  string `json:"arguments"`
}

func (e *ToolCallStartEvent) GetEventType() string { return "tool_call_start" }

// ToolCallEndEvent represents the successful completion of a tool execution
type ToolCallEndEvent struct {
	BaseEventData
	ToolName   string        `json:"tool_name"`
	Turn       int           `json:"turn"`
	ServerName string        `json:"server_name"`
	Result     string        `json:"result"`
	Duration   time.Duration `json:"duration"`
}

func (e *ToolCallEndEvent) GetEventType() string { return "tool_call_end" }

// ToolCallErrorEvent represents a failed tool execution
type ToolCallErrorEvent struct {
	BaseEventData
	ToolName   string        `json:"tool_name"`
	Turn       int           `json:"turn"`
	ServerName string        `json:"server_name"`
	Error      string        `json:"error"`
	Duration   time.Duration `json:"duration"`
}

func (e *ToolCallErrorEvent) GetEventType() string { return "tool_call_error" }

// LLMGenerationStartEvent represents the start of LLM generation
type LLMGenerationStartEvent struct {
	BaseEventData
	Turn          int     `json:"turn"`
	ModelID       string  `json:"model_id"`
	Temperature   float64 `json:"temperature"`
	ToolsCount    int     `json:"tools_count"`
	MessagesCount int     `json:"messages_count"`
}

func (e *LLMGenerationStartEvent) GetEventType() string { return "llm_generation_start" }

// LLMGenerationEndEvent represents the completion of LLM generation
type LLMGenerationEndEvent struct {
	BaseEventData
	Turn             int           `json:"turn"`
	Content          string        `json:"content"`
	ToolCalls        int           `json:"tool_calls"`
	Duration         time.Duration `json:"duration"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	TotalTokens      int           `json:"total_tokens"`
}

func (e *LLMGenerationEndEvent) GetEventType() string { return "llm_generation_end" }

// AgentStartEvent removed - no longer needed

// AgentEndEvent removed - no longer needed

// ConversationStartEvent removed - now using unified events package
// ConversationEndEvent removed - now using unified events package
// ConversationErrorEvent removed - now using unified events package
// ConversationTurnEvent removed - now using unified events package

// TokenUsageEvent represents token usage information
type TokenUsageEvent struct {
	BaseEventData
	Turn             int           `json:"turn"`
	Operation        string        `json:"operation"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	TotalTokens      int           `json:"total_tokens"`
	ModelID          string        `json:"model_id"`
	Provider         string        `json:"provider"`
	CostEstimate     float64       `json:"cost_estimate"`
	Duration         time.Duration `json:"duration"`
	Context          string        `json:"context"`
}

func (e *TokenUsageEvent) GetEventType() string { return "token_usage" }

// ReActReasoningStartEvent represents the start of ReAct reasoning
type ReActReasoningStartEvent struct {
	BaseEventData
	Turn     int    `json:"turn"`
	Question string `json:"question"`
}

func (e *ReActReasoningStartEvent) GetEventType() string { return "react_reasoning_start" }

// ReActReasoningStepEvent represents a single ReAct reasoning step
type ReActReasoningStepEvent struct {
	BaseEventData
	Turn        int    `json:"turn"`
	StepNumber  int    `json:"step_number"`
	Thought     string `json:"thought"`
	Action      string `json:"action,omitempty"`
	Observation string `json:"observation,omitempty"`
	Conclusion  string `json:"conclusion,omitempty"`
	StepType    string `json:"step_type"`
	Content     string `json:"content"`
}

func (e *ReActReasoningStepEvent) GetEventType() string { return "react_reasoning_step" }

// ReActReasoningFinalEvent represents the final ReAct reasoning step
type ReActReasoningFinalEvent struct {
	BaseEventData
	Turn        int    `json:"turn"`
	FinalAnswer string `json:"final_answer"`
	Content     string `json:"content"`
	Reasoning   string `json:"reasoning"`
}

func (e *ReActReasoningFinalEvent) GetEventType() string { return "react_reasoning_final" }

// ReActReasoningEndEvent represents the completion of ReAct reasoning
type ReActReasoningEndEvent struct {
	BaseEventData
	Turn           int    `json:"turn"`
	FinalAnswer    string `json:"final_answer"`
	TotalSteps     int    `json:"total_steps"`
	ReasoningChain string `json:"reasoning_chain"`
}

func (e *ReActReasoningEndEvent) GetEventType() string { return "react_reasoning_end" }

// SystemPromptEvent represents system prompt information
type SystemPromptEvent struct {
	BaseEventData
	Content string `json:"content"`
	Turn    int    `json:"turn"`
}

func (e *SystemPromptEvent) GetEventType() string { return "system_prompt" }

// UserMessageEvent represents user message information
type UserMessageEvent struct {
	BaseEventData
	Content string `json:"content"`
	Turn    int    `json:"turn"`
}

func (e *UserMessageEvent) GetEventType() string { return "user_message" }

// LargeToolOutputDetectedEvent represents detection of large tool output
type LargeToolOutputDetectedEvent struct {
	BaseEventData
	ToolName   string `json:"tool_name"`
	OutputSize int    `json:"output_size"`
	Threshold  int    `json:"threshold"`
}

func (e *LargeToolOutputDetectedEvent) GetEventType() string { return "large_tool_output_detected" }

// LargeToolOutputFileWrittenEvent represents writing large tool output to file
type LargeToolOutputFileWrittenEvent struct {
	BaseEventData
	ToolName string `json:"tool_name"`
	FilePath string `json:"file_path"`
	Size     int    `json:"size"`
}

func (e *LargeToolOutputFileWrittenEvent) GetEventType() string {
	return "large_tool_output_file_written"
}

// FallbackModelUsedEvent represents model fallback information
type FallbackModelUsedEvent struct {
	BaseEventData
	OriginalModel string `json:"original_model"`
	FallbackModel string `json:"fallback_model"`
	Reason        string `json:"reason"`
}

func (e *FallbackModelUsedEvent) GetEventType() string { return "fallback_model_used" }

// ThrottlingDetectedEvent represents throttling detection
type ThrottlingDetectedEvent struct {
	BaseEventData
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	RetryAfter string `json:"retry_after"`
}

func (e *ThrottlingDetectedEvent) GetEventType() string { return "throttling_detected" }

// TokenLimitExceededEvent represents token limit exceeded
type TokenLimitExceededEvent struct {
	BaseEventData
	Limit     int    `json:"limit"`
	Requested int    `json:"requested"`
	Model     string `json:"model"`
}

func (e *TokenLimitExceededEvent) GetEventType() string { return "token_limit_exceeded" }

// MaxTurnsReachedEvent represents max turns reached
type MaxTurnsReachedEvent struct {
	BaseEventData
	MaxTurns int    `json:"max_turns"`
	Message  string `json:"message"`
}

func (e *MaxTurnsReachedEvent) GetEventType() string { return "max_turns_reached" }

// ContextCancelledEvent represents context cancellation
type ContextCancelledEvent struct {
	BaseEventData
	Reason string `json:"reason"`
}

func (e *ContextCancelledEvent) GetEventType() string { return "context_cancelled" }

// MCPServerConnectionEvent represents MCP server connection
type MCPServerConnectionEvent struct {
	BaseEventData
	ServerName string `json:"server_name"`
	Status     string `json:"status"`
}

func (e *MCPServerConnectionEvent) GetEventType() string { return "mcp_server_connection" }

// MCPServerDiscoveryEvent represents MCP server discovery
type MCPServerDiscoveryEvent struct {
	BaseEventData
	ServerName string `json:"server_name"`
	ToolsCount int    `json:"tools_count"`
}

func (e *MCPServerDiscoveryEvent) GetEventType() string { return "mcp_server_discovery" }

// MCPServerSelectionEvent represents MCP server selection
type MCPServerSelectionEvent struct {
	BaseEventData
	ServerName string `json:"server_name"`
	ToolName   string `json:"tool_name"`
}

func (e *MCPServerSelectionEvent) GetEventType() string { return "mcp_server_selection" }

// TypedEventData interface is already defined in typed_events.go

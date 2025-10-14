package llm

import (
	"fmt"
	"strings"
	"time"

	"mcp-agent/agent_go/internal/observability"
)

// LLM Event Types - Constants for event type names
const (
	EventTypeLLMInitializationStart   = "llm_initialization_start"
	EventTypeLLMInitializationSuccess = "llm_initialization_success"
	EventTypeLLMInitializationError   = "llm_initialization_error"
	EventTypeLLMGenerationStart       = "llm_generation_start"
	EventTypeLLMGenerationSuccess     = "llm_generation_success"
	EventTypeLLMGenerationError       = "llm_generation_error"
)

// LLM Operation Types - Constants for operation names
const (
	OperationLLMInitialization = "llm_initialization"
	OperationLLMGeneration     = "llm_generation"
	OperationLLMToolCalling    = "llm_tool_calling"
)

// LLM Status Types - Constants for status values
const (
	StatusLLMInitialized = "initialized"
	StatusLLMFailed      = "failed"
	StatusLLMSuccess     = "success"
	StatusLLMInProgress  = "in_progress"
)

// LLM Capabilities - Constants for capability strings
const (
	CapabilityTextGeneration = "text_generation"
	CapabilityToolCalling    = "tool_calling"
	CapabilityStreaming      = "streaming"
)

// TokenUsage represents token consumption information
type TokenUsage struct {
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	TotalTokens  int    `json:"total_tokens,omitempty"`
	Unit         string `json:"unit,omitempty"`
	Cost         string `json:"cost,omitempty"`
}

// LLMMetadata represents common metadata for LLM events
type LLMMetadata struct {
	ModelVersion     string            `json:"model_version,omitempty"`
	MaxTokens        int               `json:"max_tokens,omitempty"`
	TopP             float64           `json:"top_p,omitempty"`
	FrequencyPenalty float64           `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64           `json:"presence_penalty,omitempty"`
	StopSequences    []string          `json:"stop_sequences,omitempty"`
	User             string            `json:"user,omitempty"`
	CustomFields     map[string]string `json:"custom_fields,omitempty"`
}

// LLMInitializationStartEvent represents the start of LLM initialization
type LLMInitializationStartEvent struct {
	ModelID     string      `json:"model_id"`
	Temperature float64     `json:"temperature"`
	Provider    string      `json:"provider"`
	Operation   string      `json:"operation"`
	Timestamp   time.Time   `json:"timestamp"`
	TraceID     string      `json:"trace_id"`
	Metadata    LLMMetadata `json:"metadata,omitempty"`
}

// GetModelID returns the model ID
func (e *LLMInitializationStartEvent) GetModelID() string { return e.ModelID }

// GetProvider returns the provider name
func (e *LLMInitializationStartEvent) GetProvider() string { return e.Provider }

// GetTimestamp returns the event timestamp
func (e *LLMInitializationStartEvent) GetTimestamp() time.Time { return e.Timestamp }

// GetTraceID returns the trace ID
func (e *LLMInitializationStartEvent) GetTraceID() string { return e.TraceID }

// LLMInitializationSuccessEvent represents successful LLM initialization
type LLMInitializationSuccessEvent struct {
	ModelID      string      `json:"model_id"`
	Provider     string      `json:"provider"`
	Status       string      `json:"status"`
	Capabilities []string    `json:"capabilities"`
	Timestamp    time.Time   `json:"timestamp"`
	TraceID      string      `json:"trace_id"`
	Metadata     LLMMetadata `json:"metadata,omitempty"`
}

// GetModelID returns the model ID
func (e *LLMInitializationSuccessEvent) GetModelID() string { return e.ModelID }

// GetProvider returns the provider name
func (e *LLMInitializationSuccessEvent) GetProvider() string { return e.Provider }

// GetTimestamp returns the event timestamp
func (e *LLMInitializationSuccessEvent) GetTimestamp() time.Time { return e.Timestamp }

// GetTraceID returns the trace ID
func (e *LLMInitializationSuccessEvent) GetTraceID() string { return e.TraceID }

// LLMInitializationErrorEvent represents failed LLM initialization
type LLMInitializationErrorEvent struct {
	ModelID   string      `json:"model_id"`
	Provider  string      `json:"provider"`
	Operation string      `json:"operation"`
	Error     string      `json:"error"`
	ErrorType string      `json:"error_type"`
	Status    string      `json:"status"`
	Timestamp time.Time   `json:"timestamp"`
	TraceID   string      `json:"trace_id"`
	Metadata  LLMMetadata `json:"metadata,omitempty"`
}

// GetModelID returns the model ID
func (e *LLMInitializationErrorEvent) GetModelID() string { return e.ModelID }

// GetProvider returns the provider name
func (e *LLMInitializationErrorEvent) GetProvider() string { return e.Provider }

// GetTimestamp returns the event timestamp
func (e *LLMInitializationErrorEvent) GetTimestamp() time.Time { return e.Timestamp }

// GetTraceID returns the trace ID
func (e *LLMInitializationErrorEvent) GetTraceID() string { return e.TraceID }

// LLMGenerationStartEvent represents the start of LLM generation
type LLMGenerationStartEvent struct {
	ModelID        string      `json:"model_id"`
	Provider       string      `json:"provider"`
	Operation      string      `json:"operation"`
	Messages       int         `json:"messages"`
	Temperature    float64     `json:"temperature"`
	MessageContent string      `json:"message_content"`
	Timestamp      time.Time   `json:"timestamp"`
	TraceID        string      `json:"trace_id"`
	Metadata       LLMMetadata `json:"metadata,omitempty"`
}

// GetModelID returns the model ID
func (e *LLMGenerationStartEvent) GetModelID() string { return e.ModelID }

// GetProvider returns the provider name
func (e *LLMGenerationStartEvent) GetProvider() string { return e.Provider }

// GetTimestamp returns the event timestamp
func (e *LLMGenerationStartEvent) GetTimestamp() time.Time { return e.Timestamp }

// GetTraceID returns the trace ID
func (e *LLMGenerationStartEvent) GetTraceID() string { return e.TraceID }

// LLMGenerationSuccessEvent represents successful LLM generation
type LLMGenerationSuccessEvent struct {
	ModelID        string      `json:"model_id"`
	Provider       string      `json:"provider"`
	Operation      string      `json:"operation"`
	Messages       int         `json:"messages"`
	Temperature    float64     `json:"temperature"`
	MessageContent string      `json:"message_content"`
	ResponseLength int         `json:"response_length"`
	ChoicesCount   int         `json:"choices_count"`
	TokenUsage     TokenUsage  `json:"token_usage,omitempty"`
	Timestamp      time.Time   `json:"timestamp"`
	TraceID        string      `json:"trace_id"`
	Metadata       LLMMetadata `json:"metadata,omitempty"`
}

// GetModelID returns the model ID
func (e *LLMGenerationSuccessEvent) GetModelID() string { return e.ModelID }

// GetProvider returns the provider name
func (e *LLMGenerationSuccessEvent) GetProvider() string { return e.Provider }

// GetTimestamp returns the event timestamp
func (e *LLMGenerationSuccessEvent) GetTimestamp() time.Time { return e.Timestamp }

// GetTraceID returns the trace ID
func (e *LLMGenerationSuccessEvent) GetTraceID() string { return e.TraceID }

// LLMGenerationErrorEvent represents failed LLM generation
type LLMGenerationErrorEvent struct {
	ModelID        string      `json:"model_id"`
	Provider       string      `json:"provider"`
	Operation      string      `json:"operation"`
	Messages       int         `json:"messages"`
	Temperature    float64     `json:"temperature"`
	MessageContent string      `json:"message_content"`
	Error          string      `json:"error"`
	ErrorType      string      `json:"error_type"`
	Timestamp      time.Time   `json:"timestamp"`
	TraceID        string      `json:"trace_id"`
	Metadata       LLMMetadata `json:"metadata,omitempty"`
}

// GetModelID returns the model ID
func (e *LLMGenerationErrorEvent) GetModelID() string { return e.ModelID }

// GetProvider returns the provider name
func (e *LLMGenerationErrorEvent) GetProvider() string { return e.Provider }

// GetTimestamp returns the event timestamp
func (e *LLMGenerationErrorEvent) GetTimestamp() time.Time { return e.Timestamp }

// GetTraceID returns the trace ID
func (e *LLMGenerationErrorEvent) GetTraceID() string { return e.TraceID }

// convertToLLMMetadata converts a map[string]interface{} to LLMMetadata
func convertToLLMMetadata(metadata map[string]interface{}) LLMMetadata {
	if metadata == nil {
		return LLMMetadata{}
	}

	typedMetadata := LLMMetadata{}

	// Extract common fields
	if modelVersion, ok := metadata["model_version"].(string); ok {
		typedMetadata.ModelVersion = modelVersion
	}
	if maxTokens, ok := metadata["max_tokens"].(int); ok {
		typedMetadata.MaxTokens = maxTokens
	}
	if topP, ok := metadata["top_p"].(float64); ok {
		typedMetadata.TopP = topP
	}
	if freqPenalty, ok := metadata["frequency_penalty"].(float64); ok {
		typedMetadata.FrequencyPenalty = freqPenalty
	}
	if presPenalty, ok := metadata["presence_penalty"].(float64); ok {
		typedMetadata.PresencePenalty = presPenalty
	}
	if user, ok := metadata["user"].(string); ok {
		typedMetadata.User = user
	}

	// Extract stop sequences if available
	if stopSeqs, ok := metadata["stop_sequences"].([]string); ok {
		typedMetadata.StopSequences = stopSeqs
	}

	// Extract custom fields
	customFields := make(map[string]string)
	for key, value := range metadata {
		if key != "model_version" && key != "max_tokens" && key != "top_p" &&
			key != "frequency_penalty" && key != "presence_penalty" && key != "stop_sequences" && key != "user" {
			if strVal, ok := value.(string); ok {
				customFields[key] = strVal
			}
		}
	}
	if len(customFields) > 0 {
		typedMetadata.CustomFields = customFields
	}

	return typedMetadata
}

// convertToTokenUsage converts a map[string]interface{} to TokenUsage
func convertToTokenUsage(tokenUsage map[string]interface{}) TokenUsage {
	if tokenUsage == nil {
		return TokenUsage{}
	}

	typedTokenUsage := TokenUsage{}

	// Extract token counts
	if inputTokens, ok := tokenUsage["input_tokens"].(int); ok {
		typedTokenUsage.InputTokens = inputTokens
	}
	if outputTokens, ok := tokenUsage["output_tokens"].(int); ok {
		typedTokenUsage.OutputTokens = outputTokens
	}
	if totalTokens, ok := tokenUsage["total_tokens"].(int); ok {
		typedTokenUsage.TotalTokens = totalTokens
	}

	// Extract provider-specific fields and map them
	if promptTokens, ok := tokenUsage["prompt_tokens"].(int); ok && typedTokenUsage.InputTokens == 0 {
		typedTokenUsage.InputTokens = promptTokens
	}
	if completionTokens, ok := tokenUsage["completion_tokens"].(int); ok && typedTokenUsage.OutputTokens == 0 {
		typedTokenUsage.OutputTokens = completionTokens
	}

	// Extract other fields
	if unit, ok := tokenUsage["unit"].(string); ok {
		typedTokenUsage.Unit = unit
	}
	if cost, ok := tokenUsage["cost"].(string); ok {
		typedTokenUsage.Cost = cost
	}

	return typedTokenUsage
}

// emitLLMInitializationStart emits a typed start event for LLM initialization
func emitLLMInitializationStart(tracers []observability.Tracer, provider string, modelID string, temperature float64, traceID observability.TraceID, metadata LLMMetadata) {
	if len(tracers) == 0 {
		return
	}

	event := &LLMInitializationStartEvent{
		ModelID:     modelID,
		Temperature: temperature,
		Provider:    provider,
		Operation:   OperationLLMInitialization,
		Timestamp:   time.Now(),
		TraceID:     string(traceID),
		Metadata:    metadata,
	}

	for _, tracer := range tracers {
		if err := tracer.EmitLLMEvent(event); err != nil {
			// Log error but continue with other tracers
			continue
		}
	}
}

// emitLLMInitializationSuccess emits a typed success event for LLM initialization
func emitLLMInitializationSuccess(tracers []observability.Tracer, provider string, modelID string, capabilities string, traceID observability.TraceID, metadata LLMMetadata) {
	if len(tracers) == 0 {
		return
	}

	// Convert capabilities string to slice
	capabilitiesSlice := strings.Split(capabilities, ",")
	for i, cap := range capabilitiesSlice {
		capabilitiesSlice[i] = strings.TrimSpace(cap)
	}

	event := &LLMInitializationSuccessEvent{
		ModelID:      modelID,
		Provider:     provider,
		Status:       StatusLLMInitialized,
		Capabilities: capabilitiesSlice,
		Timestamp:    time.Now(),
		TraceID:      string(traceID),
		Metadata:     metadata,
	}

	for _, tracer := range tracers {
		if err := tracer.EmitLLMEvent(event); err != nil {
			// Log error but continue with other tracers
			continue
		}
	}
}

// emitLLMInitializationError emits a typed error event for LLM initialization
func emitLLMInitializationError(tracers []observability.Tracer, provider string, modelID string, operation string, err error, traceID observability.TraceID, metadata LLMMetadata) {
	if len(tracers) == 0 {
		return
	}

	event := &LLMInitializationErrorEvent{
		ModelID:   modelID,
		Provider:  provider,
		Operation: operation,
		Error:     err.Error(),
		ErrorType: fmt.Sprintf("%T", err),
		Status:    StatusLLMFailed,
		Timestamp: time.Now(),
		TraceID:   string(traceID),
		Metadata:  metadata,
	}

	for _, tracer := range tracers {
		if err := tracer.EmitLLMEvent(event); err != nil {
			// Log error but continue with other tracers
			continue
		}
	}
}

// emitLLMGenerationStart emits a typed start event for LLM generation
func emitLLMGenerationStart(tracers []observability.Tracer, provider string, modelID string, operation string, messages int, temperature float64, messageContent string, traceID observability.TraceID, metadata LLMMetadata) {
	if len(tracers) == 0 {
		return
	}

	event := &LLMGenerationStartEvent{
		ModelID:        modelID,
		Provider:       provider,
		Operation:      operation,
		Messages:       messages,
		Temperature:    temperature,
		MessageContent: messageContent,
		Timestamp:      time.Now(),
		TraceID:        string(traceID),
		Metadata:       metadata,
	}

	for _, tracer := range tracers {
		if err := tracer.EmitLLMEvent(event); err != nil {
			// Log error but continue with other tracers
			continue
		}
	}
}

// emitLLMGenerationSuccess emits a typed success event for LLM generation
func emitLLMGenerationSuccess(tracers []observability.Tracer, provider string, modelID string, operation string, messages int, temperature float64, messageContent string, responseLength int, choicesCount int, traceID observability.TraceID, metadata LLMMetadata) {
	if len(tracers) == 0 {
		return
	}

	// Extract token usage from metadata if available
	var tokenUsage TokenUsage
	if metadata.CustomFields != nil {
		if _, ok := metadata.CustomFields["generation_info"]; ok {
			// For now, we'll create a simple TokenUsage structure
			// In the future, we could enhance this to parse more complex token usage data
			tokenUsage = TokenUsage{
				Unit: "TOKENS",
			}
		}
	}

	event := &LLMGenerationSuccessEvent{
		ModelID:        modelID,
		Provider:       provider,
		Operation:      operation,
		Messages:       messages,
		Temperature:    temperature,
		MessageContent: messageContent,
		ResponseLength: responseLength,
		ChoicesCount:   choicesCount,
		TokenUsage:     tokenUsage,
		Timestamp:      time.Now(),
		TraceID:        string(traceID),
		Metadata:       metadata,
	}

	for _, tracer := range tracers {
		if err := tracer.EmitLLMEvent(event); err != nil {
			// Log error but continue with other tracers
			continue
		}
	}
}

// emitLLMGenerationError emits a typed error event for LLM generation
func emitLLMGenerationError(tracers []observability.Tracer, provider string, modelID string, operation string, messages int, temperature float64, messageContent string, err error, traceID observability.TraceID, metadata LLMMetadata) {
	if len(tracers) == 0 {
		return
	}

	event := &LLMGenerationErrorEvent{
		ModelID:        modelID,
		Provider:       provider,
		Operation:      operation,
		Messages:       messages,
		Temperature:    temperature,
		MessageContent: messageContent,
		Error:          err.Error(),
		ErrorType:      fmt.Sprintf("%T", err),
		Timestamp:      time.Now(),
		TraceID:        string(traceID),
		Metadata:       metadata,
	}

	for _, tracer := range tracers {
		if err := tracer.EmitLLMEvent(event); err != nil {
			// Log error but continue with other tracers
			continue
		}
	}
}

// extractTokenUsageFromGenerationInfo extracts token usage from GenerationInfo
func extractTokenUsageFromGenerationInfo(generationInfo map[string]interface{}) observability.UsageMetrics {
	usage := observability.UsageMetrics{Unit: "TOKENS"}

	// Try standard field names first
	if v, ok := generationInfo["input_tokens"]; ok {
		if inputTokens, ok := v.(int); ok {
			usage.InputTokens = inputTokens
		}
	}
	if v, ok := generationInfo["output_tokens"]; ok {
		if outputTokens, ok := v.(int); ok {
			usage.OutputTokens = outputTokens
		}
	}
	if v, ok := generationInfo["total_tokens"]; ok {
		if totalTokens, ok := v.(int); ok {
			usage.TotalTokens = totalTokens
		}
	}

	// Try OpenAI-specific field names
	if v, ok := generationInfo["PromptTokens"]; ok {
		if promptTokens, ok := v.(int); ok {
			usage.InputTokens = promptTokens
		}
	}
	if v, ok := generationInfo["CompletionTokens"]; ok {
		if completionTokens, ok := v.(int); ok {
			usage.OutputTokens = completionTokens
		}
	}
	if v, ok := generationInfo["TotalTokens"]; ok {
		if totalTokens, ok := v.(int); ok {
			usage.TotalTokens = totalTokens
		}
	}

	// Try Anthropic-specific field names (used by Bedrock)
	if v, ok := generationInfo["InputTokens"]; ok {
		if inputTokens, ok := v.(int); ok {
			usage.InputTokens = inputTokens
		}
	}
	if v, ok := generationInfo["OutputTokens"]; ok {
		if outputTokens, ok := v.(int); ok {
			usage.OutputTokens = outputTokens
		}
	}

	// Try Bedrock-specific field names
	if v, ok := generationInfo["inputTokens"]; ok {
		if inputTokens, ok := v.(int); ok {
			usage.InputTokens = inputTokens
		}
	}
	if v, ok := generationInfo["outputTokens"]; ok {
		if outputTokens, ok := v.(int); ok {
			usage.OutputTokens = outputTokens
		}
	}
	if v, ok := generationInfo["totalTokens"]; ok {
		if totalTokens, ok := v.(int); ok {
			usage.TotalTokens = totalTokens
		}
	}

	// Calculate total tokens if not provided by the provider
	if usage.TotalTokens == 0 && usage.InputTokens > 0 && usage.OutputTokens > 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}

	return usage
}

// ExtractTokenUsageWithCacheInfo extracts token usage with OpenRouter cache information
func ExtractTokenUsageWithCacheInfo(generationInfo map[string]interface{}) (observability.UsageMetrics, float64, int, map[string]interface{}) {
	usage := extractTokenUsageFromGenerationInfo(generationInfo)

	var cacheDiscount float64
	var reasoningTokens int

	// Extract OpenRouter cache discount
	if v, ok := generationInfo["cache_discount"]; ok {
		if discount, ok := v.(float64); ok {
			cacheDiscount = discount
		}
	}

	// Extract reasoning tokens (for models like o3)
	if v, ok := generationInfo["ReasoningTokens"]; ok {
		if tokens, ok := v.(int); ok {
			reasoningTokens = tokens
		}
	}

	// Create a copy of generationInfo for logging
	infoCopy := make(map[string]interface{})
	for k, v := range generationInfo {
		infoCopy[k] = v
	}

	return usage, cacheDiscount, reasoningTokens, infoCopy
}

// utils.go
//
// This file contains shared helper functions for the Agent, including system prompt construction, tool choice conversion, string truncation, and usage metrics extraction.
//
// Exported:
//   - BuildSystemPrompt
//   - ConvertToolChoice
//   - TruncateString
//   - extractUsageMetrics
//   - castToInt

package mcpagent

import (
	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/pkg/mcpagent/prompt"

	"github.com/tmc/langchaingo/llms"
)

// IsReActCompletion checks if the response contains ReAct completion patterns.
func IsReActCompletion(response string) bool {
	return prompt.IsReActCompletion(response)
}

// ExtractFinalAnswer extracts the final answer from a ReAct response.
func ExtractFinalAnswer(response string) string {
	return prompt.ExtractFinalAnswer(response)
}

// GetDefaultMaxTurns returns the default max turns for a given agent mode.
func GetDefaultMaxTurns(mode AgentMode) int {
	switch mode {
	case ReActAgent:
		return 50 // ReAct agents get more turns for reasoning
	case SimpleAgent:
		fallthrough
	default:
		return 50 // Simple agents use fewer turns
	}
}

// ConvertToolChoice converts a tool choice string to the appropriate type for LLM options.
func ConvertToolChoice(choice string) interface{} {
	switch choice {
	case "auto", "none", "required":
		return choice
	default:
		return map[string]interface{}{
			"type":     "function",
			"function": map[string]interface{}{"name": choice},
		}
	}
}

// TruncateString truncates a string to a maximum length, adding ellipsis if needed.
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// extractUsageMetrics extracts token usage metrics from an LLM response.
func extractUsageMetrics(resp *llms.ContentResponse) observability.UsageMetrics {
	if resp == nil || len(resp.Choices) == 0 {
		return observability.UsageMetrics{}
	}

	m := observability.UsageMetrics{Unit: "TOKENS"}

	// Try to get token usage from GenerationInfo first
	info := resp.Choices[0].GenerationInfo
	if info != nil {
		if v, ok := info["input_tokens"]; ok {
			m.InputTokens = castToInt(v)
		}
		if v, ok := info["output_tokens"]; ok {
			m.OutputTokens = castToInt(v)
		}
		if v, ok := info["total_tokens"]; ok {
			m.TotalTokens = castToInt(v)
		}
	}

	// If we got actual token usage, return it
	if m.InputTokens > 0 || m.OutputTokens > 0 || m.TotalTokens > 0 {
		// Ensure total is calculated if not provided
		if m.TotalTokens == 0 {
			m.TotalTokens = m.InputTokens + m.OutputTokens
		}
		return m
	}

	// Fallback: Estimate tokens based on content length
	// This is a rough approximation when actual usage is not available
	content := resp.Choices[0].Content
	if content != "" {
		// Rough estimation: 1 token ≈ 4 characters for English text
		estimatedTokens := len(content) / 4
		m.OutputTokens = estimatedTokens
		m.TotalTokens = estimatedTokens

		// For input tokens, we'd need the prompt length, but we don't have it here
		// This is a limitation of the current LangChain integration
	}

	return m
}

// extractUsageMetricsWithMessages extracts token usage with improved input token estimation
func extractUsageMetricsWithMessages(resp *llms.ContentResponse, messages []llms.MessageContent) observability.UsageMetrics {
	// Get base usage metrics
	usage := extractUsageMetrics(resp)

	// If we don't have input tokens, estimate them from conversation history
	if usage.InputTokens == 0 {
		usage.InputTokens = estimateInputTokens(messages)
		// Recalculate total if we now have both input and output
		if usage.OutputTokens > 0 {
			usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		}
	}

	return usage
}

// estimateInputTokens estimates input tokens from conversation messages
func estimateInputTokens(messages []llms.MessageContent) int {
	if len(messages) == 0 {
		return 0
	}

	totalChars := 0
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if textPart, ok := part.(llms.TextContent); ok {
				totalChars += len(textPart.Text)
			}
		}
	}

	// Rough estimation: 1 token ≈ 4 characters for English text
	// Add some overhead for system prompts and formatting
	estimatedTokens := (totalChars / 4) + 50 // Add 50 tokens for system overhead
	return estimatedTokens
}

// castToInt safely casts an interface{} to int.
func castToInt(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	default:
		return 0
	}
}

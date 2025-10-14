package mcpclient

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmc/langchaingo/llms"

	"mcp-agent/agent_go/internal/utils"
)

// ToolsAsLLM converts MCP tools to langchaingo llms.Tool format for Bedrock
func ToolsAsLLM(mcpTools []mcp.Tool) ([]llms.Tool, error) {
	llmTools := make([]llms.Tool, len(mcpTools))

	for i, tool := range mcpTools {
		llmTools[i] = llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema, // MCP uses JSON Schema, which Bedrock accepts directly
			},
		}
	}

	return llmTools, nil
}

// ToolResultAsString converts a tool result to a string representation
func ToolResultAsString(result *mcp.CallToolResult, logger utils.ExtendedLogger) string {
	if result == nil {
		return "Tool execution completed but no result returned"
	}

	// Join all content parts
	var parts []string
	for _, content := range result.Content {
		switch c := content.(type) {
		case *mcp.TextContent:
			// Try to parse JSON format {"type":"text","text":"..."}
			text := c.Text
			if strings.HasPrefix(strings.TrimSpace(text), "{") && strings.HasSuffix(strings.TrimSpace(text), "}") {
				var jsonResponse map[string]interface{}
				if err := json.Unmarshal([]byte(text), &jsonResponse); err == nil {
					// Check if it's a {"type":"text","text":"..."} format
					if responseType, ok := jsonResponse["type"].(string); ok && responseType == "text" {
						if responseText, ok := jsonResponse["text"].(string); ok {
							parts = append(parts, responseText)
							logger.Infof("[DEBUG] ToolResultAsString - Successfully parsed JSON response, extracted text: %s", responseText[:min(len(responseText), 100)])
							continue
						}
					}
				}
			}
			// If not JSON or not the expected format, use the text as-is
			parts = append(parts, text)
		case *mcp.ImageContent:
			parts = append(parts, fmt.Sprintf("[Image: %s]", c.Data))
		case *mcp.EmbeddedResource:
			parts = append(parts, fmt.Sprintf("[Resource: %s]", formatResourceContents(c.Resource)))
		default:
			// For any other content type, try to marshal to JSON
			if jsonBytes, err := json.Marshal(content); err == nil {
				parts = append(parts, string(jsonBytes))
			} else {
				parts = append(parts, fmt.Sprintf("[Unknown content type: %T]", content))
			}
		}
	}

	joined := strings.Join(parts, "\n")

	// Debug logging
	logger.Infof("[DEBUG] ToolResultAsString - IsError: %v, Content: %s", result.IsError, joined)

	// If it's already marked as an error, return the error message
	if result.IsError {
		return fmt.Sprintf("Tool call failed with error: %s", joined)
	}

	// Check for implicit errors in the content (even when IsError is false)
	if strings.Contains(joined, "exit status") ||
		strings.Contains(joined, "Invalid choice") ||
		strings.Contains(joined, "usage:") ||
		strings.Contains(joined, "Error: Access denied") {
		logger.Infof("[DEBUG] ToolResultAsString - Detected implicit error: %s", joined)
		return fmt.Sprintf("Tool call failed with error: %s", joined)
	}

	return joined
}

// extractErrorMessage extracts error message from content array
func extractErrorMessage(content []mcp.Content) string {
	var errorParts []string
	for _, c := range content {
		if textContent, ok := c.(*mcp.TextContent); ok {
			errorParts = append(errorParts, textContent.Text)
		}
	}
	if len(errorParts) > 0 {
		// Join all error parts and preserve the detailed error message
		detailedError := strings.Join(errorParts, " ")

		// Check if this is a command execution error with exit status
		if strings.Contains(detailedError, "exit status") {
			return detailedError
		}

		// Check if this contains usage/help information
		if strings.Contains(detailedError, "usage:") || strings.Contains(detailedError, "Invalid choice") {
			return detailedError
		}

		// For other errors, preserve the full message
		return detailedError
	}
	return "Unknown error"
}

// formatResourceContents formats resource contents for display
func formatResourceContents(resource mcp.ResourceContents) string {
	switch r := resource.(type) {
	case *mcp.TextResourceContents:
		return r.Text
	case *mcp.BlobResourceContents:
		return fmt.Sprintf("[Binary data: %s]", r.MIMEType)
	default:
		if jsonBytes, err := json.Marshal(resource); err == nil {
			return string(jsonBytes)
		}
		return fmt.Sprintf("[Unknown resource type: %T]", resource)
	}
}

// ParseToolArguments parses JSON string arguments into a map for MCP tool calls
func ParseToolArguments(argsJSON string) (map[string]interface{}, error) {
	if argsJSON == "" {
		return make(map[string]interface{}), nil
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("failed to parse tool arguments: %w", err)
	}

	return args, nil
}

// extractTextFromJSON extracts the actual text content from JSON structure
func extractTextFromJSON(text string) string {
	// Check if the text starts with JSON structure like {"type":"text","text":"content"}
	if strings.HasPrefix(text, `{"type":"text","text":"`) {
		// Find the closing quote and brace
		startIndex := len(`{"type":"text","text":"`)
		endIndex := strings.LastIndex(text, `"}`)
		if endIndex > startIndex {
			// Extract the content between the quotes
			content := text[startIndex:endIndex]
			// Unescape the content
			content = strings.ReplaceAll(content, `\"`, `"`)
			content = strings.ReplaceAll(content, `\n`, "\n")
			content = strings.ReplaceAll(content, `\t`, "\t")
			return content
		}
	}
	// If not JSON structure, return the original text
	return text
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

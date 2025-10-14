package mcpagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmc/langchaingo/llms"
)

// VirtualTool represents a virtual tool that can be called by the LLM
type VirtualTool struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
	Handler     func(ctx context.Context, args map[string]interface{}) (string, error)
}

// CreateVirtualTools creates virtual tools for prompt and resource access
func (a *Agent) CreateVirtualTools() []llms.Tool {
	var virtualTools []llms.Tool

	// Add get_prompt tool
	getPromptTool := llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "get_prompt",
			Description: "Fetch the full content of a specific prompt by name and server",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"server": map[string]interface{}{
						"type":        "string",
						"description": "Server name",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Prompt name (e.g., aws-msk, how-it-works)",
					},
				},
				"required": []string{"server", "name"},
			},
		},
	}
	virtualTools = append(virtualTools, getPromptTool)

	// Add get_resource tool
	getResourceTool := llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "get_resource",
			Description: "Fetch the content of a specific resource by URI and server. Only use URIs that are listed in the system prompt's 'AVAILABLE RESOURCES' section.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"server": map[string]interface{}{
						"type":        "string",
						"description": "Server name",
					},
					"uri": map[string]interface{}{
						"type":        "string",
						"description": "Resource URI",
					},
				},
				"required": []string{"server", "uri"},
			},
		},
	}
	virtualTools = append(virtualTools, getResourceTool)

	// Add large output virtual tools if enabled
	largeOutputTools := a.CreateLargeOutputVirtualTools()
	virtualTools = append(virtualTools, largeOutputTools...)

	return virtualTools
}

// HandleVirtualTool handles virtual tool execution
func (a *Agent) HandleVirtualTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	switch toolName {
	case "get_prompt":
		return a.handleGetPrompt(ctx, args)
	case "get_resource":
		return a.handleGetResource(ctx, args)
	default:
		// Check if it's a large output virtual tool
		if a.EnableLargeOutputVirtualTools {
			return a.HandleLargeOutputVirtualTool(ctx, toolName, args)
		}
		return "", fmt.Errorf("unknown virtual tool: %s", toolName)
	}
}

// handleGetPrompt handles the get_prompt virtual tool
func (a *Agent) handleGetPrompt(ctx context.Context, args map[string]interface{}) (string, error) {
	server, ok := args["server"].(string)
	if !ok {
		return "", fmt.Errorf("server parameter is required")
	}

	name, ok := args["name"].(string)
	if !ok {
		return "", fmt.Errorf("name parameter is required")
	}

	// First, try to fetch from server (prioritize fresh data)
	if a.Clients != nil {
		if client, exists := a.Clients[server]; exists {
			promptResult, err := client.GetPrompt(ctx, name)
			if err == nil && promptResult != nil {
				// Extract content from messages
				if len(promptResult.Messages) > 0 {
					var contentParts []string
					for _, msg := range promptResult.Messages {
						if textContent, ok := msg.Content.(*mcp.TextContent); ok {
							contentParts = append(contentParts, textContent.Text)
						} else if textContent, ok := msg.Content.(mcp.TextContent); ok {
							contentParts = append(contentParts, textContent.Text)
						}
					}
					if len(contentParts) > 0 {
						content := strings.Join(contentParts, "\n")
						// Only return if we got actual content (not just metadata)
						if !strings.Contains(content, "Prompt loaded from") {
							return content, nil
						}
					}
				}
			}
		}
	}

	// If server fetch failed or returned metadata only, try cached data
	if a.prompts != nil {
		if serverPrompts, exists := a.prompts[server]; exists {
			for _, prompt := range serverPrompts {
				if prompt.Name == name {
					// Return the full content
					if strings.Contains(prompt.Description, "\n\nContent:\n") {
						parts := strings.Split(prompt.Description, "\n\nContent:\n")
						if len(parts) > 1 {
							return parts[1], nil
						}
					}
					return prompt.Description, nil
				}
			}
		}
	}

	return "", fmt.Errorf("prompt %s not found in server %s", name, server)
}

// handleGetResource handles the get_resource virtual tool
func (a *Agent) handleGetResource(ctx context.Context, args map[string]interface{}) (string, error) {
	server, ok := args["server"].(string)
	if !ok {
		return "", fmt.Errorf("server parameter is required")
	}

	uri, ok := args["uri"].(string)
	if !ok {
		return "", fmt.Errorf("uri parameter is required")
	}

	// Try to fetch from server
	if a.Clients != nil {
		if client, exists := a.Clients[server]; exists {
			resourceResult, err := client.GetResource(ctx, uri)
			if err != nil {
				return "", fmt.Errorf("failed to get resource %s from %s: %w", uri, server, err)
			}

			// Extract content from resource using the same approach as existing code
			if resourceResult.Contents != nil {
				var contentParts []string
				for _, content := range resourceResult.Contents {
					contentStr := formatResourceContents(content)
					contentParts = append(contentParts, contentStr)
				}
				if len(contentParts) > 0 {
					return strings.Join(contentParts, "\n"), nil
				}
			}
		}
	}

	return "", fmt.Errorf("resource %s not found in server %s", uri, server)
}

// formatResourceContents formats resource contents for display (copied from existing code)
func formatResourceContents(resource mcp.ResourceContents) string {
	switch r := resource.(type) {
	case *mcp.TextResourceContents:
		return r.Text
	case *mcp.BlobResourceContents:
		return fmt.Sprintf("[Binary data: %s]", r.MIMEType)
	default:
		return fmt.Sprintf("[Unknown resource type: %T]", resource)
	}
}

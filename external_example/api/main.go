package main

import (
	"fmt"
	"os"

	"mcp-agent/agent_go/pkg/external"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file FIRST
	if err := godotenv.Load(); err != nil {
		fmt.Printf("⚠️ Warning: Could not load .env file: %v\n", err)
	} else {
		fmt.Println("✅ Environment variables loaded successfully")
	}

	// Initialize shared logger
	if err := InitLogger("API-SERVER"); err != nil {
		fmt.Printf("❌ Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer CloseLogger()

	logger := GetLogger()
	logger.Info("🚀 Starting MCP Agent API Server")

	// Log environment information for debugging
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		// Log first 10 and last 4 characters for debugging
		if len(apiKey) > 14 {
			maskedKey := apiKey[:10] + "..." + apiKey[len(apiKey)-4:]
			logger.Info(fmt.Sprintf("🔑 OPENAI_API_KEY found: %s", maskedKey))
		} else {
			logger.Info(fmt.Sprintf("🔑 OPENAI_API_KEY found: %s", apiKey))
		}
	} else {
		logger.Error("❌ OPENAI_API_KEY not found in environment")
	}

	// Custom system prompt template for the API server
	customSystemPrompt := `You are a specialized API server AI assistant designed to help users with file system operations, web searches, and knowledge retrieval.

Your primary responsibilities:
- Help users navigate and analyze file systems
- Perform web searches for current information
- Access and process knowledge from various sources
- Provide clear, actionable responses
- Use tools efficiently without unnecessary calls

Available tools:
{{TOOLS}}

{{PROMPTS_SECTION}}

{{RESOURCES_SECTION}}

{{VIRTUAL_TOOLS_SECTION}}

API SERVER GUIDELINES:
- Always provide clear, structured responses
- Use tools when they can help answer the user's question
- Avoid redundant tool calls - be efficient
- Provide file paths and actionable information
- When searching, focus on relevance and accuracy
- Stop calling tools once you have sufficient information
- Present findings in a user-friendly format`

	// Create shared agent configuration with custom logger and custom system prompt
	config := external.DefaultConfig().
		WithAgentMode(external.SimpleAgent). // Changed from ReActAgent to SimpleAgent
		WithLLM("openai", "gpt-4o-mini", 0.1).
		WithMaxTurns(10). // Reduced from 15 since SimpleAgent needs fewer turns
		WithServer("filesystem", "configs/mcp_servers_simple.json").
		WithCustomSystemPrompt(customSystemPrompt).                // Added custom system prompt
		WithObservability("langfuse", os.Getenv("LANGFUSE_HOST")). // 🆕 FIX: Set Langfuse as trace provider with env host
		WithLogger(logger)

	logger.Info("✅ Agent configuration created with custom logger and custom system prompt")
	logger.Info("🎯 Using SimpleAgent mode with custom system prompt")

	// Create and start SSE server with shared config
	server := NewSSEServer(config)

	port := "8080"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	logger.Info(fmt.Sprintf("🎯 Starting API server on port %s", port))
	logger.Info(fmt.Sprintf("📡 SSE endpoint: http://localhost:%s/sse", port))
	logger.Info(fmt.Sprintf("🔍 Query endpoint: http://localhost:%s/api/query", port))

	if err := server.Start(port); err != nil {
		logger.Error(fmt.Sprintf("❌ Server failed: %v", err))
		os.Exit(1)
	}
}

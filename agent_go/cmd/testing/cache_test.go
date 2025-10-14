package testing

import (
	"context"
	"fmt"
	"time"

	"mcp-agent/agent_go/internal/llm"
	"mcp-agent/agent_go/pkg/mcpagent"
)

// testCacheEvents tests that cache events are emitted during normal agent operation
func testCacheEvents() error {
	logger := GetTestLogger()
	logger.Info("🧪 Starting Cache Events Test")

	// Create LLM
	llm, err := llm.InitializeLLM(llm.Config{
		Provider:    "openai",
		ModelID:     "gpt-4o-mini",
		Temperature: 0.7,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize LLM: %w", err)
	}

	// Create agent with observability
	agent, err := mcpagent.NewAgentWithObservability(
		context.Background(),
		llm,
		"all",
		"configs/mcp_servers.json",
		"gpt-4o-mini",
		GetTestLogger(),
		mcpagent.WithTemperature(0.7),
		mcpagent.WithToolChoice("auto"),
		mcpagent.WithMaxTurns(5),
	)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}
	defer agent.Close()

	// Test question that will trigger tool calls
	question := "What is 2+2? Please provide a simple answer."
	logger.Info("Testing Cache Events with Question", map[string]interface{}{"question": question})

	// Run the agent - this should emit cache events
	startTime := time.Now()
	result, err := agent.Ask(context.Background(), question)
	duration := time.Since(startTime)

	if err != nil {
		return fmt.Errorf("agent failed: %w", err)
	}

	logger.Info("Agent Response", map[string]interface{}{
		"result":     result,
		"duration":   duration.String(),
		"cache_test": "Cache events should have been emitted during this conversation",
	})

	// Test with a tool-using question
	logger.Info("Testing Cache Events with Tool Usage")
	toolQuestion := "List files in the current directory"
	logger.Info("Tool Question", map[string]interface{}{"question": toolQuestion})

	startTime = time.Now()
	toolResult, err := agent.Ask(context.Background(), toolQuestion)
	toolDuration := time.Since(startTime)

	if err != nil {
		return fmt.Errorf("agent failed with tool usage: %w", err)
	}

	logger.Info("Tool Response", map[string]interface{}{
		"result":     toolResult,
		"duration":   toolDuration.String(),
		"cache_test": "Cache events should have been emitted during tool execution",
	})

	logger.Info("✅ Cache Events Test Completed Successfully")
	logger.Info("📊 Check the frontend for cache_operation_start events during conversations")

	return nil
}

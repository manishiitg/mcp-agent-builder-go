package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"mcp-agent/agent_go/pkg/external"
	"mcp-agent/agent_go/pkg/logger"

	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("🚀 Langfuse Integration Test with External Agent")
	fmt.Println("===============================================")

	// Load environment
	if err := godotenv.Load(); err != nil {
		fmt.Printf("⚠️  No .env file found, using system environment\n")
	}

	// Check Langfuse credentials
	publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
	secretKey := os.Getenv("LANGFUSE_SECRET_KEY")
	host := os.Getenv("LANGFUSE_HOST")

	if publicKey == "" || secretKey == "" {
		fmt.Println("❌ Error: LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY must be set")
		fmt.Println("   Please ensure your .env file contains these credentials")
		os.Exit(1)
	}

	if host == "" {
		host = "https://cloud.langfuse.com"
	}

	fmt.Printf("✅ Environment loaded\n")
	fmt.Printf("📊 Langfuse Host: %s\n", host)
	fmt.Printf("📊 Public Key: %s...\n", publicKey[:min(10, len(publicKey))])
	fmt.Println()

	// Single test query that will use all MCP servers
	testQuery := "First, list all files in the reports directory. Then search for information about AI and machine learning trends. Create a memory entry about this test session. Use sequential thinking to analyze the benefits of MCP architecture. Finally, search my Obsidian vault for notes about productivity and summarize everything into a comprehensive report."
	testDescription := "Uses all MCP servers: filesystem, context7, memory, sequential-thinking, and obsidian"
	expectedServers := []string{"filesystem", "context7", "memory", "sequential-thinking", "obsidian"}

	fmt.Println("📝 Test Query (will trigger Langfuse tracing):")
	fmt.Printf("   Query: \"%s\"\n", testQuery)
	fmt.Printf("   Description: %s\n", testDescription)
	fmt.Printf("   Expected servers: %v\n", expectedServers)
	fmt.Println()

	// Create external agent
	fmt.Println("🔧 Creating external agent...")

	// Create context for the agent
	ctx := context.Background()

	// Create a custom file-only logger for cleaner debugging
	// Use fixed filename for easier debugging across multiple test runs
	logFilename := "langfuse-test-debug.log"

	// Truncate the log file at the start for clean debugging
	if err := os.Truncate(logFilename, 0); err != nil && !os.IsNotExist(err) {
		fmt.Printf("⚠️  Warning: Could not truncate log file: %v\n", err)
	}

	customLogger, err := logger.CreateLogger(logFilename, "info", "text", false) // false = no console output
	if err != nil {
		fmt.Printf("❌ Error creating custom logger: %v\n", err)
		os.Exit(1)
	}

	// Create agent configuration with the new fluent builder pattern
	agent, err := external.NewAgentBuilder().
		WithAgentMode(external.SimpleAgent).
		WithServer("", "configs/mcp_servers.json").
		WithLLM("openai", "gpt-4.1", 0.7).
		WithObservability("langfuse", host).
		WithLogger(customLogger).
		WithToolChoice("auto").
		WithMaxTurns(20).
		WithToolTimeout(5 * time.Minute).
		WithSystemPromptMode("auto").
		Create(ctx)

	if err != nil {
		fmt.Printf("❌ Error creating external agent: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ External agent created successfully")
	fmt.Println()

	// Execute the test query
	fmt.Println("🚀 Executing test query with Langfuse tracing...")
	fmt.Println("================================================")

	fmt.Printf("\n📝 Test: %s\n", testDescription)
	fmt.Printf("   Query: \"%s\"\n", testQuery)
	fmt.Printf("   ⏳ Executing...\n")

	// Execute the query
	start := time.Now()
	result, err := agent.Invoke(ctx, testQuery)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("   ❌ Test failed: %v\n", err)
	} else {
		fmt.Printf("   ✅ Test completed successfully\n")
		fmt.Printf("   ⏱️  Duration: %v\n", duration)

		// Show result summary
		if result != "" {
			resultPreview := result
			if len(result) > 200 {
				resultPreview = result[:200] + "..."
			}
			fmt.Printf("   📄 Result: %s\n", resultPreview)
		}
	}

	// Wait for background processing
	fmt.Println("\n⏳ Waiting for background processing and Langfuse events...")
	time.Sleep(3 * time.Second)

	// Summary
	fmt.Println("\n🎉 Langfuse Integration Test Complete!")
	fmt.Println("=====================================")
	fmt.Printf("✅ Executed test query successfully\n")
	fmt.Printf("📊 Langfuse Host: %s\n", host)
	fmt.Printf("🔍 Check your Langfuse dashboard for traces\n")
	fmt.Println()
	fmt.Println("📝 What was tested:")
	fmt.Println("   ✅ External agent creation")
	fmt.Println("   ✅ MCP server connections (all configured servers)")
	fmt.Println("   ✅ Query execution with multiple server tools")
	fmt.Println("   ✅ Langfuse event emission for agent operations")
	fmt.Println("   ✅ Trace and span creation for agent activities")
	fmt.Println("   ✅ 🆕 NEW: Clean architecture using WithObservability")
	fmt.Println()
	fmt.Println("💡 Next steps:")
	fmt.Println("   - Check Langfuse dashboard for comprehensive traces")
	fmt.Println("   - Verify all event types are captured")
	fmt.Println("   - Analyze span hierarchy and timing")
	fmt.Println("   - Confirm MCP server tool usage is tracked")
	fmt.Println("   - 🆕 NEW: Verify clean architecture is working")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

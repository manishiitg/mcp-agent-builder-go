package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	// Import the external package
	"mcp-agent/agent_go/pkg/external"
)

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found, using system environment variables")
	}

	// Check required environment variables
	requiredVars := []string{"AWS_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}
	for _, varName := range requiredVars {
		if os.Getenv(varName) == "" {
			log.Fatalf("❌ Required environment variable %s is not set", varName)
		}
	}

	fmt.Println("🚀 Agent with Events Example")
	fmt.Println("=============================")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create agent using the new fluent builder pattern
	agent, err := external.NewAgentBuilder().
		WithAgentMode(external.ReActAgent).
		WithServer("filesystem", "configs/mcp_servers.json").
		WithLLM("bedrock", "us.anthropic.claude-sonnet-4-20250514-v1:0", 0.2).
		WithMaxTurns(10).
		WithObservability("console", "").
		WithCustomSystemPrompt(`You are a specialized AI assistant focused on file system analysis and data processing.

Your primary responsibilities:
1. Analyze file system structures and contents
2. Process and summarize large data files
3. Provide insights about file organization and data patterns
4. Help users navigate and understand their file systems

Available tools:
{{TOOLS}}

{{PROMPTS_SECTION}}

{{RESOURCES_SECTION}}

{{VIRTUAL_TOOLS_SECTION}}

SPECIAL INSTRUCTIONS:
- Always start with a brief analysis of the file system structure
- Focus on providing actionable insights about files and data
- Use virtual tools to process large outputs efficiently
- Provide clear summaries and recommendations
- Be thorough but concise in your analysis`).
		WithAdditionalInstructions("Remember to always check file permissions and provide security recommendations when appropriate.").
		Create(ctx)

	fmt.Println("📋 Configuration:")
	fmt.Printf("  Agent Mode: %s\n", "ReActAgent")
	fmt.Printf("  Server: %s\n", "filesystem")
	fmt.Printf("  LLM Provider: %s\n", "bedrock")
	fmt.Printf("  Model: %s\n", "us.anthropic.claude-sonnet-4-20250514-v1:0")
	fmt.Printf("  Max Turns: %d\n", 10)

	// Create the agent using the external package
	fmt.Println("\n🤖 Creating agent...")
	if err != nil {
		log.Fatalf("❌ Failed to create agent: %v", err)
	}
	defer agent.Close()

	// Demonstrate runtime custom instructions
	fmt.Println("\n🔧 Adding runtime custom instructions...")
	agent.SetCustomInstructions("IMPORTANT: When analyzing files, always consider data privacy implications and suggest encryption for sensitive files.")

	fmt.Printf("📝 Current custom instructions: %s\n", agent.GetCustomInstructions())

	// Create and add our custom event listener
	eventListener := NewSimpleEventListener()
	agent.AddEventListener(eventListener)

	// Check agent health
	fmt.Println("\n🏥 Checking agent health...")
	health := agent.CheckHealth(ctx)
	for server, err := range health {
		if err != nil {
			fmt.Printf("  ❌ Server %s: %v\n", server, err)
		} else {
			fmt.Printf("  ✅ Server %s: healthy\n", server)
		}
	}

	// Get server names
	servers := agent.GetServerNames()
	fmt.Printf("  🔗 Connected servers: %v\n", servers)

	// Ask a simple question to trigger events
	fmt.Println("\n❓ Asking a simple question to trigger events...")
	answer, err := agent.Invoke(ctx, "What files are available in the filesystem?")
	if err != nil {
		log.Fatalf("❌ Failed to ask question: %v", err)
	}

	fmt.Printf("🤖 Answer: %s\n", answer)

	// Wait a moment for all events to be processed
	time.Sleep(1 * time.Second)

	// Show event summary
	fmt.Println("\n📊 Event Summary:")
	fmt.Println("==================")
	eventListener.PrintSummary()

	fmt.Println("\n✅ Agent with events example completed successfully!")
}

// SimpleEventListener captures events for analysis with typed event support
type SimpleEventListener struct {
	events []external.AgentEvent
}

func NewSimpleEventListener() *SimpleEventListener {
	return &SimpleEventListener{
		events: make([]external.AgentEvent, 0),
	}
}

func (e *SimpleEventListener) HandleEvent(ctx context.Context, event *external.AgentEvent) error {
	// Capture the event
	e.events = append(e.events, *event)

	// Print event details in real-time with typed data
	fmt.Printf("📡 Event: %s at %s\n", event.Type, event.Timestamp.Format("15:04:05.000"))

	// Show typed data if available with detailed information
	if typedData := event.GetTypedData(); typedData != nil {
		fmt.Printf("  📝 Typed Data: %s\n", typedData.GetEventType())

		// Handle specific event types with detailed information using constants
		switch event.Type {
		case "agent_start":
			fmt.Printf("    🤖 Agent started\n")
		case "agent_end":
			fmt.Printf("    🤖 Agent ended\n")
		case "conversation_start":
			fmt.Printf("    🛠️  Conversation started\n")
		case "conversation_end":
			fmt.Printf("    ⏱️  Conversation ended\n")
		case "token_usage":
			fmt.Printf("    💰 Token usage event\n")
		case "tool_call_start":
			fmt.Printf("    🔧 Tool call started\n")
		case "tool_call_end":
			fmt.Printf("    ✅ Tool call ended\n")
		case "react_reasoning_start":
			fmt.Printf("    🧠 ReAct reasoning started\n")
		case "react_reasoning_end":
			fmt.Printf("    🧠 ReAct reasoning ended\n")
		case "llm_generation_start":
			fmt.Printf("    🧠 LLM generation started\n")
		case "llm_generation_end":
			fmt.Printf("    🧠 LLM generation ended\n")
		case "llm_messages":
			fmt.Printf("    💬 LLM messages event\n")
		case "system_prompt":
			fmt.Printf("    📝 System prompt event\n")
		case "user_message":
			fmt.Printf("    👤 User message event\n")
		case "conversation_turn":
			fmt.Printf("    🔄 Conversation turn event\n")
		default:
			fmt.Printf("    📊 Event: %s\n", event.Type)
		}
	} else {
		fmt.Printf("  📝 Generic data only\n")
	}

	return nil
}

func (e *SimpleEventListener) Name() string {
	return "simple-event-listener"
}

func (e *SimpleEventListener) PrintSummary() {
	fmt.Printf("Total Events Captured: %d\n", len(e.events))

	// Count event types
	eventCounts := make(map[string]int)
	for _, event := range e.events {
		eventCounts[string(event.Type)]++
	}

	fmt.Println("\nEvent Type Breakdown:")
	for eventType, count := range eventCounts {
		fmt.Printf("  %s: %d events\n", eventType, count)
	}

	// Show first few events in detail
	if len(e.events) > 0 {
		fmt.Println("\nFirst 3 Events:")
		for i, event := range e.events[:min(3, len(e.events))] {
			fmt.Printf("  %d. %s at %s\n", i+1, event.Type, event.Timestamp.Format("15:04:05.000"))
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

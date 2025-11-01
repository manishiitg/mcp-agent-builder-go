package testing

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"mcp-agent/agent_go/internal/llmtypes"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"mcp-agent/agent_go/internal/llm"
)

var bedrockMultiTurnToolTestCmd = &cobra.Command{
	Use:   "bedrock-multi-turn-tool",
	Short: "Test Bedrock multi-turn conversations with tool calling",
	Long: `Test Bedrock multi-turn conversations with multiple tools.

This test simulates a real conversation flow where:
1. User asks a question requiring multiple tool calls
2. LLM makes tool calls
3. Tools are executed and results returned
4. LLM uses tool results to continue conversation
5. Multiple rounds of tool calls in sequence

Examples:
  go run main.go test bedrock-multi-turn-tool
  go run main.go test bedrock-multi-turn-tool --model global.anthropic.claude-sonnet-4-5-20250929-v1:0`,
	Run: runBedrockMultiTurnToolTest,
}

type bedrockMultiTurnToolTestFlags struct {
	model    string
	maxTurns int
	verbose  bool
}

var bedrockMultiTurnToolFlags bedrockMultiTurnToolTestFlags

func init() {
	bedrockMultiTurnToolTestCmd.Flags().StringVar(&bedrockMultiTurnToolFlags.model, "model", "global.anthropic.claude-sonnet-4-5-20250929-v1:0", "Bedrock model to test")
	bedrockMultiTurnToolTestCmd.Flags().IntVar(&bedrockMultiTurnToolFlags.maxTurns, "max-turns", 5, "Maximum conversation turns")
	bedrockMultiTurnToolTestCmd.Flags().BoolVar(&bedrockMultiTurnToolFlags.verbose, "verbose", false, "Enable verbose logging")
}

func runBedrockMultiTurnToolTest(cmd *cobra.Command, args []string) {
	_ = godotenv.Load(".env")

	modelID := bedrockMultiTurnToolFlags.model
	maxTurns := bedrockMultiTurnToolFlags.maxTurns
	verbose := bedrockMultiTurnToolFlags.verbose

	log.Printf("🚀 Testing Bedrock Multi-Turn Tool Calling with %s", modelID)
	log.Printf("   Max Turns: %d", maxTurns)

	// Check for AWS credentials
	if os.Getenv("AWS_REGION") == "" {
		log.Printf("⚠️  AWS_REGION not set, using default: us-east-1")
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" {
		log.Printf("❌ AWS_ACCESS_KEY_ID environment variable is required")
		return
	}
	if os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
		log.Printf("❌ AWS_SECRET_ACCESS_KEY environment variable is required")
		return
	}

	// Create Bedrock LLM using our adapter
	logger := GetTestLogger()
	bedrockLLM, err := llm.InitializeLLM(llm.Config{
		Provider:    llm.ProviderBedrock,
		ModelID:     modelID,
		Temperature: 0.7,
		Logger:      logger,
	})
	if err != nil {
		log.Printf("❌ Failed to create Bedrock LLM: %v", err)
		return
	}

	// Define multiple test tools
	tools := []llmtypes.Tool{
		{
			Type: "function",
			Function: &llmtypes.FunctionDefinition{
				Name:        "get_file_info",
				Description: "Get information about a file (size, modification time, etc.)",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filepath": map[string]interface{}{
							"type":        "string",
							"description": "Path to the file",
						},
					},
					"required": []string{"filepath"},
				},
			},
		},
		{
			Type: "function",
			Function: &llmtypes.FunctionDefinition{
				Name:        "calculate_math",
				Description: "Perform mathematical calculations",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"expression": map[string]interface{}{
							"type":        "string",
							"description": "Mathematical expression to evaluate (e.g., '2+2', '10*5')",
						},
					},
					"required": []string{"expression"},
				},
			},
		},
		{
			Type: "function",
			Function: &llmtypes.FunctionDefinition{
				Name:        "get_weather",
				Description: "Get current weather for a location",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]interface{}{
							"type":        "string",
							"description": "City name or location",
						},
					},
					"required": []string{"location"},
				},
			},
		},
		{
			Type: "function",
			Function: &llmtypes.FunctionDefinition{
				Name:        "search_knowledge",
				Description: "Search knowledge base for information",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Search query",
						},
					},
					"required": []string{"query"},
				},
			},
		},
	}

	// Test 1: Sequential tool calls (one after another)
	log.Printf("\n" + strings.Repeat("=", 80))
	log.Printf("🧪 TEST 1: Sequential Tool Calls")
	log.Printf(strings.Repeat("=", 80))
	testBedrockSequentialToolCalls(bedrockLLM, tools, maxTurns, verbose)

	// Test 2: Parallel tool calls (multiple tools in one turn)
	log.Printf("\n" + strings.Repeat("=", 80))
	log.Printf("🧪 TEST 2: Parallel Tool Calls (Multiple Tools in One Turn)")
	log.Printf(strings.Repeat("=", 80))
	testBedrockParallelToolCalls(bedrockLLM, tools, maxTurns, verbose)

	// Test 3: Multi-step reasoning with tools
	log.Printf("\n" + strings.Repeat("=", 80))
	log.Printf("🧪 TEST 3: Multi-Step Reasoning with Tools")
	log.Printf(strings.Repeat("=", 80))
	testBedrockMultiStepReasoning(bedrockLLM, tools, maxTurns, verbose)

	log.Printf("\n🎯 All Bedrock multi-turn tool calling tests completed!")
}

// testBedrockSequentialToolCalls tests tools being called one after another
func testBedrockSequentialToolCalls(llm llmtypes.Model, tools []llmtypes.Tool, maxTurns int, verbose bool) {
	ctx := context.Background()
	messages := []llmtypes.MessageContent{
		llmtypes.TextParts(llmtypes.ChatMessageTypeHuman,
			"First, calculate 15 * 23. Then, get the weather for New York. Finally, search for information about Go programming language."),
	}

	log.Printf("📝 User: First, calculate 15 * 23. Then, get the weather for New York. Finally, search for information about Go programming language.")

	var totalTokens int
	startTime := time.Now()

	for turn := 0; turn < maxTurns; turn++ {
		if verbose {
			log.Printf("\n--- Turn %d ---", turn+1)
		}

		// Generate response
		resp, err := llm.GenerateContent(ctx, messages, llmtypes.WithTools(tools), llmtypes.WithToolChoiceString("auto"))
		if err != nil {
			log.Printf("❌ Turn %d: Error generating response: %v", turn+1, err)
			return
		}

		if len(resp.Choices) == 0 {
			log.Printf("❌ Turn %d: No response choices", turn+1)
			return
		}

		choice := resp.Choices[0]

		// Track token usage
		if choice.GenerationInfo != nil {
			if input, ok := choice.GenerationInfo["prompt_tokens"].(int); ok {
				if output, ok := choice.GenerationInfo["completion_tokens"].(int); ok {
					totalTokens += input + output
					if verbose {
						log.Printf("   Tokens: input=%d, output=%d", input, output)
					}
				}
			} else if input, ok := choice.GenerationInfo["input_tokens"].(int); ok {
				if output, ok := choice.GenerationInfo["output_tokens"].(int); ok {
					totalTokens += input + output
					if verbose {
						log.Printf("   Tokens: input=%d, output=%d", input, output)
					}
				}
			}
		}

		// Check if there are tool calls
		if len(choice.ToolCalls) > 0 {
			log.Printf("🔧 Turn %d: LLM made %d tool call(s):", turn+1, len(choice.ToolCalls))

			// Append assistant message with tool calls
			assistantParts := []llmtypes.ContentPart{}
			if choice.Content != "" {
				assistantParts = append(assistantParts, llmtypes.TextContent{Text: choice.Content})
			}
			for _, tc := range choice.ToolCalls {
				assistantParts = append(assistantParts, tc)
			}
			messages = append(messages, llmtypes.MessageContent{
				Role:  llmtypes.ChatMessageTypeAI,
				Parts: assistantParts,
			})

			// Execute each tool call
			for i, tc := range choice.ToolCalls {
				log.Printf("   [%d] Tool: %s", i+1, tc.FunctionCall.Name)
				log.Printf("       Args: %s", tc.FunctionCall.Arguments)

				// Execute tool (mock execution)
				result := executeMockTool(tc.FunctionCall.Name, tc.FunctionCall.Arguments)
				log.Printf("       Result: %s", truncateString(result, 100))

				// Append tool result to conversation
				messages = append(messages, llmtypes.MessageContent{
					Role: llmtypes.ChatMessageTypeTool,
					Parts: []llmtypes.ContentPart{
						llmtypes.ToolCallResponse{
							ToolCallID: tc.ID,
							Name:       tc.FunctionCall.Name,
							Content:    result,
						},
					},
				})
			}
			log.Printf("   Waiting for LLM to process tool results...\n")
		} else {
			// No tool calls - conversation complete
			log.Printf("\n✅ Turn %d: Final Response (no more tool calls)", turn+1)
			if choice.Content != "" {
				log.Printf("📝 Assistant: %s", choice.Content)
			}
			duration := time.Since(startTime)
			log.Printf("\n📊 Test Summary:")
			log.Printf("   Total Turns: %d", turn+1)
			log.Printf("   Duration: %v", duration)
			log.Printf("   Total Tokens: %d", totalTokens)
			return
		}
	}

	log.Printf("⚠️  Reached max turns (%d) without completion", maxTurns)
}

// testBedrockParallelToolCalls tests multiple tools called in parallel
func testBedrockParallelToolCalls(llm llmtypes.Model, tools []llmtypes.Tool, maxTurns int, verbose bool) {
	ctx := context.Background()
	messages := []llmtypes.MessageContent{
		llmtypes.TextParts(llmtypes.ChatMessageTypeHuman,
			"Get the weather for Tokyo, calculate 42 * 18, and search for information about Python programming - all at once please."),
	}

	log.Printf("📝 User: Get the weather for Tokyo, calculate 42 * 18, and search for information about Python programming - all at once please.")

	var totalTokens int
	startTime := time.Now()

	for turn := 0; turn < maxTurns; turn++ {
		if verbose {
			log.Printf("\n--- Turn %d ---", turn+1)
		}

		resp, err := llm.GenerateContent(ctx, messages, llmtypes.WithTools(tools), llmtypes.WithToolChoiceString("auto"))
		if err != nil {
			log.Printf("❌ Turn %d: Error: %v", turn+1, err)
			return
		}

		if len(resp.Choices) == 0 {
			log.Printf("❌ Turn %d: No response", turn+1)
			return
		}

		choice := resp.Choices[0]

		// Track token usage
		if choice.GenerationInfo != nil {
			if input, ok := choice.GenerationInfo["prompt_tokens"].(int); ok {
				if output, ok := choice.GenerationInfo["completion_tokens"].(int); ok {
					totalTokens += input + output
				}
			} else if input, ok := choice.GenerationInfo["input_tokens"].(int); ok {
				if output, ok := choice.GenerationInfo["output_tokens"].(int); ok {
					totalTokens += input + output
				}
			}
		}

		if len(choice.ToolCalls) > 0 {
			log.Printf("🔧 Turn %d: LLM made %d parallel tool call(s):", turn+1, len(choice.ToolCalls))

			// Append assistant message
			assistantParts := []llmtypes.ContentPart{}
			if choice.Content != "" {
				assistantParts = append(assistantParts, llmtypes.TextContent{Text: choice.Content})
			}
			for _, tc := range choice.ToolCalls {
				assistantParts = append(assistantParts, tc)
			}
			messages = append(messages, llmtypes.MessageContent{
				Role:  llmtypes.ChatMessageTypeAI,
				Parts: assistantParts,
			})

			// Execute all tools in parallel conceptually
			for i, tc := range choice.ToolCalls {
				log.Printf("   [%d] Tool: %s", i+1, tc.FunctionCall.Name)
				log.Printf("       Args: %s", tc.FunctionCall.Arguments)

				result := executeMockTool(tc.FunctionCall.Name, tc.FunctionCall.Arguments)
				log.Printf("       Result: %s", truncateString(result, 100))

				messages = append(messages, llmtypes.MessageContent{
					Role: llmtypes.ChatMessageTypeTool,
					Parts: []llmtypes.ContentPart{
						llmtypes.ToolCallResponse{
							ToolCallID: tc.ID,
							Name:       tc.FunctionCall.Name,
							Content:    result,
						},
					},
				})
			}
			log.Printf("   All tool results returned, waiting for LLM response...\n")
		} else {
			log.Printf("\n✅ Turn %d: Final Response", turn+1)
			if choice.Content != "" {
				log.Printf("📝 Assistant: %s", choice.Content)
			}
			duration := time.Since(startTime)
			log.Printf("\n📊 Test Summary:")
			log.Printf("   Total Turns: %d", turn+1)
			log.Printf("   Duration: %v", duration)
			log.Printf("   Total Tokens: %d", totalTokens)
			return
		}
	}

	log.Printf("⚠️  Reached max turns")
}

// testBedrockMultiStepReasoning tests complex multi-step reasoning with tool results
func testBedrockMultiStepReasoning(llm llmtypes.Model, tools []llmtypes.Tool, maxTurns int, verbose bool) {
	ctx := context.Background()
	messages := []llmtypes.MessageContent{
		llmtypes.TextParts(llmtypes.ChatMessageTypeHuman,
			"I need to plan a trip. First, check the weather in San Francisco and Seattle. Based on the weather, recommend which city is better for a trip. Then calculate the total cost if the trip costs $500 per day for 3 days."),
	}

	log.Printf("📝 User: I need to plan a trip. First, check the weather in San Francisco and Seattle. Based on the weather, recommend which city is better for a trip. Then calculate the total cost if the trip costs $500 per day for 3 days.")

	var totalTokens int
	startTime := time.Now()

	for turn := 0; turn < maxTurns; turn++ {
		if verbose {
			log.Printf("\n--- Turn %d ---", turn+1)
		}

		resp, err := llm.GenerateContent(ctx, messages, llmtypes.WithTools(tools), llmtypes.WithToolChoiceString("auto"))
		if err != nil {
			log.Printf("❌ Turn %d: Error: %v", turn+1, err)
			return
		}

		if len(resp.Choices) == 0 {
			log.Printf("❌ Turn %d: No response", turn+1)
			return
		}

		choice := resp.Choices[0]

		// Track token usage
		if choice.GenerationInfo != nil {
			if input, ok := choice.GenerationInfo["prompt_tokens"].(int); ok {
				if output, ok := choice.GenerationInfo["completion_tokens"].(int); ok {
					totalTokens += input + output
				}
			} else if input, ok := choice.GenerationInfo["input_tokens"].(int); ok {
				if output, ok := choice.GenerationInfo["output_tokens"].(int); ok {
					totalTokens += input + output
				}
			}
		}

		if len(choice.ToolCalls) > 0 {
			log.Printf("🔧 Turn %d: LLM made %d tool call(s):", turn+1, len(choice.ToolCalls))

			// Append assistant message
			assistantParts := []llmtypes.ContentPart{}
			if choice.Content != "" {
				assistantParts = append(assistantParts, llmtypes.TextContent{Text: choice.Content})
				if verbose {
					log.Printf("   Assistant reasoning: %s", truncateString(choice.Content, 150))
				}
			}
			for _, tc := range choice.ToolCalls {
				assistantParts = append(assistantParts, tc)
			}
			messages = append(messages, llmtypes.MessageContent{
				Role:  llmtypes.ChatMessageTypeAI,
				Parts: assistantParts,
			})

			// Execute tools
			for i, tc := range choice.ToolCalls {
				log.Printf("   [%d] Tool: %s", i+1, tc.FunctionCall.Name)
				log.Printf("       Args: %s", tc.FunctionCall.Arguments)

				result := executeMockTool(tc.FunctionCall.Name, tc.FunctionCall.Arguments)
				log.Printf("       Result: %s", truncateString(result, 100))

				messages = append(messages, llmtypes.MessageContent{
					Role: llmtypes.ChatMessageTypeTool,
					Parts: []llmtypes.ContentPart{
						llmtypes.ToolCallResponse{
							ToolCallID: tc.ID,
							Name:       tc.FunctionCall.Name,
							Content:    result,
						},
					},
				})
			}
			log.Printf("   Waiting for LLM to analyze results and continue...\n")
		} else {
			log.Printf("\n✅ Turn %d: Final Response", turn+1)
			if choice.Content != "" {
				log.Printf("📝 Assistant: %s", choice.Content)
			}
			duration := time.Since(startTime)
			log.Printf("\n📊 Test Summary:")
			log.Printf("   Total Turns: %d", turn+1)
			log.Printf("   Duration: %v", duration)
			log.Printf("   Total Tokens: %d", totalTokens)
			return
		}
	}

	log.Printf("⚠️  Reached max turns")
}

// Note: executeMockTool and truncateString are already defined in openai-multi-turn-tool-test.go
// and are available in the same package, so we don't need to redefine them here.

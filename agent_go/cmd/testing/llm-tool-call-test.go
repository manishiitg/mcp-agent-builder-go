package testing

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/bedrock"
)

var llmToolCallTestCmd = &cobra.Command{
	Use:   "llm-tool-call",
	Short: "Test LLM tool calling with Bedrock",
	Run:   runLLMToolCallTest,
}

type llmToolCallTestFlags struct {
	model string
}

var llmToolCallFlags llmToolCallTestFlags

func init() {
	llmToolCallTestCmd.Flags().StringVar(&llmToolCallFlags.model, "model", "", "Bedrock model to test")
}

func runLLMToolCallTest(cmd *cobra.Command, args []string) {
	_ = godotenv.Load(".env")

	// Set AWS credentials
	os.Setenv("AWS_ACCESS_KEY_ID", viper.GetString("AWS_ACCESS_KEY_ID"))
	os.Setenv("AWS_SECRET_ACCESS_KEY", viper.GetString("AWS_SECRET_ACCESS_KEY"))
	os.Setenv("AWS_REGION", viper.GetString("AWS_REGION"))

	// Get model ID
	modelID := llmToolCallFlags.model
	if modelID == "" {
		modelID = os.Getenv("BEDROCK_PRIMARY_MODEL")
		if modelID == "" {
			modelID = "claude-3.5-sonnet"
		}
	}

	log.Printf("🚀 Testing LLM Tool Calling with %s", modelID)

	// Create Bedrock LLM
	llm, err := bedrock.New(
		bedrock.WithModel(modelID),
		bedrock.WithModelProvider("anthropic"),
	)
	if err != nil {
		log.Printf("❌ Failed to create Bedrock LLM: %v", err)
		return
	}

	// Define test tool
	tool := llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "read_file",
			Description: "Read contents of a file",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "File path to read",
					},
				},
				"required": []string{"path"},
			},
		},
	}

	// Test tool calling
	ctx := context.Background()
	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, "Read the contents of config.json"),
	}

	startTime := time.Now()
	resp, err := llm.GenerateContent(ctx, messages,
		llms.WithTools([]llms.Tool{tool}),
		llms.WithToolChoice("required"),
	)
	duration := time.Since(startTime)

	if err != nil {
		log.Printf("❌ Tool call failed: %v", err)
		return
	}

	// Validate response
	if len(resp.Choices[0].ToolCalls) == 0 {
		log.Printf("❌ No tool calls detected")
		return
	}

	toolCall := resp.Choices[0].ToolCalls[0]
	log.Printf("✅ Tool call successful in %s", duration)
	log.Printf("   Tool: %s", toolCall.FunctionCall.Name)
	log.Printf("   Args: %s", toolCall.FunctionCall.Arguments)

	// Test second tool call
	secondMessages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, "Read the contents of go.mod"),
	}

	secondResp, err := llm.GenerateContent(ctx, secondMessages,
		llms.WithTools([]llms.Tool{tool}),
		llms.WithToolChoice("required"),
	)

	if err != nil {
		log.Printf("❌ Second tool call failed: %v", err)
		return
	}

	if len(secondResp.Choices[0].ToolCalls) == 0 {
		log.Printf("❌ Second tool call failed")
		return
	}

	secondToolCall := secondResp.Choices[0].ToolCalls[0]
	log.Printf("✅ Second tool call successful")
	log.Printf("   Tool: %s", secondToolCall.FunctionCall.Name)
	log.Printf("   Args: %s", secondToolCall.FunctionCall.Arguments)

	log.Printf("🎯 Test completed successfully!")
}

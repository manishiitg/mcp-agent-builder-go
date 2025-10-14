package testing

import (
	"context"
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/bedrock"
)

var bedrockCmd = &cobra.Command{
	Use:   "bedrock",
	Short: "Test Bedrock tool calling",
	Long:  "Test AWS Bedrock LLM with tool calling capabilities",
	Run:   runBedrock,
}

// bedrockTestFlags holds the bedrock test specific flags
type bedrockTestFlags struct {
	model        string
	region       string
	verbose      bool
	showResponse bool
}

var bedrockFlags bedrockTestFlags

func init() {
	// Bedrock test specific flags
	bedrockCmd.Flags().StringVar(&bedrockFlags.model, "model", "claude-3.5-sonnet", "Bedrock model to test")
	bedrockCmd.Flags().StringVar(&bedrockFlags.region, "region", "us-east-1", "AWS region for Bedrock")
	bedrockCmd.Flags().BoolVar(&bedrockFlags.verbose, "verbose", false, "enable verbose output")
	bedrockCmd.Flags().BoolVar(&bedrockFlags.showResponse, "show-response", true, "show full response")
}

func runBedrock(cmd *cobra.Command, args []string) {
	// Get logging configuration from viper
	logFile := viper.GetString("log-file")
	logLevel := viper.GetString("log-level")

	// Initialize test logger
	InitTestLogger(logFile, logLevel)
	logger := GetTestLogger()

	logger.Info("🚀 Testing Bedrock Tool Calling...")

	// Create Bedrock LLM
	llm, err := bedrock.New()
	if err != nil {
		log.Fatalf("Failed to create Bedrock LLM: %v", err)
	}

	// Define a simple tool
	weatherTool := llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "get_weather",
			Description: "Get current weather for a location",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{
						"type":        "string",
						"description": "City name",
					},
				},
				"required": []string{"location"},
			},
		},
	}

	// Test messages
	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, "What's the weather like in Tokyo?"),
	}

	ctx := context.Background()

	// Call with tool
	logger.Info("📞 Calling Bedrock with tool...")
	resp, err := llm.GenerateContent(ctx, messages,
		llms.WithTools([]llms.Tool{weatherTool}),
		llms.WithToolChoice("required"),
	)
	if err != nil {
		logger.Fatal("❌ Tool call failed", map[string]interface{}{"error": err.Error()})
	}

	logger.Info("✅ Response received", map[string]interface{}{
		"response": resp.Choices[0].Content,
	})
}

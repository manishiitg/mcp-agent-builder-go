package external

import (
	"fmt"
	"os"

	"mcp-agent/agent_go/internal/llm"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/bedrock"
	"github.com/tmc/langchaingo/llms/openai"
)

// initializeLLM creates and configures an LLM based on the provider
func initializeLLM(provider llm.Provider, modelID string, temperature float64) (llms.Model, error) {
	switch provider {
	case llm.ProviderBedrock:
		return initializeBedrockLLM(modelID, temperature)
	case llm.ProviderOpenAI:
		return initializeOpenAILLM(modelID, temperature)
	case llm.ProviderAnthropic:
		return nil, fmt.Errorf("anthropic provider not yet implemented in external agent")
	case llm.ProviderOpenRouter:
		return nil, fmt.Errorf("openrouter provider not yet implemented in external agent")
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", provider)
	}
}

// initializeBedrockLLM creates a Bedrock LLM
func initializeBedrockLLM(modelID string, temperature float64) (llms.Model, error) {
	// Create Bedrock LLM with model
	llm, err := bedrock.New(bedrock.WithModel(modelID))
	if err != nil {
		return nil, fmt.Errorf("failed to create Bedrock LLM: %w", err)
	}

	return llm, nil
}

// initializeOpenAILLM creates an OpenAI LLM
func initializeOpenAILLM(modelID string, temperature float64) (llms.Model, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not found in environment variables")
	}

	// Create OpenAI LLM
	llm, err := openai.New(openai.WithModel(modelID))
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI LLM: %w", err)
	}

	return llm, nil
}

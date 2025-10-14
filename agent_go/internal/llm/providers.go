package llm

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/internal/utils"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/bedrock"
	"github.com/tmc/langchaingo/llms/openai"
)

// Provider represents the available LLM providers
type Provider string

const (
	ProviderBedrock    Provider = "bedrock"
	ProviderOpenAI     Provider = "openai"
	ProviderAnthropic  Provider = "anthropic"
	ProviderOpenRouter Provider = "openrouter"
)

// Config holds configuration for LLM initialization
type Config struct {
	Provider    Provider
	ModelID     string
	Temperature float64
	Tracers     []observability.Tracer
	TraceID     observability.TraceID
	// Fallback configuration for rate limiting
	FallbackModels []string
	MaxRetries     int
	// Logger for structured logging
	Logger utils.ExtendedLogger
}

// InitializeLLM creates and initializes an LLM based on the provider configuration
func InitializeLLM(config Config) (llms.Model, error) {
	var llm llms.Model
	var err error

	switch config.Provider {
	case ProviderBedrock:
		llm, err = initializeBedrockWithFallback(config)
	case ProviderOpenAI:
		llm, err = initializeOpenAIWithFallback(config)
	case ProviderAnthropic:
		llm, err = initializeAnthropic(config)
	case ProviderOpenRouter:
		llm, err = initializeOpenRouterWithFallback(config)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", config.Provider)
	}

	if err != nil {
		return nil, err
	}

	// Wrap the LLM with provider information and tracing
	return NewProviderAwareLLM(llm, config.Provider, config.ModelID, config.Tracers, config.TraceID, config.Logger), nil
}

// initializeBedrockWithFallback creates a Bedrock LLM with fallback models for rate limiting
func initializeBedrockWithFallback(config Config) (llms.Model, error) {
	// Try primary model first
	llm, err := initializeBedrock(config)
	if err == nil {
		return llm, nil
	}

	// If primary fails and we have fallback models, try them
	if len(config.FallbackModels) > 0 {
		logger := config.Logger
		logger.Infof("Primary Bedrock model failed, trying fallback models - primary_model: %s, fallback_models: %v, error: %s", config.ModelID, config.FallbackModels, err.Error())

		for _, fallbackModel := range config.FallbackModels {
			fallbackConfig := config
			fallbackConfig.ModelID = fallbackModel

			llm, err := initializeBedrock(fallbackConfig)
			if err == nil {
				logger.Infof("Successfully initialized fallback Bedrock model - fallback_model: %s", fallbackModel)
				return llm, nil
			}

			logger.Infof("Fallback Bedrock model failed - fallback_model: %s, error: %s", fallbackModel, err.Error())
		}
	}

	// If all models fail, return the original error
	return nil, fmt.Errorf("all Bedrock models failed: %w", err)
}

// initializeOpenAIWithFallback creates an OpenAI LLM with fallback models for rate limiting
func initializeOpenAIWithFallback(config Config) (llms.Model, error) {
	// Try primary model first
	llm, err := initializeOpenAI(config)
	if err == nil {
		return llm, nil
	}

	// If primary fails and we have fallback models, try them
	if len(config.FallbackModels) > 0 {
		logger := config.Logger
		logger.Infof("Primary OpenAI model failed, trying fallback models - primary_model: %s, fallback_models: %v, error: %s", config.ModelID, config.FallbackModels, err.Error())

		for _, fallbackModel := range config.FallbackModels {
			fallbackConfig := config
			fallbackConfig.ModelID = fallbackModel

			llm, err := initializeOpenAI(fallbackConfig)
			if err == nil {
				logger.Infof("Successfully initialized fallback OpenAI model - fallback_model: %s", fallbackModel)
				return llm, nil
			}

			logger.Infof("Fallback OpenAI model failed - fallback_model: %s, error: %s", fallbackModel, err.Error())
		}
	}

	// If all models fail, return the original error
	return nil, fmt.Errorf("all OpenAI models failed: %w", err)
}

// initializeOpenRouterWithFallback creates an OpenRouter LLM with fallback models for rate limiting
func initializeOpenRouterWithFallback(config Config) (llms.Model, error) {
	// Try primary model first
	llm, err := initializeOpenRouter(config)
	if err == nil {
		return llm, nil
	}

	// If primary fails and we have fallback models, try them
	if len(config.FallbackModels) > 0 {
		logger := config.Logger
		logger.Infof("Primary OpenRouter model failed, trying fallback models - primary_model: %s, fallback_models: %v, error: %s", config.ModelID, config.FallbackModels, err.Error())

		for _, fallbackModel := range config.FallbackModels {
			fallbackConfig := config
			fallbackConfig.ModelID = fallbackModel

			llm, err := initializeOpenRouter(fallbackConfig)
			if err == nil {
				logger.Infof("Successfully initialized fallback OpenRouter model - fallback_model: %s", fallbackModel)
				return llm, nil
			}

			logger.Infof("Fallback OpenRouter model failed - fallback_model: %s, error: %s", fallbackModel, err.Error())
		}
	}

	// If all models fail, return the original error
	return nil, fmt.Errorf("all OpenRouter models failed: %w", err)
}

// initializeBedrock creates and configures a Bedrock LLM instance
func initializeBedrock(config Config) (llms.Model, error) {
	// LLM Initialization event data - use typed structure directly
	llmMetadata := LLMMetadata{
		ModelVersion: config.ModelID,
		MaxTokens:    0, // Will be set at call time
		TopP:         config.Temperature,
		User:         "bedrock_user",
		CustomFields: map[string]string{
			"provider":  "bedrock",
			"operation": "llm_initialization",
		},
	}

	var logger = config.Logger

	// Emit LLM initialization start event
	emitLLMInitializationStart(config.Tracers, string(config.Provider), config.ModelID, config.Temperature, config.TraceID, llmMetadata)

	// Debug: Log AWS environment variables
	logger.Infof("Initializing Bedrock LLM with model: %s", config.ModelID)
	logger.Infof("AWS_REGION: %s", os.Getenv("AWS_REGION"))
	logger.Infof("AWS_ACCESS_KEY_ID: %s", os.Getenv("AWS_ACCESS_KEY_ID"))
	logger.Infof("AWS_SECRET_ACCESS_KEY: %s", os.Getenv("AWS_SECRET_ACCESS_KEY"))

	// Create Bedrock LLM with additional configuration
	// Note: The region is determined by AWS SDK configuration
	// Set AWS_REGION environment variable to us-east-1 for Bedrock access
	llm, err := bedrock.New(bedrock.WithModel(config.ModelID))
	if err != nil {
		logger.Errorf("Failed to create Bedrock LLM: %v", err)

		// Emit LLM initialization error event - use typed structure directly
		errorMetadata := LLMMetadata{
			ModelVersion: config.ModelID,
			User:         "bedrock_user",
			CustomFields: map[string]string{
				"provider":  "bedrock",
				"operation": OperationLLMInitialization,
				"error":     err.Error(),
				"status":    StatusLLMFailed,
			},
		}
		emitLLMInitializationError(config.Tracers, string(config.Provider), config.ModelID, OperationLLMInitialization, err, config.TraceID, errorMetadata)

		return nil, fmt.Errorf("create bedrock LLM: %w", err)
	}

	// Emit LLM initialization success event - use typed structure directly
	successMetadata := LLMMetadata{
		ModelVersion: config.ModelID,
		User:         "bedrock_user",
		CustomFields: map[string]string{
			"provider":     "bedrock",
			"status":       StatusLLMInitialized,
			"capabilities": CapabilityTextGeneration + "," + CapabilityToolCalling,
		},
	}
	emitLLMInitializationSuccess(config.Tracers, string(config.Provider), config.ModelID, CapabilityTextGeneration+","+CapabilityToolCalling, config.TraceID, successMetadata)

	logger.Infof("Initialized Bedrock LLM - model_id: %s", config.ModelID)
	return llm, nil
}

// IsO3O4Model detects o3/o4 models (OpenAI) for conditional logic in agent
func IsO3O4Model(modelID string) bool {
	// Covers gpt-4o, gpt-4.0, gpt-4.1, gpt-4, gpt-3.5, etc
	return strings.HasPrefix(modelID, "o3") ||
		strings.HasPrefix(modelID, "o4")
}

// initializeOpenAI creates and configures an OpenAI LLM instance
func initializeOpenAI(config Config) (llms.Model, error) {
	// Check for API key
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable is required for OpenAI provider")
	}

	// LLM Initialization event data - use typed structure directly
	llmMetadata := LLMMetadata{
		ModelVersion: config.ModelID,
		MaxTokens:    0, // Will be set at call time
		TopP:         config.Temperature,
		User:         "openai_user",
		CustomFields: map[string]string{
			"provider":  "openai",
			"operation": "llm_initialization",
		},
	}

	// Emit LLM initialization start event
	emitLLMInitializationStart(config.Tracers, string(config.Provider), config.ModelID, config.Temperature, config.TraceID, llmMetadata)

	// Set default model if not specified
	modelID := config.ModelID
	if modelID == "" {
		modelID = "gpt-4.1"
	}

	// Create OpenAI LLM
	var llm llms.Model
	var err error

	// Temperature is only set at call time (llms.WithTemperature), not at model creation for OpenAI models
	llm, err = openai.New(openai.WithModel(modelID))

	if err != nil {
		// Emit LLM initialization error event - use typed structure directly
		errorMetadata := LLMMetadata{
			ModelVersion: modelID,
			User:         "openai_user",
			CustomFields: map[string]string{
				"provider":  "openai",
				"operation": OperationLLMInitialization,
				"error":     err.Error(),
				"status":    StatusLLMFailed,
			},
		}
		emitLLMInitializationError(config.Tracers, string(config.Provider), modelID, OperationLLMInitialization, err, config.TraceID, errorMetadata)

		return nil, fmt.Errorf("create openai LLM: %w", err)
	}

	// Emit LLM initialization success event - use typed structure directly
	successMetadata := LLMMetadata{
		ModelVersion: modelID,
		User:         "openai_user",
		CustomFields: map[string]string{
			"provider":     "openai",
			"status":       StatusLLMInitialized,
			"capabilities": CapabilityTextGeneration + "," + CapabilityToolCalling,
		},
	}
	emitLLMInitializationSuccess(config.Tracers, string(config.Provider), modelID, CapabilityTextGeneration+","+CapabilityToolCalling, config.TraceID, successMetadata)

	logger := config.Logger
	logger.Infof("Initialized OpenAI LLM - model_id: %s", modelID)
	return llm, nil
}

// initializeAnthropic creates and configures an Anthropic LLM instance
func initializeAnthropic(config Config) (llms.Model, error) {
	// LLM Initialization event data - use typed structure directly
	llmMetadata := LLMMetadata{
		ModelVersion: config.ModelID,
		MaxTokens:    0, // Will be set at call time
		TopP:         config.Temperature,
		User:         "anthropic_user",
		CustomFields: map[string]string{
			"provider":  "anthropic",
			"operation": "llm_initialization",
		},
	}

	// Emit LLM initialization start event
	emitLLMInitializationStart(config.Tracers, string(config.Provider), config.ModelID, config.Temperature, config.TraceID, llmMetadata)

	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is required")
	}

	// Use provided model or default
	modelID := config.ModelID
	if modelID == "" {
		modelID = "claude-3-5-sonnet-20241022"
	}

	// Create Anthropic LLM
	llm, err := anthropic.New(
		anthropic.WithModel(modelID),
	)
	if err != nil {
		// Emit LLM initialization error event - use typed structure directly
		errorMetadata := LLMMetadata{
			ModelVersion: modelID,
			User:         "anthropic_user",
			CustomFields: map[string]string{
				"provider":  "anthropic",
				"operation": OperationLLMInitialization,
				"error":     err.Error(),
				"status":    StatusLLMFailed,
			},
		}
		emitLLMInitializationError(config.Tracers, string(config.Provider), modelID, OperationLLMInitialization, err, config.TraceID, errorMetadata)

		return nil, fmt.Errorf("create anthropic LLM: %w", err)
	}

	// Emit LLM initialization success event - use typed structure directly
	successMetadata := LLMMetadata{
		ModelVersion: modelID,
		User:         "anthropic_user",
		CustomFields: map[string]string{
			"provider":     "anthropic",
			"status":       StatusLLMInitialized,
			"capabilities": CapabilityTextGeneration + "," + CapabilityToolCalling,
		},
	}
	emitLLMInitializationSuccess(config.Tracers, string(config.Provider), modelID, CapabilityTextGeneration+","+CapabilityToolCalling, config.TraceID, successMetadata)

	logger := config.Logger
	logger.Infof("Initialized Anthropic LLM - model_id: %s", modelID)
	return llm, nil
}

// initializeOpenRouter creates and configures an OpenRouter LLM instance
func initializeOpenRouter(config Config) (llms.Model, error) {
	// LLM Initialization event data - use typed structure directly
	llmMetadata := LLMMetadata{
		ModelVersion: config.ModelID,
		MaxTokens:    0, // Will be set at call time
		TopP:         config.Temperature,
		User:         "openrouter_user",
		CustomFields: map[string]string{
			"provider":  "openrouter",
			"operation": OperationLLMInitialization,
		},
	}

	// Emit LLM initialization start event
	emitLLMInitializationStart(config.Tracers, string(config.Provider), config.ModelID, config.Temperature, config.TraceID, llmMetadata)

	// Check for API key
	if os.Getenv("OPEN_ROUTER_API_KEY") == "" {
		return nil, fmt.Errorf("OPEN_ROUTER_API_KEY environment variable is required for OpenRouter provider")
	}

	// Set default model if not specified
	modelID := config.ModelID
	if modelID == "" {
		modelID = "moonshotai/kimi-k2"
	}

	logger := config.Logger
	logger.Infof("🔧 Initializing OpenRouter LLM - model_id: %s, base_url: https://openrouter.ai/api/v1", modelID)

	// 🆕 DETAILED OPENROUTER INITIALIZATION LOGGING
	logger.Infof("🔧 [DEBUG] Creating OpenRouter LLM with OpenAI client...")
	logger.Infof("🔧 [DEBUG] Model: %s", modelID)
	logger.Infof("🔧 [DEBUG] Base URL: https://openrouter.ai/api/v1")
	logger.Infof("🔧 [DEBUG] API Key present: %v", os.Getenv("OPEN_ROUTER_API_KEY") != "")

	// Create OpenRouter LLM using OpenAI client with OpenRouter base URL
	llm, err := openai.New(
		openai.WithModel(modelID),
		openai.WithBaseURL("https://openrouter.ai/api/v1"),
		openai.WithToken(os.Getenv("OPEN_ROUTER_API_KEY")),
	)

	// 🆕 POST-INITIALIZATION LOGGING
	logger.Infof("🔧 [DEBUG] OpenRouter LLM creation completed - Error: %v, LLM: %v", err != nil, llm != nil)

	if err != nil {
		logger.Errorf("❌ Failed to create OpenRouter LLM - model: %s, error: %v", modelID, err)
		logger.Errorf("🔍 OpenRouter API Error Details:")
		logger.Errorf("   Error type: %T", err)
		logger.Errorf("   Error message: %s", err.Error())

		// Log additional error context for debugging
		logger.Errorf("🔍 OpenRouter API Error Context:")
		logger.Errorf("   Base URL: https://openrouter.ai/api/v1")
		logger.Errorf("   Model ID: %s", modelID)
		logger.Errorf("   API Key: %s", maskAPIKey(os.Getenv("OPEN_ROUTER_API_KEY")))

		// Emit LLM initialization error event - use typed structure directly
		errorMetadata := LLMMetadata{
			ModelVersion: modelID,
			User:         "openrouter_user",
			CustomFields: map[string]string{
				"provider":  "openrouter",
				"operation": OperationLLMInitialization,
				"error":     err.Error(),
				"status":    StatusLLMFailed,
			},
		}
		emitLLMInitializationError(config.Tracers, string(config.Provider), modelID, OperationLLMInitialization, err, config.TraceID, errorMetadata)

		return nil, fmt.Errorf("create openrouter LLM: %w", err)
	}

	// Emit LLM initialization success event - use typed structure directly
	successMetadata := LLMMetadata{
		ModelVersion: modelID,
		User:         "openrouter_user",
		CustomFields: map[string]string{
			"provider":     "openrouter",
			"status":       StatusLLMInitialized,
			"capabilities": CapabilityTextGeneration + "," + CapabilityToolCalling,
		},
	}
	emitLLMInitializationSuccess(config.Tracers, string(config.Provider), modelID, CapabilityTextGeneration+","+CapabilityToolCalling, config.TraceID, successMetadata)

	logger.Infof("✅ Successfully initialized OpenRouter LLM - model_id: %s", modelID)
	return llm, nil
}

// GetDefaultModel returns the default model for each provider from environment variables
func GetDefaultModel(provider Provider) string {
	switch provider {
	case ProviderBedrock:
		// Get primary model from environment variable
		if primaryModel := os.Getenv("BEDROCK_PRIMARY_MODEL"); primaryModel != "" {
			return primaryModel
		}
		return "us.anthropic.claude-sonnet-4-20250514-v1:0"
	case ProviderOpenAI:
		// Get primary model from environment variable
		if primaryModel := os.Getenv("OPENAI_PRIMARY_MODEL"); primaryModel != "" {
			return primaryModel
		}
		return "gpt-4.1-mini"
	case ProviderAnthropic:
		// Get primary model from environment variable
		if primaryModel := os.Getenv("ANTHROPIC_PRIMARY_MODEL"); primaryModel != "" {
			return primaryModel
		}
		return "claude-3-5-sonnet-20241022"
	case ProviderOpenRouter:
		// Get primary model from environment variable
		if primaryModel := os.Getenv("OPENROUTER_PRIMARY_MODEL"); primaryModel != "" {
			return primaryModel
		}
		return "moonshotai/kimi-k2"
	default:
		return ""
	}
}

// GetDefaultFallbackModels returns fallback models for each provider from environment variables
func GetDefaultFallbackModels(provider Provider) []string {
	switch provider {
	case ProviderBedrock:
		// Get Bedrock fallback models from environment variable
		fallbackModelsEnv := os.Getenv("BEDROCK_FALLBACK_MODELS")
		if fallbackModelsEnv != "" {
			// Split by comma and trim whitespace
			models := strings.Split(fallbackModelsEnv, ",")
			for i, model := range models {
				models[i] = strings.TrimSpace(model)
			}
			return models
		}
		// No fallback models if environment variable is not set
		return []string{}
	case ProviderOpenAI:
		// Get fallback models from environment variable
		fallbackModelsEnv := os.Getenv("OPENAI_FALLBACK_MODELS")
		if fallbackModelsEnv != "" {
			// Split by comma and trim whitespace
			models := strings.Split(fallbackModelsEnv, ",")
			for i, model := range models {
				models[i] = strings.TrimSpace(model)
			}
			return models
		}
		// No fallback models if environment variable is not set
		return []string{}
	case ProviderOpenRouter:
		// Get fallback models from environment variable
		fallbackModelsEnv := os.Getenv("OPENROUTER_FALLBACK_MODELS")
		if fallbackModelsEnv != "" {
			// Split by comma and trim whitespace
			models := strings.Split(fallbackModelsEnv, ",")
			for i, model := range models {
				models[i] = strings.TrimSpace(model)
			}
			return models
		}
		// No fallback models if environment variable is not set
		return []string{}
	default:
		return []string{}
	}
}

// GetCrossProviderFallbackModels returns cross-provider fallback models (e.g., OpenAI for Bedrock)
func GetCrossProviderFallbackModels(provider Provider) []string {
	switch provider {
	case ProviderBedrock:
		// Get OpenAI cross-provider fallback models
		openaiFallbackEnv := os.Getenv("BEDROCK_OPENAI_FALLBACK_MODELS")
		if openaiFallbackEnv != "" {
			// Split by comma and trim whitespace
			models := strings.Split(openaiFallbackEnv, ",")
			for i, model := range models {
				models[i] = strings.TrimSpace(model)
			}
			return models
		}
		// No cross-provider fallbacks if environment variable is not set
		return []string{}
	case ProviderOpenAI:
		// For OpenAI provider, no cross-provider fallbacks by default
		return []string{}
	case ProviderOpenRouter:
		// Get cross-provider fallback models for OpenRouter
		crossFallbackEnv := os.Getenv("OPENROUTER_CROSS_FALLBACK_MODELS")
		if crossFallbackEnv != "" {
			// Split by comma and trim whitespace
			models := strings.Split(crossFallbackEnv, ",")
			for i, model := range models {
				models[i] = strings.TrimSpace(model)
			}
			return models
		}
		// No cross-provider fallbacks if environment variable is not set
		return []string{}
	default:
		return []string{}
	}
}

// ValidateProvider checks if the provider is supported
func ValidateProvider(provider string) (Provider, error) {
	switch Provider(provider) {
	case ProviderBedrock, ProviderOpenAI, ProviderAnthropic, ProviderOpenRouter:
		return Provider(provider), nil
	default:
		return "", fmt.Errorf("unsupported provider: %s. Supported providers: bedrock, openai, anthropic, openrouter", provider)
	}
}

// ProviderAwareLLM is a wrapper around langchaingo LLM that preserves provider information
// and automatically captures token usage in LLM events
type ProviderAwareLLM struct {
	llms.Model
	provider Provider
	modelID  string
	tracers  []observability.Tracer
	traceID  observability.TraceID
	logger   utils.ExtendedLogger
}

// NewProviderAwareLLM creates a new provider-aware LLM wrapper
func NewProviderAwareLLM(llm llms.Model, provider Provider, modelID string, tracers []observability.Tracer, traceID observability.TraceID, logger utils.ExtendedLogger) *ProviderAwareLLM {
	return &ProviderAwareLLM{
		Model:    llm,
		provider: provider,
		modelID:  modelID,
		tracers:  tracers,
		traceID:  traceID,
		logger:   logger,
	}
}

// GetProvider returns the provider of this LLM
func (p *ProviderAwareLLM) GetProvider() Provider {
	return p.provider
}

// GetModelID returns the model ID of this LLM
func (p *ProviderAwareLLM) GetModelID() string {
	return p.modelID
}

// GenerateContent wraps the underlying LLM's GenerateContent method to automatically capture token usage
func (p *ProviderAwareLLM) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	// Note: LLM generation start event is now emitted at the agent level to avoid duplication

	// 🆕 DETAILED DEBUG LOGGING - Track execution flow
	startTime := time.Now()
	p.logger.Infof("🚀 [DEBUG] GenerateContent START - Provider: %s, Model: %s, Messages: %d",
		string(p.provider), p.modelID, len(messages))

	// 🆕 CONTEXT DEBUGGING
	if deadline, ok := ctx.Deadline(); ok {
		timeUntilDeadline := time.Until(deadline)
		p.logger.Infof("⏰ [DEBUG] Context deadline: %v, Time until deadline: %v", deadline, timeUntilDeadline)
	} else {
		p.logger.Infof("⏰ [DEBUG] Context has no deadline")
	}

	// 🆕 GOROUTINE DEBUGGING
	p.logger.Infof("🧵 [DEBUG] Goroutine count before LLM call: %d", runtime.NumGoroutine())

	// Automatically add usage parameter for OpenRouter requests to get cache token information
	if p.provider == ProviderOpenRouter {
		p.logger.Infof("🔧 Adding OpenRouter usage parameter for cache token information")
		options = append(options, WithOpenRouterUsage())
		p.logger.Infof("🔧 OpenRouter options count after adding usage parameter: %d", len(options))

		// 🆕 DETAILED OPENROUTER DEBUGGING
		p.logger.Infof("🔧 [DEBUG] About to call OpenRouter API - Time: %v", time.Now())
		p.logger.Infof("🔧 [DEBUG] OpenRouter request details - Messages: %d, Options: %d", len(messages), len(options))

		// Log message content lengths for debugging
		for i, msg := range messages {
			contentLength := 0
			for _, part := range msg.Parts {
				if textPart, ok := part.(llms.TextContent); ok {
					contentLength += len(textPart.Text)
				}
			}
			p.logger.Infof("🔧 [DEBUG] Message %d - Role: %s, Content length: %d", i+1, msg.Role, contentLength)
		}
	}

	// 🆕 TIMING DEBUGGING - Track the actual LLM call
	llmCallStart := time.Now()
	p.logger.Infof("📞 [DEBUG] About to call p.Model.GenerateContent - Time: %v", llmCallStart)

	// 🆕 DETAILED EXECUTION TRACKING
	p.logger.Infof("🔍 [DEBUG] Context details - Err: %v, Done: %v", ctx.Err(), ctx.Done())
	p.logger.Infof("🔍 [DEBUG] Options count: %d", len(options))
	for i, opt := range options {
		p.logger.Infof("🔍 [DEBUG] Option %d: %T", i+1, opt)
	}
	p.logger.Infof("🔍 [DEBUG] Messages count: %d", len(messages))
	p.logger.Infof("🔍 [DEBUG] About to call underlying LLM.GenerateContent...")

	// Call the underlying LLM
	resp, err := p.Model.GenerateContent(ctx, messages, options...)

	// 🆕 IMMEDIATE POST-CALL LOGGING
	p.logger.Infof("🔍 [DEBUG] Underlying LLM.GenerateContent returned - Time: %v", time.Now())
	p.logger.Infof("🔍 [DEBUG] Return values - Error: %v, Response: %v", err != nil, resp != nil)

	// 🆕 TIMING DEBUGGING - Track LLM call completion
	llmCallDuration := time.Since(llmCallStart)
	totalDuration := time.Since(startTime)
	p.logger.Infof("📞 [DEBUG] p.Model.GenerateContent completed - Duration: %v, Total duration: %v", llmCallDuration, totalDuration)

	// 🆕 POST-CALL DEBUGGING
	p.logger.Infof("🧵 [DEBUG] Goroutine count after LLM call: %d", runtime.NumGoroutine())
	if err != nil {
		p.logger.Infof("❌ [DEBUG] LLM call failed - Error: %v, Error type: %T", err, err)
	} else {
		p.logger.Infof("✅ [DEBUG] LLM call succeeded - Response: %v", resp != nil)
	}

	// 🆕 ENHANCED BEDROCK RESPONSE DEBUGGING
	p.logger.Infof("🔍 Raw Bedrock response received - err: %v, resp: %v", err, resp != nil)

	// 🆕 DETAILED BEDROCK RESPONSE ANALYSIS
	if resp != nil {
		p.logger.Infof("🔍 Response type: %T", resp)
		p.logger.Infof("🔍 Response pointer: %p", resp)
		p.logger.Infof("🔍 Response.Choices pointer: %p", resp.Choices)
		if resp.Choices != nil {
			p.logger.Infof("🔍 Response.Choices length: %d", len(resp.Choices))
			for i, choice := range resp.Choices {
				p.logger.Infof("🔍 Choice %d - Type: %T, Content: %v, Content length: %d",
					i, choice, choice.Content != "", len(choice.Content))
				if choice.Content != "" {
					p.logger.Infof("🔍 Choice %d - First 100 chars: %s", i, truncateString(choice.Content, 100))
				}

				// 🆕 OPENROUTER CACHE DEBUGGING
				if p.provider == ProviderOpenRouter && choice.GenerationInfo != nil {
					p.logger.Infof("🔍 OpenRouter GenerationInfo keys: %v", getMapKeys(choice.GenerationInfo))
					for key, value := range choice.GenerationInfo {
						if strings.Contains(strings.ToLower(key), "cache") {
							p.logger.Infof("🔍 OpenRouter Cache Field - %s: %v (type: %T)", key, value, value)
						}
					}
				}
			}
		}
	}

	// 🆕 AWS BEDROCK SPECIFIC ERROR DETAILS
	if err != nil && p.provider == ProviderBedrock {
		p.logger.Infof("🔍 AWS Bedrock Error Details:")
		p.logger.Infof("🔍 Error type: %T", err)
		p.logger.Infof("🔍 Error message: %s", err.Error())

		// Check for AWS-specific error types
		if awsErr, ok := err.(interface{ Code() string }); ok {
			p.logger.Infof("🔍 AWS Error Code: %s", awsErr.Code())
		}
		if awsErr, ok := err.(interface{ Message() string }); ok {
			p.logger.Infof("🔍 AWS Error Message: %s", awsErr.Message())
		}
		if awsErr, ok := err.(interface{ RequestID() string }); ok {
			p.logger.Infof("🔍 AWS Request ID: %s", awsErr.RequestID())
		}

		// Log the full error for debugging
		p.logger.Infof("🔍 Full error details: %+v", err)
	}

	if resp != nil {
		p.logger.Infof("🔍 Response structure - Choices: %v, Choices count: %d", resp.Choices != nil, len(resp.Choices))
		if resp.Choices != nil && len(resp.Choices) > 0 {
			choice := resp.Choices[0]
			p.logger.Infof("🔍 First choice - Content: %v, Content length: %d, GenerationInfo: %v",
				choice.Content != "", len(choice.Content), choice.GenerationInfo != nil)
			if choice.GenerationInfo != nil {
				p.logger.Infof("🔍 GenerationInfo keys: %v", getMapKeys(choice.GenerationInfo))
			}
		}
	}

	// Check if we have a valid response
	if err != nil {
		// 🆕 ENHANCED ERROR LOGGING FOR TURN 2 DEBUGGING
		p.logger.Infof("❌ LLM generation failed - provider: %s, model: %s, error: %v", string(p.provider), p.modelID, err)
		p.logger.Infof("❌ Error details - type: %T, message: %s", err, err.Error())

		// 🆕 SERVER ERROR DETECTION AND LOGGING
		if strings.Contains(err.Error(), "502") || strings.Contains(err.Error(), "Provider returned error") {
			p.logger.Warnf("🔄 502 Bad Gateway error detected, will trigger fallback mechanism")
			p.logger.Warnf("🔄 Server error details - provider: %s, model: %s, error: %s", string(p.provider), p.modelID, err.Error())
		} else if strings.Contains(err.Error(), "503") {
			p.logger.Warnf("🔄 503 Service Unavailable error detected, will trigger fallback mechanism")
		} else if strings.Contains(err.Error(), "504") {
			p.logger.Warnf("🔄 504 Gateway Timeout error detected, will trigger fallback mechanism")
		} else if strings.Contains(err.Error(), "500") {
			p.logger.Warnf("🔄 500 Internal Server Error detected, will trigger fallback mechanism")
		}

		// Log the messages that were sent to help debug
		p.logger.Infof("📤 Messages sent to LLM - count: %d", len(messages))
		for i, msg := range messages {
			// Calculate actual content length from message parts
			contentLength := 0
			for _, part := range msg.Parts {
				if textPart, ok := part.(llms.TextContent); ok {
					contentLength += len(textPart.Text)
				}
			}
			p.logger.Infof("📤 Message %d - Role: %s, Content length: %d", i+1, msg.Role, contentLength)
		}

		// Emit LLM generation error event with rich debugging information
		errorMetadata := LLMMetadata{
			User: "llm_generation_user",
			CustomFields: map[string]string{
				"provider":        string(p.provider),
				"model_id":        p.modelID,
				"messages":        fmt.Sprintf("%d", len(messages)),
				"temperature":     fmt.Sprintf("%f", getTemperatureFromOptions(options)),
				"message_content": extractMessageContentAsString(messages),
				"error":           err.Error(),
				"error_type":      fmt.Sprintf("%T", err),
				"debug_note":      "Enhanced error logging for turn 2 debugging",
			},
		}
		emitLLMGenerationError(p.tracers, string(p.provider), p.modelID, OperationLLMGeneration, len(messages), getTemperatureFromOptions(options), extractMessageContentAsString(messages), err, p.traceID, errorMetadata)

		return nil, err
	}

	// 🆕 ENHANCED RESPONSE VALIDATION LOGGING
	p.logger.Infof("✅ LLM generation succeeded - provider: %s, model: %s", string(p.provider), p.modelID)

	// Validate response structure
	if resp == nil {
		p.logger.Infof("❌ Response is nil - this will cause 'no results' error")

		// Emit LLM generation error event for nil response
		errorMetadata := LLMMetadata{
			User: "llm_generation_user",
			CustomFields: map[string]string{
				"debug_note": "Response validation failed - nil response",
			},
		}
		emitLLMGenerationError(p.tracers, string(p.provider), p.modelID, OperationLLMGeneration, len(messages), getTemperatureFromOptions(options), extractMessageContentAsString(messages), fmt.Errorf("response validation failed - nil response"), p.traceID, errorMetadata)

		return nil, fmt.Errorf("response is nil")
	}

	if resp.Choices == nil {
		p.logger.Infof("❌ Response.Choices is nil - this will cause 'no results' error")

		// Enhanced logging for ALL providers when choices is nil
		p.logger.Errorf("🔍 Nil Choices Debug Information for %s:", string(p.provider))
		p.logger.Errorf("   Model ID: %s", p.modelID)
		p.logger.Errorf("   Provider: %s", string(p.provider))
		p.logger.Errorf("   Response Type: %T", resp)
		p.logger.Errorf("   Response Pointer: %p", resp)
		p.logger.Errorf("   Response Nil: %v", resp == nil)

		// Log the ENTIRE response structure for comprehensive debugging
		p.logger.Errorf("🔍 COMPLETE LLM RESPONSE STRUCTURE:")
		p.logger.Errorf("   Full Response: %+v", resp)

		// Log the options that were passed to the LLM
		p.logger.Errorf("🔍 LLM CALL OPTIONS:")
		for i, opt := range options {
			p.logger.Errorf("   Option %d: %T = %+v", i+1, opt, opt)
		}

		// Log the messages that were sent to the LLM
		p.logger.Errorf("🔍 MESSAGES SENT TO LLM:")
		for i, msg := range messages {
			p.logger.Errorf("   Message %d - Role: %s, Parts: %d", i+1, msg.Role, len(msg.Parts))
			for j, part := range msg.Parts {
				p.logger.Errorf("     Part %d - Type: %T, Content: %+v", j+1, part, part)
			}
		}

		// Emit LLM generation error event for nil choices
		errorMetadata := LLMMetadata{
			User: "llm_generation_user",
			CustomFields: map[string]string{
				"provider":        string(p.provider),
				"model_id":        p.modelID,
				"messages":        fmt.Sprintf("%d", len(messages)),
				"temperature":     fmt.Sprintf("%f", getTemperatureFromOptions(options)),
				"message_content": extractMessageContentAsString(messages),
				"error":           "Response.Choices is nil",
				"debug_note":      "Response validation failed - nil choices",
			},
		}
		emitLLMGenerationError(p.tracers, string(p.provider), p.modelID, OperationLLMGeneration, len(messages), getTemperatureFromOptions(options), extractMessageContentAsString(messages), fmt.Errorf("response.Choices is nil"), p.traceID, errorMetadata)

		return nil, fmt.Errorf("response.Choices is nil")
	}

	if len(resp.Choices) == 0 {
		p.logger.Infof("❌ Response.Choices is empty array - this will cause 'no results' error")

		// Enhanced logging for ALL providers when choices array is empty
		p.logger.Errorf("🔍 Empty Choices Array Debug Information for %s:", string(p.provider))
		p.logger.Errorf("   Model ID: %s", p.modelID)
		p.logger.Errorf("   Provider: %s", string(p.provider))
		p.logger.Errorf("   Response Type: %T", resp)
		p.logger.Errorf("   Response Pointer: %p", resp)
		p.logger.Errorf("   Choices Array Length: %d", len(resp.Choices))
		p.logger.Errorf("   Choices Array Nil: %v", resp.Choices == nil)
		p.logger.Errorf("   Choices Array Cap: %d", cap(resp.Choices))

		// Log the ENTIRE response structure for comprehensive debugging
		p.logger.Errorf("🔍 COMPLETE LLM RESPONSE STRUCTURE:")
		p.logger.Errorf("   Full Response: %+v", resp)

		// Log the options that were passed to the LLM
		p.logger.Errorf("🔍 LLM CALL OPTIONS:")
		for i, opt := range options {
			p.logger.Errorf("   Option %d: %T = %+v", i+1, opt, opt)
		}

		// Log the messages that were sent to the LLM
		p.logger.Errorf("🔍 MESSAGES SENT TO LLM:")
		for i, msg := range messages {
			p.logger.Errorf("   Message %d - Role: %s, Parts: %d", i+1, msg.Role, len(msg.Parts))
			for j, part := range msg.Parts {
				p.logger.Errorf("     Part %d - Type: %T, Content: %+v", j+1, part, part)
			}
		}

		// Emit LLM generation error event for empty choices
		errorMetadata := LLMMetadata{
			User: "llm_generation_user",
			CustomFields: map[string]string{
				"provider":        string(p.provider),
				"model_id":        p.modelID,
				"messages":        fmt.Sprintf("%d", len(messages)),
				"temperature":     fmt.Sprintf("%f", getTemperatureFromOptions(options)),
				"message_content": extractMessageContentAsString(messages),
				"error":           "Response.Choices is empty",
				"debug_note":      "Response validation failed - empty choices array",
			},
		}
		emitLLMGenerationError(p.tracers, string(p.provider), p.modelID, OperationLLMGeneration, len(messages), getTemperatureFromOptions(options), extractMessageContentAsString(messages), fmt.Errorf("response.Choices is empty"), p.traceID, errorMetadata)

		return nil, fmt.Errorf("response.Choices is empty")
	}

	// Validate first choice has content
	firstChoice := resp.Choices[0]
	if firstChoice.Content == "" {
		// Check if this is a valid tool call response
		if firstChoice.ToolCalls != nil && len(firstChoice.ToolCalls) > 0 {
			p.logger.Infof("✅ Valid tool call response detected - Content is empty but ToolCalls present")
			p.logger.Infof("   Tool Calls: %d", len(firstChoice.ToolCalls))
			for i, toolCall := range firstChoice.ToolCalls {
				p.logger.Infof("   Tool Call %d: ID=%s, Type=%s", i+1, toolCall.ID, toolCall.Type)
			}
			// This is a valid response, continue processing
		} else if firstChoice.FuncCall != nil { // Legacy function call handling
			p.logger.Infof("✅ Valid function call response detected - Content is empty but FuncCall present")
			p.logger.Infof("   Function Call: Name=%s", firstChoice.FuncCall.Name)
			// This is a valid response, continue processing
		} else {
			// This is actually an empty content error
			p.logger.Infof("❌ Choice.Content is empty - this will cause 'no results' error")

			// Enhanced logging for ALL providers when choice content is empty
			p.logger.Errorf("🔍 Empty Choice Content Debug Information for %s:", string(p.provider))
			p.logger.Errorf("   Model ID: %s", p.modelID)
			p.logger.Errorf("   Provider: %s", string(p.provider))
			p.logger.Errorf("   Response Type: %T", resp)
			p.logger.Errorf("   Response Pointer: %p", resp)
			p.logger.Errorf("   Choices Count: %d", len(resp.Choices))
			p.logger.Errorf("   First Choice Type: %T", firstChoice)
			p.logger.Errorf("   First Choice Content Empty: %v", firstChoice.Content == "")
			p.logger.Errorf("   First Choice Content Length: %d", len(firstChoice.Content))

			// Log the ENTIRE response structure for comprehensive debugging
			p.logger.Errorf("🔍 COMPLETE LLM RESPONSE STRUCTURE:")
			p.logger.Errorf("   Full Response: %+v", resp)

			// Log the options that were passed to the LLM
			p.logger.Errorf("🔍 LLM CALL OPTIONS:")
			for i, opt := range options {
				p.logger.Errorf("   Option %d: %T = %+v", i+1, opt, opt)
			}

			// Log the messages that were sent to the LLM
			p.logger.Errorf("🔍 MESSAGES SENT TO LLM:")
			for i, msg := range messages {
				p.logger.Errorf("   Message %d - Role: %s, Parts: %d", i+1, msg.Role, len(msg.Parts))
				for j, part := range msg.Parts {
					p.logger.Errorf("     Part %d - Type: %T, Content: %+v", j+1, part, part)
				}
			}

			// Emit LLM generation error event for empty choice content
			errorMetadata := LLMMetadata{
				User: "llm_generation_user",
				CustomFields: map[string]string{
					"provider":        string(p.provider),
					"model_id":        p.modelID,
					"messages":        fmt.Sprintf("%d", len(messages)),
					"temperature":     fmt.Sprintf("%f", getTemperatureFromOptions(options)),
					"message_content": extractMessageContentAsString(messages),
					"error":           "Choice.Content is empty",
					"debug_note":      "Response validation failed - empty content",
				},
			}
			emitLLMGenerationError(p.tracers, string(p.provider), p.modelID, OperationLLMGeneration, len(messages), getTemperatureFromOptions(options), extractMessageContentAsString(messages), fmt.Errorf("choice.Content is empty"), p.traceID, errorMetadata)

			return nil, fmt.Errorf("choice.Content is empty")
		}
	}

	// 🆕 ENHANCED SUCCESS LOGGING
	p.logger.Infof("✅ LLM generation validation passed - provider: %s, model: %s", string(p.provider), p.modelID)
	p.logger.Infof("✅ Response structure - Choices: %v, Choices count: %d", resp.Choices != nil, len(resp.Choices))
	if resp.Choices != nil && len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		p.logger.Infof("✅ First choice - Content: %v, Content length: %d, GenerationInfo: %v",
			choice.Content != "", len(choice.Content), choice.GenerationInfo != nil)
		if choice.GenerationInfo != nil {
			p.logger.Infof("✅ GenerationInfo keys: %v", getMapKeys(choice.GenerationInfo))
		}
	}

	// Extract token usage from GenerationInfo if available
	if resp.Choices != nil && len(resp.Choices) > 0 && resp.Choices[0].GenerationInfo != nil {
		// Extract token usage and create success event with comprehensive data
		usage := extractTokenUsageFromGenerationInfo(resp.Choices[0].GenerationInfo)

		// Calculate total tokens if not provided by the provider
		if usage.TotalTokens == 0 && usage.InputTokens > 0 && usage.OutputTokens > 0 {
			usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		}

		p.logger.Infof("Token usage extracted: Input=%d, Output=%d, Total=%d", usage.InputTokens, usage.OutputTokens, usage.TotalTokens)

		// Emit LLM generation success event with token usage
		successMetadata := LLMMetadata{
			User: "llm_generation_user",
			CustomFields: map[string]string{
				"provider":        string(p.provider),
				"model_id":        p.modelID,
				"messages":        fmt.Sprintf("%d", len(messages)),
				"temperature":     fmt.Sprintf("%f", getTemperatureFromOptions(options)),
				"message_content": extractMessageContentAsString(messages),
				"response_length": fmt.Sprintf("%d", len(resp.Choices[0].Content)),
				"choices_count":   fmt.Sprintf("%d", len(resp.Choices)),
				"input_tokens":    fmt.Sprintf("%d", usage.InputTokens),
				"output_tokens":   fmt.Sprintf("%d", usage.OutputTokens),
				"total_tokens":    fmt.Sprintf("%d", usage.TotalTokens),
				"note":            "Token usage extracted from GenerationInfo",
			},
		}
		emitLLMGenerationSuccess(p.tracers, string(p.provider), p.modelID, OperationLLMGeneration, len(messages), getTemperatureFromOptions(options), extractMessageContentAsString(messages), len(resp.Choices[0].Content), len(resp.Choices), p.traceID, successMetadata)
	} else {
		// No token usage available, emit success event without usage
		p.logger.Infof("No GenerationInfo available")

		// Emit LLM generation success event without token usage
		successMetadata := LLMMetadata{
			User: "llm_generation_user",
			CustomFields: map[string]string{
				"provider":        string(p.provider),
				"model_id":        p.modelID,
				"messages":        fmt.Sprintf("%d", len(messages)),
				"temperature":     fmt.Sprintf("%f", getTemperatureFromOptions(options)),
				"message_content": extractMessageContentAsString(messages),
				"response_length": fmt.Sprintf("%d", len(resp.Choices[0].Content)),
				"choices_count":   fmt.Sprintf("%d", len(resp.Choices)),
				"note":            "No GenerationInfo available for token usage",
			},
		}
		emitLLMGenerationSuccess(p.tracers, string(p.provider), p.modelID, OperationLLMGeneration, len(messages), getTemperatureFromOptions(options), extractMessageContentAsString(messages), len(resp.Choices[0].Content), len(resp.Choices), p.traceID, successMetadata)
	}

	return resp, nil
}

// extractMessageContentAsString converts message content to a readable string
func extractMessageContentAsString(messages []llms.MessageContent) string {
	if len(messages) == 0 {
		return "no messages"
	}

	var result strings.Builder
	for i, msg := range messages {
		if i > 0 {
			result.WriteString(" | ")
		}
		result.WriteString(fmt.Sprintf("Role:%s", msg.Role))

		for j, part := range msg.Parts {
			if j > 0 {
				result.WriteString(",")
			}
			if textPart, ok := part.(llms.TextContent); ok {
				content := textPart.Text
				if len(content) > 100 {
					content = content[:100] + "..."
				}
				result.WriteString(fmt.Sprintf("Text:%s", content))
			} else {
				result.WriteString(fmt.Sprintf("Part:%T", part))
			}
		}
	}
	return result.String()
}

// getTemperatureFromOptions extracts temperature from call options
func getTemperatureFromOptions(options []llms.CallOption) float64 {
	// For now, return default temperature since CallOption is a function type
	// and we can't easily extract the temperature value
	return 0.7 // default temperature
}

// extractMessageContent converts message content to a structured format
func extractMessageContent(messages []llms.MessageContent) []map[string]interface{} {
	var messageList []map[string]interface{}

	for i, msg := range messages {
		messageData := map[string]interface{}{
			"index": i,
			"role":  msg.Role,
		}

		// Extract text content from message parts
		var contentParts []string
		for _, part := range msg.Parts {
			if textPart, ok := part.(llms.TextContent); ok {
				contentParts = append(contentParts, textPart.Text)
			}
		}

		if len(contentParts) > 0 {
			messageData["content"] = contentParts
		}

		messageList = append(messageList, messageData)
	}

	return messageList
}

// getMapKeys extracts keys from a map for debugging purposes
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// truncateString truncates a string to a specified length
func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}

// maskAPIKey masks the API key in logs for security
func maskAPIKey(key string) string {
	if len(key) < 4 {
		return "***" // Too short to mask
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// WithOpenRouterUsage enables usage parameter for OpenRouter requests to get cache token information
func WithOpenRouterUsage() llms.CallOption {
	return func(opts *llms.CallOptions) {
		// 🆕 DETAILED OPENROUTER USAGE LOGGING
		fmt.Printf("🔧 [DEBUG] WithOpenRouterUsage called - opts: %+v\n", opts)

		// Set the usage parameter in the request metadata (not CallOptions metadata)
		// This will be passed to the actual HTTP request body
		if opts.Metadata == nil {
			fmt.Printf("🔧 [DEBUG] Creating new metadata map\n")
			opts.Metadata = make(map[string]interface{})
		} else {
			fmt.Printf("🔧 [DEBUG] Using existing metadata map: %+v\n", opts.Metadata)
		}

		fmt.Printf("🔧 [DEBUG] Setting usage parameter...\n")
		opts.Metadata["usage"] = map[string]interface{}{
			"include": true,
		}

		// Debug logging to verify metadata is being set
		fmt.Printf("🔧 DEBUG: Set OpenRouter usage metadata: %+v\n", opts.Metadata)
		fmt.Printf("🔧 [DEBUG] WithOpenRouterUsage completed\n")
	}
}

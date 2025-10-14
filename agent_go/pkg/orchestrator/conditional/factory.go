package conditional

import (
	"context"
	"fmt"
	"mcp-agent/agent_go/internal/llm"
	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/internal/utils"
	"mcp-agent/agent_go/pkg/events"
	"mcp-agent/agent_go/pkg/orchestrator/agents"
	"time"
)

// ConditionalLLMFactory creates conditional LLM instances with automatic event emission
type ConditionalLLMFactory struct {
	logger utils.ExtendedLogger
	tracer observability.Tracer
}

// NewConditionalLLMFactory creates a new conditional LLM factory
func NewConditionalLLMFactory(logger utils.ExtendedLogger, tracer observability.Tracer) *ConditionalLLMFactory {
	return &ConditionalLLMFactory{
		logger: logger,
		tracer: tracer,
	}
}

// CreateConditionalLLM creates a conditional LLM with automatic event emission setup
func (f *ConditionalLLMFactory) CreateConditionalLLM(
	config *agents.OrchestratorAgentConfig,
	eventEmitter func(context.Context, events.EventData),
) (*ConditionalLLM, error) {
	f.logger.Infof("🔧 Creating conditional LLM for decision making with automatic event emission")

	// Generate trace ID for this conditional LLM
	traceID := observability.TraceID(fmt.Sprintf("conditional-llm-%d", time.Now().UnixNano()))

	// Build fallback models list
	var fallbackModels []string

	// Add custom fallback models from config if provided
	if len(config.FallbackModels) > 0 {
		fallbackModels = append(fallbackModels, config.FallbackModels...)
		f.logger.Infof("🔧 Using custom fallback models for conditional LLM: %v", config.FallbackModels)
	} else {
		// Use default fallback models for the provider
		fallbackModels = append(fallbackModels, llm.GetDefaultFallbackModels(llm.Provider(config.Provider))...)
		f.logger.Infof("🔧 Using default fallback models for conditional LLM provider: %s", config.Provider)
	}

	// Add cross-provider fallback models if configured
	if config.CrossProviderFallback != nil && len(config.CrossProviderFallback.Models) > 0 {
		crossProviderFallbacks := llm.GetCrossProviderFallbackModels(llm.Provider(config.CrossProviderFallback.Provider))
		fallbackModels = append(fallbackModels, crossProviderFallbacks...)
		f.logger.Infof("🔧 Added cross-provider fallback models for conditional LLM: %v", crossProviderFallbacks)
	} else {
		// Add default cross-provider fallbacks
		crossProviderFallbacks := llm.GetCrossProviderFallbackModels(llm.Provider(config.Provider))
		fallbackModels = append(fallbackModels, crossProviderFallbacks...)
		f.logger.Infof("🔧 Added default cross-provider fallback models for conditional LLM: %v", crossProviderFallbacks)
	}

	// Create LLM configuration for conditional decisions
	// Use low temperature (0.1) for consistent decisions
	llmConfig := llm.Config{
		Provider:       llm.Provider(config.Provider),
		ModelID:        config.Model,
		Temperature:    0.1, // Low temperature for consistent decisions
		Tracers:        nil, // Tracers will be set later if needed
		TraceID:        traceID,
		FallbackModels: fallbackModels,
		MaxRetries:     config.MaxRetries,
		Logger:         f.logger,
	}

	// Initialize LLM using the existing factory
	llmInstance, err := llm.InitializeLLM(llmConfig)
	if err != nil {
		f.logger.Errorf("❌ Failed to create conditional LLM: %v", err)
		return nil, fmt.Errorf("failed to create conditional LLM: %w", err)
	}

	// Create conditional LLM instance with event emitter
	conditionalLLM := NewConditionalLLMWithEventEmitter(llmInstance, f.logger, f.tracer, eventEmitter)

	f.logger.Infof("✅ Conditional LLM created successfully with automatic event emission - Provider: %s, Model: %s, Temperature: %.1f",
		config.Provider, config.Model, 0.1)

	return conditionalLLM, nil
}

// CreateConditionalLLMWithEventBridge creates a conditional LLM with event bridge integration
func (f *ConditionalLLMFactory) CreateConditionalLLMWithEventBridge(
	config *agents.OrchestratorAgentConfig,
	eventBridge interface{},
) (*ConditionalLLM, error) {
	// Create event emitter function that uses the event bridge
	eventEmitter := func(ctx context.Context, data events.EventData) {
		if eventBridge == nil {
			f.logger.Warnf("⚠️ No event bridge available, cannot emit conditional LLM event")
			return
		}

		// Create agent event
		agentEvent := &events.AgentEvent{
			Type:      events.OrchestratorAgentStart, // Will be overridden by the conditional LLM
			Timestamp: time.Now(),
			Data:      data,
		}

		// Emit through event bridge - cast to the proper interface
		if bridge, ok := eventBridge.(interface {
			HandleEvent(context.Context, *events.AgentEvent) error
		}); ok {
			if err := bridge.HandleEvent(ctx, agentEvent); err != nil {
				f.logger.Warnf("⚠️ Failed to emit conditional LLM event: %v", err)
			}
		} else {
			f.logger.Warnf("⚠️ Event bridge does not implement HandleEvent method: %T", eventBridge)
		}
	}

	return f.CreateConditionalLLM(config, eventEmitter)
}

package conditional

import (
	"context"
	"encoding/json"
	"fmt"
	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/internal/utils"
	"mcp-agent/agent_go/pkg/events"
	"mcp-agent/agent_go/pkg/mcpagent"
	"time"

	"github.com/tmc/langchaingo/llms"
)

// ConditionalResponse represents a true/false response with reasoning
type ConditionalResponse struct {
	Result bool   `json:"result"`
	Reason string `json:"reason"`
}

// GetResult returns the boolean result
func (cr *ConditionalResponse) GetResult() bool {
	return cr.Result
}

// ConditionalLLM provides a simple true/false decision service
type ConditionalLLM struct {
	llm          llms.Model
	logger       utils.ExtendedLogger
	tracer       observability.Tracer
	eventEmitter func(context.Context, events.EventData) // Event emitter function
}

// NewConditionalLLM creates a new conditional LLM instance
func NewConditionalLLM(
	llm llms.Model,
	logger utils.ExtendedLogger,
	tracer observability.Tracer,
) *ConditionalLLM {
	return &ConditionalLLM{
		llm:    llm,
		logger: logger,
		tracer: tracer,
	}
}

// NewConditionalLLMWithEventEmitter creates a new conditional LLM instance with event emitter
func NewConditionalLLMWithEventEmitter(
	llm llms.Model,
	logger utils.ExtendedLogger,
	tracer observability.Tracer,
	eventEmitter func(context.Context, events.EventData),
) *ConditionalLLM {
	return &ConditionalLLM{
		llm:          llm,
		logger:       logger,
		tracer:       tracer,
		eventEmitter: eventEmitter,
	}
}

// SetEventEmitter sets the event emitter function
func (cl *ConditionalLLM) SetEventEmitter(emitter func(context.Context, events.EventData)) {
	cl.eventEmitter = emitter
}

// Decide makes a true/false decision based on context and question
func (cl *ConditionalLLM) Decide(ctx context.Context, context, question string, stepIndex, iteration int) (*ConditionalResponse, error) {
	startTime := time.Now()

	cl.logger.Infof("🤔 Making conditional decision: %s", question)

	// Build prompt
	prompt := GetPrompt(context, question)
	schema := GetSchema()

	// Create structured output generator
	config := mcpagent.LangchaingoStructuredOutputConfig{
		UseJSONMode:    true,
		ValidateOutput: true,
		MaxRetries:     2,
	}
	generator := mcpagent.NewLangchaingoStructuredOutputGenerator(cl.llm, config, cl.logger)

	// Generate structured output
	jsonOutput, err := generator.GenerateStructuredOutput(ctx, prompt, schema)
	if err != nil {
		duration := time.Since(startTime)
		cl.logger.Errorf("❌ Conditional decision failed: %v", err)

		// Emit orchestrator agent error event
		if cl.eventEmitter != nil {
			errorEvent := &events.OrchestratorAgentErrorEvent{
				BaseEventData: events.BaseEventData{
					Timestamp: time.Now(),
				},
				AgentType: "conditional",
				AgentName: "conditional-llm",
				Objective: fmt.Sprintf("Conditional decision: %s", question),
				Error:     err.Error(),
				Duration:  duration,
				StepIndex: stepIndex,
				Iteration: iteration,
			}
			cl.eventEmitter(ctx, errorEvent)
		}

		return nil, fmt.Errorf("failed to make conditional decision: %w", err)
	}

	// Parse JSON
	var result ConditionalResponse
	if err := json.Unmarshal([]byte(jsonOutput), &result); err != nil {
		duration := time.Since(startTime)
		cl.logger.Errorf("❌ Failed to parse conditional response: %v", err)

		// Emit orchestrator agent error event
		if cl.eventEmitter != nil {
			errorEvent := &events.OrchestratorAgentErrorEvent{
				BaseEventData: events.BaseEventData{
					Timestamp: time.Now(),
				},
				AgentType: "conditional",
				AgentName: "conditional-llm",
				Objective: fmt.Sprintf("Conditional decision: %s", question),
				Error:     err.Error(),
				Duration:  duration,
				StepIndex: stepIndex,
				Iteration: iteration,
			}
			cl.eventEmitter(ctx, errorEvent)
		}

		return nil, fmt.Errorf("failed to parse conditional response: %w", err)
	}

	duration := time.Since(startTime)
	cl.logger.Infof("✅ Conditional decision made: result=%t, reason=%s", result.Result, result.Reason)

	// Emit orchestrator agent end event
	if cl.eventEmitter != nil {
		resultText := fmt.Sprintf("Decision: %t, Reason: %s", result.Result, result.Reason)
		endEvent := &events.OrchestratorAgentEndEvent{
			BaseEventData: events.BaseEventData{
				Timestamp: time.Now(),
			},
			AgentType: "conditional",
			AgentName: "conditional-llm",
			Objective: fmt.Sprintf("Conditional decision: %s", question),
			InputData: map[string]string{
				"context":  context,
				"question": question,
			},
			Result:    resultText,
			Duration:  duration,
			StepIndex: stepIndex,
			Iteration: iteration,
		}
		cl.eventEmitter(ctx, endEvent)
	}

	return &result, nil
}

// Close cleans up resources
func (cl *ConditionalLLM) Close() error {
	return nil
}

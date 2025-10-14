package mcpagent

import (
	"context"
	"mcp-agent/agent_go/pkg/events"
	"strings"
)

// ReActReasoningTracker tracks ReAct reasoning in real-time during LLM generation
type ReActReasoningTracker struct {
	agent              *Agent
	ctx                context.Context
	turn               int
	stepNumber         int
	isReasoning        bool
	buffer             strings.Builder
	stepBuffer         strings.Builder // Buffer for current reasoning step
	lastEmitted        string          // Last emitted reasoning step to avoid duplicates
	finalAnswerEmitted bool            // 🔧 FIX: Track if final answer was already emitted
}

// NewReActReasoningTracker creates a new ReAct reasoning tracker
func NewReActReasoningTracker(agent *Agent, ctx context.Context, turn int) *ReActReasoningTracker {
	return &ReActReasoningTracker{
		agent:       agent,
		ctx:         ctx,
		turn:        turn,
		stepNumber:  0,
		isReasoning: false,
	}
}

// ProcessChunk processes a chunk of LLM output and emits ReAct reasoning events in real-time
func (rt *ReActReasoningTracker) ProcessChunk(chunk string) {
	rt.buffer.WriteString(chunk)
	content := rt.buffer.String()

	// Check if we're starting ReAct reasoning
	if !rt.isReasoning && (strings.Contains(content, "Let me think about this step by step") ||
		strings.Contains(content, "Let me analyze this step by step") ||
		strings.Contains(content, "Let me break this down") ||
		strings.Contains(content, "Let me approach this systematically")) {
		rt.isReasoning = true
		rt.stepNumber++

		// Emit reasoning start event
		reasoningStartEvent := events.NewReActReasoningStartEvent(rt.turn, content)
		rt.agent.EmitTypedEvent(rt.ctx, reasoningStartEvent)
	}

	// If we're in reasoning mode, handle reasoning steps intelligently
	if rt.isReasoning {
		// Check for completion patterns (final answer) - but only if not already emitted
		if !rt.finalAnswerEmitted && (strings.Contains(content, "Final Answer:") ||
			strings.Contains(content, "FINAL ANSWER:") ||
			strings.Contains(content, "Final answer:") ||
			strings.Contains(content, "final answer:") ||
			strings.Contains(content, "In conclusion") ||
			strings.Contains(content, "To summarize") ||
			strings.Contains(content, "Based on my analysis")) {

			// Extract the final answer
			finalAnswer := extractFinalAnswer(content)

			// Emit final reasoning event
			reasoningFinalEvent := events.NewReActReasoningFinalEvent(rt.turn, finalAnswer, content, "Final answer provided")
			rt.agent.EmitTypedEvent(rt.ctx, reasoningFinalEvent)

			// Mark final answer as emitted to prevent duplicates
			rt.finalAnswerEmitted = true

			// End of reasoning
			rt.isReasoning = false
			return
		}

		// 🔧 FIXED: Batch reasoning steps instead of emitting every character
		// Add chunk to step buffer
		rt.stepBuffer.WriteString(chunk)
		currentStep := rt.stepBuffer.String()

		// Emit reasoning step when we hit sentence boundaries or meaningful breaks
		shouldEmitStep := rt.shouldEmitReasoningStep(currentStep, chunk)

		if shouldEmitStep {
			// Only emit if this step is different from the last one (avoid duplicates)
			stepContent := strings.TrimSpace(currentStep)
			if stepContent != "" && stepContent != rt.lastEmitted {
				reasoningStepEvent := events.NewReActReasoningStepEvent(rt.turn, rt.stepNumber, stepContent, "reasoning_step", content)
				rt.agent.EmitTypedEvent(rt.ctx, reasoningStepEvent)
				rt.stepNumber++
				rt.lastEmitted = stepContent

				// Reset step buffer for next step
				rt.stepBuffer.Reset()
			}
		}
	}
}

// shouldEmitReasoningStep determines if we should emit a reasoning step based on content patterns
func (rt *ReActReasoningTracker) shouldEmitReasoningStep(currentStep, chunk string) bool {
	// Emit on sentence boundaries (periods, exclamation marks, question marks)
	if strings.HasSuffix(chunk, ".") || strings.HasSuffix(chunk, "!") || strings.HasSuffix(chunk, "?") {
		return true
	}

	// Emit on line breaks (newlines)
	if strings.Contains(chunk, "\n") {
		return true
	}

	// Emit on reasoning keywords that indicate step completion
	reasoningKeywords := []string{
		"Therefore", "Thus", "So", "Next", "Now", "First", "Second", "Third",
		"Finally", "In addition", "Furthermore", "Moreover", "However", "But",
		"Let me", "I need to", "I should", "I will", "I can", "I must",
	}

	for _, keyword := range reasoningKeywords {
		if strings.Contains(currentStep, keyword) && len(currentStep) > 20 {
			return true
		}
	}

	// Emit if step buffer gets too long (prevent infinite buffering)
	if rt.stepBuffer.Len() > 200 {
		return true
	}

	return false
}

// Reset resets the reasoning tracker for a new turn or after max token errors
func (rt *ReActReasoningTracker) Reset() {
	rt.buffer.Reset()
	rt.stepBuffer.Reset()
	rt.stepNumber = 0
	rt.isReasoning = false
	rt.lastEmitted = ""
	rt.finalAnswerEmitted = false // 🔧 FIX: Reset final answer emission flag
}

// extractFinalAnswer extracts the final answer from the LLM content
func extractFinalAnswer(content string) string {
	// Look for various final answer patterns
	patterns := []string{"Final Answer:", "FINAL ANSWER:", "Final answer:", "final answer:"}

	for _, pattern := range patterns {
		if idx := strings.Index(content, pattern); idx != -1 {
			// Extract everything after the pattern
			finalAnswer := strings.TrimSpace(content[idx+len(pattern):])
			// Remove any trailing patterns or newlines
			if newlineIdx := strings.Index(finalAnswer, "\n"); newlineIdx != -1 {
				finalAnswer = strings.TrimSpace(finalAnswer[:newlineIdx])
			}
			return finalAnswer
		}
	}

	return ""
}

package mcpagent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/pkg/events"
)

// StreamingTracer extends the basic tracer with streaming capabilities
type StreamingTracer interface {
	observability.Tracer
	// GetEventStream returns a channel for real-time event streaming
	GetEventStream() <-chan *events.AgentEvent
	// SubscribeToEvents allows external systems to subscribe to events
	SubscribeToEvents(ctx context.Context) (<-chan *events.AgentEvent, func())
}

// streamingTracerImpl is a custom tracer that provides streaming capabilities
type streamingTracerImpl struct {
	baseTracer   observability.Tracer
	eventStream  chan *events.AgentEvent
	bufferSize   int
	subscribers  map[string]chan *events.AgentEvent
	subscriberMu sync.RWMutex
	closed       bool
	mu           sync.RWMutex
}

// NewStreamingTracer creates a new streaming tracer that wraps an existing tracer
func NewStreamingTracer(baseTracer observability.Tracer, bufferSize int) StreamingTracer {
	if bufferSize <= 0 {
		bufferSize = 100 // Default buffer size
	}

	st := &streamingTracerImpl{
		baseTracer:  baseTracer,
		eventStream: make(chan *events.AgentEvent, bufferSize),
		bufferSize:  bufferSize,
		subscribers: make(map[string]chan *events.AgentEvent),
	}

	// Start event forwarding goroutine
	go st.forwardEvents()

	return st
}

// GetEventStream returns the main event stream channel
func (st *streamingTracerImpl) GetEventStream() <-chan *events.AgentEvent {
	return st.eventStream
}

// SubscribeToEvents allows external systems to subscribe to events
func (st *streamingTracerImpl) SubscribeToEvents(ctx context.Context) (<-chan *events.AgentEvent, func()) {
	st.subscriberMu.Lock()
	defer st.subscriberMu.Unlock()

	if st.closed {
		return nil, func() {}
	}

	// Create unique subscriber ID
	subscriberID := fmt.Sprintf("subscriber-%d", time.Now().UnixNano())
	subscriberChan := make(chan *events.AgentEvent, st.bufferSize)

	st.subscribers[subscriberID] = subscriberChan

	// Return unsubscribe function
	unsubscribe := func() {
		st.subscriberMu.Lock()
		defer st.subscriberMu.Unlock()
		if ch, exists := st.subscribers[subscriberID]; exists {
			close(ch)
			delete(st.subscribers, subscriberID)
		}
	}

	// Handle context cancellation
	go func() {
		<-ctx.Done()
		unsubscribe()
	}()

	return subscriberChan, unsubscribe
}

// forwardEvents forwards events to all subscribers
func (st *streamingTracerImpl) forwardEvents() {
	for event := range st.eventStream {
		st.subscriberMu.RLock()
		subscribers := make([]chan *events.AgentEvent, 0, len(st.subscribers))
		for _, ch := range st.subscribers {
			subscribers = append(subscribers, ch)
		}
		st.subscriberMu.RUnlock()

		// Send to all subscribers (non-blocking)
		for _, ch := range subscribers {
			select {
			case ch <- event:
				// Event sent successfully
			default:
				// Channel is full, skip this subscriber
			}
		}
	}
}

// EmitEvent implements observability.Tracer interface
func (st *streamingTracerImpl) EmitEvent(event observability.AgentEvent) error {
	// Forward to base tracer
	if st.baseTracer != nil {
		if err := st.baseTracer.EmitEvent(event); err != nil {
			// Log error but continue
			// Note: We don't have access to logger here, so we'll just continue
		}
	}

	// Try to convert to our AgentEvent type for streaming
	if agentEvent, ok := event.(*events.AgentEvent); ok {
		// Send to our event stream (non-blocking)
		select {
		case st.eventStream <- agentEvent:
			// Event queued successfully
		default:
			// Event stream is full, skip
		}
	}

	return nil
}

// EmitLLMEvent implements observability.Tracer interface
func (st *streamingTracerImpl) EmitLLMEvent(event observability.LLMEvent) error {
	// Forward to base tracer
	if st.baseTracer != nil {
		return st.baseTracer.EmitLLMEvent(event)
	}
	return nil
}

// StartTrace implements observability.Tracer interface
func (st *streamingTracerImpl) StartTrace(name string, input interface{}) observability.TraceID {
	if st.baseTracer != nil {
		return st.baseTracer.StartTrace(name, input)
	}
	return ""
}

// EndTrace implements observability.Tracer interface
func (st *streamingTracerImpl) EndTrace(traceID observability.TraceID, output interface{}) {
	if st.baseTracer != nil {
		st.baseTracer.EndTrace(traceID, output)
	}
}

// Close closes the streaming tracer and cleans up resources
func (st *streamingTracerImpl) Close() error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.closed {
		return nil
	}

	st.closed = true

	// Close main event stream
	close(st.eventStream)

	// Close all subscriber channels
	st.subscriberMu.Lock()
	for _, ch := range st.subscribers {
		close(ch)
	}
	st.subscribers = make(map[string]chan *events.AgentEvent)
	st.subscriberMu.Unlock()

	return nil
}

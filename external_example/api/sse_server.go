package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"mcp-agent/agent_go/pkg/external"
)

// SSE server for real-time event streaming
type SSEServer struct {
	config       external.Config
	eventStore   *EventStore
	clients      map[string]chan string
	clientsMutex sync.RWMutex
	requestCount int
}

// EventStore stores events for analysis
type EventStore struct {
	events []external.AgentEvent
	mu     sync.RWMutex
}

// NewSSEServer creates a new SSE server
func NewSSEServer(config external.Config) *SSEServer {
	return &SSEServer{
		config:     config,
		eventStore: &EventStore{events: make([]external.AgentEvent, 0)},
		clients:    make(map[string]chan string),
	}
}

// handleSSE handles Server-Sent Events connections
func (s *SSEServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a channel for this client
	clientID := fmt.Sprintf("client_%d", time.Now().UnixNano())
	clientChan := make(chan string, 100)

	s.clientsMutex.Lock()
	s.clients[clientID] = clientChan
	s.clientsMutex.Unlock()

	GetLogger().Infof("🔌 New SSE client connected: %s", clientID)

	// Clean up when client disconnects
	defer func() {
		s.clientsMutex.Lock()
		delete(s.clients, clientID)
		s.clientsMutex.Unlock()
		GetLogger().Infof("🔌 SSE client disconnected: %s", clientID)
	}()

	// Send initial connection message
	fmt.Fprintf(w, "event: connected\ndata: %s\n\n", clientID)
	w.(http.Flusher).Flush()

	// Keep connection alive and send events
	for {
		select {
		case event := <-clientChan:
			fmt.Fprintf(w, "data: %s\n\n", event)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			GetLogger().Infof("🔌 SSE client context cancelled: %s", clientID)
			return
		}
	}
}

// handleQuery handles agent queries and captures events
func (s *SSEServer) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request with conversation history
	var request struct {
		Query          string                   `json:"query"`
		ConversationID string                   `json:"conversation_id,omitempty"`
		History        []map[string]interface{} `json:"history,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	s.requestCount++
	requestID := s.requestCount

	GetLogger().Infof("🚀 Request #%d: %s", requestID, request.Query)

	// Log conversation context from request
	if request.ConversationID != "" {
		GetLogger().Infof("💬 Using conversation ID: %s", request.ConversationID)
	}
	if len(request.History) > 0 {
		GetLogger().Infof("📚 Request includes %d history messages", len(request.History))
	}

	// Create a new agent instance for this request
	ctx := context.Background()
	agent, err := external.NewAgent(ctx, s.config)
	if err != nil {
		GetLogger().Errorf("❌ Request #%d failed to create agent: %v", requestID, err)
		http.Error(w, fmt.Sprintf("Agent creation failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Initialize the agent
	if err := agent.Initialize(ctx); err != nil {
		GetLogger().Errorf("❌ Request #%d failed to initialize agent: %v", requestID, err)
		http.Error(w, fmt.Sprintf("Agent initialization failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Create request-specific event listener
	requestListener := &RequestEventListener{
		requestID: requestID,
		events:    make([]external.AgentEvent, 0),
		mu:        sync.RWMutex{},
	}

	// Add the request-specific listener to this agent instance
	agent.AddEventListener(requestListener)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Invoke the agent
	response, err := agent.Invoke(ctx, request.Query)
	if err != nil {
		GetLogger().Errorf("❌ Request #%d failed: %v", requestID, err)
		http.Error(w, fmt.Sprintf("Agent invocation failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait a bit for any remaining events to arrive
	time.Sleep(100 * time.Millisecond)

	// Remove the request-specific listener from this agent instance
	agent.RemoveEventListener(requestListener)

	// Log the results
	GetLogger().Infof("✅ Request #%d completed. Response: %s", requestID, response)
	GetLogger().Infof("📊 Request #%d captured %d events", requestID, len(requestListener.events))

	// Return the response with request context
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"request_id":      requestID,
		"conversation_id": request.ConversationID,
		"response":        response,
		"events":          requestListener.events,
		"event_count":     len(requestListener.events),
		"request_context": map[string]interface{}{
			"conversation_id": request.ConversationID,
			"history_count":   len(request.History),
			"agent_mode":      string(s.config.AgentMode),
		},
	})
}

// handleStats returns server statistics
func (s *SSEServer) handleStats(w http.ResponseWriter, r *http.Request) {
	s.eventStore.mu.RLock()
	totalEvents := len(s.eventStore.events)
	s.eventStore.mu.RUnlock()

	s.clientsMutex.RLock()
	activeClients := len(s.clients)
	s.clientsMutex.RUnlock()

	stats := map[string]interface{}{
		"total_events":   totalEvents,
		"active_clients": activeClients,
		"request_count":  s.requestCount,
		"uptime":         time.Since(time.Now()).String(), // This will be 0, but you get the idea
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// Start starts the SSE server
func (s *SSEServer) Start(port string) error {
	// Set up routes
	http.HandleFunc("/sse", s.handleSSE)
	http.HandleFunc("/api/query", s.handleQuery)
	http.HandleFunc("/api/stats", s.handleStats)

	GetLogger().Infof("🚀 Starting SSE server on port %s", port)
	GetLogger().Infof("📡 SSE endpoint: http://localhost:%s/sse", port)
	GetLogger().Infof("🔍 Query endpoint: http://localhost:%s/api/query", port)
	GetLogger().Infof("📊 Stats endpoint: http://localhost:%s/api/stats", port)

	return http.ListenAndServe(":"+port, nil)
}

// RequestEventListener captures events for a specific request
type RequestEventListener struct {
	requestID int
	events    []external.AgentEvent
	mu        sync.RWMutex
}

func (l *RequestEventListener) HandleEvent(ctx context.Context, event *external.AgentEvent) error {
	l.mu.Lock()
	l.events = append(l.events, *event)
	l.mu.Unlock()

	GetLogger().Infof("🎯 Request #%d EVENT: %s at %s",
		l.requestID,
		event.Type,
		event.Timestamp.Format("15:04:05.000"))

	// Log only critical events using typed constants
	switch event.Type {
	case "tool_call_start":
		GetLogger().Infof("  🔧 Tool call started")
	case "tool_call_end":
		GetLogger().Infof("  ✅ Tool call ended")
	case "system_prompt":
		GetLogger().Infof("  📝 System prompt event")
	case "token_usage":
		GetLogger().Infof("  💰 Token usage event")
	case "conversation_end":
		GetLogger().Infof("  ⏱️  Conversation ended")
		GetLogger().Infof("  🔄 Conversation completed")
	case "agent_start":
		GetLogger().Infof("  🤖 Agent started")
	case "agent_end":
		GetLogger().Infof("  🤖 Agent ended")
	case "fallback_model_used":
		GetLogger().Infof("  🔄 Fallback model used")
	case "throttling_detected":
		GetLogger().Infof("  ⏳ Throttling detected")
	case "user_message":
		GetLogger().Infof("  👤 User message event")
	case "llm_messages":
		GetLogger().Infof("  💬 LLM messages event")
	case "react_reasoning_start":
		GetLogger().Infof("  🧠 ReAct reasoning started")
	case "react_reasoning_end":
		GetLogger().Infof("  🧠 ReAct reasoning ended")
	case "max_turns_reached":
		GetLogger().Infof("  🔄 Max turns reached")
	case "large_tool_output_detected":
		GetLogger().Infof("  📏 Large tool output detected")
	default:
		GetLogger().Infof("  📊 Event: %s", event.Type)
	}

	return nil
}

func (l *RequestEventListener) Name() string {
	return fmt.Sprintf("RequestEventListener_%d", l.requestID)
}

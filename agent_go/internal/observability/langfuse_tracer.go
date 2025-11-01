//go:build !langfuse_disabled
// +build !langfuse_disabled

package observability

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"mcp-agent/agent_go/internal/utils"

	"github.com/joho/godotenv"
)

// Event type constants for type safety
const (
	EventTypeAgentStart         = "agent_start"
	EventTypeAgentEnd           = "agent_end"
	EventTypeAgentError         = "agent_error"
	EventTypeConversationStart  = "conversation_start"
	EventTypeConversationEnd    = "conversation_end"
	EventTypeLLMGenerationStart = "llm_generation_start"
	EventTypeLLMGenerationEnd   = "llm_generation_end"
	EventTypeToolCallStart      = "tool_call_start"
	EventTypeToolCallEnd        = "tool_call_end"
	EventTypeTokenUsage         = "token_usage"

	// MCP Server connection events
	EventTypeMCPServerConnectionStart = "mcp_server_connection_start"
	EventTypeMCPServerConnectionEnd   = "mcp_server_connection_end"
	EventTypeMCPServerConnectionError = "mcp_server_connection_error"
	EventTypeMCPServerDiscovery       = "mcp_server_discovery"
	EventTypeMCPServerSelection       = "mcp_server_selection"
)

// LangfuseTracer implements the Tracer interface using Langfuse v2 API patterns.
// Implements shared state pattern similar to Python implementation with proper
// authentication, error handling, and debug logging.
type LangfuseTracer struct {
	client    *http.Client
	host      string
	publicKey string
	secretKey string
	debug     bool

	// Shared state for all instances (similar to Python class-level state)
	traces map[string]*langfuseTrace
	spans  map[string]*langfuseSpan

	// Hierarchy tracking: traceID -> spanID mappings
	agentSpans         map[string]string // traceID -> agent span ID
	conversationSpans  map[string]string // traceID -> conversation span ID
	llmGenerationSpans map[string]string // traceID -> current LLM generation span ID

	mu sync.RWMutex

	// Background processing
	eventQueue chan *langfuseEvent
	stopCh     chan struct{}
	wg         sync.WaitGroup

	logger utils.ExtendedLogger
}

// Shared state across all instances (similar to Python class-level variables)
var (
	sharedLangfuseClient *LangfuseTracer
	sharedInitialized    bool
	sharedMutex          sync.Mutex
)

// langfuseTrace represents a trace in Langfuse v2 API format
type langfuseTrace struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Input     interface{}            `json:"input,omitempty"`
	Output    interface{}            `json:"output,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	UserID    string                 `json:"userId,omitempty"`
	SessionID string                 `json:"sessionId,omitempty"`
	Tags      []string               `json:"tags,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Public    bool                   `json:"public,omitempty"`
	Release   string                 `json:"release,omitempty"`
	Version   string                 `json:"version,omitempty"`
}

// langfuseSpan represents a span/observation in Langfuse v2 API format
// langfuseObservation represents a Langfuse observation following the proper data model
type langfuseObservation struct {
	ID                  string                 `json:"id"`
	TraceID             string                 `json:"traceId"`
	ParentObservationID string                 `json:"parentObservationId,omitempty"`
	Name                string                 `json:"name"`
	Type                string                 `json:"type"` // "SPAN", "GENERATION", "AGENT", "TOOL", etc.
	Input               interface{}            `json:"input,omitempty"`
	Output              interface{}            `json:"output,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	StartTime           time.Time              `json:"startTime"`
	EndTime             *time.Time             `json:"endTime,omitempty"`
	Level               string                 `json:"level,omitempty"`
	StatusMessage       string                 `json:"statusMessage,omitempty"`
	Version             string                 `json:"version,omitempty"`

	// Generation-specific fields
	Model               string                 `json:"model,omitempty"`
	ModelParameters     map[string]interface{} `json:"modelParameters,omitempty"`
	Usage               *LangfuseUsage         `json:"usage,omitempty"`
	CompletionStartTime *time.Time             `json:"completionStartTime,omitempty"`
	PromptTokens        int                    `json:"promptTokens,omitempty"`
	CompletionTokens    int                    `json:"completionTokens,omitempty"`
	TotalTokens         int                    `json:"totalTokens,omitempty"`

	// Tool-specific fields
	ToolName        string `json:"toolName,omitempty"`
	ToolDescription string `json:"toolDescription,omitempty"`

	// Agent-specific fields
	AgentName string `json:"agentName,omitempty"`
}

// langfuseSpan is kept for backward compatibility but now uses langfuseObservation
type langfuseSpan = langfuseObservation

// LangfuseUsage represents usage metrics in Langfuse format
type LangfuseUsage struct {
	Input      int    `json:"input,omitempty"`
	Output     int    `json:"output,omitempty"`
	Total      int    `json:"total,omitempty"`
	Unit       string `json:"unit,omitempty"`
	InputCost  int    `json:"inputCost,omitempty"`
	OutputCost int    `json:"outputCost,omitempty"`
	TotalCost  int    `json:"totalCost,omitempty"`
}

// langfuseEvent represents an event for the ingestion API
type langfuseEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"` // "trace-create", "span-create", "span-update", "generation-create", "generation-update"
	Timestamp time.Time              `json:"timestamp"`
	Body      interface{}            `json:"body"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// langfuseIngestionPayload represents the batch ingestion payload
type langfuseIngestionPayload struct {
	Batch []langfuseEvent `json:"batch"`
}

// newLangfuseTracerWithLogger creates a new Langfuse tracer with an injected logger
func newLangfuseTracerWithLogger(logger utils.ExtendedLogger) (Tracer, error) {
	sharedMutex.Lock()
	defer sharedMutex.Unlock()

	if !sharedInitialized {
		if err := initializeSharedLangfuseClientWithLogger(logger); err != nil {
			sharedInitialized = true // Mark as initialized even on failure to prevent retry loops
			return nil, err
		}
		sharedInitialized = true
	}

	if sharedLangfuseClient == nil {
		return nil, errors.New("failed to initialize shared Langfuse client")
	}

	return sharedLangfuseClient, nil
}

// NewLangfuseTracer creates a new Langfuse tracer (public function for direct use)
// DEPRECATED: Use NewLangfuseTracerWithLogger instead to provide a proper logger
func NewLangfuseTracer() (Tracer, error) {
	return nil, errors.New("NewLangfuseTracer() is deprecated. Use NewLangfuseTracerWithLogger(logger) instead to provide a proper logger")
}

// NewLangfuseTracerWithLogger creates a new Langfuse tracer with an injected logger
func NewLangfuseTracerWithLogger(logger utils.ExtendedLogger) (Tracer, error) {
	return newLangfuseTracerWithLogger(logger)
}

// initializeSharedLangfuseClientWithLogger initializes the shared Langfuse client with an injected logger
func initializeSharedLangfuseClientWithLogger(logger utils.ExtendedLogger) error {
	// Auto-load .env file if present (similar to Python dotenv)
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			// Don't fail if .env can't be loaded, just log
			log.Printf("Warning: Could not load .env file: %w", err)
		}
	}

	// Load credentials from environment
	publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
	secretKey := os.Getenv("LANGFUSE_SECRET_KEY")
	host := os.Getenv("LANGFUSE_HOST")

	if host == "" {
		host = "https://cloud.langfuse.com"
	}

	if publicKey == "" || secretKey == "" {
		return fmt.Errorf("langfuse credentials missing. Required environment variables:\n"+
			"- LANGFUSE_PUBLIC_KEY\n"+
			"- LANGFUSE_SECRET_KEY\n"+
			"- LANGFUSE_HOST (optional, default: %s)", host)
	}

	// Always enable debug for comprehensive observability (similar to Python)
	debug := true

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	tracer := &LangfuseTracer{
		client:             client,
		host:               host,
		publicKey:          publicKey,
		secretKey:          secretKey,
		debug:              debug,
		traces:             make(map[string]*langfuseTrace),
		spans:              make(map[string]*langfuseSpan),
		agentSpans:         make(map[string]string),
		conversationSpans:  make(map[string]string),
		llmGenerationSpans: make(map[string]string),
		eventQueue:         make(chan *langfuseEvent, 1000),
		stopCh:             make(chan struct{}),
		logger:             logger, // Use injected logger instead of default
	}

	// Test authentication (similar to Python auth_check)
	if err := tracer.authCheck(); err != nil {
		return fmt.Errorf("langfuse authentication failed for %s...: %w", publicKey[:10], err)
	}

	// Start background event processor
	tracer.wg.Add(1)
	go tracer.eventProcessor()

	sharedLangfuseClient = tracer

	if tracer.debug {
		tracer.logger.Infof("✅ Langfuse: Authentication successful (%s...) [Debug: Always Enabled]", publicKey[:10])
	}

	return nil
}

// authCheck verifies authentication with Langfuse API using health endpoint
func (l *LangfuseTracer) authCheck() error {
	req, err := http.NewRequest("GET", l.host+"/api/public/health", nil)
	if err != nil {
		return err
	}

	req.SetBasicAuth(l.publicKey, l.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// generateID generates a unique ID for traces and spans
func generateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// StartTrace starts a new trace using Langfuse v2 API pattern
func (l *LangfuseTracer) StartTrace(name string, input interface{}) TraceID {
	id := generateID()
	trace := &langfuseTrace{
		ID:        id,
		Name:      name,
		Input:     input,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	l.mu.Lock()
	l.traces[id] = trace
	l.mu.Unlock()

	// Queue trace creation event
	event := &langfuseEvent{
		ID:        generateID(),
		Type:      "trace-create",
		Timestamp: time.Now(),
		Body:      trace,
	}

	select {
	case l.eventQueue <- event:
	default:
		if l.debug {
			l.logger.Errorf("⚠️ Langfuse: Event queue full, dropping trace-create event")
		}
	}

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Started trace '%s' (ID: %s)", name, id)
	}

	return TraceID(id)
}

// StartSpan starts a new observation using Langfuse v2 API pattern
func (l *LangfuseTracer) StartSpan(parentID string, name string, input interface{}) SpanID {
	return l.StartObservation(parentID, "SPAN", name, input)
}

// generateAgentSpanName creates an informative name for agent spans
func (l *LangfuseTracer) generateAgentSpanName(eventData interface{}) string {
	if l.debug {
		l.logger.Infof("🔍 Langfuse: generateAgentSpanName called with data type: %T", eventData)
	}

	// Use reflection to access struct fields
	if reflect.TypeOf(eventData).Kind() == reflect.Ptr && !reflect.ValueOf(eventData).IsNil() {
		elem := reflect.ValueOf(eventData).Elem()
		if elem.Kind() == reflect.Struct {
			// Try to get ModelID field
			modelField := elem.FieldByName("ModelID")
			if modelField.IsValid() && modelField.Kind() == reflect.String {
				modelID := modelField.String()

				// Try to get AvailableTools field
				toolsField := elem.FieldByName("AvailableTools")
				availableTools := 0
				if toolsField.IsValid() {
					switch toolsField.Kind() {
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						availableTools = int(toolsField.Int())
					}
				}

				if l.debug {
					l.logger.Infof("🔍 Langfuse: generateAgentSpanName - modelID: %s, tools: %d", modelID, availableTools)
				}

				// Extract model name (e.g., "gpt-4.1" from full model ID)
				modelParts := strings.Split(modelID, "/")
				shortModel := modelParts[len(modelParts)-1]

				result := fmt.Sprintf("agent_%s_%d_tools", shortModel, availableTools)
				if l.debug {
					l.logger.Infof("🔍 Langfuse: generateAgentSpanName returning: %s", result)
				}
				return result
			}
		}
	}

	// Fallback for map data
	if data, ok := eventData.(map[string]interface{}); ok {
		modelID := "unknown"
		availableTools := 0

		if modelVal, ok := data["model_id"].(string); ok {
			modelID = modelVal
		}
		if toolsVal, ok := data["available_tools"].(float64); ok {
			availableTools = int(toolsVal)
		}

		// Extract model name (e.g., "gpt-4.1" from full model ID)
		modelParts := strings.Split(modelID, "/")
		shortModel := modelParts[len(modelParts)-1]

		result := fmt.Sprintf("agent_%s_%d_tools", shortModel, availableTools)
		if l.debug {
			l.logger.Infof("🔍 Langfuse: generateAgentSpanName (map) returning: %s", result)
		}
		return result
	}

	if l.debug {
		l.logger.Infof("🔍 Langfuse: generateAgentSpanName falling back to default")
	}
	return "agent_execution"
}

// extractFinalResult extracts the final result from event data
func (l *LangfuseTracer) extractFinalResult(eventData interface{}) string {
	if l.debug {
		l.logger.Infof("🔍 Langfuse: extractFinalResult called with data type: %T", eventData)
	}

	// Use reflection to access struct fields
	if reflect.TypeOf(eventData).Kind() == reflect.Ptr && !reflect.ValueOf(eventData).IsNil() {
		elem := reflect.ValueOf(eventData).Elem()
		if elem.Kind() == reflect.Struct {
			// Try to get result/output fields
			resultFields := []string{"Result", "Output", "Response", "Answer", "FinalAnswer", "Content"}

			for _, fieldName := range resultFields {
				field := elem.FieldByName(fieldName)
				if field.IsValid() && field.Kind() == reflect.String {
					result := field.String()
					if result != "" {
						if l.debug {
							l.logger.Infof("🔍 Langfuse: extractFinalResult found %s: %s", fieldName, result)
						}
						return result
					}
				}
			}
		}
	}

	// Fallback for map data
	if data, ok := eventData.(map[string]interface{}); ok {
		// Try different field names that might contain the final result
		resultFields := []string{"result", "output", "response", "answer", "final_answer", "content", "text"}

		for _, fieldName := range resultFields {
			if value, ok := data[fieldName]; ok {
				if strValue, ok := value.(string); ok && strValue != "" {
					if l.debug {
						l.logger.Infof("🔍 Langfuse: extractFinalResult (map) found %s: %s", fieldName, strValue)
					}
					return strValue
				}
			}
		}

		// Try nested structures
		if resultMap, ok := data["result"].(map[string]interface{}); ok {
			if text, ok := resultMap["text"].(string); ok && text != "" {
				if l.debug {
					l.logger.Infof("🔍 Langfuse: extractFinalResult found nested result.text: %s", text)
				}
				return text
			}
		}
	}

	if l.debug {
		l.logger.Infof("🔍 Langfuse: extractFinalResult - no result found")
	}
	return ""
}

// generateTraceName generates a meaningful name for traces based on user query
func (l *LangfuseTracer) generateTraceName(eventData interface{}) string {
	if l.debug {
		l.logger.Infof("🔍 Langfuse: generateTraceName called with eventData type: %T", eventData)
	}

	// Use reflection to access struct fields
	if reflect.TypeOf(eventData).Kind() == reflect.Ptr && !reflect.ValueOf(eventData).IsNil() {
		elem := reflect.ValueOf(eventData).Elem()
		if elem.Kind() == reflect.Struct {
			// Try to get Question field
			questionField := elem.FieldByName("Question")
			if questionField.IsValid() && questionField.Kind() == reflect.String {
				question := questionField.String()

				if l.debug {
					l.logger.Infof("🔍 Langfuse: generateTraceName - question: %s", question)
				}

				// Create a meaningful trace name from the query
				// Clean up the question: remove extra whitespace, convert to lowercase, replace spaces with underscores
				cleanQuestion := strings.TrimSpace(strings.ToLower(question))

				// Replace common punctuation and multiple spaces with single underscores
				cleanQuestion = strings.ReplaceAll(cleanQuestion, "?", "")
				cleanQuestion = strings.ReplaceAll(cleanQuestion, "!", "")
				cleanQuestion = strings.ReplaceAll(cleanQuestion, ".", "")
				cleanQuestion = strings.ReplaceAll(cleanQuestion, ",", "")
				cleanQuestion = strings.ReplaceAll(cleanQuestion, ";", "")
				cleanQuestion = strings.ReplaceAll(cleanQuestion, ":", "")

				// Replace multiple spaces with single space, then spaces with underscores
				words := strings.Fields(cleanQuestion)
				cleanQuestion = strings.Join(words, "_")

				// Truncate to a reasonable length for trace names (max 80 chars)
				if len(cleanQuestion) > 80 {
					// Try to truncate at word boundary if possible
					if strings.Contains(cleanQuestion[:80], "_") {
						lastUnderscore := strings.LastIndex(cleanQuestion[:80], "_")
						if lastUnderscore > 50 { // Only truncate at underscore if we have a reasonable length
							cleanQuestion = cleanQuestion[:lastUnderscore]
						} else {
							cleanQuestion = cleanQuestion[:77] + "..."
						}
					} else {
						cleanQuestion = cleanQuestion[:77] + "..."
					}
				}

				result := fmt.Sprintf("query_%s", cleanQuestion)
				if l.debug {
					l.logger.Infof("🔍 Langfuse: generateTraceName returning: %s", result)
					l.logger.Infof("🔍 Langfuse: Original question was: %s", question)
				}
				return result
			}
		}
	}

	// Fallback for map data
	if data, ok := eventData.(map[string]interface{}); ok {
		if question, ok := data["question"].(string); ok {
			// Create a meaningful trace name from the query
			// Clean up the question: remove extra whitespace, convert to lowercase, replace spaces with underscores
			cleanQuestion := strings.TrimSpace(strings.ToLower(question))

			// Replace common punctuation and multiple spaces with single underscores
			cleanQuestion = strings.ReplaceAll(cleanQuestion, "?", "")
			cleanQuestion = strings.ReplaceAll(cleanQuestion, "!", "")
			cleanQuestion = strings.ReplaceAll(cleanQuestion, ".", "")
			cleanQuestion = strings.ReplaceAll(cleanQuestion, ",", "")
			cleanQuestion = strings.ReplaceAll(cleanQuestion, ";", "")
			cleanQuestion = strings.ReplaceAll(cleanQuestion, ":", "")

			// Replace multiple spaces with single space, then spaces with underscores
			words := strings.Fields(cleanQuestion)
			cleanQuestion = strings.Join(words, "_")

			// Truncate to a reasonable length for trace names (max 80 chars)
			if len(cleanQuestion) > 80 {
				// Try to truncate at word boundary if possible
				if strings.Contains(cleanQuestion[:80], "_") {
					lastUnderscore := strings.LastIndex(cleanQuestion[:80], "_")
					if lastUnderscore > 50 { // Only truncate at underscore if we have a reasonable length
						cleanQuestion = cleanQuestion[:lastUnderscore]
					} else {
						cleanQuestion = cleanQuestion[:77] + "..."
					}
				} else {
					cleanQuestion = cleanQuestion[:77] + "..."
				}
			}

			result := fmt.Sprintf("query_%s", cleanQuestion)
			if l.debug {
				l.logger.Infof("🔍 Langfuse: generateTraceName returning: %s", result)
				l.logger.Infof("🔍 Langfuse: Original question was: %s", question)
			}
			return result
		}
	}

	// Fallback to default name if no question found
	defaultName := "agent_conversation"
	if l.debug {
		l.logger.Infof("🔍 Langfuse: generateTraceName - no question found, using default: %s", defaultName)
	}
	return defaultName
}

// generateConversationSpanName creates an informative name for conversation spans
func (l *LangfuseTracer) generateConversationSpanName(eventData interface{}) string {
	if l.debug {
		l.logger.Infof("🔍 Langfuse: generateConversationSpanName called with data type: %T", eventData)
	}

	// Use reflection to access struct fields
	if reflect.TypeOf(eventData).Kind() == reflect.Ptr && !reflect.ValueOf(eventData).IsNil() {
		elem := reflect.ValueOf(eventData).Elem()
		if elem.Kind() == reflect.Struct {
			// Try to get Question field
			questionField := elem.FieldByName("Question")
			if questionField.IsValid() && questionField.Kind() == reflect.String {
				question := questionField.String()

				if l.debug {
					l.logger.Infof("🔍 Langfuse: generateConversationSpanName - question: %s", question)
				}

				// Create a meaningful conversation name from the query
				// Clean up the question: remove extra whitespace, convert to lowercase, replace spaces with underscores
				cleanQuestion := strings.TrimSpace(strings.ToLower(question))

				// Replace common punctuation and multiple spaces with single underscores
				cleanQuestion = strings.ReplaceAll(cleanQuestion, "?", "")
				cleanQuestion = strings.ReplaceAll(cleanQuestion, "!", "")
				cleanQuestion = strings.ReplaceAll(cleanQuestion, ".", "")
				cleanQuestion = strings.ReplaceAll(cleanQuestion, ",", "")
				cleanQuestion = strings.ReplaceAll(cleanQuestion, ";", "")
				cleanQuestion = strings.ReplaceAll(cleanQuestion, ":", "")

				// Replace multiple spaces with single space, then spaces with underscores
				words := strings.Fields(cleanQuestion)
				cleanQuestion = strings.Join(words, "_")

				// Truncate to a reasonable length for span names (max 50 chars)
				if len(cleanQuestion) > 50 {
					// Try to truncate at word boundary if possible
					if strings.Contains(cleanQuestion[:50], "_") {
						lastUnderscore := strings.LastIndex(cleanQuestion[:50], "_")
						if lastUnderscore > 30 { // Only truncate at underscore if we have a reasonable length
							cleanQuestion = cleanQuestion[:lastUnderscore]
						} else {
							cleanQuestion = cleanQuestion[:47] + "..."
						}
					} else {
						cleanQuestion = cleanQuestion[:47] + "..."
					}
				}

				result := fmt.Sprintf("conversation_%s", cleanQuestion)
				if l.debug {
					l.logger.Infof("🔍 Langfuse: generateConversationSpanName returning: %s", result)
					l.logger.Infof("🔍 Langfuse: Original question was: %s", question)
				}
				return result
			}
		}
	}

	// Fallback for map data
	if data, ok := eventData.(map[string]interface{}); ok {
		if question, ok := data["question"].(string); ok {
			// Create a meaningful conversation name from the query
			// Clean up the question: remove extra whitespace, convert to lowercase, replace spaces with underscores
			cleanQuestion := strings.TrimSpace(strings.ToLower(question))

			// Replace common punctuation and multiple spaces with single underscores
			cleanQuestion = strings.ReplaceAll(cleanQuestion, "?", "")
			cleanQuestion = strings.ReplaceAll(cleanQuestion, "!", "")
			cleanQuestion = strings.ReplaceAll(cleanQuestion, ".", "")
			cleanQuestion = strings.ReplaceAll(cleanQuestion, ",", "")
			cleanQuestion = strings.ReplaceAll(cleanQuestion, ";", "")
			cleanQuestion = strings.ReplaceAll(cleanQuestion, ":", "")

			// Replace multiple spaces with single space, then spaces with underscores
			words := strings.Fields(cleanQuestion)
			cleanQuestion = strings.Join(words, "_")

			// Truncate to a reasonable length for span names (max 50 chars)
			if len(cleanQuestion) > 50 {
				// Try to truncate at word boundary if possible
				if strings.Contains(cleanQuestion[:50], "_") {
					lastUnderscore := strings.LastIndex(cleanQuestion[:50], "_")
					if lastUnderscore > 30 { // Only truncate at underscore if we have a reasonable length
						cleanQuestion = cleanQuestion[:lastUnderscore]
					} else {
						cleanQuestion = cleanQuestion[:47] + "..."
					}
				} else {
					cleanQuestion = cleanQuestion[:47] + "..."
				}
			}

			result := fmt.Sprintf("conversation_%s", cleanQuestion)
			if l.debug {
				l.logger.Infof("🔍 Langfuse: generateConversationSpanName (map) returning: %s", result)
			}
			return result
		}
	}

	if l.debug {
		l.logger.Infof("🔍 Langfuse: generateConversationSpanName falling back to default")
	}
	return "conversation_execution"
}

// generateLLMSpanName creates an informative name for LLM generation spans
func (l *LangfuseTracer) generateLLMSpanName(eventData interface{}) string {
	if l.debug {
		l.logger.Infof("🔍 Langfuse: generateLLMSpanName called with data type: %T", eventData)
	}

	// Use reflection to access struct fields
	if reflect.TypeOf(eventData).Kind() == reflect.Ptr && !reflect.ValueOf(eventData).IsNil() {
		elem := reflect.ValueOf(eventData).Elem()
		if elem.Kind() == reflect.Struct {
			// Try to get Turn field
			turnField := elem.FieldByName("Turn")
			turn := 0
			if turnField.IsValid() {
				switch turnField.Kind() {
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					turn = int(turnField.Int())
				}
			}

			// Try to get ModelID field
			modelField := elem.FieldByName("ModelID")
			modelID := "unknown"
			if modelField.IsValid() && modelField.Kind() == reflect.String {
				modelID = modelField.String()
			}

			// Try to get ToolsCount field
			toolsField := elem.FieldByName("ToolsCount")
			toolsCount := 0
			if toolsField.IsValid() {
				switch toolsField.Kind() {
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					toolsCount = int(toolsField.Int())
				}
			}

			if l.debug {
				l.logger.Infof("🔍 Langfuse: generateLLMSpanName - turn: %d, modelID: %s, tools: %d", turn, modelID, toolsCount)
			}

			// Extract model name (e.g., "gpt-4.1" from full model ID)
			modelParts := strings.Split(modelID, "/")
			shortModel := modelParts[len(modelParts)-1]

			result := fmt.Sprintf("llm_generation_turn_%d_%s_%d_tools", turn, shortModel, toolsCount)
			if l.debug {
				l.logger.Infof("🔍 Langfuse: generateLLMSpanName returning: %s", result)
			}
			return result
		}
	}

	// Fallback for map data
	if data, ok := eventData.(map[string]interface{}); ok {
		turn := 0
		modelID := "unknown"
		toolsCount := 0

		if turnVal, ok := data["turn"].(float64); ok {
			turn = int(turnVal)
		}
		if modelVal, ok := data["model_id"].(string); ok {
			modelID = modelVal
		}
		if toolsVal, ok := data["tools_count"].(float64); ok {
			toolsCount = int(toolsVal)
		}

		// Extract model name (e.g., "gpt-4.1" from full model ID)
		modelParts := strings.Split(modelID, "/")
		shortModel := modelParts[len(modelParts)-1]

		result := fmt.Sprintf("llm_generation_turn_%d_%s_%d_tools", turn, shortModel, toolsCount)
		if l.debug {
			l.logger.Infof("🔍 Langfuse: generateLLMSpanName (map) returning: %s", result)
		}
		return result
	}

	if l.debug {
		l.logger.Infof("🔍 Langfuse: generateLLMSpanName falling back to default")
	}
	return "llm_generation"
}

// generateToolSpanName creates an informative name for tool call spans
func (l *LangfuseTracer) generateToolSpanName(eventData interface{}) string {
	if l.debug {
		l.logger.Infof("🔍 Langfuse: generateToolSpanName called with data type: %T", eventData)
	}

	// Use reflection to access struct fields
	if reflect.TypeOf(eventData).Kind() == reflect.Ptr && !reflect.ValueOf(eventData).IsNil() {
		elem := reflect.ValueOf(eventData).Elem()
		if elem.Kind() == reflect.Struct {
			// Try to get Turn field
			turnField := elem.FieldByName("Turn")
			turn := 0
			if turnField.IsValid() {
				switch turnField.Kind() {
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					turn = int(turnField.Int())
				}
			}

			// Try to get ToolName field
			toolField := elem.FieldByName("ToolName")
			toolName := "unknown"
			if toolField.IsValid() && toolField.Kind() == reflect.String {
				toolName = toolField.String()
			}

			// Try to get ServerName field
			serverField := elem.FieldByName("ServerName")
			serverName := "unknown"
			if serverField.IsValid() && serverField.Kind() == reflect.String {
				serverName = serverField.String()
			}

			if l.debug {
				l.logger.Infof("🔍 Langfuse: generateToolSpanName - turn: %d, tool: %s, server: %s", turn, toolName, serverName)
			}

			result := fmt.Sprintf("tool_%s_%s_turn_%d", serverName, toolName, turn)
			if l.debug {
				l.logger.Infof("🔍 Langfuse: generateToolSpanName returning: %s", result)
			}
			return result
		}
	}

	// Fallback for map data
	if data, ok := eventData.(map[string]interface{}); ok {
		turn := 0
		toolName := "unknown"
		serverName := "unknown"

		if turnVal, ok := data["turn"].(float64); ok {
			turn = int(turnVal)
		}
		if toolVal, ok := data["tool_name"].(string); ok {
			toolName = toolVal
		}
		if serverVal, ok := data["server_name"].(string); ok {
			serverName = serverVal
		}

		result := fmt.Sprintf("tool_%s_%s_turn_%d", serverName, toolName, turn)
		if l.debug {
			l.logger.Infof("🔍 Langfuse: generateToolSpanName (map) returning: %s", result)
		}
		return result
	}

	if l.debug {
		l.logger.Infof("🔍 Langfuse: generateToolSpanName falling back to default")
	}
	return "tool_execution"
}

// StartObservation starts a new observation with the specified type
func (l *LangfuseTracer) StartObservation(parentID string, obsType string, name string, input interface{}) SpanID {
	id := generateID()
	observation := &langfuseObservation{
		ID:                  id,
		TraceID:             parentID, // For root observations, this is the trace ID
		ParentObservationID: "",       // Will be set if this is a child observation
		Name:                name,
		Type:                obsType, // Use the specified observation type
		Input:               input,
		StartTime:           time.Now(),
		Metadata:            make(map[string]interface{}),
	}

	// Check if parentID is actually a span ID (child observation)
	l.mu.RLock()
	if parentObservation, exists := l.spans[parentID]; exists {
		observation.TraceID = parentObservation.TraceID
		observation.ParentObservationID = parentID
	}
	l.mu.RUnlock()

	l.mu.Lock()
	l.spans[id] = observation
	l.mu.Unlock()

	// Queue observation creation event
	event := &langfuseEvent{
		ID:        generateID(),
		Type:      "observation-create",
		Timestamp: time.Now(),
		Body:      observation,
	}

	select {
	case l.eventQueue <- event:
	default:
		if l.debug {
			l.logger.Errorf("⚠️ Langfuse: Event queue full, dropping span-create event")
		}
	}

	if l.debug {
		l.logger.Infof("📊 Langfuse: Started span '%s' (ID: %s, Parent: %s)", name, id, parentID)
	}

	return SpanID(id)
}

// EndSpan ends a span with optional output and error
func (l *LangfuseTracer) EndSpan(spanID SpanID, output interface{}, err error) {
	l.mu.Lock()
	span, exists := l.spans[string(spanID)]
	if !exists {
		l.mu.Unlock()
		if l.debug {
			l.logger.Errorf("⚠️ Langfuse: Span %s not found for end", spanID)
		}
		return
	}

	endTime := time.Now()
	span.EndTime = &endTime
	span.Output = output

	if err != nil {
		span.Level = "ERROR"
		span.StatusMessage = err.Error()
	} else {
		span.Level = "DEFAULT"
	}
	l.mu.Unlock()

	// Queue span update event
	event := &langfuseEvent{
		ID:        generateID(),
		Type:      "span-update",
		Timestamp: time.Now(),
		Body:      span,
	}

	select {
	case l.eventQueue <- event:
	default:
		if l.debug {
			l.logger.Errorf("⚠️ Langfuse: Event queue full, dropping span-update event")
		}
	}

	if l.debug {
		status := "✅"
		if err != nil {
			status = ""
		}
		l.logger.Infof("%s Langfuse: Ended span '%s' (ID: %s)", status, span.Name, spanID)
	}
}

// EndTrace ends a trace with optional output
func (l *LangfuseTracer) EndTrace(traceID TraceID, output interface{}) {
	l.mu.Lock()
	trace, exists := l.traces[string(traceID)]
	if !exists {
		l.mu.Unlock()
		if l.debug {
			l.logger.Errorf("⚠️ Langfuse: Trace %s not found for end", traceID)
		}
		return
	}

	trace.Output = output
	l.mu.Unlock()

	// Queue trace update event (using trace-create with updated data)
	event := &langfuseEvent{
		ID:        generateID(),
		Type:      "trace-create",
		Timestamp: time.Now(),
		Body:      trace,
	}

	select {
	case l.eventQueue <- event:
	default:
		if l.debug {
			l.logger.Errorf("⚠️ Langfuse: Event queue full, dropping trace-update event")
		}
	}

	if l.debug {
		l.logger.Infof("🏁 Langfuse: Ended trace '%s' (ID: %s)", trace.Name, traceID)
	}
}

// CreateGenerationSpan creates a generation span for LLM calls
func (l *LangfuseTracer) CreateGenerationSpan(traceID TraceID, parentID SpanID, name, model string, input interface{}) SpanID {
	id := generateID()
	span := &langfuseSpan{
		ID:                  id,
		TraceID:             string(traceID),
		ParentObservationID: string(parentID),
		Name:                name,
		Type:                "GENERATION",
		Input:               input,
		StartTime:           time.Now(),
		Model:               model,
		Metadata:            make(map[string]interface{}),
	}

	// If parentID is empty, this is a root generation
	if parentID == "" {
		span.ParentObservationID = ""
	}

	l.mu.Lock()
	l.spans[id] = span
	l.mu.Unlock()

	// Queue generation creation event
	event := &langfuseEvent{
		ID:        generateID(),
		Type:      "generation-create",
		Timestamp: time.Now(),
		Body:      span,
	}

	select {
	case l.eventQueue <- event:
	default:
		if l.debug {
			l.logger.Errorf("⚠️ Langfuse: Event queue full, dropping generation-create event")
		}
	}

	if l.debug {
		l.logger.Infof("🤖 Langfuse: Started generation '%s' (ID: %s, Model: %s)", name, id, model)
	}

	return SpanID(id)
}

// EndGenerationSpan ends a generation span with metadata, usage metrics, and optional error
func (l *LangfuseTracer) EndGenerationSpan(spanID SpanID, metadata map[string]interface{}, usage UsageMetrics, err error) {
	l.mu.Lock()
	span, exists := l.spans[string(spanID)]
	if !exists {
		l.mu.Unlock()
		if l.debug {
			l.logger.Errorf("⚠️ Langfuse: Generation span %s not found for end", spanID)
		}
		return
	}

	endTime := time.Now()
	span.EndTime = &endTime

	// Store metadata in span input
	span.Input = metadata

	// Convert usage metrics to Langfuse format
	if usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.TotalTokens > 0 {
		span.Usage = &LangfuseUsage{
			Input:  usage.InputTokens,
			Output: usage.OutputTokens,
			Total:  usage.TotalTokens,
			Unit:   usage.Unit,
		}
		span.PromptTokens = usage.InputTokens
		span.CompletionTokens = usage.OutputTokens
		span.TotalTokens = usage.TotalTokens
	}

	if err != nil {
		span.Level = "ERROR"
		span.StatusMessage = err.Error()
	} else {
		span.Level = "DEFAULT"
	}
	l.mu.Unlock()

	// Queue generation update event
	event := &langfuseEvent{
		ID:        generateID(),
		Type:      "generation-update",
		Timestamp: time.Now(),
		Body:      span,
	}

	select {
	case l.eventQueue <- event:
	default:
		if l.debug {
			l.logger.Errorf("⚠️ Langfuse: Event queue full, dropping generation-update event")
		}
	}

	if l.debug {
		status := "✅"
		if err != nil {
			status = "❌"
		}
		l.logger.Infof("%s Langfuse: Ended generation '%s' (ID: %s, Tokens: %d/%d/%d)",
			status, span.Name, spanID, usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
	}
}

// eventProcessor processes events in the background and sends them to Langfuse
func (l *LangfuseTracer) eventProcessor() {
	defer l.wg.Done()

	ticker := time.NewTicker(2 * time.Second) // Batch events every 2 seconds
	defer ticker.Stop()

	var batch []*langfuseEvent

	for {
		select {
		case event := <-l.eventQueue:
			batch = append(batch, event)

			// Send batch when it reaches size limit
			if len(batch) >= 50 {
				l.sendBatch(batch)
				batch = nil
			}

		case <-ticker.C:
			// Send batch on timer if there are events
			if len(batch) > 0 {
				l.sendBatch(batch)
				batch = nil
			}

		case <-l.stopCh:
			// Send final batch and exit
			if len(batch) > 0 {
				l.sendBatch(batch)
			}
			return
		}
	}
}

// sendBatch sends a batch of events to Langfuse ingestion API
func (l *LangfuseTracer) sendBatch(events []*langfuseEvent) {
	if len(events) == 0 {
		return
	}

	// Convert to slice of values instead of pointers
	batch := make([]langfuseEvent, len(events))
	for i, event := range events {
		batch[i] = *event
	}

	payload := langfuseIngestionPayload{
		Batch: batch,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		if l.debug {
			l.logger.Errorf("❌ Langfuse: Failed to marshal batch: %w", err)
		}
		return
	}

	req, err := http.NewRequest("POST", l.host+"/api/public/ingestion", bytes.NewBuffer(jsonData))
	if err != nil {
		if l.debug {
			l.logger.Errorf("❌ Langfuse: Failed to create request: %w", err)
		}
		return
	}

	req.SetBasicAuth(l.publicKey, l.secretKey)
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := l.client.Do(req)
	if err != nil {
		if l.debug {
			l.logger.Errorf("❌ Langfuse: Failed to send batch: %w", err)
		}
		return
	}
	defer resp.Body.Close()

	// Read response body once
	body, _ := io.ReadAll(resp.Body)

	// Handle response - accept 200 (OK), 201 (Created), and 207 (Multi-Status for batch operations)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != 207 {
		if l.debug {
			l.logger.Errorf("❌ Langfuse: Batch failed with status %d: %s", resp.StatusCode, string(body))
		}
		return
	}

	// For status 207 (Multi-Status), check if there are any actual errors
	if resp.StatusCode == 207 {
		var batchResult map[string]interface{}
		if err := json.Unmarshal(body, &batchResult); err == nil {
			if errors, ok := batchResult["errors"].([]interface{}); ok && len(errors) > 0 {
				if l.debug {
					l.logger.Errorf("❌ Langfuse: Batch had errors: %s", string(body))
				}
				return
			}
		}
		// If no errors or can't parse, treat as success
	}

	if l.debug {
		l.logger.Infof("📤 Langfuse: Sent batch of %d events successfully", len(events))
	}
}

// Flush sends any pending events immediately
func (l *LangfuseTracer) Flush() {
	// Send a flush signal by closing and reopening the stop channel
	// This is a simple way to trigger immediate batch sending
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Wait for queue to drain
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if len(l.eventQueue) == 0 {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// Shutdown gracefully shuts down the tracer
func (l *LangfuseTracer) Shutdown() {
	close(l.stopCh)
	l.wg.Wait()
	close(l.eventQueue)
}

// EmitEvent processes an agent event and takes appropriate tracing actions
func (l *LangfuseTracer) EmitEvent(event AgentEvent) error {
	if l.debug {
		l.logger.Infof("🔍 Langfuse: Processing event %s (correlation: %s)", event.GetType(), event.GetCorrelationID())
	}

	switch event.GetType() {
	case EventTypeAgentStart:
		return l.handleAgentStart(event)
	case EventTypeAgentEnd:
		return l.handleAgentEnd(event)
	case EventTypeAgentError:
		return l.handleAgentError(event)
	case EventTypeConversationStart:
		return l.handleConversationStart(event)
	case EventTypeConversationEnd:
		return l.handleConversationEnd(event)
	case EventTypeLLMGenerationStart:
		return l.handleLLMGenerationStart(event)
	case EventTypeLLMGenerationEnd:
		return l.handleLLMGenerationEnd(event)
	case EventTypeToolCallStart:
		return l.handleToolCallStart(event)
	case EventTypeToolCallEnd:
		return l.handleToolCallEnd(event)
	case EventTypeTokenUsage:
		return l.handleTokenUsage(event)

	// MCP Server connection events
	case EventTypeMCPServerConnectionStart:
		return l.handleMCPServerConnectionStart(event)
	case EventTypeMCPServerConnectionEnd:
		return l.handleMCPServerConnectionEnd(event)
	case EventTypeMCPServerConnectionError:
		return l.handleMCPServerConnectionError(event)
	case EventTypeMCPServerDiscovery:
		return l.handleMCPServerDiscovery(event)
	case EventTypeMCPServerSelection:
		return l.handleMCPServerSelection(event)

	default:
		if l.debug {
			l.logger.Infof("🔍 Langfuse: Unhandled event type: %s", event.GetType())
		}
		return nil
	}
}

// EmitLLMEvent handles LLM events by forwarding them to the primary tracer
func (l *LangfuseTracer) EmitLLMEvent(event LLMEvent) error {
	// For now, just log that we received an LLM event
	// In the future, we could implement specific LLM event handling
	if l.debug {
		l.logger.Infof("🔍 Langfuse: Received LLM event (model: %s, provider: %s)", event.GetModelID(), event.GetProvider())
	}
	return nil
}

// handleAgentStart creates a new trace and agent execution span
func (l *LangfuseTracer) handleAgentStart(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Generate meaningful trace name from user query
	traceName := l.generateTraceName(event.GetData())

	// Create trace in Langfuse
	trace := &langfuseTrace{
		ID:        traceID,
		Name:      traceName,
		Input:     event.GetData(),
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"event_type": "agent_start",
			"agent_mode": "simple", // Will be updated when we have more context
		},
	}

	// Store trace
	l.mu.Lock()
	l.traces[traceID] = trace
	l.mu.Unlock()

	// Generate informative span name based on event data
	spanName := l.generateAgentSpanName(event.GetData())
	agentObsID := l.StartObservation(traceID, "AGENT", spanName, event.GetData())

	// Store the agent span ID for this trace to maintain hierarchy
	l.mu.Lock()
	l.agentSpans[traceID] = string(agentObsID)
	l.mu.Unlock()

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Created trace %s and agent observation '%s' (ID: %s)", traceID, spanName, agentObsID)
	}

	// Queue trace creation event
	traceEvent := &langfuseEvent{
		ID:        generateID(),
		Type:      "trace-create",
		Timestamp: time.Now(),
		Body:      trace,
	}
	select {
	case l.eventQueue <- traceEvent:
	default:
		if l.debug {
			l.logger.Warnf("🔍 Langfuse: Event queue full, dropping trace creation event")
		}
	}

	return nil
}

// handleAgentEnd creates a new span for agent completion and updates trace output
func (l *LangfuseTracer) handleAgentEnd(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Create a new span for agent completion instead of trying to find previous one
	spanID := l.StartSpan(traceID, "agent_completion", event.GetData())

	// End the span immediately since this is a completion event
	l.EndSpan(spanID, event.GetData(), nil)

	// Extract final result from event data and update trace output
	finalResult := l.extractFinalResult(event.GetData())
	if finalResult != "" {
		l.mu.Lock()
		if trace, exists := l.traces[traceID]; exists {
			trace.Output = finalResult
			if l.debug {
				l.logger.Infof("🔍 Langfuse: Updated trace %s with final output: %s", traceID, finalResult)
			}
		}
		l.mu.Unlock()
	}

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Created and ended agent completion span (span: %s)", spanID)
	}
	return nil
}

// handleAgentError creates a new span for agent error
func (l *LangfuseTracer) handleAgentError(event AgentEvent) error {
	// Create a new span for agent error instead of trying to find previous one
	spanID := l.StartSpan(event.GetTraceID(), "agent_error", event.GetData())

	// Extract error from event data
	var err error
	if data, ok := event.GetData().(map[string]interface{}); ok {
		if errorMsg, ok := data["error"].(string); ok {
			err = fmt.Errorf("%s", errorMsg)
		}
	}

	// End the span immediately with error
	l.EndSpan(spanID, event.GetData(), err)

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Created and ended agent error span (span: %s)", spanID)
	}
	return nil
}

// handleConversationStart creates a new conversation span as child of agent span
func (l *LangfuseTracer) handleConversationStart(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Get the agent span ID for this trace to use as parent
	l.mu.RLock()
	parentSpanID := l.agentSpans[traceID]
	l.mu.RUnlock()

	// If no agent span ID found, use trace ID as fallback
	if parentSpanID == "" {
		parentSpanID = traceID
		if l.debug {
			l.logger.Warnf("🔍 Langfuse: No agent span found for trace %s, using trace ID as parent", traceID)
		}
	}

	// Generate informative span name based on event data
	spanName := l.generateConversationSpanName(event.GetData())
	conversationSpanID := l.StartSpan(parentSpanID, spanName, event.GetData())

	// Store the conversation span ID for this trace to maintain hierarchy
	l.mu.Lock()
	l.conversationSpans[traceID] = string(conversationSpanID)
	l.mu.Unlock()

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Started conversation span '%s' (span: %s, parent: %s)", spanName, conversationSpanID, parentSpanID)
	}
	return nil
}

// handleConversationEnd creates a new span for conversation completion and captures final output
func (l *LangfuseTracer) handleConversationEnd(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Create a new span for conversation completion instead of trying to find previous one
	spanID := l.StartSpan(traceID, "conversation_completion", event.GetData())

	// End the span immediately since this is a completion event
	l.EndSpan(spanID, event.GetData(), nil)

	// Try to extract final result from conversation end event and update trace output
	finalResult := l.extractFinalResult(event.GetData())
	if finalResult != "" {
		l.mu.Lock()
		if trace, exists := l.traces[traceID]; exists && (trace.Output == nil || trace.Output == "") {
			trace.Output = finalResult
			if l.debug {
				l.logger.Infof("🔍 Langfuse: Updated trace %s with final output from conversation end: %s", traceID, finalResult)
			}
		}
		l.mu.Unlock()
	}

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Created and ended conversation completion span (span: %s)", spanID)
	}
	return nil
}

// handleLLMGenerationStart creates a new LLM generation span as child of conversation span
func (l *LangfuseTracer) handleLLMGenerationStart(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Get the conversation span ID for this trace to use as parent
	l.mu.RLock()
	parentSpanID := l.conversationSpans[traceID]
	l.mu.RUnlock()

	// If no conversation span ID found, use trace ID as fallback
	if parentSpanID == "" {
		parentSpanID = traceID
		if l.debug {
			l.logger.Warnf("🔍 Langfuse: No conversation span found for trace %s, using trace ID as parent", traceID)
		}
	}

	// Generate informative span name based on event data
	spanName := l.generateLLMSpanName(event.GetData())
	llmGenerationID := l.StartObservation(parentSpanID, "GENERATION", spanName, event.GetData())

	// Store the LLM generation span ID for this trace to maintain hierarchy
	l.mu.Lock()
	l.llmGenerationSpans[traceID] = string(llmGenerationID)
	l.mu.Unlock()

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Started LLM generation observation '%s' (ID: %s, parent: %s)", spanName, llmGenerationID, parentSpanID)
	}
	return nil
}

// handleLLMGenerationEnd creates a new span for LLM generation completion
func (l *LangfuseTracer) handleLLMGenerationEnd(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Get the LLM generation span ID for this trace to use as parent
	l.mu.RLock()
	parentSpanID := l.llmGenerationSpans[traceID]
	l.mu.RUnlock()

	// If no LLM generation span ID found, use trace ID as fallback
	if parentSpanID == "" {
		parentSpanID = traceID
	}

	spanID := l.StartSpan(parentSpanID, "llm_generation_completion", event.GetData())

	// End the span immediately since this is a completion event
	l.EndSpan(spanID, event.GetData(), nil)

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Created and ended LLM generation completion span (span: %s)", spanID)
	}
	return nil
}

// handleToolCallStart creates a new tool call span as child of LLM generation span
func (l *LangfuseTracer) handleToolCallStart(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Get the LLM generation span ID for this trace to use as parent
	l.mu.RLock()
	parentSpanID := l.llmGenerationSpans[traceID]
	l.mu.RUnlock()

	// If no LLM generation span ID found, use trace ID as fallback
	if parentSpanID == "" {
		parentSpanID = traceID
		if l.debug {
			l.logger.Warnf("🔍 Langfuse: No LLM generation span found for trace %s, using trace ID as parent", traceID)
		}
	}

	// Generate informative span name based on event data
	spanName := l.generateToolSpanName(event.GetData())
	toolID := l.StartObservation(parentSpanID, "TOOL", spanName, event.GetData())

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Started tool observation '%s' (ID: %s, parent: %s)", spanName, toolID, parentSpanID)
	}
	return nil
}

// handleToolCallEnd creates a new span for tool call completion
func (l *LangfuseTracer) handleToolCallEnd(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Get the LLM generation span ID for this trace to use as parent
	l.mu.RLock()
	parentSpanID := l.llmGenerationSpans[traceID]
	l.mu.RUnlock()

	// If no LLM generation span ID found, use trace ID as fallback
	if parentSpanID == "" {
		parentSpanID = traceID
	}

	spanID := l.StartSpan(parentSpanID, "tool_call_completion", event.GetData())

	// End the span immediately since this is a completion event
	l.EndSpan(spanID, event.GetData(), nil)

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Created and ended tool call completion span (span: %s)", spanID)
	}
	return nil
}

// handleTokenUsage creates a token usage span
func (l *LangfuseTracer) handleTokenUsage(event AgentEvent) error {
	spanID := l.StartSpan(event.GetTraceID(), "token_usage", event.GetData())

	// Token usage spans are typically short-lived, so end them immediately
	l.EndSpan(spanID, event.GetData(), nil)

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Processed token usage (span: %s)", spanID)
	}
	return nil
}

// MCP Server connection event handlers

// handleMCPServerConnectionStart creates a new span for MCP server connection start
func (l *LangfuseTracer) handleMCPServerConnectionStart(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Create a new span for MCP server connection start
	spanID := l.StartSpan(traceID, "mcp_server_connection_start", event.GetData())

	// Store the span ID for later completion
	l.mu.Lock()
	l.spans[string(spanID)] = &langfuseSpan{
		ID:        string(spanID),
		TraceID:   traceID,
		StartTime: time.Now(),
	}
	l.mu.Unlock()

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Created MCP server connection start span %s for trace %s", spanID, traceID)
	}

	return nil
}

// handleMCPServerConnectionEnd creates a new span for MCP server connection end
func (l *LangfuseTracer) handleMCPServerConnectionEnd(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Create a new span for MCP server connection end
	spanID := l.StartSpan(traceID, "mcp_server_connection_end", event.GetData())

	// End the span immediately since connection end is a point-in-time event
	l.EndSpan(spanID, event.GetData(), nil)

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Created MCP server connection end span %s for trace %s", spanID, traceID)
	}

	return nil
}

// handleMCPServerConnectionError creates a new span for MCP server connection error
func (l *LangfuseTracer) handleMCPServerConnectionError(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Create a new span for MCP server connection error
	spanID := l.StartSpan(traceID, "mcp_server_connection_error", event.GetData())

	// End the span immediately since connection error is a point-in-time event
	l.EndSpan(spanID, event.GetData(), nil)

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Created MCP server connection error span %s for trace %s", spanID, traceID)
	}

	return nil
}

// handleMCPServerDiscovery creates a new span for MCP server discovery
func (l *LangfuseTracer) handleMCPServerDiscovery(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Create a new span for MCP server discovery
	spanID := l.StartSpan(traceID, "mcp_server_discovery", event.GetData())

	// End the span immediately since discovery is a point-in-time event
	l.EndSpan(spanID, event.GetData(), nil)

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Created MCP server discovery span %s for trace %s", spanID, traceID)
	}

	return nil
}

// handleMCPServerSelection creates a new span for MCP server selection
func (l *LangfuseTracer) handleMCPServerSelection(event AgentEvent) error {
	traceID := event.GetTraceID()

	// Create a new span for MCP server selection
	spanID := l.StartSpan(traceID, "mcp_server_selection", event.GetData())

	// End the span immediately since selection is a point-in-time event
	l.EndSpan(spanID, event.GetData(), nil)

	if l.debug {
		l.logger.Infof("🔍 Langfuse: Created MCP server selection span %s for trace %s", spanID, traceID)
	}

	return nil
}

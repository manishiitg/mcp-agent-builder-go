package main

import (
	"context"
	"sync"

	mcpagent "github.com/manishiitg/mcpagent/agent"
	"github.com/manishiitg/mcpagent/events"
)

// toolCallCollector adapts mcpagent's canonical typed events to SparkQuill's
// API/SSE boundary. It deliberately accepts only the bridge-side receipt:
// coding-CLI transcript events describe intent and can omit results, while
// direct_execution fires exactly where the host handler really runs.
type toolCallCollector struct {
	mu             sync.Mutex
	calls          []events.ToolCallRecord
	indexByCallID  map[string]int
	conversationID string
	trace          *turnTrace
}

func newToolCallCollector(conversationID string, trace *turnTrace) *toolCallCollector {
	return &toolCallCollector{
		indexByCallID:  make(map[string]int),
		conversationID: conversationID,
		trace:          trace,
	}
}

func (c *toolCallCollector) Name() string { return "sparkquill-tool-call-collector" }

func (c *toolCallCollector) HandleEvent(_ context.Context, event *events.AgentEvent) error {
	record, ok := events.ToolCallRecordFromEvent(event.Data)
	if !ok || record.ServerName != "direct_execution" {
		return nil
	}

	c.mu.Lock()
	if record.Status == "running" {
		if c.trace != nil {
			c.trace.tool(record.ToolName)
		}
		c.indexByCallID[record.ToolCallID] = len(c.calls)
		c.calls = append(c.calls, record)
	} else if i, found := c.indexByCallID[record.ToolCallID]; found {
		// The end/error carries the authoritative result. Keep the start's
		// arguments so the completed record remains self-contained.
		record.Arguments = c.calls[i].Arguments
		c.calls[i] = record
	} else {
		// Defensive: an executor completion without a start is still useful
		// evidence, and should never be silently discarded.
		c.indexByCallID[record.ToolCallID] = len(c.calls)
		c.calls = append(c.calls, record)
	}
	c.mu.Unlock()

	if record.Status == "running" {
		if label, ok := toolStatusLabels[record.ToolName]; ok {
			statusHubs.publish(c.conversationID, label)
		}
	}
	statusHubs.publishToolCall(c.conversationID, record)
	return nil
}

func (c *toolCallCollector) Snapshot() []events.ToolCallRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]events.ToolCallRecord(nil), c.calls...)
}

var _ mcpagent.AgentEventListener = (*toolCallCollector)(nil)

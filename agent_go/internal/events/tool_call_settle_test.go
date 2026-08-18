package events

import (
	"testing"
	"time"

	"github.com/manishiitg/mcpagent/events"
)

func startEvent(sessionID, callID, name string) Event {
	return Event{
		Type: "tool_call_start", Timestamp: time.Now(), SessionID: sessionID,
		Data: &events.AgentEvent{
			Type: events.EventType("tool_call_start"), Timestamp: time.Now(), SessionID: sessionID,
			Data: &events.ToolCallStartEvent{ToolName: name, ToolCallID: callID},
		},
	}
}

func turnEndEvent(sessionID string) Event {
	return Event{
		Type: "unified_completion", Timestamp: time.Now(), SessionID: sessionID, ExecutionID: "exec-1",
		Data: &events.AgentEvent{Type: events.EventType("unified_completion"), Timestamp: time.Now(), SessionID: sessionID},
	}
}

func toolEndIDs(t *testing.T, store *EventStore, sessionID string) []string {
	t.Helper()
	ids := []string{}
	for _, e := range store.GetEvents(sessionID, GetEventsOptions{}).Events {
		if e.Type != "tool_call_end" || e.Data == nil {
			continue
		}
		if d, ok := e.Data.Data.(*events.ToolCallEndEvent); ok {
			ids = append(ids, d.ToolCallID)
		}
	}
	return ids
}

// TestTurnEndSettlesToolCallsThatNeverReported is the spinning-chip case.
//
// The store already detected this and logged, verbatim, that the chips "will
// spin forever" — then did nothing. A user watching that chat sees a spinner
// outliving the turn and concludes the agent is still working. salesoutreach
// showed exactly that: a chat that looked busy while its session had completed.
func TestTurnEndSettlesToolCallsThatNeverReported(t *testing.T) {
	store := NewEventStore(500)
	const sessionID = "session-spinner"

	store.AddEvent(sessionID, startEvent(sessionID, "call-1", "run_shell"))
	store.AddEvent(sessionID, startEvent(sessionID, "call-2", "read_file"))
	store.AddEvent(sessionID, turnEndEvent(sessionID))

	settled := toolEndIDs(t, store, sessionID)
	if len(settled) != 2 {
		t.Fatalf("settled %d tool calls (%v), want both call-1 and call-2 closed", len(settled), settled)
	}
}

// TestTurnEndLeavesProperlyClosedCallsAlone: a tool that reported its own end
// must not be settled a second time, or the UI sees a duplicate completion.
func TestTurnEndLeavesProperlyClosedCallsAlone(t *testing.T) {
	store := NewEventStore(500)
	const sessionID = "session-clean"

	store.AddEvent(sessionID, startEvent(sessionID, "call-1", "run_shell"))
	store.AddEvent(sessionID, Event{
		Type: "tool_call_end", Timestamp: time.Now(), SessionID: sessionID,
		Data: &events.AgentEvent{
			Type: events.EventType("tool_call_end"), Timestamp: time.Now(), SessionID: sessionID,
			Data: &events.ToolCallEndEvent{ToolName: "run_shell", ToolCallID: "call-1", Result: "ok"},
		},
	})
	before := len(toolEndIDs(t, store, sessionID))
	store.AddEvent(sessionID, turnEndEvent(sessionID))

	if after := len(toolEndIDs(t, store, sessionID)); after != before {
		t.Errorf("tool_call_end count went %d -> %d; a already-closed call was settled again", before, after)
	}
}

// TestSettledCallSaysItWasNotAReportedResult. A chip that silently turns green
// would swap a visible wrong state for an invisible one — the tool genuinely
// never reported, and the record has to say so.
func TestSettledCallSaysItWasNotAReportedResult(t *testing.T) {
	store := NewEventStore(500)
	const sessionID = "session-honest"

	store.AddEvent(sessionID, startEvent(sessionID, "call-1", "run_shell"))
	store.AddEvent(sessionID, turnEndEvent(sessionID))

	for _, e := range store.GetEvents(sessionID, GetEventsOptions{}).Events {
		if e.Type == "tool_call_end" {
			if e.Error == "" {
				t.Error("settled call is indistinguishable from a real completion")
			}
			return
		}
	}
	t.Fatal("no tool_call_end was emitted at all")
}

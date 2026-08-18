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
		if e.Data == nil {
			continue
		}
		switch d := e.Data.Data.(type) {
		case *events.ToolCallEndEvent:
			ids = append(ids, d.ToolCallID)
		case *events.ToolCallErrorEvent:
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
	restore := shortenSettleGrace(t)
	defer restore()
	store := NewEventStore(500)
	const sessionID = "session-spinner"

	store.AddEvent(sessionID, startEvent(sessionID, "call-1", "run_shell"))
	store.AddEvent(sessionID, startEvent(sessionID, "call-2", "read_file"))
	store.AddEvent(sessionID, turnEndEvent(sessionID))
	time.Sleep(200 * time.Millisecond)

	settled := toolEndIDs(t, store, sessionID)
	if len(settled) != 2 {
		t.Fatalf("settled %d tool calls (%v), want both call-1 and call-2 closed", len(settled), settled)
	}
}

// TestTurnEndLeavesProperlyClosedCallsAlone: a tool that reported its own end
// must not be settled a second time, or the UI sees a duplicate completion.
func TestTurnEndLeavesProperlyClosedCallsAlone(t *testing.T) {
	restore := shortenSettleGrace(t)
	defer restore()
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
	time.Sleep(200 * time.Millisecond)

	if after := len(toolEndIDs(t, store, sessionID)); after != before {
		t.Errorf("tool_call_end count went %d -> %d; a already-closed call was settled again", before, after)
	}
}

// TestSettledCallSaysItWasNotAReportedResult. A chip that silently turns green
// would swap a visible wrong state for an invisible one — the tool genuinely
// never reported, and the record has to say so.
func TestSettledCallSaysItWasNotAReportedResult(t *testing.T) {
	restore := shortenSettleGrace(t)
	defer restore()
	store := NewEventStore(500)
	const sessionID = "session-honest"

	store.AddEvent(sessionID, startEvent(sessionID, "call-1", "run_shell"))
	store.AddEvent(sessionID, turnEndEvent(sessionID))
	time.Sleep(200 * time.Millisecond)

	for _, e := range store.GetEvents(sessionID, GetEventsOptions{}).Events {
		if e.Type == string(events.ToolCallError) {
			if e.Error == "" {
				t.Error("settled call is indistinguishable from a real completion")
			}
			if _, ok := e.Data.Data.(*events.ToolCallErrorEvent); !ok {
				t.Error("settled call was emitted as a successful end event — the UI renders that as a green tick")
			}
			return
		}
		if e.Type == "tool_call_end" {
			t.Fatal("settled call emitted as tool_call_end; the UI shows that as Command Completed")
		}
	}
	t.Fatal("no settle event was emitted at all")
}

func shortenSettleGrace(t *testing.T) func() {
	t.Helper()
	previous := toolCallSettleGrace
	toolCallSettleGrace = 20 * time.Millisecond
	return func() { toolCallSettleGrace = previous }
}

// TestLateToolResultIsNotSettled is the measured race that made this necessary.
//
// tectonicusadaytrading, 2026-08-18: the turn ended at 20:34:28 and the tool's
// completion was processed at 20:34:29. Settling on the turn-end signal alone
// condemned fourteen shell commands that had in fact completed — the session's
// native transcript shows 215 tool_use and 215 tool_result, so the model
// received every one of them. Only the UI's copy was ever missing.
func TestLateToolResultIsNotSettled(t *testing.T) {
	previous := toolCallSettleGrace
	toolCallSettleGrace = 400 * time.Millisecond
	defer func() { toolCallSettleGrace = previous }()
	store := NewEventStore(500)
	const sessionID = "session-late"

	store.AddEvent(sessionID, startEvent(sessionID, "call-1", "run_shell"))
	store.AddEvent(sessionID, turnEndEvent(sessionID))
	// The real end lands measurably after the turn-end signal, but inside the
	// window — the one-second gap observed in production, scaled down.
	time.Sleep(60 * time.Millisecond)
	store.AddEvent(sessionID, Event{
		Type: "tool_call_end", Timestamp: time.Now(), SessionID: sessionID,
		Data: &events.AgentEvent{
			Type: events.EventType("tool_call_end"), Timestamp: time.Now(), SessionID: sessionID,
			Data: &events.ToolCallEndEvent{ToolName: "run_shell", ToolCallID: "call-1", Result: "real output"},
		},
	})
	time.Sleep(700 * time.Millisecond)

	for _, e := range store.GetEvents(sessionID, GetEventsOptions{}).Events {
		if e.Type == string(events.ToolCallError) {
			t.Fatal("a tool call that reported one second late was settled as never having reported")
		}
	}
}

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

// TestSettledCallSaysNothingInTheChat.
//
// The settle exists to stop a chip spinning forever, not to explain a platform
// gap to a reader. Three message variants were tried on this chip and all three
// were reported as confusing or alarming; the diagnostic now lives only in the
// server log (PLAT-141).
func TestSettledCallSaysNothingInTheChat(t *testing.T) {
	restore := shortenSettleGrace(t)
	defer restore()
	store := NewEventStore(500)
	const sessionID = "session-honest"

	store.AddEvent(sessionID, startEvent(sessionID, "call-1", "run_shell"))
	store.AddEvent(sessionID, turnEndEvent(sessionID))
	time.Sleep(200 * time.Millisecond)

	for _, e := range store.GetEvents(sessionID, GetEventsOptions{}).Events {
		if e.Type == "tool_call_end" {
			d, ok := e.Data.Data.(*events.ToolCallEndEvent)
			if !ok {
				t.Fatal("settle emitted the wrong payload type")
			}
			// The chip closes and says nothing. The tool ran and the agent used
			// its result; our missing copy is a server-side diagnostic, not
			// something to explain to whoever is reading the chat.
			if d.Result != "" {
				t.Errorf("settled call put text in the chat: %q — this belongs in the log", d.Result)
			}
			if e.Error != "" {
				t.Errorf("settled call carries an error (%q) for a tool that did not fail", e.Error)
			}
			return
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
		if e.Type != "tool_call_end" || e.Data == nil {
			continue
		}
		if d, ok := e.Data.Data.(*events.ToolCallEndEvent); ok && d.Result == "" {
			t.Fatal("a tool call that reported one second late was overwritten with an empty settle instead of its real output")
		}
	}
}

// TestSettleRecoversTheRealResultWhenTheProviderHasIt is the PLAT-141 fix.
//
// The live stream produces no end for some calls — one measured completing in
// 41ms with no end event under any session — so the chip closed blank and the
// only duration available was how long we had waited (45.4s on screen for a
// 41ms command). The provider's own transcript has both, so the settle asks for
// them instead of inventing either.
func TestSettleRecoversTheRealResultWhenTheProviderHasIt(t *testing.T) {
	restore := shortenSettleGrace(t)
	defer restore()
	SetToolResultResolver(func(sessionID, toolCallID, name string, startedAt time.Time) (string, time.Duration, bool) {
		if toolCallID == "call-1" {
			return "yfinance 1.2.2\nnews items: 10", 41 * time.Millisecond, true
		}
		return "", 0, false
	})
	defer SetToolResultResolver(nil)

	store := NewEventStore(500)
	const sessionID = "session-recovered"
	store.AddEvent(sessionID, startEvent(sessionID, "call-1", "execute_shell_command"))
	store.AddEvent(sessionID, turnEndEvent(sessionID))
	time.Sleep(300 * time.Millisecond)

	for _, e := range store.GetEvents(sessionID, GetEventsOptions{}).Events {
		d, ok := e.Data.Data.(*events.ToolCallEndEvent)
		if !ok || d.ToolCallID != "call-1" {
			continue
		}
		if d.Result != "yfinance 1.2.2\nnews items: 10" {
			t.Errorf("result = %q, want the output recovered from the provider", d.Result)
		}
		if d.Duration != 41*time.Millisecond {
			t.Errorf("duration = %v, want the tool's real 41ms rather than how long we waited", d.Duration)
		}
		return
	}
	t.Fatal("no settle event was emitted")
}

// TestSettleStaysBlankWhenTheProviderHasNothing. A resolver that cannot find
// the call must not cause an empty string to be presented as the tool's output
// with a fabricated runtime; the chip closes with nothing, as before.
func TestSettleStaysBlankWhenTheProviderHasNothing(t *testing.T) {
	restore := shortenSettleGrace(t)
	defer restore()
	SetToolResultResolver(func(string, string, string, time.Time) (string, time.Duration, bool) { return "", 0, false })
	defer SetToolResultResolver(nil)

	store := NewEventStore(500)
	const sessionID = "session-unrecovered"
	store.AddEvent(sessionID, startEvent(sessionID, "call-1", "execute_shell_command"))
	store.AddEvent(sessionID, turnEndEvent(sessionID))
	time.Sleep(300 * time.Millisecond)

	for _, e := range store.GetEvents(sessionID, GetEventsOptions{}).Events {
		if d, ok := e.Data.Data.(*events.ToolCallEndEvent); ok && d.ToolCallID == "call-1" {
			if d.Result != "" {
				t.Errorf("result = %q, want empty when the provider has no record", d.Result)
			}
			return
		}
	}
	t.Fatal("no settle event was emitted")
}

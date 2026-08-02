package events

import "testing"

func TestResolveTerminalOwnerIDUsesPositiveMainIdentity(t *testing.T) {
	sessionID := "session-main-owner"
	event := Event{
		SessionID:     sessionID,
		ExecutionID:   "main:" + sessionID,
		ExecutionKind: "main_agent",
	}

	if got := ResolveTerminalOwnerID(sessionID, event, nil); got != "main:"+sessionID {
		t.Fatalf("owner = %q, want positive main owner", got)
	}
}

func TestResolveTerminalOwnerIDPrefersExplicitChildOwnerOverInheritedMainKind(t *testing.T) {
	sessionID := "session-child-owner"
	event := Event{
		SessionID:     sessionID,
		ExecutionID:   "main:" + sessionID,
		ExecutionKind: "main_agent",
	}

	got := ResolveTerminalOwnerID(sessionID, event, map[string]interface{}{
		"execution_owner_id": "background:reviewer",
	})
	if got != "background:reviewer" {
		t.Fatalf("owner = %q, want explicit child owner", got)
	}
}

func TestTerminalIDForOwnerMatchesTerminalStoreIdentity(t *testing.T) {
	if got := TerminalIDForOwner("session-1", "background:reviewer"); got != "session-1:background:reviewer" {
		t.Fatalf("terminal id = %q", got)
	}
}

func TestRetainEventInSessionWorkingSetOmitsOnlyChildTranscriptDetail(t *testing.T) {
	sessionID := "session-working-set"
	child := Event{
		Type:            "tool_call_end",
		TerminalOwnerID: "background:reviewer",
		TerminalID:      sessionID + ":background:reviewer",
	}
	if RetainEventInSessionWorkingSet(sessionID, child) {
		t.Fatal("child tool detail must be loaded from its terminal page, not the session channel")
	}

	child.Type = "background_agent_completed"
	if !RetainEventInSessionWorkingSet(sessionID, child) {
		t.Fatal("child lifecycle event must remain in the session working set")
	}

	main := child
	main.Type = "tool_call_end"
	main.TerminalOwnerID = "main:" + sessionID
	main.TerminalID = TerminalIDForOwner(sessionID, main.TerminalOwnerID)
	if !RetainEventInSessionWorkingSet(sessionID, main) {
		t.Fatal("main-agent detail must remain in the eager session working set")
	}

	legacy := Event{Type: "tool_call_end"}
	if !RetainEventInSessionWorkingSet(sessionID, legacy) {
		t.Fatal("legacy event without ownership must fail open")
	}
}

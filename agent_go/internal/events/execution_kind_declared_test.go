package events

import (
	"testing"
	"time"

	orchEvents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
	unifiedevents "github.com/manishiitg/mcpagent/events"
)

func backgroundStartedEvent(agentID, name string, kind orchEvents.ExecutionKind) Event {
	return Event{
		ID:        "e1",
		Type:      "background_agent_started",
		Timestamp: time.Now(),
		SessionID: "s1",
		Data: &unifiedevents.AgentEvent{
			Type:      "background_agent_started",
			SessionID: "s1",
			Component: "background-agent",
			Data: &orchEvents.BackgroundAgentStartedEvent{
				BaseEventData: unifiedevents.BaseEventData{Timestamp: time.Now(), SessionID: "s1"},
				AgentID:       agentID,
				Name:          name,
				Kind:          kind,
			},
		},
	}
}

func backgroundCompletedEvent(agentID, name string, kind orchEvents.ExecutionKind) Event {
	return Event{
		ID:        "e2",
		Type:      "background_agent_completed",
		Timestamp: time.Now(),
		SessionID: "s1",
		Data: &unifiedevents.AgentEvent{
			Type:      "background_agent_completed",
			SessionID: "s1",
			Component: "background-agent",
			Data: &orchEvents.BackgroundAgentCompletedEvent{
				BaseEventData: unifiedevents.BaseEventData{Timestamp: time.Now(), SessionID: "s1"},
				AgentID:       agentID,
				Name:          name,
				Status:        "failed",
				Kind:          kind,
			},
		},
	}
}

func autoNotificationSteeredEvent(agentID, name string, kind orchEvents.ExecutionKind) Event {
	return Event{
		ID:        "e3",
		Type:      string(orchEvents.AutoNotificationSteered),
		Timestamp: time.Now(),
		SessionID: "s1",
		Data: &unifiedevents.AgentEvent{
			Type:      orchEvents.AutoNotificationSteered,
			SessionID: "s1",
			Component: "background-agent",
			Data: &orchEvents.AutoNotificationSteeredEvent{
				BaseEventData: unifiedevents.BaseEventData{Timestamp: time.Now(), SessionID: "s1"},
				AgentID:       agentID,
				Name:          name,
				Status:        "failed",
				Provider:      "codex-cli",
				Kind:          kind,
			},
		},
	}
}

// A full run declares ExecutionKindFullRun because it is a CONTAINER with no
// conversation of its own. Flattening every background_agent_* event to
// "background_agent" discarded that declaration -- exactly what
// BackgroundAgentStartedEvent.Kind documents must not happen -- and the
// container then rendered as if it were an agent.
func TestDeclaredExecutionKindSurvivesBackgroundAgentEvents(t *testing.T) {
	store := NewEventStore(10)
	store.AddEvent("s1", backgroundStartedEvent(
		"workflow-full-abc", "full-run [Toptal Bid / iteration-0]", orchEvents.ExecutionKindFullRun))

	got := store.GetAllEventsRaw("s1")
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].ExecutionKind != string(orchEvents.ExecutionKindFullRun) {
		t.Errorf("ExecutionKind = %q, want the declared %q", got[0].ExecutionKind, orchEvents.ExecutionKindFullRun)
	}
	if got[0].ExecutionID != "workflow-full-abc" {
		t.Errorf("ExecutionID = %q, want the agent id", got[0].ExecutionID)
	}
}

func TestDeclaredExecutionKindSurvivesBackgroundAgentCompletion(t *testing.T) {
	store := NewEventStore(10)
	store.AddEvent("s1", backgroundCompletedEvent(
		"workflow-full-abc", "Full Workflow Execution", orchEvents.ExecutionKindFullRun))

	got := store.GetAllEventsRaw("s1")
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].ExecutionKind != string(orchEvents.ExecutionKindFullRun) {
		t.Errorf("ExecutionKind = %q, want the declared %q", got[0].ExecutionKind, orchEvents.ExecutionKindFullRun)
	}
	if got[0].ExecutionID != "workflow-full-abc" {
		t.Errorf("ExecutionID = %q, want the agent id", got[0].ExecutionID)
	}
}

func TestDeclaredExecutionKindSurvivesAutoNotification(t *testing.T) {
	store := NewEventStore(10)
	store.AddEvent("s1", autoNotificationSteeredEvent(
		"workflow-full-abc", "full-run [job-search / iteration-0]", orchEvents.ExecutionKindFullRun))

	got := store.GetAllEventsRaw("s1")
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].ExecutionKind != string(orchEvents.ExecutionKindFullRun) {
		t.Errorf("ExecutionKind = %q, want the declared %q", got[0].ExecutionKind, orchEvents.ExecutionKindFullRun)
	}
}

// An undeclared execution must keep the existing default rather than becoming
// empty: an empty kind means "unknown", which is not the same as "not an agent"
// and is handled differently downstream.
func TestUndeclaredBackgroundAgentKeepsDefaultKind(t *testing.T) {
	store := NewEventStore(10)
	store.AddEvent("s1", backgroundStartedEvent("bg-123", "some delegated agent", ""))

	got := store.GetAllEventsRaw("s1")
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].ExecutionKind != "background_agent" {
		t.Errorf("ExecutionKind = %q, want the background_agent default", got[0].ExecutionKind)
	}
}

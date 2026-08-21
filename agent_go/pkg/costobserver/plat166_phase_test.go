package costobserver

import (
	"testing"

	unifiedevents "github.com/manishiitg/mcpagent/events"
)

// TestObserverDefaultsToExecutionOnlyPhase pins PLAT-166: a freshly
// constructed observer attributes its entries to the execution turn until
// told otherwise — this is the state a step's own (non-reflection) work runs
// under for its entire lifetime unless a reflection turn later toggles it.
func TestObserverDefaultsToExecutionOnlyPhase(t *testing.T) {
	observer := New(nil, "sess-1", "user-1", "simple",
		WithAttribution(ScopeWorkflowExecution, "Workflow/demo", "", "sess-1:review-measure"),
	)
	if got := observer.Phase(); got != PhaseExecutionOnly {
		t.Fatalf("Phase() = %q, want %q", got, PhaseExecutionOnly)
	}
	entry := observer.baseEntry(&unifiedevents.AgentEvent{})
	if entry.Phase != PhaseExecutionOnly {
		t.Fatalf("baseEntry().Phase = %q, want %q", entry.Phase, PhaseExecutionOnly)
	}
}

// TestObserverSetPhaseTogglesEntryAttribution pins the reflection-turn
// bracket pattern (reflection_turn_run.go): SetPhase changes what every
// subsequent entry from this SAME observer instance carries, and a later
// SetPhase back restores the default — since the reflection turn reuses the
// step's own agent/observer rather than getting a fresh one.
func TestObserverSetPhaseTogglesEntryAttribution(t *testing.T) {
	observer := New(nil, "sess-1", "user-1", "simple",
		WithAttribution(ScopeWorkflowExecution, "Workflow/demo", "", "sess-1:review-measure"),
	)

	observer.SetPhase(PhaseReflection)
	if got := observer.Phase(); got != PhaseReflection {
		t.Fatalf("Phase() after SetPhase(reflection) = %q, want %q", got, PhaseReflection)
	}
	reflectionEntry := observer.baseEntry(&unifiedevents.AgentEvent{})
	if reflectionEntry.Phase != PhaseReflection {
		t.Fatalf("entry during reflection: Phase = %q, want %q", reflectionEntry.Phase, PhaseReflection)
	}

	observer.SetPhase(PhaseExecutionOnly)
	if got := observer.Phase(); got != PhaseExecutionOnly {
		t.Fatalf("Phase() after restoring = %q, want %q", got, PhaseExecutionOnly)
	}
	restoredEntry := observer.baseEntry(&unifiedevents.AgentEvent{})
	if restoredEntry.Phase != PhaseExecutionOnly {
		t.Fatalf("entry after restore: Phase = %q, want %q", restoredEntry.Phase, PhaseExecutionOnly)
	}
}

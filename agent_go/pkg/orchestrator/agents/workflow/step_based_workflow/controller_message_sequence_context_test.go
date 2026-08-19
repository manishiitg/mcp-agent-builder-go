package step_based_workflow

import "testing"

func TestMessageSequenceContinuationUsesOnlyNewDelegationInstruction(t *testing.T) {
	opts := messageSequenceCallOptions{
		ReentryMessage:      "durable route description\n\n## Orchestrator Instructions\n\ninspect the latest run",
		ContinuationMessage: "inspect the latest run",
	}

	if got := messageSequenceContinuationMessage(opts); got != "inspect the latest run" {
		t.Fatalf("continuation message = %q; want only the new parent instruction", got)
	}
}

func TestMessageSequenceContinuationFallsBackForLegacyCallers(t *testing.T) {
	opts := messageSequenceCallOptions{ReentryMessage: " continue the route "}

	if got := messageSequenceContinuationMessage(opts); got != "continue the route" {
		t.Fatalf("continuation message = %q; want trimmed legacy re-entry message", got)
	}
}

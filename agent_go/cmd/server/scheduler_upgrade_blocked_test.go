package server

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// A blocked contract-upgrade preflight means the workflow never executed. Pulse
// is a post-run steward, so there is nothing for it to gate, review, or publish
// — all a pass can do is spend an LLM turn restating the blocker, on every
// trigger, for as long as the workflow waits on an owner decision.
// confida-login did exactly that.

func TestBlockedUpgradePreflightIsDistinguishableFromAnOrdinaryRunFailure(t *testing.T) {
	blocked := workflowUpgradePreflightStampError("upgrade-current-artifact-contract", "1.0.21", "1.0.20", 1)
	if !errors.Is(blocked, errWorkflowUpgradePreflightBlocked) {
		t.Fatal("a declined stamp must be recognizable as a blocked preflight, or Pulse still runs")
	}

	// The operator-facing text has to survive the wrap — it is what lands in the
	// run record and the scheduler log.
	for _, want := range []string{"upgrade-current-artifact-contract", `"1.0.21"`, `"1.0.20"`, "failure 1/3 consecutive"} {
		if !strings.Contains(blocked.Error(), want) {
			t.Errorf("preflight error lost %q: %s", want, blocked)
		}
	}

	// A workflow that ran and failed still deserves a Pulse pass; only the
	// never-started case is skipped.
	for _, other := range []error{
		errors.New("step execute-tests failed"),
		errWorkshopSequenceInterrupted,
		errWorkshopIdleWaitTimeout,
		context.Canceled,
	} {
		if errors.Is(other, errWorkflowUpgradePreflightBlocked) {
			t.Errorf("%v was misclassified as a blocked upgrade preflight", other)
		}
	}
}

// A manifest whose version this server does not know has no upgrade path at
// all. That blocks every trigger permanently, so it must skip Pulse too rather
// than notifying about the same wall forever.
func TestMissingUpgradePathIsAlsoABlockedPreflight(t *testing.T) {
	_, err := scheduledWorkshopTurns(&WorkflowManifest{Version: "9.9.9"}, []string{"run it"})
	if err == nil {
		t.Fatal("a version with no upgrade path should not produce runnable turns")
	}
	if !errors.Is(err, errWorkflowUpgradePreflightBlocked) {
		t.Fatalf("missing upgrade path is not marked as blocked: %v", err)
	}
	if !strings.Contains(err.Error(), "no complete upgrade path") {
		t.Errorf("error lost its explanation: %v", err)
	}
}

// Failing open is the opposite decision: the schedule's normal messages do run,
// so there is a run and Pulse should review it as usual.
func TestFailOpenPathDoesNotMarkTheRunAsBlocked(t *testing.T) {
	turns, err := scheduledWorkshopTurns(&WorkflowManifest{Version: "1.0.20"}, []string{"run it"})
	if err != nil {
		t.Fatalf("a known version should produce turns: %v", err)
	}
	if len(turns) == 0 || turns[len(turns)-1].upgradeTarget != "" {
		t.Fatalf("expected the schedule message to be the final turn: %+v", turns)
	}
}

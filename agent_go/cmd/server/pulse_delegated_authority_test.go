package server

import (
	"context"
	"strings"
	"testing"
	"time"

	step_based_workflow "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	mcpexecutor "github.com/manishiitg/mcpagent/executor"
)

// TestDelegatedPulseAuthorityCannotBeSelfGranted covers the property that makes
// a background Fixer safe: authority is keyed by session id, so a child gets it
// only when a parent that already holds it hands it over. A read-only reviewer
// never holds it, which is what structurally stops reviewers writing state.
func TestDelegatedPulseAuthorityCannotBeSelfGranted(t *testing.T) {
	const runID = "schedule-cron--delegation"

	// A session with no authority of its own cannot mint any.
	if _, err := DelegateTrustedPulseSessionToChild("workshop-review-1", "workshop-fixer-1", runID); err == nil ||
		!strings.Contains(err.Error(), "does not hold Pulse write authority") {
		t.Fatalf("unauthorized session was able to delegate: %v", err)
	}

	releaseParent := registerTrustedPulseSession("schedule-cron--parent", runID)
	defer releaseParent()
	parentCtx := mcpexecutor.WithSessionID(context.Background(), "schedule-cron--parent")

	// A parent cannot delegate authority it does not hold for that run.
	if _, err := DelegateTrustedPulseSessionToChild("schedule-cron--parent", "workshop-fixer-1", "some-other-run"); err == nil ||
		!strings.Contains(err.Error(), "does not hold Pulse write authority") {
		t.Fatalf("parent delegated authority for a run it does not own: %v", err)
	}

	// The real path: the child can write for exactly this run.
	release, err := DelegateTrustedPulseSessionToChild("schedule-cron--parent", "workshop-fixer-1", runID)
	if err != nil {
		t.Fatalf("delegate to fixer child: %v", err)
	}
	childCtx := mcpexecutor.WithSessionID(context.Background(), "workshop-fixer-1")
	if err := validateTrustedPulseToolRunID(childCtx, runID); err != nil {
		t.Fatalf("delegated child could not write its own run: %v", err)
	}
	if err := validateTrustedPulseToolRunID(childCtx, "some-other-run"); err == nil {
		t.Fatal("delegated child could write a run it was never granted")
	}

	// Releasing the child revokes only the child.
	release()
	if err := validateTrustedPulseToolRunID(childCtx, runID); err == nil ||
		!strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("released child retained write authority: %v", err)
	}
	if err := validateTrustedPulseToolRunID(parentCtx, runID); err != nil {
		t.Fatalf("releasing the child revoked the parent: %v", err)
	}
}

// TestDelegatedPulseAuthorityInheritsParentExpiry stops a manual /pulse-fixer
// turn from laundering its bounded window into an unbounded one by handing it
// to a background child.
func TestDelegatedPulseAuthorityInheritsParentExpiry(t *testing.T) {
	const runID = "manual-fixer--bounded"
	registerTemporaryTrustedPulseSession("manual-parent", runID, 40*time.Millisecond)

	release, err := DelegateTrustedPulseSessionToChild("manual-parent", "manual-fixer-child", runID)
	if err != nil {
		t.Fatalf("delegate bounded authority: %v", err)
	}
	defer release()

	childCtx := mcpexecutor.WithSessionID(context.Background(), "manual-fixer-child")
	if err := validateTrustedPulseToolRunID(childCtx, runID); err != nil {
		t.Fatalf("child could not write inside the parent's window: %v", err)
	}

	time.Sleep(60 * time.Millisecond)
	if err := validateTrustedPulseToolRunID(childCtx, runID); err == nil ||
		!strings.Contains(err.Error(), "expired") {
		t.Fatalf("delegated child outlived the parent's bounded window: %v", err)
	}
}

// TestPulseWriteAuthorityDelegatorIsInstalled guards the cross-package seam.
// Authority lives here; children are spawned in step_based_workflow, which
// cannot import this package. If the init wiring is dropped, writer children
// stop being possible and the only symptom is a fail-closed error at spawn.
func TestPulseWriteAuthorityDelegatorIsInstalled(t *testing.T) {
	const runID = "schedule-cron--seam"
	releaseParent := registerTrustedPulseSession("schedule-cron--seam-parent", runID)
	defer releaseParent()

	release, err := step_based_workflow.LendPulseWriteAuthorityForTest(
		"schedule-cron--seam-parent", "workshop-fixer-seam", runID,
	)
	if err != nil {
		t.Fatalf("server init did not install the delegator: %v", err)
	}
	defer release()

	childCtx := mcpexecutor.WithSessionID(context.Background(), "workshop-fixer-seam")
	if err := validateTrustedPulseToolRunID(childCtx, runID); err != nil {
		t.Fatalf("authority delegated through the seam did not authorize the child: %v", err)
	}
}

// TestExpiredParentCannotDelegate closes the same hole from the other side.
func TestExpiredParentCannotDelegate(t *testing.T) {
	const runID = "manual-fixer--already-expired"
	registerTemporaryTrustedPulseSession("stale-parent", runID, 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)

	if _, err := DelegateTrustedPulseSessionToChild("stale-parent", "late-child", runID); err == nil {
		t.Fatal("expired parent was able to delegate write authority")
	}
}

// TestDelegationRefusesAnEmptyParentSession pins the failure this signature
// change fixed. Tool executors run on a context derived from the long-lived
// workshop session, not the MCP request, so reading the caller's session id
// from that context found nothing and refused every real delegation — the
// fixer stage never started and the failure looked like a rejected identity.
func TestDelegationRefusesAnEmptyParentSession(t *testing.T) {
	if _, err := DelegateTrustedPulseSessionToChild("", "workshop-fixer-x", "run-x"); err == nil ||
		!strings.Contains(err.Error(), "calling session's id") {
		t.Fatalf("empty parent session was not refused clearly: %v", err)
	}
}

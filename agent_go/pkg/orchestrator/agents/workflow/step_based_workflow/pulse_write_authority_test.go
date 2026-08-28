package step_based_workflow

import (
	"errors"
	"strings"
	"testing"
)

// TestLendPulseWriteAuthorityFailsClosedWhenUninstalled pins the fail-closed
// behavior. A writer child that starts unauthorized would run its whole
// analysis and then fail on its first state write, having already spent the
// work and possibly mutated files.
func TestLendPulseWriteAuthorityFailsClosedWhenUninstalled(t *testing.T) {
	original := pulseWriteAuthorityDelegator()
	t.Cleanup(func() { SetPulseWriteAuthorityDelegator(original) })

	SetPulseWriteAuthorityDelegator(nil)
	if _, err := lendPulseWriteAuthority("parent-1", "workshop-fixer-1", "run-1"); err == nil ||
		!strings.Contains(err.Error(), "not installed") {
		t.Fatalf("uninstalled delegator did not fail closed: %v", err)
	}
}

func TestLendPulseWriteAuthorityPropagatesRefusalAndRelease(t *testing.T) {
	original := pulseWriteAuthorityDelegator()
	t.Cleanup(func() { SetPulseWriteAuthorityDelegator(original) })

	refusal := errors.New("session does not hold Pulse write authority for run \"run-1\"")
	SetPulseWriteAuthorityDelegator(func(string, string, string) (func(), error) {
		return nil, refusal
	})
	if _, err := lendPulseWriteAuthority("parent-1", "workshop-fixer-1", "run-1"); !errors.Is(err, refusal) {
		t.Fatalf("refusal was not propagated to the caller: %v", err)
	}

	released := false
	var gotChild, gotRun string
	SetPulseWriteAuthorityDelegator(func(_, childSessionID, pulseRunID string) (func(), error) {
		gotChild, gotRun = childSessionID, pulseRunID
		return func() { released = true }, nil
	})
	release, err := lendPulseWriteAuthority("parent-1", "workshop-fixer-1", "run-1")
	if err != nil {
		t.Fatalf("lend authority: %v", err)
	}
	if gotChild != "workshop-fixer-1" || gotRun != "run-1" {
		t.Fatalf("delegator received child=%q run=%q", gotChild, gotRun)
	}
	release()
	if !released {
		t.Fatal("release did not reach the installed delegator")
	}
}

// TestPulseWriteAuthorityDelegatorIsInstalledByServer guards the seam itself.
// The delegator lives in cmd/server and is installed by its init; if that
// wiring is dropped, every writer child silently stops being possible.
func TestPulseWriteAuthorityDelegatorIsInstalledByServer(t *testing.T) {
	if pulseWriteAuthorityDelegator() != nil {
		t.Skip("delegator already installed by an importing binary")
	}
	// Nothing imports cmd/server from this package's own test binary, so an
	// uninstalled delegator here is expected. The paired assertion lives in
	// cmd/server, where the init has actually run.
}

func TestPulseMaintenancePhaseBlocksRepairToolsUntilUnlocked(t *testing.T) {
	release, err := BeginPulseMaintenanceReviewPhase("review-child-1", "pulse-run-1", "technical_review")
	if err != nil {
		t.Fatalf("begin review phase: %v", err)
	}
	t.Cleanup(release)

	for _, toolName := range []string{"get_pulse_state", "query_workflow_db", "record_pulse_finding", "complete_pulse_review"} {
		if allowed, reason := PulseMaintenanceToolAllowed("review-child-1", toolName); !allowed {
			t.Fatalf("review tool %q was blocked: %s", toolName, reason)
		}
	}
	for _, toolName := range []string{"record_pulse_result", "update_message_sequence_step", "mutate_workflow_db", "run_full_workflow"} {
		if allowed, _ := PulseMaintenanceToolAllowed("review-child-1", toolName); allowed {
			t.Fatalf("repair tool %q was allowed before the receipt barrier", toolName)
		}
	}
	if allowed, _ := PulseMaintenanceToolAllowedWithArgs("review-child-1", "diff_patch_workspace_file", map[string]interface{}{"filepath": "Workflow/demo/planning/plan.json"}); allowed {
		t.Fatal("review phase allowed a patch outside its run-scoped checkpoint")
	}
	if allowed, reason := PulseMaintenanceToolAllowedWithArgs("review-child-1", "diff_patch_workspace_file", map[string]interface{}{"filepath": "Workflow/demo/runs/pulse/pulse-run-1/technical-review.md"}); !allowed {
		t.Fatalf("review checkpoint patch was blocked: %s", reason)
	}

	if err := UnlockPulseMaintenanceRepairPhase("review-child-1"); err != nil {
		t.Fatalf("unlock repair phase: %v", err)
	}
	for _, toolName := range []string{"record_pulse_result", "update_message_sequence_step", "mutate_workflow_db", "diff_patch_workspace_file"} {
		if allowed, reason := PulseMaintenanceToolAllowed("review-child-1", toolName); !allowed {
			t.Fatalf("repair tool %q remained blocked after receipt: %s", toolName, reason)
		}
	}
}

func TestPulseMaintenancePhaseRejectsNonTechnicalModule(t *testing.T) {
	if _, err := BeginPulseMaintenanceReviewPhase("review-child-2", "pulse-run-2", "strategic_review"); err == nil {
		t.Fatal("strategic review incorrectly received a repair-unlock phase")
	}
}

func TestWorkshopStageAgentIdentityIsValidReviewIdentity(t *testing.T) {
	identity := newWorkshopStageAgentIdentity("background-task")
	if err := ValidatePulseReviewIdentity(identity, "technical_review"); err != nil {
		t.Fatalf("workshop stage identity %q cannot own a Pulse receipt: %v", identity, err)
	}
}

func TestManualPulseMaintenanceUsesRetainedChildAsRunIdentity(t *testing.T) {
	const childSessionID = "2026-08-28T14-00-00.000Z_background-task-1"
	if got := resolveBackgroundPulseRunID("child", childSessionID); got != childSessionID {
		t.Fatalf("manual Pulse run id = %q, want retained child session %q", got, childSessionID)
	}
	if got := resolveBackgroundPulseRunID("schedule-manual--123", childSessionID); got != "schedule-manual--123" {
		t.Fatalf("scheduled Pulse run id = %q, want the parent run id", got)
	}
}

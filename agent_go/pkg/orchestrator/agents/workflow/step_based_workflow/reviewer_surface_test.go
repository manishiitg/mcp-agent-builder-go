package step_based_workflow

import (
	"testing"
	"time"
)

func toolSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

// TestReviewerSurfaceCoversEveryToolItsContractNames stops the failure that has
// shipped here twice: guidance instructs an agent to call a tool it was never
// given, and the call fails as if the tool were broken rather than as a wiring
// bug. resolve_run_concern missing from ToolCategories and stores_health
// missing from the reviewer whitelist were both this.
//
// Reviewers are told to reconcile candidates against the tracked backlog, so
// the tools that reconciliation needs must be present.
func TestReviewerSurfaceCoversEveryToolItsContractNames(t *testing.T) {
	surface := toolSet(workshopStageToolAgentToolNames())
	for _, required := range []string{
		"get_pulse_state",
		"get_workflow_command_guidance",
		"execute_shell_command",
	} {
		if !surface[required] {
			t.Fatalf("reviewer contract calls %q but the surface does not include it", required)
		}
	}
	if surface["read_skill"] {
		t.Fatal("reviewer allowlist should not own mcpagent's intrinsic read_skill identity tool")
	}
}

// TestPulseFixerSurfaceCannotImpersonateACompletePass pins what the Fixer is
// deliberately denied. Recording a worklist would let it decide what was due,
// which belongs to Gate, and a fixer holding it could report a complete pass it
// never ran.
func TestPulseFixerSurfaceCannotImpersonateACompletePass(t *testing.T) {
	surface := toolSet(workshopStageToolAgentToolNames())
	for _, denied := range []string{
		"record_pulse_worklist",
	} {
		if surface[denied] {
			t.Fatalf("Pulse Fixer surface includes %q, which lets it impersonate or overreach a scheduled pass", denied)
		}
	}
	for _, required := range []string{
		"record_pulse_result",
		"get_pulse_state",
		"update_step_config",
	} {
		if !surface[required] {
			t.Fatalf("Pulse Fixer surface is missing %q, which its contract requires", required)
		}
	}
}

// TestLifecycleWritersAreOnTheSharedSurface records where the real boundary sits.
//
// The surface is not a security boundary and never was: execute_shell_command is
// registered with no path parameters, so a reviewer could always write files or
// the database through a shell. What separates the roles is delegated Pulse
// write authority, keyed by session id and unreachable by prompt — a reviewer
// calling a lifecycle writer is refused there, not here.
func TestLifecycleWritersAreOnTheSharedSurface(t *testing.T) {
	surface := toolSet(workshopStageToolAgentToolNames())
	for _, name := range []string{"record_pulse_result", "record_pulse_impact", "get_pulse_state"} {
		if !surface[name] {
			t.Fatalf("shared stage surface is missing %q, which the Fixer contract requires", name)
		}
	}
}

// TestDerivedReviewIdentityKeepsTheCallersPulseRun lets a standalone
// /pulse-fixer reach the same fixer stage the scheduled path uses.
//
// A manual fixer already owns a pulse_run_id from begin_pulse_fixer_run and
// must keep writing under it — its authority is bound to that exact run. If
// deriving a review identity replaced that id, every lifecycle write from the
// stage would be refused as a run mismatch.
func TestDerivedReviewIdentityKeepsTheCallersPulseRun(t *testing.T) {
	const owned = "manual-fixer--abc123"
	effective, reviewRunID := newDerivedPulseReviewIdentity(time.Date(2026, 7, 31, 20, 15, 0, 0, time.UTC), owned, "fix-bug-review", "bug_review")
	if effective != owned {
		t.Fatalf("derived identity changed the caller's run id to %q; its authority is bound to %q", effective, owned)
	}
	if reviewRunID == "" || reviewRunID == owned {
		t.Fatalf("review_run_id %q must be its own value, not empty and not the pulse run id", reviewRunID)
	}
}

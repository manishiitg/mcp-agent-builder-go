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
	surface := toolSet(goalAdvisorReadOnlyToolAgentAllowedToolNames())
	for _, required := range []string{
		"get_pulse_module_state",
		"get_pulse_finding_backlog",
		"get_pulse_review_result",
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
// deliberately denied. Recording a worklist would let it decide what was due;
// marking final commands would let it declare a Pulse finished. Both belong to
// Gate and the finalizer, and a fixer holding either could report a complete
// pass it never ran.
func TestPulseFixerSurfaceCannotImpersonateACompletePass(t *testing.T) {
	surface := toolSet(pulseFixerStageToolAgentAllowedToolNames())
	for _, denied := range []string{
		"record_pulse_worklist",
		"mark_pulse_final_command_result",
		"create_plan",
		"delete_plan_steps",
	} {
		if surface[denied] {
			t.Fatalf("Pulse Fixer surface includes %q, which lets it impersonate or overreach a scheduled pass", denied)
		}
	}
	for _, required := range []string{
		"start_pulse_fix_attempt",
		"mark_pulse_module_result",
		"get_pulse_finding_backlog",
		"update_step_config",
	} {
		if !surface[required] {
			t.Fatalf("Pulse Fixer surface is missing %q, which its contract requires", required)
		}
	}
}

// TestOnlyTheFixerHoldsLifecycleWriters records where the real boundary sits.
//
// The reviewer surface is not a security boundary: execute_shell_command is
// registered with no path parameters, so the folder guard cannot constrain it
// and a reviewer could write files or the database through a shell. What
// actually separates the two roles is delegated Pulse write authority, keyed by
// session id and unreachable by prompt.
//
// The lifecycle writers stay out of the reviewer surface anyway, because a
// reviewer holding them could only ever be refused — offering a tool whose every
// call fails is worse than not offering it.
func TestOnlyTheFixerHoldsLifecycleWriters(t *testing.T) {
	reviewer := toolSet(goalAdvisorReadOnlyToolAgentAllowedToolNames())
	fixer := toolSet(pulseFixerStageToolAgentAllowedToolNames())
	for _, writer := range []string{"start_pulse_fix_attempt", "mark_pulse_module_result"} {
		if reviewer[writer] {
			t.Fatalf("reviewer surface holds %q, which it can never be authorized to call", writer)
		}
		if !fixer[writer] {
			t.Fatalf("Fixer surface is missing %q, which its contract requires", writer)
		}
	}
	// Both read the backlog: reconciliation is a reviewer duty, not a fixer one.
	for _, reader := range []string{"get_pulse_module_state", "get_pulse_finding_backlog"} {
		if !reviewer[reader] || !fixer[reader] {
			t.Fatalf("%q must be readable by both roles", reader)
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

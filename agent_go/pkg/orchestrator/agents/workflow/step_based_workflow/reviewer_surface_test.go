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
		"get_pulse_state",
		"record_pulse_finding",
		"record_pulse_verification",
		"complete_pulse_review",
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

// The Fixer is a workflow writer, so it uses the same capability definition as
// Workshop. Keeping a smaller hand-maintained list repeatedly turned ordinary
// repairs into false blockers when Builder gained a tool that Fixer did not.
func TestPulseFixerUsesTheCanonicalWorkshopWriterSurface(t *testing.T) {
	want := GetToolsForWorkshopMode("workshop")
	got := pulseFixerStageToolAgentAllowedToolNames()
	if len(got) != len(want) {
		t.Fatalf("Pulse Fixer has %d tools, Workshop has %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("Pulse Fixer tool[%d] = %q, want Workshop tool %q", index, got[index], want[index])
		}
	}
}

// TestOnlyTheFixerHoldsMutationWriters records where the real boundary sits.
//
// The reviewer surface is not a security boundary: execute_shell_command is
// registered with no path parameters, so the folder guard cannot constrain it
// and a reviewer could write files or the database through a shell. What
// actually separates the two roles is delegated Pulse write authority, keyed by
// session id and unreachable by prompt.
//
// Reviewers hold only the narrow append/finalize tools bound to their exact
// review identity. Mutation/disposition and impact writers remain Fixer-only.
func TestOnlyTheFixerHoldsMutationWriters(t *testing.T) {
	reviewer := toolSet(goalAdvisorReadOnlyToolAgentAllowedToolNames())
	fixer := toolSet(pulseFixerStageToolAgentAllowedToolNames())
	for _, writer := range []string{"record_pulse_result", "record_pulse_impact"} {
		if reviewer[writer] {
			t.Fatalf("reviewer surface holds %q, which it can never be authorized to call", writer)
		}
		if !fixer[writer] {
			t.Fatalf("Fixer surface is missing %q, which its contract requires", writer)
		}
	}
	// Both read the backlog: reconciliation is a reviewer duty, not a fixer one.
	for _, reader := range []string{"get_pulse_state"} {
		if !reviewer[reader] || !fixer[reader] {
			t.Fatalf("%q must be readable by both roles", reader)
		}
	}
}

// TestDerivedReviewIdentityKeepsTheCallersPulseRun covers trusted recovery
// callers that already own a pulse_run_id but need a derived review identity.
// Their authority is bound to that exact run. If
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

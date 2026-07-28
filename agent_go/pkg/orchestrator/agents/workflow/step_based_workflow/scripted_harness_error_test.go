package step_based_workflow

import (
	"strings"
	"testing"
)

// The scripted fast path builds its own HTTP request rather than going through
// workspace.Client, so it must attach the workspace token itself. Without it,
// every scripted run failed before the interpreter started.
func TestScriptedFastPathAttachesWorkspaceToken(t *testing.T) {
	src := readSourceFile(t, "controller_scripted.go")
	idx := strings.Index(src, `apiURL := getWorkspaceAPIURL() + "/api/execute"`)
	if idx < 0 {
		t.Fatal("could not locate the scripted execute request")
	}
	// Look at the request construction that follows, up to the Do() call.
	end := strings.Index(src[idx:], "http.DefaultClient.Do(req)")
	if end < 0 {
		t.Fatal("could not locate the request dispatch")
	}
	block := src[idx : idx+end]
	if !strings.Contains(block, "X-Workspace-Token") {
		t.Fatal("scripted /api/execute request must set X-Workspace-Token; /api/execute is token-protected and this path bypasses workspace.Client.doRequest")
	}
	if !strings.Contains(block, "WORKSPACE_API_TOKEN") {
		t.Fatal("the token must come from WORKSPACE_API_TOKEN, matching workspace.Client")
	}
}

// A rejected request means the script never ran, so it must not reach the
// relearn path. That path hands the agent the source plus the error and tells it
// to "fix the bug and rewrite it", then persists the rewrite — which is how two
// workflows replaced working scripts with workarounds for a bug in the harness.
func TestHarnessRejectionAbortsInsteadOfRelearning(t *testing.T) {
	d := decideScriptedFastPath(&ScriptedFastPathResult{
		HarnessFailure: true,
		HarnessError:   "workspace execution authorization required",
		ExistingScript: "print('working code')",
	})
	if !d.HarnessFailure {
		t.Fatal("a harness rejection must be surfaced as such, not as a script failure")
	}
	if d.PriorError != "" || d.PriorScript != "" {
		t.Fatalf("nothing may be handed to the relearn path: %#v", d)
	}
	if d.FastPathDone {
		t.Fatal("a script that never ran cannot have completed the step")
	}
	if !strings.Contains(d.HarnessError, "authorization required") {
		t.Fatalf("the underlying error must survive for the operator: %q", d.HarnessError)
	}
}

// The ordinary failure path must keep working: a script that genuinely ran and
// failed is exactly what relearn exists for.
func TestGenuineScriptFailureStillRelearns(t *testing.T) {
	d := decideScriptedFastPath(&ScriptedFastPathResult{
		RanScript:      true,
		Success:        false,
		Error:          "KeyError: 'ticker'",
		ExistingScript: "print('broken')",
	})
	if d.HarnessFailure {
		t.Fatal("a real script failure must not be mistaken for a harness failure")
	}
	if d.PriorError == "" || d.PriorScript == "" {
		t.Fatalf("relearn context must still be handed to the LLM: %#v", d)
	}
}

// The step must abort rather than fall through, and say plainly that the script
// is not at fault — this error is what the operator sees in the failure
// notification, and it is the only account of what happened.
func TestHarnessRejectionStepErrorBlamesTheHarness(t *testing.T) {
	src := readSourceFile(t, "controller_execution.go")
	idx := strings.Index(src, "if scriptedDecision.HarnessFailure {")
	if idx < 0 {
		t.Fatal("the harness-failure branch must abort the step before the relearn switch")
	}
	relearn := strings.Index(src, "learnCodePriorError = scriptedDecision.PriorError")
	if relearn < idx {
		t.Fatal("the abort must come before relearn context is assigned")
	}
	block := src[idx:relearn]
	if !strings.Contains(block, "return \"\", updatedContextFiles, fmt.Errorf(") {
		t.Fatalf("the branch must return an error, not fall through:\n%s", block)
	}
	for _, want := range []string{"not a fault in the script", "do not modify main.py", "produced no output"} {
		if !strings.Contains(block, want) {
			t.Fatalf("step error must state %q:\n%s", want, block)
		}
	}
}

// A refused request must not count against the script's record. These counters
// drive needs_review and the argument for unlocking a locked script; charging
// them for runs that never happened would condemn working code.
func TestHarnessRejectionResultCarriesNoRunEvidence(t *testing.T) {
	src := readSourceFile(t, "controller_scripted.go")
	idx := strings.Index(src, "if errors.Is(execErr, ErrScriptedHarnessRejection) {")
	if idx < 0 {
		t.Fatal("harness rejection must be detected in the fast path")
	}
	// Bound to this if-body only. Anything looser runs past the early return into
	// the ordinary failure path, which legitimately validates and records.
	rel := strings.Index(src[idx:], "\n\t}\n")
	if rel < 0 {
		t.Fatal("could not find the end of the rejection branch")
	}
	block := src[idx : idx+rel]
	if strings.Contains(block, "RunPreValidation") {
		t.Fatal("nothing ran, so there is no output to pre-validate")
	}
	if strings.Contains(block, "updateScriptedRunStats") {
		t.Fatal("a refused request must not be recorded against the script's run history")
	}
	if !strings.Contains(block, "HarnessFailure: true") || strings.Contains(block, "RanScript:") {
		t.Fatalf("result must report a non-run, not a failed run:\n%s", block)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

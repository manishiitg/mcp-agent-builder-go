package step_based_workflow

import (
	"strings"
	"testing"
)

// The controller must run a saved script through the same client the agent's own
// execute_shell_command uses. When it hand-rolled its own request it silently
// dropped X-Workspace-Token and X-User-ID, so the agent's self-test passed and
// only the real run failed — leaving the agent to "fix" a script that worked.
func TestScriptedRunGoesThroughTheSharedWorkspaceClient(t *testing.T) {
	src := readSourceFile(t, "controller_scripted.go")
	fn := scriptedExecFunc(t, src)

	if !strings.Contains(fn, "hcpo.WorkspaceClient.ExecuteShellCommand(ctx, reqParams)") {
		t.Fatal("the saved script must be dispatched via workspace.Client.ExecuteShellCommand")
	}
	for _, banned := range []string{"http.DefaultClient", "http.NewRequestWithContext", "X-Workspace-Token"} {
		if strings.Contains(fn, banned) {
			t.Fatalf("execScriptedScript must not build its own request (%q) — that is how the two paths diverged", banned)
		}
	}
}

// A request that never became a process must be reported as a harness rejection,
// whatever shape the failure arrives in.
func TestTransportAndAPIFailuresBothReadAsHarnessRejection(t *testing.T) {
	src := readSourceFile(t, "controller_scripted.go")
	fn := scriptedExecFunc(t, src)
	if strings.Count(fn, "ErrScriptedHarnessRejection") < 3 {
		t.Fatalf("missing client, transport/non-2xx, and API-error cases must all map to ErrScriptedHarnessRejection:\n%s", fn)
	}
	if !strings.Contains(fn, "if hcpo.WorkspaceClient == nil") {
		t.Fatal("a missing client is infrastructure, not a script fault")
	}
}

// scriptedExecFunc returns the body of execScriptedScript. These properties are
// about which code path is taken, which cannot be observed without a live
// workspace service.
func scriptedExecFunc(t *testing.T, src string) string {
	t.Helper()
	start := strings.Index(src, "func (hcpo *StepBasedWorkflowOrchestrator) execScriptedScript(")
	if start < 0 {
		t.Fatal("execScriptedScript not found")
	}
	rel := strings.Index(src[start:], "\n}\n")
	if rel < 0 {
		t.Fatal("could not find the end of execScriptedScript")
	}
	// Strip comments: the prose legitimately names the headers and the old
	// dispatch, and matching those would defeat the point of the check.
	var code []string
	for _, line := range strings.Split(src[start:start+rel], "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		code = append(code, line)
	}
	return strings.Join(code, "\n")
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

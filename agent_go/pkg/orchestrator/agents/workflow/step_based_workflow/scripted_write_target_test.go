package step_based_workflow

import (
	"strings"
	"testing"
)

// PLAT-062. The scripted MODE NOTE told the agent to save its code to
// `learnings/{step-id}/main.py` — a path setupExecutionFolderGuard never puts in
// writePaths. Meanwhile the Code Execution Mode section, appended to the same
// prompt, correctly said to write the run's own code/main.py. Two instructions,
// two paths, nothing checking they agreed.
//
// Observed on hetznerssh 2026-08-09: create-final-security-report repaired its
// script, obeyed the MODE NOTE, was correctly denied, recovered to code/main.py,
// and then filed a concern reporting persistence had failed. It had not — the
// platform saved the script back 42 seconds later, byte-identical. So the cost
// was a wasted turn plus a false finding that consumes a Pulse reviewer slot.
//
// The step had carried learn_code_max_fix_iterations: 0 (a migration artifact
// removed in PLAT-061), so it had never attempted a repair and this contradiction
// had never been exercised.

func TestScriptedModeNoteDoesNotNameAReadOnlyWriteTarget(t *testing.T) {
	// The template is pre-parsed at init; read its source back from the parse tree.
	tmpl := executionOnlyUserTemplate.Tree.Root.String()

	start := strings.Index(tmpl, "**MODE NOTE (scripted)**")
	if start < 0 {
		t.Fatal("scripted MODE NOTE not found in the execution-only template")
	}
	note := tmpl[start:]
	if end := strings.Index(note, "{{end}}"); end > 0 {
		note = note[:end]
	}

	// The write target must be the run's code dir, which the folder guard opens.
	if !strings.Contains(note, "code/main.py") {
		t.Error("MODE NOTE does not name the run's code/main.py as the write target")
	}
	// It may still mention the learnings path — that is where the platform
	// persists to — but only while stating it is not writable by the step.
	if strings.Contains(note, "learnings/{step-id}/main.py") {
		for _, want := range []string{"read-only", "persists"} {
			if !strings.Contains(note, want) {
				t.Errorf("MODE NOTE names learnings/{step-id}/main.py without saying %q — an agent will try to write it and burn a turn on the denial", want)
			}
		}
	}
}

func TestScriptedInstructionsAgreeOnTheWriteTarget(t *testing.T) {
	// The detailed section was always right; this pins that the two halves of the
	// same prompt keep pointing at the same place.
	instructions := GetScriptedModeInstructions(
		"/ws/runs/iteration-0/default/execution/my-step/code",
		"/ws/runs/iteration-0/default/execution/my-step",
		false, "", "", nil, nil, nil, "", false, false, false,
	)
	if !strings.Contains(instructions, "/code/main.py") {
		t.Error("Code Execution Mode section no longer names the code-dir write target")
	}
	if strings.Contains(instructions, "learnings/") && !strings.Contains(instructions, "saved script") {
		t.Error("Code Execution Mode section references learnings/ without explaining the platform saves it")
	}
}

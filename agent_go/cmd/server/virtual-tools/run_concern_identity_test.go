package virtualtools

import (
	"context"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
)

// TestRunConcernRefusalNamesTheDestinationThatExists pins the recovery path.
//
// `record_run_concern` is registered for every workflow-mode session, but it
// files against the step that raised the concern, so a Pulse or maintenance
// session — which has no step identity — can never use it. That refusal is
// correct; a concern filed against an empty step consumes backlog attention
// while pointing nowhere.
//
// What was not correct is refusing without a destination. On 2026-08-17 a
// salesoutreach Pulse pass raised a fully-evidenced finding — the evaluator
// binds to the wrong batch_id, so a stale 20/20 makes an unevaluated run look
// verified — and the refusal discarded it. The same session called
// record_pulse_finding successfully 160 times; it had the right tool and was
// never told to use it.
func TestRunConcernRefusalNamesTheDestinationThatExists(t *testing.T) {
	registry := CreateRunConcernToolRegistry("pulse-session-with-no-step", func(context.Context, string, string, string, string, string, map[string]any) (string, error) {
		t.Fatal("recorder must not run without a trusted step identity")
		return "", nil
	})

	executor := registry.Executors["record_run_concern"]
	if executor == nil {
		t.Fatal("record_run_concern executor missing")
	}

	_, err := executor(context.Background(), map[string]any{"concern": "evaluator grades an older batch"})
	if err == nil {
		t.Fatal("expected a refusal when the session has no step identity")
	}

	// The caller is a model. A refusal it cannot act on silently discards work
	// it has already done, so the message must name where the concern goes.
	if !strings.Contains(err.Error(), "record_pulse_finding") {
		t.Errorf("refusal does not name the destination that exists, so the concern is simply dropped:\n%v", err)
	}
	if !strings.Contains(err.Error(), "Do not discard this concern") {
		t.Errorf("refusal does not tell the caller to preserve the concern:\n%v", err)
	}
}

// TestRunConcernRecordsWhenTheSessionHasAStepIdentity is the positive half:
// the refusal path must not have swallowed the normal one.
func TestRunConcernRecordsWhenTheSessionHasAStepIdentity(t *testing.T) {
	const sessionID = "step-session-with-identity"
	common.SetRunConcernSessionContext(sessionID, common.RunConcernSessionContext{
		WorkspacePath: "Workflow/demo",
		RunFolder:     "iteration-0",
		GroupName:     "default",
		StepID:        "step-7-execute",
		Phase:         "execution",
	})

	var gotStepID, gotPhase string
	registry := CreateRunConcernToolRegistry(sessionID, func(_ context.Context, _, _, _, stepID, phase string, _ map[string]any) (string, error) {
		gotStepID, gotPhase = stepID, phase
		return "recorded", nil
	})
	executor := registry.Executors["record_run_concern"]

	out, err := executor(context.Background(), map[string]any{"concern": "a real step concern"})
	if err != nil {
		t.Fatalf("unexpected refusal for a session with a step identity: %v", err)
	}
	if out != "recorded" {
		t.Errorf("result = %q, want %q", out, "recorded")
	}
	if gotStepID != "step-7-execute" || gotPhase != "execution" {
		t.Errorf("filed against step=%q phase=%q, want step-7-execute/execution", gotStepID, gotPhase)
	}
}

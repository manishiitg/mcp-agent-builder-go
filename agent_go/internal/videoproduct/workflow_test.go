package videoproduct

import (
	"strings"
	"testing"

	stepworkflow "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
)

func TestWorkflowActivityContextUsesWorkflowAndStepTitles(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		args     map[string]interface{}
		workflow string
		step     string
	}{
		{name: "single step", tool: "execute_step", args: map[string]interface{}{"step_id": "infographic-research"}, workflow: "Product explainer / infographic", step: "Brief and evidence"},
		// A full run shows only the workflow name; the arrow form is reserved for a single stage.
		{name: "full workflow", tool: "run_full_workflow", args: map[string]interface{}{}, workflow: "Product explainer / infographic", step: ""},
		{name: "future step fallback", tool: "execute_step", args: map[string]interface{}{"step_id": "custom-review"}, workflow: "Product explainer / infographic", step: "custom-review"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow, step := workflowActivityContext(DefaultPipeline().Name, DefaultPipeline().Steps(), test.tool, test.args)
			if workflow != test.workflow || step != test.step {
				t.Fatalf("context = %q -> %q, want %q -> %q", workflow, step, test.workflow, test.step)
			}
		})
	}

	workflow, step := workflowActivityContext("Social reel", []WorkflowStep{{ID: "hook", Title: "Opening hook"}}, "execute_step", map[string]interface{}{"step_id": "hook"})
	if workflow != "Social reel" || step != "Opening hook" {
		t.Fatalf("future workflow context = %q -> %q", workflow, step)
	}
}

func TestApplyWorkflowHumanInput(t *testing.T) {
	history := []Message{{Role: "assistant", Body: "What should we make?"}, {Role: "user", Body: "Create a calm product launch film."}}

	stepArgs := map[string]interface{}{"step_id": "infographic-research"}
	applyWorkflowHumanInput(DefaultPipeline(), "execute_step", stepArgs, history)
	stepInput, _ := stepArgs["human_input"].(string)
	if !strings.Contains(stepInput, "Assistant: What should we make?") || !strings.Contains(stepInput, "User: Create a calm product launch film.") {
		t.Fatalf("execute_step human_input = %#v", stepArgs["human_input"])
	}

	explicitArgs := map[string]interface{}{"step_id": "infographic-research", "human_input": "Use the approved brief."}
	applyWorkflowHumanInput(DefaultPipeline(), "execute_step", explicitArgs, history)
	if explicitArgs["human_input"] != "Use the approved brief." {
		t.Fatalf("explicit human_input was replaced: %#v", explicitArgs["human_input"])
	}

	fullArgs := map[string]interface{}{"human_inputs": map[string]interface{}{"proposal": "Use option B."}}
	applyWorkflowHumanInput(DefaultPipeline(), "run_full_workflow", fullArgs, history)
	inputs := fullArgs["human_inputs"].(map[string]interface{})
	researchInput, _ := inputs["infographic-research"].(string)
	if !strings.Contains(researchInput, "User: Create a calm product launch film.") || inputs["proposal"] != "Use option B." {
		t.Fatalf("run_full_workflow human_inputs = %#v", inputs)
	}
}

func TestWorkflowCompletionDispatchesSyntheticAgentTurn(t *testing.T) {
	server, client := newTestServer(t)
	user := loginUser(t, client, "manish", "12345")
	project := createProjectForTest(t, client)
	run, err := server.store.BeginWorkflowRun(project.ID, cinematicWorkflowName, "launch-film", copyCinematicSteps())
	if err != nil {
		t.Fatal(err)
	}
	notifications := make(chan workflowAutoNotification, 1)
	notifier := newVideoWorkflowNotifier(server.store, project.ID, DefaultPipeline(), func(notification workflowAutoNotification) {
		notifications <- notification
	})
	notifier.Prepare(run.ID, "execute_step", "launch-film", user.ID)
	notifier.OnExecutionStart(stepworkflow.WorkshopExecutionStart{ID: "exec-research", Name: "internal-research-agent", Metadata: map[string]string{"step_id": "research"}})
	notifier.OnExecutionComplete("exec-research", "internal-research-agent", "Created research.md.\nSTATUS: COMPLETED", map[string]string{"step_id": "research", "step_name": "internal-research-agent"}, nil)

	notification := <-notifications
	if notification.ProjectID != project.ID || notification.UserID != user.ID || notification.RunID != run.ID || notification.FinalStatus != "ready" {
		t.Fatalf("notification context = %+v", notification)
	}
	// Same facts as before, now stated as structured fields rather than prose so
	// the terminal outcome cannot be contradicted by the free-text summary.
	for _, expected := range []string{"[AUTO-NOTIFICATION]", "Stage: Research", "Project group: launch-film", "Outcome: COMPLETED", "Created research.md", "do not mention notification internals"} {
		if !strings.Contains(notification.Message, expected) {
			t.Fatalf("notification missing %q: %s", expected, notification.Message)
		}
	}
	messages, err := server.store.Messages(user.ID, project.ID)
	if err != nil || len(messages) != 0 {
		t.Fatalf("raw notification leaked into visible chat: %+v, err=%v", messages, err)
	}
	runs, err := server.store.WorkflowRuns(user.ID, project.ID)
	if err != nil || len(runs) != 1 || runs[0].Status != "running" || runs[0].Steps[0].Status != "completed" {
		t.Fatalf("run state before synthetic turn = %+v, err=%v", runs, err)
	}
}

// The original failure: a run failed after some steps had succeeded, and the
// workflow's own summary still read "completed successfully". The old
// notification put that blob next to the word "failed" with no precedence, so
// the resumed agent produced a reply asserting both.
func TestBuildWorkflowNotificationFailureOverridesContradictorySummary(t *testing.T) {
	msg := buildWorkflowNotification(workflowNotificationFacts{
		Label:   "Compose",
		Group:   "coffee-shop-teaser",
		Status:  "failed",
		StepID:  "compose",
		RunRoot: "projects/p1/runs/iteration-0/coffee-shop-teaser",
		Result:  "All steps completed successfully.",
	})
	for _, want := range []string{"DID NOT COMPLETE", "Failed at stage: compose", "the outcome above wins", "projects/p1/runs/iteration-0/coffee-shop-teaser/execution/"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("notification missing %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "Outcome: COMPLETED") {
		t.Fatalf("failed run must not report a completed outcome:\n%s", msg)
	}
}

func TestBuildWorkflowNotificationCompletedRun(t *testing.T) {
	msg := buildWorkflowNotification(workflowNotificationFacts{
		Label: "Research", Group: "g1", Status: "completed", StepID: "research",
	})
	if !strings.Contains(msg, "Outcome: COMPLETED") || strings.Contains(msg, "DID NOT COMPLETE") {
		t.Fatalf("completed run rendered wrong:\n%s", msg)
	}
	// No run root known — must not emit a dangling "artifacts live under" line.
	if strings.Contains(msg, "Artifacts for this run live ONLY under") {
		t.Fatalf("emitted artifact path with no run folder:\n%s", msg)
	}
}

func TestWorkflowRunRootUsesRunFolderMeta(t *testing.T) {
	if got := workflowRunRoot("p1", map[string]string{"run_folder": "iteration-0/g1"}); got != "projects/p1/runs/iteration-0/g1" {
		t.Fatalf("run root = %q", got)
	}
	if got := workflowRunRoot("p1", map[string]string{}); got != "" {
		t.Fatalf("missing run_folder should yield empty root, got %q", got)
	}
	if got := workflowRunRoot("p1", nil); got != "" {
		t.Fatalf("nil meta should yield empty root, got %q", got)
	}
}

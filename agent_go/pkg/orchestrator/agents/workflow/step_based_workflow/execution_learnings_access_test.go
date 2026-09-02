package step_based_workflow

import "testing"

func TestResolveExecutionLearningsAccessMatchesPromptGate(t *testing.T) {
	config := &AgentConfigs{LearningsAccess: LearningsAccessReadWrite, LearningObjective: "Capture reusable execution mechanics."}

	tests := []struct {
		name     string
		step     PlanStepInterface
		evalMode bool
		want     string
	}{
		{name: "regular", step: &RegularPlanStep{}, want: LearningsAccessReadWrite},
		{name: "message sequence", step: &MessageSequencePlanStep{}, want: LearningsAccessReadWrite},
		{name: "todo orchestrator", step: &OrchestratorPlanStep{}, want: LearningsAccessReadWrite},
		{name: "evaluation", step: &EvaluationStep{}, evalMode: true, want: LearningsAccessNone},
		{name: "deterministic routing", step: &RoutingPlanStep{}, want: LearningsAccessNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveExecutionLearningsAccess(config, tt.step, tt.evalMode); got != tt.want {
				t.Fatalf("access = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveExecutionLearningsAccessHonorsExplicitNone(t *testing.T) {
	config := &AgentConfigs{LearningsAccess: LearningsAccessNone}
	if got := resolveExecutionLearningsAccess(config, &RegularPlanStep{}, false); got != LearningsAccessNone {
		t.Fatalf("explicit none resolved to %q", got)
	}
}

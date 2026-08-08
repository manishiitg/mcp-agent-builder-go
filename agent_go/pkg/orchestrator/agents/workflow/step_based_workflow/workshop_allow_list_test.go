package step_based_workflow

import "testing"

// Every stage agent must be able to discover its own tool surface.
//
// The allow-list gates the code-execution bridge as well, and it is checked
// before get_api_spec reaches its virtual-tool partition — so a list that omits
// it leaves the agent unable to find out what it has. A denial then cannot be
// self-corrected: on 2026-08-04 a Pulse Fixer denied update_schedule had no way
// to check, and concluded its session was in the wrong workshop mode.
func TestStageToolAgentsCanDiscoverTheirOwnToolSurface(t *testing.T) {
	surfaces := map[string][]string{"workshopStage": workshopStageToolAgentToolNames()}
	for name, tools := range surfaces {
		if !containsToolName(tools, "get_api_spec") {
			t.Errorf("%s cannot call get_api_spec, so it cannot discover which tools it has", name)
		}
	}
}

// The Pulse Fixer's practices doc gives it a "Scheduler and lifecycle repair"
// section. An instruction the grant does not cover is an instruction the agent
// burns calls failing to follow.
func TestPulseFixerCanActOnItsSchedulerRepairInstructions(t *testing.T) {
	tools := workshopStageToolAgentToolNames()
	for _, tool := range []string{"list_schedules", "get_schedule_runs", "update_schedule", "trigger_schedule"} {
		if !containsToolName(tools, tool) {
			t.Errorf("pulse-fixer-practices.md instructs scheduler repair but %q is withheld", tool)
		}
	}
}

func containsToolName(tools []string, want string) bool {
	for _, tool := range tools {
		if tool == want {
			return true
		}
	}
	return false
}

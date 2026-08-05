package step_based_workflow

import (
	"strings"
	"testing"
)

// A tool the prompts tell an agent to call must be in the mode's allow-list.
//
// list_llm_capabilities was registered in multiagent_llm_tools.go, described to
// the agent, and named by llm-selection.md — but absent here, so every call was
// rejected as "not available in the current workshop mode". Registration and the
// allow-list live in different files, so nothing connected them and they drifted.
//
// The rejection also cannot teach the agent anything: "not available in the
// current workshop mode" implies some other mode would work, so it has no reason
// to stop asking.
func TestWorkshopModeGrantsToolsThePromptsInstructAgentsToCall(t *testing.T) {
	allowed := make(map[string]bool)
	for _, tool := range GetToolsForWorkshopMode("workshop") {
		allowed[tool] = true
	}

	// Read-only discovery tools an agent is told to use before making a choice.
	// Withholding any of these makes the instruction unfollowable.
	for _, tool := range []string{
		"list_llm_capabilities",
		"get_api_spec",
		"query_workflow_db",
	} {
		if !allowed[tool] {
			t.Errorf("%q is registered and named by the prompts but missing from the workshop allow-list; "+
				"calls to it are rejected as \"not available in the current workshop mode\"", tool)
		}
	}
}

// The allow-list is only meaningful if it actually withholds something.
func TestWorkshopModeAllowListIsNotEmptyOrUnbounded(t *testing.T) {
	tools := GetToolsForWorkshopMode("workshop")
	if len(tools) == 0 {
		t.Fatal("workshop mode grants no tools")
	}
	seen := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool) == "" {
			t.Error("allow-list contains an empty tool name")
		}
		if seen[tool] {
			t.Errorf("allow-list lists %q more than once", tool)
		}
		seen[tool] = true
	}
}

// Every stage agent must be able to discover its own tool surface.
//
// The allow-list gates the code-execution bridge as well, and it is checked
// before get_api_spec reaches its virtual-tool partition — so a list that omits
// it leaves the agent unable to find out what it has. A denial then cannot be
// self-corrected: on 2026-08-04 a Pulse Fixer denied update_schedule had no way
// to check, and concluded its session was in the wrong workshop mode.
func TestStageToolAgentsCanDiscoverTheirOwnToolSurface(t *testing.T) {
	surfaces := map[string][]string{
		"goalAdvisorCommonMutation":    goalAdvisorCommonMutationToolAgentAllowedToolNames(),
		"goalAdvisorFinalizerProposal": goalAdvisorFinalizerProposalToolAgentAllowedToolNames(),
		"goalAdvisorFinalizerApproved": goalAdvisorFinalizerApprovedToolAgentAllowedToolNames(),
		"goalAdvisorReadOnly":          goalAdvisorReadOnlyToolAgentAllowedToolNames(),
		"pulseFixerStage":              pulseFixerStageToolAgentAllowedToolNames(),
	}
	for name, tools := range surfaces {
		if !containsToolName(tools, "get_api_spec") {
			t.Errorf("%s cannot call get_api_spec, so it cannot discover which tools it has", name)
		}
	}
}

// The Fixer uses the Workshop writer profile, including every schedule repair
// tool. Product/lifecycle rules govern when it may use them; a second capability
// subset would only recreate the drift this invariant prevents.
func TestPulseFixerCanActOnAllWorkshopRepairInstructions(t *testing.T) {
	tools := pulseFixerStageToolAgentAllowedToolNames()
	for _, tool := range []string{
		"create_plan", "add_scripted_step", "delete_plan_steps",
		"create_schedule", "delete_schedule", "trigger_schedule",
		"install_skill", "set_workflow_llm_config", "run_full_evaluation",
		"mutate_workflow_db", "record_pulse_result",
	} {
		if !containsToolName(tools, tool) {
			t.Errorf("Workshop grants %q but the Pulse Fixer withholds it", tool)
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

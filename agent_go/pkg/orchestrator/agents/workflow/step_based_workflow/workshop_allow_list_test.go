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
		"create_workflow_database_snapshot",
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

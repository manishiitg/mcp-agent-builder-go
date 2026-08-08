package step_based_workflow

import (
	"slices"
	"testing"
)

// read_skill must NOT appear in any builder allow-list, and this is easy to get
// wrong in the direction of adding it.
//
// The reasoning that fails: prompts tell every agent to load reference docs with
// read_skill, get_reference_doc no longer exists, and a tool policy also gates the
// code-execution registry — so an unlisted tool is refused over the bridge. That
// is exactly why the stage tool-agent surfaces below list get_api_spec. By analogy
// read_skill looks like it needs listing too.
//
// It does not. mcpagent injects it itself, per turn, in turn_session.go:
//
//	allowed := policy.allowedMap()
//	if allowed != nil && s.agent.isIntrinsicIdentityTool(readSkillToolName) {
//	    allowed[readSkillToolName] = true
//	}
//
// So read_skill is reachable over the bridge with no builder grant at all, and a
// builder list that claims it is asserting ownership of something mcpagent
// already guarantees — two sources for one fact, which is the failure mode this
// package has repeatedly hit. get_api_spec has no such injection, which is why
// the analogy breaks.
func TestBuilderAllowListsDoNotClaimIntrinsicSkillAccess(t *testing.T) {
	for name, list := range stageAgentAllowLists() {
		if slices.Contains(list, "read_skill") {
			t.Errorf("%s allow-lists read_skill; mcpagent injects it per turn (turn_session.go) and the builder must not own that grant", name)
		}
	}
}

// Reading workflow data is symmetric across stage agents: any agent asked to
// reason about the backlog must be able to query it.
func TestEveryStageAgentCanQueryTheWorkflowDatabase(t *testing.T) {
	for name, list := range stageAgentAllowLists() {
		if !slices.Contains(list, "query_workflow_db") {
			t.Errorf("%s cannot call query_workflow_db; it is asked to reason about data it cannot read", name)
		}
	}
}

// Writing is no longer asymmetric. The Pulse Fixer is still the single writer
// for a pass, but that is enforced by session-keyed Pulse write authority, not
// by withholding the tool: a reviewer holding execute_shell_command could always
// reach the database through sqlite3 regardless of this list.
func TestEveryStageAgentCanMutateTheWorkflowDatabase(t *testing.T) {
	for name, list := range stageAgentAllowLists() {
		if !slices.Contains(list, "mutate_workflow_db") {
			t.Errorf("%s cannot call mutate_workflow_db", name)
		}
	}
}

// A duplicate is harmless at runtime but means two places believe they own the
// same grant, which is how these lists drift apart.
func TestAllowListsHaveNoDuplicates(t *testing.T) {
	for name, list := range stageAgentAllowLists() {
		seen := map[string]bool{}
		for _, tool := range list {
			if seen[tool] {
				t.Errorf("%s lists %q twice", name, tool)
			}
			seen[tool] = true
		}
	}
}

func stageAgentAllowLists() map[string][]string {
	return map[string][]string{"workshopStage": workshopStageToolAgentToolNames()}
}

package step_based_workflow

import (
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/guidance"
)

// The four docs asserted here predate PLAT-124 and are exactly the set the
// capability-derived selection produces for a step that holds a browser, the
// workflow DB, the MCP bridge, and authors main.py. The signals below are what
// that step's session reports; the expectation itself is unchanged.
func TestWorkflowReferenceSkillContainsExecutionDocsForEveryTransport(t *testing.T) {
	skill := workflowReferenceSkill(guidance.StepExecutionSignals{
		ToolNames:         []string{"agent_browser", "query_workflow_db", "mutate_workflow_db"},
		CodeExecutionMode: true,
		ScriptedStep:      true,
	})
	if skill == nil {
		t.Fatal("coding CLI step must receive the workflow reference skill")
	}
	if skill.Name != "builder-reference" {
		t.Fatalf("projected skill name = %q, want builder-reference", skill.Name)
	}

	wantFiles := map[string]bool{
		"references/code-authoring.md": false,
		"references/browser-usage.md":  false,
		"references/mcp-bridge.md":     false,
		"references/stores.md":         false,
	}
	for _, file := range skill.SupportingFiles {
		if _, ok := wantFiles[file.RelPath]; ok {
			wantFiles[file.RelPath] = true
		}
	}
	for file, found := range wantFiles {
		if !found {
			t.Errorf("projected workflow reference is missing %s", file)
		}
	}

}

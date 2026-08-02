package step_based_workflow

import "testing"

func TestWorkflowReferenceSkillContainsExecutionDocsForEveryTransport(t *testing.T) {
	skill := workflowReferenceSkill()
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

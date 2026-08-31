package step_based_workflow

import (
	"encoding/json"
	"testing"
)

func TestDefaultRunRetentionCountIsTen(t *testing.T) {
	if defaultRunRetentionCount != 10 {
		t.Fatalf("defaultRunRetentionCount = %d, want 10", defaultRunRetentionCount)
	}
}

func TestIsPartialGroupRun(t *testing.T) {
	hcpo := &StepBasedWorkflowOrchestrator{
		variablesManifest: &VariablesManifest{Groups: []VariableGroup{
			{Name: "instagram", Enabled: true},
			{Name: "linkedin", Enabled: true},
		}},
	}

	hcpo.executionOptions = &ExecutionOptions{EnabledGroupNames: []string{"instagram"}}
	if !hcpo.isPartialGroupRun() {
		t.Fatal("one of two enabled groups should reuse the current run")
	}

	hcpo.executionOptions.EnabledGroupNames = []string{"instagram", "linkedin"}
	if hcpo.isPartialGroupRun() {
		t.Fatal("all enabled groups should rotate the current run")
	}
}

func TestRetainedIterationNamesExcludesActiveAndSortsNumerically(t *testing.T) {
	got := retainedIterationNames([]string{"iteration-12", "draft", "iteration-0", "iteration-3", "iteration-2"})
	want := []string{"iteration-2", "iteration-3", "iteration-12"}
	if len(got) != len(want) {
		t.Fatalf("retained iterations = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("retained iterations = %v, want %v", got, want)
		}
	}
}

func TestPlanRevisionForFilesIsCanonicalAndChangesWithContract(t *testing.T) {
	leftPlan, err := canonicalJSONDocument(`{"steps":[{"id":"one","title":"First"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	rightPlan, err := canonicalJSONDocument(`{ "steps": [ { "title": "First", "id": "one" } ] }`)
	if err != nil {
		t.Fatal(err)
	}
	leftID, _, err := planRevisionForFiles(map[string]interface{}{"planning/plan.json": leftPlan})
	if err != nil {
		t.Fatal(err)
	}
	rightID, payload, err := planRevisionForFiles(map[string]interface{}{"planning/plan.json": rightPlan})
	if err != nil {
		t.Fatal(err)
	}
	if leftID != rightID {
		t.Fatalf("semantically identical JSON produced revisions %q and %q", leftID, rightID)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("revision payload is invalid JSON: %v", err)
	}

	changed, _ := canonicalJSONDocument(`{"steps":[{"id":"one","title":"Changed"}]}`)
	changedID, _, err := planRevisionForFiles(map[string]interface{}{"planning/plan.json": changed})
	if err != nil {
		t.Fatal(err)
	}
	if changedID == leftID {
		t.Fatal("behavioral plan change did not change the revision")
	}
}

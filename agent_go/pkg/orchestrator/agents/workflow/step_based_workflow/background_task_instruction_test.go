package step_based_workflow

import "testing"

func TestBackgroundTaskInstructionRejectsMissingPayload(t *testing.T) {
	for _, value := range []interface{}{nil, "", " \n\t", "null", " null\n", "NULL", 42, map[string]interface{}{"guidance": "review"}} {
		if _, err := parseBackgroundTaskInstruction(map[string]interface{}{"instruction": value}); err == nil {
			t.Errorf("accepted unusable instruction %#v", value)
		}
	}
	if _, err := parseBackgroundTaskInstruction(nil); err == nil {
		t.Error("accepted missing instruction")
	}
}

func TestBackgroundTaskInstructionPreservesActualTask(t *testing.T) {
	for _, task := range []string{
		"Review the changed step and report affected dependencies.",
		"Inspect why a task payload became null; do not edit the workflow.",
		"\nReview the evidence.\nReturn findings.\n",
	} {
		got, err := parseBackgroundTaskInstruction(map[string]interface{}{"instruction": task})
		if err != nil || got != task {
			t.Errorf("instruction changed or rejected: got %q, err %v", got, err)
		}
	}
}

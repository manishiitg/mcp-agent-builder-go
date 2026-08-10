package step_based_workflow

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeScriptedSuccessCountsRepairsCapturedAgenticAttribution(t *testing.T) {
	// Captured from build-in-public/step-route-selector after four successful
	// scripted_fast_path executions. The runtime had attributed all four to
	// "agentic", even though no agentic fallback ran.
	var metadata ScriptedMetadata
	if err := json.Unmarshal([]byte(`{
  "step_id":"step-route-selector",
  "script_version":5,
  "total_runs":4,
  "successful_runs":{"agentic":4},
  "failed_runs":0,
  "relearn_count":4
}`), &metadata); err != nil {
		t.Fatal(err)
	}
	normalizeScriptedSuccessCounts(&metadata)
	want := map[string]int{"scripted": 4}
	if !reflect.DeepEqual(metadata.SuccessfulRuns, want) {
		t.Fatalf("successful_runs = %#v, want %#v", metadata.SuccessfulRuns, want)
	}
}

func TestNormalizeScriptedSuccessCountsDoesNotDoubleCountMirroredLegacyKeys(t *testing.T) {
	metadata := ScriptedMetadata{
		TotalRuns:      9,
		FailedRuns:     0,
		SuccessfulRuns: map[string]int{"agentic": 9, "scripted": 9},
	}
	normalizeScriptedSuccessCounts(&metadata)
	if got := metadata.SuccessfulRuns["scripted"]; got != 9 {
		t.Fatalf("scripted successes = %d, want 9", got)
	}
}

func TestScriptArtifactsChangedUsesBytesNotRewriteAttempt(t *testing.T) {
	captured := map[string]string{"main.py": "print('same')\n"}
	if scriptArtifactsChanged(captured, map[string]string{"main.py": "print('same')\n"}) {
		t.Fatal("byte-identical script was classified as a new version")
	}
	if !scriptArtifactsChanged(captured, map[string]string{"main.py": "print('changed')\n"}) {
		t.Fatal("changed script was not classified as a new version")
	}
	if !scriptArtifactsChanged(captured, map[string]string{"main.py": "print('same')\n", "helper.py": "pass\n"}) {
		t.Fatal("added helper file was not classified as a new version")
	}
}

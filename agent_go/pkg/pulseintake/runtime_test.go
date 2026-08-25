package pulseintake

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckRuntimeFindsOnlyStructuredStatusDisagreements(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	run := filepath.Join(root, "Workflow", "demo", "runs", "iteration-1", "default")
	writeRuntimeFixture(t, filepath.Join(run, "run_metadata.json"), `{"status":"completed"}`)
	writeRuntimeFixture(t, filepath.Join(run, "logs", "step-a", "execution", "execution-attempt-1-iteration-1-timing.json"), `{
  "step_id":"step-a",
  "llm":{"errored_count":1,"canceled_count":0},
  "tools":{"errored_count":0,"calls":[
    {"tool_name":"shell","status":"success","result":"{\"exit_code\":2}"},
    {"tool_name":"normal","status":"success","result":{"stdout":"the word error is ordinary prose"}}
  ]}
}`)

	got := CheckRuntime("Workflow/demo", time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))
	if got.CoverageStatus != CoverageVerified || got.RunsInspected != 1 {
		t.Fatalf("coverage/result = %+v", got)
	}
	if len(got.Findings) != 2 {
		t.Fatalf("findings = %+v, want two structured signals", got.Findings)
	}
	if got.Findings[0].Kind != "runtime_status_disagreement" || got.Findings[1].Kind != "tool_success_with_structured_failure" {
		t.Fatalf("finding kinds = %+v", got.Findings)
	}
}

func TestCheckRuntimeFlagsExplicitNonCompletion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	writeRuntimeFixture(t, filepath.Join(root, "Workflow", "demo", "runs", "iteration-1", "default", "run_metadata.json"), `{"status":"failed"}`)
	got := CheckRuntime("Workflow/demo", time.Now())
	if len(got.Findings) != 1 || got.Findings[0].Kind != "run_not_completed" {
		t.Fatalf("findings = %+v", got.Findings)
	}
}

func TestCheckRuntimeDoesNotTreatMissingRunsAsClean(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	got := CheckRuntime("Workflow/demo", time.Now())
	if got.CoverageStatus != CoverageNotInstrumented || len(got.Findings) != 0 {
		t.Fatalf("result = %+v", got)
	}
}

func writeRuntimeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

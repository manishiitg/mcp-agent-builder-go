package server

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLoadWorkflowTimingFilesAggregatesAgentTimeByDateAndStep(t *testing.T) {
	root := t.TempDir()
	timingDir := filepath.Join(root, "iteration-0", "default", "logs", "collect", "execution")
	if err := os.MkdirAll(timingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTiming := func(name, started string, wall, llm, tools int64) {
		t.Helper()
		content := []byte(`{
  "step_id": "collect",
  "agent": {"started_at": "` + started + `", "duration_ms": 1},
  "breakdown": {"wall_duration_ms": ` + strconv.FormatInt(wall, 10) + `, "llm_duration_ms": ` + strconv.FormatInt(llm, 10) + `, "tool_duration_ms": ` + strconv.FormatInt(tools, 10) + `}
}`)
		if err := os.WriteFile(filepath.Join(timingDir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeTiming("execution-attempt-1-timing.json", "2026-08-12T01:17:21Z", 1000, 700, 200)
	writeTiming("reflection-timing.json", "2026-08-12T01:20:21Z", 500, 450, 25)

	summary := newWorkflowActivityTimingSummary()
	loadWorkflowTimingFiles(&summary, root, "workflow_execution", "step:")

	scope := summary.ByScope["workflow_execution"]
	if scope == nil || scope.DurationMS != 1500 || scope.LLMDurationMS != 1150 || scope.ToolDurationMS != 225 {
		t.Fatalf("scope timing = %#v", scope)
	}
	step := scope.ByExecution["step:collect"]
	if step == nil || step.DurationMS != 1500 {
		t.Fatalf("step timing = %#v", step)
	}
	day := summary.ByDate["2026-08-12"]
	if day == nil || day.ByScope["workflow_execution"].DurationMS != 1500 {
		t.Fatalf("daily timing = %#v", day)
	}
}

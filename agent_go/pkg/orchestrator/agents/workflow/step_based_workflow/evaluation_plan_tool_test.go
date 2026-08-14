package step_based_workflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

func evalPlanHarness(t *testing.T, plan string) (string, map[string]string, func(context.Context, string) (string, error), func(context.Context, string, string) error) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	files := map[string]string{workspacePath + "/" + evaluationPlanRelPath: plan}
	read := func(_ context.Context, path string) (string, error) { return files[path], nil }
	// The changelog writer goes through writeFile, but CollectPlanChangeBacklog
	// reads planning/changelog from disk — so this has to land in both.
	write := func(_ context.Context, path, content string) error {
		files[path] = content
		abs := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		return os.WriteFile(abs, []byte(content), 0o644)
	}
	return workspacePath, files, read, write
}

// TestUpdateEvaluationPlanPreservesFieldsTheStructDoesNotModel is the reason
// this tool edits decoded JSON instead of EvaluationStep.
//
// The struct has no max_score and no context_dependencies, but real plans carry
// both on every step. A read-modify-write through the struct would delete them
// while producing valid JSON — and max_score is the subject of four of
// social-media's open findings, so the tool would have deepened the problem it
// exists to fix.
func TestUpdateEvaluationPlanPreservesFieldsTheStructDoesNotModel(t *testing.T) {
	plan := `{"steps":[
		{"id":"eval-a","title":"A","description":"old","max_score":10,
		 "context_dependencies":["runs/latest"],"context_output":"a.json",
		 "applies_to_routes":[{"routing_step_id":"route-step","route_ids":["r1"]}],
		 "some_future_field":{"kept":true}},
		{"id":"eval-b","title":"B","description":"b","max_score":4}
	]}`
	workspacePath, files, read, write := evalPlanHarness(t, plan)

	out, err := UpdateEvaluationPlanStep(context.Background(), workspacePath, "eval-a",
		map[string]interface{}{"description": "new"}, "Clarify the rubric.", read, write, loggerv2.NewNoop())
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out, "eval-a") {
		t.Fatalf("result did not name the step: %s", out)
	}

	var document map[string]interface{}
	if err := json.Unmarshal([]byte(files[workspacePath+"/"+evaluationPlanRelPath]), &document); err != nil {
		t.Fatalf("written plan does not parse: %v", err)
	}
	steps := document["steps"].([]interface{})
	if len(steps) != 2 {
		t.Fatalf("step count changed to %d", len(steps))
	}
	edited := steps[0].(map[string]interface{})
	if edited["description"] != "new" {
		t.Fatalf("edit did not apply: %v", edited["description"])
	}
	for _, field := range []string{"max_score", "context_dependencies", "applies_to_routes", "some_future_field"} {
		if _, ok := edited[field]; !ok {
			t.Fatalf("%q was destroyed by the write; the struct does not model it and a round-trip would drop it", field)
		}
	}
	if edited["max_score"].(float64) != 10 {
		t.Fatalf("max_score changed to %v", edited["max_score"])
	}
	// Per-route gating must survive intact.
	routes := edited["applies_to_routes"].([]interface{})
	if len(routes) != 1 || routes[0].(map[string]interface{})["routing_step_id"] != "route-step" {
		t.Fatalf("applies_to_routes was not preserved: %#v", routes)
	}
	// An untouched step must be byte-identical in content.
	if other := steps[1].(map[string]interface{}); other["max_score"].(float64) != 4 || other["title"] != "B" {
		t.Fatalf("an unedited step was modified: %#v", other)
	}
}

func TestUpdateEvaluationPlanRequiresReasonAndKnownFields(t *testing.T) {
	workspacePath, _, read, write := evalPlanHarness(t, `{"steps":[{"id":"eval-a","title":"A"}]}`)
	ctx := context.Background()
	logger := loggerv2.NewNoop()

	if _, err := UpdateEvaluationPlanStep(ctx, workspacePath, "eval-a",
		map[string]interface{}{"title": "B"}, "", read, write, logger); err == nil ||
		!strings.Contains(err.Error(), "reason is required") {
		t.Fatalf("a change with no rationale was accepted: %v", err)
	}
	// Silently ignoring an unknown field would let an agent believe it made a
	// change that never happened.
	if _, err := UpdateEvaluationPlanStep(ctx, workspacePath, "eval-a",
		map[string]interface{}{"scoring_mode": "strict"}, "why", read, write, logger); err == nil ||
		!strings.Contains(err.Error(), "cannot set scoring_mode") {
		t.Fatalf("unknown field was not rejected: %v", err)
	}
	if _, err := UpdateEvaluationPlanStep(ctx, workspacePath, "missing",
		map[string]interface{}{"title": "B"}, "why", read, write, logger); err == nil ||
		!strings.Contains(err.Error(), "no evaluation step") {
		t.Fatalf("unknown step id was not rejected: %v", err)
	}
}

// TestUpdateEvaluationPlanRecordsWhatAndWhy covers the gap that made
// AR-20260729-2 unclosable: the changelog is what drift review reads, and a
// record without the rationale and the field-level diff cannot be reviewed.
func TestUpdateEvaluationPlanRecordsWhatAndWhy(t *testing.T) {
	workspacePath, _, read, write := evalPlanHarness(t,
		`{"steps":[{"id":"eval-a","title":"A","max_score":2}]}`)

	if _, err := UpdateEvaluationPlanStep(context.Background(), workspacePath, "eval-a",
		map[string]interface{}{"max_score": 10}, "Two-point scale hid partial progress.",
		read, write, loggerv2.NewNoop()); err != nil {
		t.Fatalf("update: %v", err)
	}

	backlog := CollectPlanChangeBacklog(workspacePath)
	if backlog == nil || len(backlog.Changes) == 0 {
		t.Fatal("the eval-plan edit produced no changelog entry, so drift review would never see it")
	}
	var found bool
	for _, change := range backlog.Changes {
		if strings.Contains(change.Reason, "Two-point scale") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the changelog entry lost its rationale: %+v", backlog.Changes)
	}
}

// TestUpdateEvaluationPlanIsQuietWhenNothingChanged keeps re-asserting a current
// value from filling the changelog with entries that bury the real ones.
func TestUpdateEvaluationPlanIsQuietWhenNothingChanged(t *testing.T) {
	workspacePath, _, read, write := evalPlanHarness(t,
		`{"steps":[{"id":"eval-a","title":"A"}]}`)

	out, err := UpdateEvaluationPlanStep(context.Background(), workspacePath, "eval-a",
		map[string]interface{}{"title": "A"}, "no-op", read, write, loggerv2.NewNoop())
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out, "No change") {
		t.Fatalf("a no-op edit was reported as a change: %s", out)
	}
	if backlog := CollectPlanChangeBacklog(workspacePath); backlog != nil && len(backlog.Changes) > 0 {
		t.Fatalf("a no-op edit wrote %d changelog entries", len(backlog.Changes))
	}
}

// TestWorkshopBlocksDirectWritesToTheEvaluationPlan pins the protection that
// makes update_evaluation_plan the only way to change the file.
//
// planning/ is safe by never being a write path, so plan.json has always been
// tool-only. evaluation/ must stay writable for runs/, so the plan file inside
// it needs an explicit write deny — without one the tool is merely the polite
// option and a direct write still leaves no changelog entry.
func TestWorkshopBlocksDirectWritesToTheEvaluationPlan(t *testing.T) {
	workspacePath := "Workflow/example"
	blocked := workshopBlockedWritePaths(workspacePath, workshopWritePaths(workspacePath))

	want := workspacePath + "/" + evaluationPlanRelPath
	var found bool
	for _, path := range blocked {
		if path == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("evaluation_plan.json is not write-denied for a workshop session: %v", blocked)
	}

	// The rest of evaluation/ must stay writable: eval runs write runs/ on every
	// execution, and update_step_config writes step_config.json.
	for _, mustNotBlock := range []string{
		workspacePath + "/evaluation/runs",
		workspacePath + "/evaluation/step_config.json",
	} {
		for _, path := range blocked {
			if strings.HasPrefix(mustNotBlock, path) {
				t.Fatalf("%q is denied by block %q; only the plan file may be denied", mustNotBlock, path)
			}
		}
	}
}

// TestWorkshopClaimsNoProtectionItDoesNotProvide keeps the deny list honest for
// sessions that could not write evaluation/ anyway.
func TestWorkshopClaimsNoProtectionItDoesNotProvide(t *testing.T) {
	readOnlySession := workshopBlockedWritePaths("Workflow/example", []string{})
	if len(readOnlySession) != 0 {
		t.Fatalf("a session with no write paths reported denies it is not enforcing: %v", readOnlySession)
	}
	if got := workshopBlockedWritePaths("", workshopWritePaths("Workflow/example")); len(got) != 0 {
		t.Fatalf("an empty workspace produced deny paths: %v", got)
	}
}

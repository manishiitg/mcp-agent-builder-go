package step_based_workflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

func TestValidateStepDriftChecksRejectsEmptySlice(t *testing.T) {
	if err := validateStepDriftChecks(nil); err == nil {
		t.Fatal("expected error for empty checks slice")
	}
}

func TestValidateStepDriftChecksRejectsMissingCheckID(t *testing.T) {
	err := validateStepDriftChecks([]StepDriftCheck{
		{CheckID: "", Status: "pass", Evidence: "a real evidence sentence here"},
	})
	if err == nil {
		t.Fatal("expected error for missing check_id")
	}
}

func TestValidateStepDriftChecksRejectsInvalidStatus(t *testing.T) {
	err := validateStepDriftChecks([]StepDriftCheck{
		{CheckID: "x", Status: "maybe", Evidence: "a real evidence sentence here"},
	})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestValidateStepDriftChecksRejectsPlaceholderEvidence(t *testing.T) {
	for _, placeholder := range []string{"", "ok", "fine", "n/a", "looks good"} {
		err := validateStepDriftChecks([]StepDriftCheck{
			{CheckID: "x", Status: "pass", Evidence: placeholder},
		})
		if err == nil {
			t.Fatalf("expected error for placeholder evidence %q", placeholder)
		}
	}
}

func TestValidateStepDriftChecksAcceptsRealEvidence(t *testing.T) {
	err := validateStepDriftChecks([]StepDriftCheck{
		{CheckID: "report_query_compatibility", Status: "pass", Evidence: "all 3 report queries ran cleanly against the current schema"},
		{CheckID: "step_description_accuracy", Status: "fail", Evidence: "description says it reads leads; step only reads campaigns"},
	})
	if err != nil {
		t.Fatalf("expected valid checks to pass, got: %v", err)
	}
}

func newPlanDriftReviewTestExecutor(files map[string]string) func(context.Context, map[string]interface{}) (string, error) {
	readFile := func(_ context.Context, path string) (string, error) {
		return files[path], nil
	}
	writeFile := func(_ context.Context, path, content string) error {
		files[path] = content
		return nil
	}
	return createRecordPlanDriftReviewExecutor("", loggerv2.NewNoop(), readFile, writeFile)
}

func TestRecordPlanDriftReviewExecutorWritesNewRecord(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{
		"planning/step_config.json": `{"steps":[{"id":"step-a"}]}`,
	}
	executor := newPlanDriftReviewTestExecutor(files)

	result, err := executor(ctx, map[string]interface{}{
		"step_id": "step-a",
		"checks": []interface{}{
			map[string]interface{}{"check_id": "report_query_compatibility", "status": "pass", "evidence": "all report queries ran cleanly against the current schema"},
		},
	})
	if err != nil {
		t.Fatalf("executor returned error: %v", err)
	}
	if !strings.Contains(result, "step-a") {
		t.Fatalf("result = %q, want it to mention the step id", result)
	}

	var out StepConfigFile
	if err := json.Unmarshal([]byte(files["planning/step_config.json"]), &out); err != nil {
		t.Fatalf("updated step_config.json is invalid JSON: %v", err)
	}
	if len(out.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(out.Steps))
	}
	dr := out.Steps[0].AgentConfigs.DriftReview
	if dr == nil {
		t.Fatal("expected drift_review to be set")
	}
	if len(dr.Checks) != 1 || dr.Checks[0].CheckID != "report_query_compatibility" || dr.Checks[0].Status != "pass" {
		t.Fatalf("drift_review.checks = %+v, unexpected", dr.Checks)
	}
	if dr.ReviewedAt == "" || dr.ReviewedBy == "" {
		t.Fatalf("expected reviewed_at/reviewed_by to be populated: %+v", dr)
	}
}

func TestRecordPlanDriftReviewExecutorCreatesStepConfigEntryWhenMissing(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{
		"planning/step_config.json": `{"steps":[]}`,
	}
	executor := newPlanDriftReviewTestExecutor(files)

	if _, err := executor(ctx, map[string]interface{}{
		"step_id": "brand-new-step",
		"checks": []interface{}{
			map[string]interface{}{"check_id": "orphaned_tables", "status": "pass", "evidence": "all live tables have at least one known reference"},
		},
	}); err != nil {
		t.Fatalf("executor returned error: %v", err)
	}

	var out StepConfigFile
	if err := json.Unmarshal([]byte(files["planning/step_config.json"]), &out); err != nil {
		t.Fatalf("updated step_config.json is invalid JSON: %v", err)
	}
	if len(out.Steps) != 1 || out.Steps[0].ID != "brand-new-step" {
		t.Fatalf("expected a new step_config entry for brand-new-step, got %+v", out.Steps)
	}
	if out.Steps[0].AgentConfigs == nil || out.Steps[0].AgentConfigs.DriftReview == nil {
		t.Fatal("expected drift_review to be set on the newly created entry")
	}
}

func TestRecordPlanDriftReviewExecutorOverwritesPriorRecord(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{
		"planning/step_config.json": `{"steps":[{"id":"step-a","agent_configs":{"drift_review":{"reviewed_at":"2020-01-01T00:00:00Z","reviewed_by":"old","checks":[{"check_id":"old_check","status":"fail","evidence":"stale evidence from a prior review pass"}]}}}]}`,
	}
	executor := newPlanDriftReviewTestExecutor(files)

	if _, err := executor(ctx, map[string]interface{}{
		"step_id": "step-a",
		"checks": []interface{}{
			map[string]interface{}{"check_id": "report_query_compatibility", "status": "pass", "evidence": "all report queries ran cleanly against the current schema"},
		},
		"reviewed_by": "pulse:plan_drift_review",
	}); err != nil {
		t.Fatalf("executor returned error: %v", err)
	}

	var out StepConfigFile
	if err := json.Unmarshal([]byte(files["planning/step_config.json"]), &out); err != nil {
		t.Fatalf("updated step_config.json is invalid JSON: %v", err)
	}
	dr := out.Steps[0].AgentConfigs.DriftReview
	if len(dr.Checks) != 1 || dr.Checks[0].CheckID != "report_query_compatibility" {
		t.Fatalf("expected the old record to be fully replaced, got %+v", dr.Checks)
	}
	if dr.ReviewedBy != "pulse:plan_drift_review" {
		t.Fatalf("reviewed_by = %q, want explicit override to be honored", dr.ReviewedBy)
	}
}

func TestRecordPlanDriftReviewExecutorRejectsInvalidChecksWithoutWriting(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{
		"planning/step_config.json": `{"steps":[{"id":"step-a"}]}`,
	}
	original := files["planning/step_config.json"]
	executor := newPlanDriftReviewTestExecutor(files)

	_, err := executor(ctx, map[string]interface{}{
		"step_id": "step-a",
		"checks": []interface{}{
			map[string]interface{}{"check_id": "x", "status": "pass", "evidence": "ok"},
		},
	})
	if err == nil {
		t.Fatal("expected error for placeholder evidence")
	}
	if files["planning/step_config.json"] != original {
		t.Fatal("step_config.json must not be written when validation fails")
	}
}

func TestRecordPlanDriftReviewExecutorRequiresStepID(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{"planning/step_config.json": `{"steps":[]}`}
	executor := newPlanDriftReviewTestExecutor(files)

	_, err := executor(ctx, map[string]interface{}{
		"checks": []interface{}{
			map[string]interface{}{"check_id": "x", "status": "pass", "evidence": "a real evidence sentence here"},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing step_id")
	}
}

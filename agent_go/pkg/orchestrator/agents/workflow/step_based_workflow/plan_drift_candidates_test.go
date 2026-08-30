package step_based_workflow

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func planDriftCandidateWorkspace(t *testing.T, workspacePath, stepConfigJSON string) {
	t.Helper()
	docsDir := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", docsDir)
	planningDir := filepath.Join(docsDir, workspacePath, PlanningFolderName)
	if err := os.MkdirAll(planningDir, 0o755); err != nil {
		t.Fatalf("mkdir planning dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planningDir, "step_config.json"), []byte(stepConfigJSON), 0o644); err != nil {
		t.Fatalf("write step_config.json: %v", err)
	}
	dbDir := filepath.Join(docsDir, workspacePath, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dbDir, "db.sqlite"))
	if err != nil {
		t.Fatalf("open db.sqlite: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE emails(id INTEGER PRIMARY KEY, status TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	_ = db.Close()
}

const reviewedStepConfig = `{"steps":[
  {"id":"step-a","agent_configs":{"drift_review":{"reviewed_at":"2026-08-01T00:00:00Z","reviewed_by":"pulse:plan_drift_review","checks":[{"check_id":"report_query_compatibility","status":"pass","evidence":"all report queries ran cleanly"}]}}}
]}`

func TestCollectPlanDriftCandidatesNilWhenNoStepConfig(t *testing.T) {
	docsDir := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", docsDir)
	got := CollectPlanDriftCandidates(context.Background(), "Workflow/drift-candidates")
	if got != nil {
		t.Fatalf("CollectPlanDriftCandidates = %#v, want nil when step_config.json does not exist", got)
	}
}

func TestCollectPlanDriftCandidatesNilWhenAllReviewed(t *testing.T) {
	planDriftCandidateWorkspace(t, "Workflow/drift-candidates", reviewedStepConfig)
	got := CollectPlanDriftCandidates(context.Background(), "Workflow/drift-candidates")
	if got != nil {
		t.Fatalf("CollectPlanDriftCandidates = %#v, want nil when every step already has a drift_review record", got)
	}
}

func TestCollectPlanDriftCandidatesSurvivesMalformedStepConfig(t *testing.T) {
	planDriftCandidateWorkspace(t, "Workflow/drift-candidates", "{not valid json")
	got := CollectPlanDriftCandidates(context.Background(), "Workflow/drift-candidates")
	if got != nil {
		t.Fatalf("CollectPlanDriftCandidates = %#v, want nil on malformed step_config.json, not a panic", got)
	}
}

func TestCollectPlanDriftCandidatesListsUnreviewedStepsOnly(t *testing.T) {
	stepConfig := `{"steps":[
  {"id":"step-a","agent_configs":{"drift_review":{"reviewed_at":"2026-08-01T00:00:00Z","reviewed_by":"x","checks":[{"check_id":"report_query_compatibility","status":"pass","evidence":"already reviewed, must not reappear"}]}}},
  {"id":"step-b"}
]}`
	planDriftCandidateWorkspace(t, "Workflow/drift-candidates", stepConfig)
	got := CollectPlanDriftCandidates(context.Background(), "Workflow/drift-candidates")
	if len(got) != 1 {
		t.Fatalf("CollectPlanDriftCandidates returned %d candidates, want exactly 1 (step-a is already reviewed)", len(got))
	}
	if got[0].StepID != "step-b" {
		t.Fatalf("candidate StepID = %q, want step-b", got[0].StepID)
	}
}

// Every candidate carries the workflow-wide checks (report query compatibility,
// db/README contract) plus its own scripted-code check, even when the step has
// no learnings/<id>/main.py — CheckScriptedCodeDBQueries passes trivially in
// that case rather than being omitted, so a reviewer always sees a result for
// every check id it's responsible for filling in the gaps around.
func TestCollectPlanDriftCandidatesIncludesWorkflowWideAndScriptedChecks(t *testing.T) {
	stepConfig := `{"steps":[{"id":"step-b"}]}`
	planDriftCandidateWorkspace(t, "Workflow/drift-candidates", stepConfig)
	got := CollectPlanDriftCandidates(context.Background(), "Workflow/drift-candidates")
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got))
	}
	checkIDs := map[string]bool{}
	for _, c := range got[0].Checks {
		checkIDs[c.CheckID] = true
	}
	for _, want := range []string{reportDriftCheckID, dbReadmeDriftCheckID, scriptedCodeDriftCheckID} {
		if !checkIDs[want] {
			t.Fatalf("candidate checks %v missing expected check_id %q", checkIDs, want)
		}
	}
	if checkIDs[validationSchemaDBDriftCheckID] {
		t.Fatalf("candidate checks unexpectedly include %q; step has no validation_schema.db[] override", validationSchemaDBDriftCheckID)
	}
}

// The DB-rules check only runs when step_config.json itself carries a
// validation_schema.db[] override — a plan.json-only declared schema is out
// of scope for this precompute pass (documented as a known coverage gap).
func TestCollectPlanDriftCandidatesIncludesDBRuleCheckWhenOverridePresent(t *testing.T) {
	stepConfig := `{"steps":[{"id":"step-b","validation_schema":{"db":[{"name":"emails exist","sql":"SELECT id FROM emails","min_rows":0}]}}]}`
	planDriftCandidateWorkspace(t, "Workflow/drift-candidates", stepConfig)
	got := CollectPlanDriftCandidates(context.Background(), "Workflow/drift-candidates")
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got))
	}
	found := false
	for _, c := range got[0].Checks {
		if c.CheckID == validationSchemaDBDriftCheckID {
			found = true
		}
	}
	if !found {
		t.Fatalf("candidate checks missing %q despite a validation_schema.db[] override on the step", validationSchemaDBDriftCheckID)
	}
}

func TestCollectPlanDriftCandidatesTrimsWorkspacePath(t *testing.T) {
	docsDir := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", docsDir)
	got := CollectPlanDriftCandidates(context.Background(), "  ")
	if got != nil {
		t.Fatalf("CollectPlanDriftCandidates = %#v, want nil for a blank workspace path", got)
	}
}

// StepType lets the reviewer turn tell a routing step (the "route"/major-fork
// concept, PLAT-259) apart from every other candidate without an extra
// lookup, so it can apply the two route-only judgment checks
// (route_structural_isolation, route_eval_pairing) precisely.
func TestCollectPlanDriftCandidatesPopulatesStepTypeFromPlanJSON(t *testing.T) {
	const workspacePath = "Workflow/drift-candidates-types"
	stepConfig := `{"steps":[{"id":"router-a"},{"id":"regular-b"}]}`
	planDriftCandidateWorkspace(t, workspacePath, stepConfig)

	planJSON := `{"steps":[
  {"type":"routing","id":"router-a","title":"Router","routing_question":"Which path?","routes":[
    {"route_id":"r1","route_name":"One","condition":"c","next_step_id":"end"},
    {"route_id":"r2","route_name":"Two","condition":"c","next_step_id":"end"}
  ]},
  {"type":"regular","id":"regular-b","title":"Regular","description":"d","validation_schema":{}}
]}`
	planPath := filepath.Join(os.Getenv("WORKSPACE_DOCS_PATH"), workspacePath, PlanningFolderName, "plan.json")
	if err := os.WriteFile(planPath, []byte(planJSON), 0o644); err != nil {
		t.Fatalf("write plan.json: %v", err)
	}

	got := CollectPlanDriftCandidates(context.Background(), workspacePath)
	byID := map[string]PlanDriftCandidate{}
	for _, c := range got {
		byID[c.StepID] = c
	}
	if len(byID) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %#v", len(byID), got)
	}
	if byID["router-a"].StepType != "routing" {
		t.Fatalf("router-a StepType = %q, want %q", byID["router-a"].StepType, "routing")
	}
	if byID["regular-b"].StepType != "regular" {
		t.Fatalf("regular-b StepType = %q, want %q", byID["regular-b"].StepType, "regular")
	}
}

// A missing/unparseable plan.json must not fail the whole scan -- StepType
// just stays empty, matching this file's existing tolerance for a missing
// step_config.json elsewhere.
func TestCollectPlanDriftCandidatesToleratesMissingPlanJSON(t *testing.T) {
	stepConfig := `{"steps":[{"id":"step-b"}]}`
	planDriftCandidateWorkspace(t, "Workflow/drift-candidates-no-plan", stepConfig)

	got := CollectPlanDriftCandidates(context.Background(), "Workflow/drift-candidates-no-plan")
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got))
	}
	if got[0].StepType != "" {
		t.Fatalf("StepType = %q, want empty when plan.json does not exist", got[0].StepType)
	}
}

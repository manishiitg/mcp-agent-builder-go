package step_based_workflow

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workspace"
	workspacehandlers "github.com/manishiitg/coding-agent-loop/workspace/handlers"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
	"github.com/spf13/viper"
)

func newEvalResultsTestOrchestrator(t *testing.T) *StepBasedWorkflowOrchestrator {
	t.Helper()
	base, err := orchestrator.NewBaseOrchestrator(
		loggerv2.NewDefault(), nil, orchestrator.OrchestratorTypeWorkflow,
		"", 0, "", nil, nil, false, &orchestrator.LLMConfig{}, 1, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewBaseOrchestrator: %v", err)
	}
	hcpo := &StepBasedWorkflowOrchestrator{BaseOrchestrator: base}
	hcpo.SetWorkspacePath(concernsWorkspace(t))
	return hcpo
}

// TestPersistEvalResultsToDBWritesRealVerdicts proves the report's real,
// extracted per-step verdicts (see extractEvalVerdictFromOutputContent) reach
// db/db.sqlite, using the same real shape captured from social-media's
// eval-workflow-success and eval-content-quality steps.
func TestPersistEvalResultsToDBWritesRealVerdicts(t *testing.T) {
	hcpo := newEvalResultsTestOrchestrator(t)
	ctx := context.Background()

	report := &EvaluationReport{
		TargetRunFolder: "iteration-202",
		GeneratedAt:     "2026-07-28T00:00:00Z",
		StepScores: []*EvaluationStepScore{
			{StepID: "eval-workflow-success", Score: 0, MaxScore: 10, ScoreCaptured: true, Reasoning: "Score 0.0/10.", Evidence: "see output_content.json"},
			{StepID: "eval-content-quality", Score: 10, MaxScore: 10, ScoreCaptured: true, Reasoning: "All checks passed.", Evidence: "see output_content.json"},
		},
	}
	if err := hcpo.persistEvalResultsToDB(ctx, report); err != nil {
		t.Fatalf("persistEvalResultsToDB: %v", err)
	}

	db, err := openRunConcernsDB(ctx, hcpo.GetWorkspacePath(), false)
	if err != nil || db == nil {
		t.Fatalf("open db: %v (db=%v)", err, db)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT step_id, score, max_score, reasoning, group_name FROM eval_results WHERE run_folder = ? ORDER BY step_id`, "iteration-202")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	type row struct {
		stepID, reasoning, group string
		score, maxScore          int
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.stepID, &r.score, &r.maxScore, &r.reasoning, &r.group); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d: %#v", len(got), got)
	}
	if got[0].stepID != "eval-content-quality" || got[0].score != 10 || got[0].maxScore != 10 {
		t.Fatalf("row 0 wrong: %#v", got[0])
	}
	if got[1].stepID != "eval-workflow-success" || got[1].score != 0 || got[1].reasoning != "Score 0.0/10." {
		t.Fatalf("row 1 wrong: %#v", got[1])
	}
	if got[0].group == "" {
		t.Fatal("expected a non-empty group_name")
	}
}

// TestPersistEvalResultsToDBReplacesStaleRowsOnRerun proves a re-run against
// the same run_folder replaces that run's rows instead of accumulating stale
// step ids — important because evaluation_plan.json can change step-to-step
// (a step removed, another's score changed) and the report is always rebuilt
// wholesale, never incrementally.
func TestPersistEvalResultsToDBReplacesStaleRowsOnRerun(t *testing.T) {
	hcpo := newEvalResultsTestOrchestrator(t)
	ctx := context.Background()

	first := &EvaluationReport{
		TargetRunFolder: "iteration-5",
		GeneratedAt:     "2026-07-28T00:00:00Z",
		StepScores: []*EvaluationStepScore{
			{StepID: "eval-a", Score: 3, MaxScore: 10, Reasoning: "first pass", Evidence: "e"},
			{StepID: "eval-b", Score: 5, MaxScore: 10, Reasoning: "first pass", Evidence: "e"},
		},
	}
	if err := hcpo.persistEvalResultsToDB(ctx, first); err != nil {
		t.Fatalf("first persist: %v", err)
	}

	second := &EvaluationReport{
		TargetRunFolder: "iteration-5",
		GeneratedAt:     "2026-07-28T01:00:00Z",
		StepScores: []*EvaluationStepScore{
			{StepID: "eval-a", Score: 9, MaxScore: 10, Reasoning: "second pass", Evidence: "e"},
		},
	}
	if err := hcpo.persistEvalResultsToDB(ctx, second); err != nil {
		t.Fatalf("second persist: %v", err)
	}

	db, err := openRunConcernsDB(ctx, hcpo.GetWorkspacePath(), false)
	if err != nil || db == nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT step_id, score FROM eval_results WHERE run_folder = ?`, "iteration-5")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var stepID string
		var score int
		if err := rows.Scan(&stepID, &score); err != nil {
			t.Fatalf("scan: %v", err)
		}
		count++
		if stepID != "eval-a" || score != 9 {
			t.Fatalf("expected only eval-a/9 to survive, got %s/%d", stepID, score)
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 surviving row after re-run, got %d", count)
	}
}

// TestLoadEvalResultsJoinsPlanTitlesAndOrdersNewestRunFirst proves the read
// path backing the Pulse popup's eval display: newest run first, each row
// carries its step's title/description from evaluation_plan.json, and a
// step_id no longer in the plan degrades to an empty title/description
// instead of failing the whole read.
func TestLoadEvalResultsJoinsPlanTitlesAndOrdersNewestRunFirst(t *testing.T) {
	hcpo := newEvalResultsTestOrchestrator(t)
	ctx := context.Background()
	workspacePath := hcpo.GetWorkspacePath()

	older := &EvaluationReport{
		TargetRunFolder: "iteration-1",
		GeneratedAt:     "2026-07-28T00:00:00Z",
		StepScores: []*EvaluationStepScore{
			{StepID: "eval-a", Score: 3, MaxScore: 10, ScoreCaptured: true, Reasoning: "first pass", Evidence: "e"},
		},
	}
	newer := &EvaluationReport{
		TargetRunFolder: "iteration-2",
		GeneratedAt:     "2026-07-29T00:00:00Z",
		StepScores: []*EvaluationStepScore{
			{StepID: "eval-a", Score: 9, MaxScore: 10, ScoreCaptured: true, Reasoning: "second pass", Evidence: "e"},
			{StepID: "eval-orphaned", Score: 0, MaxScore: 0, Skipped: true, Reasoning: "route not selected", Evidence: "e"},
		},
	}
	if err := hcpo.persistEvalResultsToDB(ctx, older); err != nil {
		t.Fatalf("persist older: %v", err)
	}
	if err := hcpo.persistEvalResultsToDB(ctx, newer); err != nil {
		t.Fatalf("persist newer: %v", err)
	}

	planDir := filepath.Join(fsutil.WorkspaceDocsRoot(), workspacePath, "evaluation")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir evaluation dir: %v", err)
	}
	plan := `{"steps":[{"id":"eval-a","title":"Signal accuracy","description":"Checks the signal matches the exchange feed."}]}`
	if err := os.WriteFile(filepath.Join(planDir, "evaluation_plan.json"), []byte(plan), 0o644); err != nil {
		t.Fatalf("write evaluation_plan.json: %v", err)
	}

	results, err := LoadEvalResults(ctx, workspacePath, 0)
	if err != nil {
		t.Fatalf("LoadEvalResults: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 rows, got %d: %#v", len(results), results)
	}
	if results[0].RunFolder != "iteration-2" {
		t.Fatalf("expected the newer run first, got %#v", results[0])
	}
	if results[2].RunFolder != "iteration-1" {
		t.Fatalf("expected the older run last, got %#v", results[2])
	}

	var joined, orphaned *EvalResultRecord
	for i := range results {
		switch {
		case results[i].RunFolder == "iteration-2" && results[i].StepID == "eval-a":
			joined = &results[i]
		case results[i].StepID == "eval-orphaned":
			orphaned = &results[i]
		}
	}
	if joined == nil || joined.Title != "Signal accuracy" || joined.Description != "Checks the signal matches the exchange feed." {
		t.Fatalf("expected the newer eval-a row joined against the plan, got %#v", joined)
	}
	if !joined.ScoreCaptured || joined.Score != 9 {
		t.Fatalf("expected the newer eval-a score to survive the join, got %#v", joined)
	}
	if orphaned == nil || orphaned.Title != "" || orphaned.Description != "" || !orphaned.Skipped {
		t.Fatalf("expected a step absent from the plan to degrade to an empty title/description, not fail, got %#v", orphaned)
	}
}

// TestLoadEvalResultsBackfillsFromScoreLedger proves the gap found live
// against confida-login: eval_results only started being written the first
// time a workflow ran eval after this table existed, so a workflow with real
// history sitting in the older scores/evaluation/<group>/<date>.json ledger
// (survives iteration-0 rotation; eval_results does not backfill itself) had
// only its single newest run visible in a cross-run view -- 10 dated ledger
// files, 1 row in eval_results. LoadEvalResults must pull the older runs in
// too, covering both the v2 (Evaluations, keyed by evaluation_id) and legacy
// v1 (RunFolders, keyed by run_folder) ledger shapes, and must never
// overwrite a run_folder eval_results already has from a real write.
func TestLoadEvalResultsBackfillsFromScoreLedger(t *testing.T) {
	hcpo := newEvalResultsTestOrchestrator(t)
	ctx := context.Background()
	workspacePath := hcpo.GetWorkspacePath()

	// A real write already covers iteration-3; the ledger's own entry for it
	// must not overwrite this with different content.
	if err := hcpo.persistEvalResultsToDB(ctx, &EvaluationReport{
		TargetRunFolder: "iteration-3",
		GeneratedAt:     "2026-07-15T00:00:00Z",
		StepScores:      []*EvaluationStepScore{{StepID: "eval-a", Score: 7, MaxScore: 10, ScoreCaptured: true, Reasoning: "real write"}},
	}); err != nil {
		t.Fatalf("persist real row: %v", err)
	}

	ledgerDir := filepath.Join(fsutil.WorkspaceDocsRoot(), workspacePath, "scores", "evaluation", "default")
	if err := os.MkdirAll(ledgerDir, 0o755); err != nil {
		t.Fatalf("mkdir ledger dir: %v", err)
	}
	// v2 shape: keyed by evaluation_id under "evaluations".
	v2 := `{"date":"2026-07-10","group_folder":"default","evaluations":{"eval-id-1":{"run_folder":"iteration-1","report":{"target_run_folder":"iteration-1","generated_at":"2026-07-10T00:00:00Z","step_scores":[{"step_id":"eval-a","score":3,"max_score":10,"score_captured":true,"reasoning":"ledger v2"}]}}}}`
	if err := os.WriteFile(filepath.Join(ledgerDir, "2026-07-10.json"), []byte(v2), 0o644); err != nil {
		t.Fatalf("write v2 ledger file: %v", err)
	}
	// Legacy v1 shape: keyed by run_folder under "run_folders", and a stale
	// entry for iteration-3 that must NOT clobber the real write above.
	v1 := `{"date":"2026-07-12","group_folder":"default","run_folders":{"iteration-2":{"target_run_folder":"iteration-2","generated_at":"2026-07-12T00:00:00Z","step_scores":[{"step_id":"eval-a","score":5,"max_score":10,"score_captured":true,"reasoning":"ledger v1"}]},"iteration-3":{"target_run_folder":"iteration-3","generated_at":"2026-07-12T00:00:00Z","step_scores":[{"step_id":"eval-a","score":1,"max_score":10,"score_captured":true,"reasoning":"stale ledger copy"}]}}}`
	if err := os.WriteFile(filepath.Join(ledgerDir, "2026-07-12.json"), []byte(v1), 0o644); err != nil {
		t.Fatalf("write v1 ledger file: %v", err)
	}

	results, err := LoadEvalResults(ctx, workspacePath, 0)
	if err != nil {
		t.Fatalf("LoadEvalResults: %v", err)
	}
	byRun := map[string]EvalResultRecord{}
	for _, r := range results {
		byRun[r.RunFolder] = r
	}
	if len(byRun) != 3 {
		t.Fatalf("expected 3 distinct runs (1 real write + 2 backfilled), got %d: %#v", len(byRun), results)
	}
	if r := byRun["iteration-1"]; r.Reasoning != "ledger v2" || r.Score != 3 {
		t.Fatalf("v2 ledger entry not backfilled correctly: %#v", r)
	}
	if r := byRun["iteration-2"]; r.Reasoning != "ledger v1" || r.Score != 5 {
		t.Fatalf("v1 ledger entry not backfilled correctly: %#v", r)
	}
	if r := byRun["iteration-3"]; r.Reasoning != "real write" || r.Score != 7 {
		t.Fatalf("backfill must not overwrite a run eval_results already has from a real write: %#v", r)
	}

	// A second call must be a no-op over the same data, not duplicate rows.
	again, err := LoadEvalResults(ctx, workspacePath, 0)
	if err != nil {
		t.Fatalf("second LoadEvalResults: %v", err)
	}
	if len(again) != len(results) {
		t.Fatalf("backfill is not idempotent: first=%d second=%d", len(results), len(again))
	}
}

// newEvalReportPhaseWiringTestOrchestrator wires a REAL workspace-docs HTTP
// server AND aligns fsutil.WorkspaceDocsRoot() (WORKSPACE_DOCS_PATH) to the
// same backing directory, so a direct db/db.sqlite open (used by
// persistEvalResultsToDB) and the HTTP-based file writes runEvaluationReportPhase
// makes (evaluation_report.json, the score ledger) land in the same tree.
func newEvalReportPhaseWiringTestOrchestrator(t *testing.T) (*StepBasedWorkflowOrchestrator, string) {
	t.Helper()

	base, err := orchestrator.NewBaseOrchestrator(
		loggerv2.NewDefault(), nil, orchestrator.OrchestratorTypeWorkflow,
		"", 0, "", nil, nil, false, &orchestrator.LLMConfig{}, 1, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewBaseOrchestrator: %v", err)
	}
	workflowRelPath := "Workflow/eval-results-wiring-test"
	hcpo := &StepBasedWorkflowOrchestrator{BaseOrchestrator: base}

	docsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(docsDir, workflowRelPath, "db"), 0o755); err != nil {
		t.Fatalf("mkdir workflow dir: %v", err)
	}
	t.Setenv("WORKSPACE_DOCS_PATH", docsDir)
	gin.SetMode(gin.TestMode)
	viper.Set("docs-dir", docsDir)
	router := gin.New()
	router.Any("/api/documents/*filepath", workspacehandlers.HandleDocumentRequest)
	wsServer := httptest.NewServer(router)
	t.Cleanup(wsServer.Close)

	hcpo.WorkspaceClient = workspace.NewClient(wsServer.URL)
	hcpo.SetWorkspacePath(workflowRelPath)

	return hcpo, workflowRelPath
}

// TestRunEvaluationReportPhasePersistsToDB is the integration-level
// counterpart to TestPersistEvalResultsToDBWritesRealVerdicts: it proves the
// db.sqlite write is actually wired into runEvaluationReportPhase (the real
// caller, invoked by ExecuteEvaluationOnly after every eval run), not just
// correct as a standalone function.
func TestRunEvaluationReportPhasePersistsToDB(t *testing.T) {
	hcpo, workspacePath := newEvalReportPhaseWiringTestOrchestrator(t)
	ctx := context.Background()

	plan := &EvaluationPlan{Steps: []*EvaluationStep{
		{ID: "eval-workflow-success"},
		{ID: "eval-content-quality"},
	}}

	if _, err := hcpo.runEvaluationReportPhase(ctx, plan, "iteration-9", "iteration-0", nil); err != nil {
		t.Fatalf("runEvaluationReportPhase: %v", err)
	}

	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		t.Fatalf("open db: %v (db=%v)", err, db)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT step_id FROM eval_results WHERE run_folder = ? ORDER BY step_id`, "iteration-9")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var stepIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		stepIDs = append(stepIDs, id)
	}
	if len(stepIDs) != 2 || stepIDs[0] != "eval-content-quality" || stepIDs[1] != "eval-workflow-success" {
		t.Fatalf("expected both step ids to reach db.sqlite through the real pipeline call, got %#v", stepIDs)
	}
}

package step_based_workflow

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
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
			{StepID: "eval-workflow-success", Score: 0, MaxScore: 10, Reasoning: "Score 0.0/10.", Evidence: "see output_content.json"},
			{StepID: "eval-content-quality", Score: 10, MaxScore: 10, Reasoning: "All checks passed.", Evidence: "see output_content.json"},
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

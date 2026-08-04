package step_based_workflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilterEvaluationPlanEmitsExplicitSkippedSentinel(t *testing.T) {
	hcpo, docsRoot := newEvalReportEnrichmentTestOrchestrator(t)
	ctx := context.Background()
	routeDir := filepath.Join(docsRoot, "runs", "iteration-0", "default", "logs", "choose-route")
	if err := os.MkdirAll(routeDir, 0o755); err != nil {
		t.Fatalf("mkdir route log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(routeDir, "routing-evaluation.json"), []byte(`{"selected_route_id":"route-b"}`), 0o644); err != nil {
		t.Fatalf("write route log: %v", err)
	}

	plan := &EvaluationPlan{Steps: []*EvaluationStep{
		{ID: "eval-always"},
		{ID: "eval-route-a", AppliesToRoutes: []EvaluationRouteApplicability{{
			RoutingStepID: "choose-route",
			RouteIDs:      []string{"route-a"},
		}}},
	}}
	filtered, skipped := hcpo.filterEvaluationPlanForTargetRun(ctx, plan, "iteration-0/default")

	if len(filtered.Steps) != 1 || filtered.Steps[0].ID != "eval-always" {
		t.Fatalf("active evaluation steps = %#v, want only eval-always", filtered.Steps)
	}
	if len(skipped) != 1 {
		t.Fatalf("skipped scores = %#v, want one explicit sentinel", skipped)
	}
	sentinel := skipped[0]
	if sentinel.StepID != "eval-route-a" || !sentinel.Skipped || sentinel.Score != 0 || sentinel.MaxScore != 0 {
		t.Fatalf("skipped sentinel = %#v", sentinel)
	}
	if !strings.Contains(sentinel.Reasoning, "selected route \"route-b\"") || strings.TrimSpace(sentinel.Evidence) == "" {
		t.Fatalf("skipped sentinel lacks applicability proof: %#v", sentinel)
	}
	encoded, err := json.Marshal(sentinel)
	if err != nil {
		t.Fatalf("marshal skipped sentinel: %v", err)
	}
	if !strings.Contains(string(encoded), `"score":0`) || !strings.Contains(string(encoded), `"skipped":true`) {
		t.Fatalf("serialized skipped sentinel lost state: %s", encoded)
	}
}

func TestPersistEvalResultsKeepsSkippedSentinelDistinct(t *testing.T) {
	hcpo := newEvalResultsTestOrchestrator(t)
	ctx := context.Background()
	report := &EvaluationReport{
		TargetRunFolder: "iteration-skipped",
		GeneratedAt:     "2026-08-04T00:00:00Z",
		StepScores: []*EvaluationStepScore{{
			StepID:    "eval-route-a",
			Score:     0,
			MaxScore:  0,
			Reasoning: "Skipped because route-b was selected.",
			Evidence:  "Route gating marked this eval step as not applicable.",
			Skipped:   true,
		}},
	}
	if err := hcpo.persistEvalResultsToDB(ctx, report); err != nil {
		t.Fatalf("persist skipped sentinel: %v", err)
	}

	db, err := openRunConcernsDB(ctx, hcpo.GetWorkspacePath(), false)
	if err != nil || db == nil {
		t.Fatalf("open eval results db: %v", err)
	}
	defer db.Close()
	var score, maxScore float64
	var scoreCaptured, skipped int
	var reasoning, evidence string
	if err := db.QueryRowContext(ctx, `SELECT score,max_score,score_captured,skipped,reasoning,evidence
		FROM eval_results WHERE run_folder=? AND step_id=?`, "iteration-skipped", "eval-route-a").
		Scan(&score, &maxScore, &scoreCaptured, &skipped, &reasoning, &evidence); err != nil {
		t.Fatalf("read skipped sentinel: %v", err)
	}
	if score != 0 || maxScore != 0 || scoreCaptured != 0 || skipped != 1 || reasoning == "" || evidence == "" {
		t.Fatalf("persisted skipped sentinel = score=%v max=%v captured=%d skipped=%d reasoning=%q evidence=%q", score, maxScore, scoreCaptured, skipped, reasoning, evidence)
	}
}

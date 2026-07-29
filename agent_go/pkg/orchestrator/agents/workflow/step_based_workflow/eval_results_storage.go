package step_based_workflow

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Durable, report-queryable copy of each eval step's verdict.
//
// Why this exists: eval verdicts already live correctly in evaluation_report.json
// and the scores/evaluation/ ledger (see evaluation_score_storage.go), but both are
// files under evaluation/, invisible to the report UI's window.report.query, which
// only ever reads db/db.sqlite. A user watching the normal report has no way to see
// "did the workflow pass its own success criteria this run" without knowing to look
// in a separate, eval-specific place. This table closes that gap the same way
// run_concerns.go closes the equivalent gap for CONCERNS: lines — written directly
// by Go at report-assembly time, never by an agent calling a tool, so there is no
// step for an author to forget to add.
//
// One row per (run_folder, step_id); a re-run of eval for the same run_folder
// replaces that run's rows rather than accumulating duplicates, since the report
// is regenerated wholesale each time (see persistEvalResultsToDB).
const evalResultsSchema = `CREATE TABLE IF NOT EXISTS eval_results (
	run_folder TEXT NOT NULL,
	group_name TEXT NOT NULL DEFAULT '',
	step_id TEXT NOT NULL,
	score INTEGER NOT NULL DEFAULT 0,
	max_score INTEGER NOT NULL DEFAULT 0,
	reasoning TEXT NOT NULL DEFAULT '',
	evidence TEXT NOT NULL DEFAULT '',
	skipped INTEGER NOT NULL DEFAULT 0,
	generated_at TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (run_folder, step_id)
)`

// persistEvalResultsToDB mirrors report.StepScores into db/db.sqlite so the normal
// report surfaces eval verdicts alongside everything else a workflow measures,
// per soul.md's "no blob score" rule: one row per criterion, never a blended total.
//
// Best-effort by contract, like recordStepConcerns: the JSON report and ledger
// (see runEvaluationReportPhase) are already the durable source of truth, so a
// db.sqlite write failure must not fail the evaluation phase — it would only lose
// this run's contribution to a UI convenience surface, not any data.
func (hcpo *StepBasedWorkflowOrchestrator) persistEvalResultsToDB(ctx context.Context, report *EvaluationReport) error {
	if report == nil || len(report.StepScores) == 0 {
		return nil
	}
	workspacePath := strings.TrimSpace(hcpo.GetWorkspacePath())
	if workspacePath == "" {
		return nil
	}
	runFolder := strings.TrimSpace(report.TargetRunFolder)
	if runFolder == "" {
		return nil
	}

	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil || db == nil {
		return err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, evalResultsSchema); err != nil {
		return err
	}

	// The report is rebuilt wholesale every eval run, so a re-run against the same
	// run_folder must replace its rows rather than let stale step ids accumulate
	// (e.g. an eval step removed from evaluation_plan.json between runs).
	if _, err := db.ExecContext(ctx, `DELETE FROM eval_results WHERE run_folder = ?`, runFolder); err != nil {
		return err
	}

	groupName := evaluationScoreGroupFolder(runFolder)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, score := range report.StepScores {
		if score == nil || strings.TrimSpace(score.StepID) == "" {
			continue
		}
		skipped := 0
		if score.Skipped {
			skipped = 1
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO eval_results
			(run_folder, group_name, step_id, score, max_score, reasoning, evidence, skipped, generated_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			runFolder, groupName, score.StepID, score.Score, score.MaxScore, score.Reasoning, score.Evidence, skipped, report.GeneratedAt, now,
		); err != nil {
			return fmt.Errorf("insert eval_results row for step %q: %w", score.StepID, err)
		}
	}
	return nil
}

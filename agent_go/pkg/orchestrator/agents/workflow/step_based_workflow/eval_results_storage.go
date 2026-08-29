package step_based_workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
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
	score REAL NOT NULL DEFAULT 0,
	max_score REAL NOT NULL DEFAULT 0,
	score_captured INTEGER NOT NULL DEFAULT 0,
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
	if err := ensureEvalResultsScoreCapturedColumn(ctx, db); err != nil {
		return err
	}

	return insertEvalResultsRows(ctx, db, runFolder, report)
}

// insertEvalResultsRows writes one report's step scores into eval_results for
// the given run_folder, replacing any rows already there for that run first
// (a report is always rebuilt wholesale, never incrementally, so a re-run or
// a backfill must not let stale step ids from an earlier plan accumulate).
// Shared by the live write path (persistEvalResultsToDB, called after every
// real eval run) and the historical backfill (backfillEvalResultsFromScoreLedger,
// called lazily on read) so both populate the table identically.
func insertEvalResultsRows(ctx context.Context, db *sql.DB, runFolder string, report *EvaluationReport) error {
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
		scoreCaptured := 0
		if score.ScoreCaptured {
			scoreCaptured = 1
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO eval_results
			(run_folder, group_name, step_id, score, max_score, score_captured, reasoning, evidence, skipped, generated_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			runFolder, groupName, score.StepID, score.Score, score.MaxScore, scoreCaptured, score.Reasoning, score.Evidence, skipped, report.GeneratedAt, now,
		); err != nil {
			return fmt.Errorf("insert eval_results row for step %q: %w", score.StepID, err)
		}
	}
	return nil
}

// backfillEvalResultsFromScoreLedger fills in eval_results for any run_folder
// present in the older scores/evaluation/<group>/<date>.json ledger but
// missing from eval_results. eval_results only started being written the
// first time a workflow ran eval after this table existed, so a workflow
// with real history predating that has months of runs sitting in the ledger
// with nothing to show in a cross-run view -- confirmed live against
// confida-login: 10 dated ledger files back to July, one row in eval_results.
// Idempotent (skips any run_folder already present) and best-effort: a
// missing/unreadable ledger directory, or one already fully backfilled, is a
// no-op, not an error, matching persistEvalResultsToDB's own contract that a
// failure here must never block the caller from reading what IS there.
func backfillEvalResultsFromScoreLedger(ctx context.Context, db *sql.DB, workspacePath string) error {
	existing := map[string]bool{}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT run_folder FROM eval_results`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var runFolder string
		if err := rows.Scan(&runFolder); err != nil {
			rows.Close()
			return err
		}
		existing[runFolder] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	ledgerRoot := filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(strings.Trim(strings.TrimSpace(workspacePath), "/")), "scores", "evaluation")
	groupDirs, err := os.ReadDir(ledgerRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	insert := func(runFolder string, report *EvaluationReport) error {
		runFolder = strings.TrimSpace(runFolder)
		if runFolder == "" || report == nil || existing[runFolder] {
			return nil
		}
		if err := insertEvalResultsRows(ctx, db, runFolder, report); err != nil {
			return err
		}
		existing[runFolder] = true
		return nil
	}

	for _, groupDir := range groupDirs {
		if !groupDir.IsDir() {
			continue
		}
		dateFiles, err := os.ReadDir(filepath.Join(ledgerRoot, groupDir.Name()))
		if err != nil {
			continue
		}
		for _, dateFile := range dateFiles {
			if dateFile.IsDir() || !strings.HasSuffix(dateFile.Name(), ".json") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(ledgerRoot, groupDir.Name(), dateFile.Name()))
			if err != nil {
				continue
			}
			var daily EvaluationScoreDailyFile
			if err := json.Unmarshal(content, &daily); err != nil {
				continue
			}
			for _, stored := range daily.Evaluations {
				if stored == nil {
					continue
				}
				runFolder := stored.RunFolder
				if strings.TrimSpace(runFolder) == "" && stored.Report != nil {
					runFolder = stored.Report.TargetRunFolder
				}
				if err := insert(runFolder, stored.Report); err != nil {
					return err
				}
			}
			// v1 ledger shape, kept readable per StoredEvaluationReport's own doc.
			for runFolder, report := range daily.RunFolders {
				if err := insert(runFolder, report); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// EvalResultRecord is one step's score for one run, joined with that step's
// title/description from evaluation_plan.json. eval_results itself stores
// neither (see EvaluationStepScore's doc: the plan is the single source for
// what a step means, so a UI reads scores and titles from one place instead
// of two copies drifting apart).
type EvalResultRecord struct {
	RunFolder     string  `json:"run_folder"`
	StepID        string  `json:"step_id"`
	Title         string  `json:"title,omitempty"`
	Description   string  `json:"description,omitempty"`
	Score         float64 `json:"score"`
	MaxScore      float64 `json:"max_score"`
	ScoreCaptured bool    `json:"score_captured"`
	Reasoning     string  `json:"reasoning"`
	Evidence      string  `json:"evidence"`
	Skipped       bool    `json:"skipped"`
	GeneratedAt   string  `json:"generated_at"`
}

// LoadEvalResults returns the most recent eval_results rows for a workflow,
// most recent run first, joined against evaluation_plan.json for each step's
// title and description. The plan read is best-effort: a missing or
// unparseable plan (e.g. a step later removed from the plan, or a workflow
// with no evaluation configured) still returns the scores, just without
// title/description -- the score data is never withheld because of it.
func LoadEvalResults(ctx context.Context, workspacePath string, limit int) ([]EvalResultRecord, error) {
	if limit <= 0 {
		limit = 200
	}
	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil || db == nil {
		return []EvalResultRecord{}, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, evalResultsSchema); err != nil {
		return nil, err
	}
	if err := ensureEvalResultsScoreCapturedColumn(ctx, db); err != nil {
		return nil, err
	}
	if err := backfillEvalResultsFromScoreLedger(ctx, db, workspacePath); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `SELECT run_folder, step_id, score, max_score, score_captured, reasoning, evidence, skipped, generated_at
		FROM eval_results ORDER BY generated_at DESC, run_folder DESC, step_id ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []EvalResultRecord{}
	for rows.Next() {
		var record EvalResultRecord
		var scoreCaptured, skipped int
		if err := rows.Scan(&record.RunFolder, &record.StepID, &record.Score, &record.MaxScore, &scoreCaptured, &record.Reasoning, &record.Evidence, &skipped, &record.GeneratedAt); err != nil {
			return nil, err
		}
		record.ScoreCaptured = scoreCaptured != 0
		record.Skipped = skipped != 0
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	planTitles, planDescriptions := loadEvaluationPlanStepText(workspacePath)
	for i := range records {
		records[i].Title = planTitles[records[i].StepID]
		records[i].Description = planDescriptions[records[i].StepID]
	}
	return records, nil
}

// loadEvaluationPlanStepText reads evaluation/evaluation_plan.json directly
// off disk (the same workspace-root resolution eval_results_storage.go
// already uses for db/db.sqlite) and indexes each step's title/description
// by id. Returns empty maps on any read/parse failure -- there is no
// evaluation plan to describe for a workflow that never configured one, and
// that is not an error condition for the caller.
func loadEvaluationPlanStepText(workspacePath string) (titles, descriptions map[string]string) {
	titles = map[string]string{}
	descriptions = map[string]string{}
	planPath := filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(strings.Trim(strings.TrimSpace(workspacePath), "/")), "evaluation", "evaluation_plan.json")
	content, err := os.ReadFile(planPath)
	if err != nil {
		return titles, descriptions
	}
	var plan EvaluationPlan
	if err := json.Unmarshal(content, &plan); err != nil {
		return titles, descriptions
	}
	for _, step := range plan.Steps {
		if step == nil || strings.TrimSpace(step.ID) == "" {
			continue
		}
		titles[step.ID] = step.Title
		descriptions[step.ID] = step.Description
	}
	return titles, descriptions
}

func ensureEvalResultsScoreCapturedColumn(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(eval_results)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "score_captured" {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE eval_results ADD COLUMN score_captured INTEGER NOT NULL DEFAULT 0`)
	return err
}

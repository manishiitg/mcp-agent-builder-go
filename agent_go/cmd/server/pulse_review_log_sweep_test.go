package server

import (
	"context"
	"testing"
)

// PLAT-017 reproduction / PLAT-054. pulse_review_log is written only by an
// agent, so a pass that dies between "review started" and "review recorded"
// strands its row at status='running' forever — the startup sweep previously
// reconciled pulse_final_command_state only. Upwork accumulated three stranded
// workflow_review rows from three separate runs in one day before this existed.

func seedPulseReviewLogRow(ctx context.Context, t *testing.T, workspacePath, module, runID, status, verdict string) {
	t.Helper()
	_, db, err := openPulseModuleStateDB(ctx, workspacePath, true)
	if err != nil || db == nil {
		t.Fatalf("open pulse db: %v", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS pulse_review_log (
		_id INTEGER PRIMARY KEY AUTOINCREMENT,
		module TEXT NOT NULL,
		review_run_id TEXT NOT NULL DEFAULT '',
		pulse_run_id TEXT NOT NULL DEFAULT '',
		verdict TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT '',
		finding_count INTEGER NOT NULL DEFAULT 0,
		verification_count INTEGER NOT NULL DEFAULT 0,
		verifications_json TEXT NOT NULL DEFAULT '[]',
		recorded_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create pulse_review_log: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO pulse_review_log (module, review_run_id, pulse_run_id, verdict, status, finding_count, verification_count, recorded_at)
		 VALUES (?, ?, ?, ?, ?, 24, 13, '2026-08-08T17:35:52Z')`,
		module, runID, runID, verdict, status); err != nil {
		t.Fatalf("seed review row: %v", err)
	}
}

func readPulseReviewLogRow(ctx context.Context, t *testing.T, workspacePath, runID string) (status, verdict string, findings int) {
	t.Helper()
	_, db, err := openPulseModuleStateDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		t.Fatalf("open pulse db: %v", err)
	}
	defer db.Close()
	if err := db.QueryRowContext(ctx,
		`SELECT status, verdict, finding_count FROM pulse_review_log WHERE review_run_id = ?`,
		runID).Scan(&status, &verdict, &findings); err != nil {
		t.Fatalf("read review row: %v", err)
	}
	return status, verdict, findings
}

func TestFinalizeAllRunningPulseReviewLogsClosesStrandedRows(t *testing.T) {
	ctx := context.Background()
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/example"

	seedPulseReviewLogRow(ctx, t, workspacePath, "workflow_review", "run-stranded", "running", "")
	seedPulseReviewLogRow(ctx, t, workspacePath, "llm_ops_review", "run-finished", "completed", "clean")

	changed, err := finalizeAllRunningPulseReviewLogs(ctx, workspacePath, "Server restarted")
	if err != nil {
		t.Fatalf("finalize review logs: %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1 (only the running row)", changed)
	}

	status, verdict, findings := readPulseReviewLogRow(ctx, t, workspacePath, "run-stranded")
	if status != "failed" {
		t.Fatalf("stranded row status = %q, want failed", status)
	}
	if verdict != "Server restarted" {
		t.Fatalf("stranded row verdict = %q, want the reason recorded", verdict)
	}
	// The dead pass genuinely recorded these; closing the row must not erase them.
	if findings != 24 {
		t.Fatalf("finding_count = %d, want 24 preserved", findings)
	}

	// A completed review is already terminal and must be left exactly as it was.
	status, verdict, _ = readPulseReviewLogRow(ctx, t, workspacePath, "run-finished")
	if status != "completed" || verdict != "clean" {
		t.Fatalf("terminal row was rewritten: status=%q verdict=%q", status, verdict)
	}
}

func TestFinalizeAllRunningPulseReviewLogsToleratesMissingTable(t *testing.T) {
	ctx := context.Background()
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/never-reviewed"

	// A workflow that has never run a Pulse review has no such table; the
	// startup sweep runs for every discovered workflow and must not error.
	if err := initializePulseFinalCommandStates(ctx, workspacePath, "schedule-cron--x"); err != nil {
		t.Fatalf("initialize final commands: %v", err)
	}
	changed, err := finalizeAllRunningPulseReviewLogs(ctx, workspacePath, "Server restarted")
	if err != nil {
		t.Fatalf("missing table should not error: %v", err)
	}
	if changed != 0 {
		t.Fatalf("changed = %d, want 0", changed)
	}
}

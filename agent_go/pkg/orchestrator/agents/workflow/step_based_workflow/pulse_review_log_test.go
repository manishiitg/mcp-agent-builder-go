package step_based_workflow

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestTypedReviewPersistsOnlyCompactReceipt(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	if _, err := RecordPulseReviewFinding(ctx, ws, "pulse-1", "review-1", PulseReviewFindingInput{
		Concern: "collector drops failures", Module: "workflow_review", PulseFindingDetails: PulseFindingDetails{
			IssueKind: "workflow_issue", Classification: "correctness_bug", Severity: "high", Summary: "Failures disappear",
			Impact: "The report can claim healthy on missing data.", Evidence: []string{"runs/iteration-0/result.json"},
		},
	}); err != nil {
		t.Fatalf("RecordPulseReviewFinding: %v", err)
	}
	if err := CompletePulseReview(ctx, ws, []string{"workflow_review"}, "review-1", "pulse-1", "One correctness issue remains.", "completed"); err != nil {
		t.Fatalf("CompletePulseReview: %v", err)
	}
	record, err := LoadPulseReviewReceiptForRun(ctx, ws, "review-1", "workflow_review")
	if err != nil {
		t.Fatalf("load receipt: %v", err)
	}
	if record.Verdict != "One correctness issue remains." || record.FindingCount != 1 || record.VerificationCount != 0 {
		t.Fatalf("unexpected receipt: %#v", record)
	}
	db, err := openRunConcernsDB(ctx, ws, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(pulse_review_log)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(name, "artifact") || name == "markdown" {
			t.Fatalf("legacy review content column still exists: %s", name)
		}
	}
}

func TestTypedReviewDecisionRequiredCreatesNoOrphanFinding(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	input := PulseReviewFindingInput{
		Concern: "the fixed pipeline has no runtime orchestration choice", Module: "llm_ops_review",
		PulseFindingDetails: PulseFindingDetails{
			IssueKind: "workflow_issue", Classification: "step_type_fitness", Severity: "medium",
			Summary: "A fixed sequence uses an unnecessary orchestrator.", Impact: "It adds cost and hides lifecycle state.",
			Evidence: []string{"planning/plan.json"}, RecommendedRoute: pulseFindingRouteDecisionRequired,
		},
	}
	if _, err := RecordPulseReviewFinding(ctx, ws, "pulse-1", "review-1", input); err == nil || !strings.Contains(err.Error(), "requires human_input_id") {
		t.Fatalf("decision_required finding was accepted without a decision: %v", err)
	}
	db, err := openRunConcernsDB(ctx, ws, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS report_human_inputs (id TEXT PRIMARY KEY, source TEXT, status TEXT)`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO report_human_inputs (id, source, status) VALUES
		('ops-decision-fixed-pipeline-wrong-source', 'pulse', 'pending'),
		('ops-decision-fixed-pipeline', 'ops_review', 'pending')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()

	input.HumanInputID = "ops-decision-fixed-pipeline-wrong-source"
	if _, err := RecordPulseReviewFinding(ctx, ws, "pulse-1", "review-1", input); err == nil || !strings.Contains(err.Error(), `source "pulse"`) {
		t.Fatalf("Ops Review accepted a generic Pulse decision: %v", err)
	}
	input.HumanInputID = "ops-decision-fixed-pipeline"
	record, err := RecordPulseReviewFinding(ctx, ws, "pulse-1", "review-1", input)
	if err != nil {
		t.Fatalf("record linked decision finding: %v", err)
	}
	if record.Status != ConcernStatusAcknowledged {
		t.Fatalf("linked decision status = %q, want %q", record.Status, ConcernStatusAcknowledged)
	}
	db, err = openRunConcernsDB(ctx, ws, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var linkedID string
	if err := db.QueryRowContext(ctx, `SELECT json_extract(metadata_json, '$.human_input_id')
		FROM pulse_finding_events WHERE fingerprint=? AND event_type='awaiting_user'`, record.Fingerprint).Scan(&linkedID); err != nil {
		t.Fatalf("load awaiting_user link: %v", err)
	}
	if linkedID != input.HumanInputID {
		t.Fatalf("linked decision id = %q, want %q", linkedID, input.HumanInputID)
	}
}

func TestPulseReviewLogMigratesLegacyAdvisorModules(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	db, err := openRunConcernsDB(ctx, ws, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, pulseReviewLogSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO pulse_review_log
		(module, review_run_id, pulse_run_id, verdict, status, recorded_at) VALUES
		('strategy_auditor','review-a','pulse-a','audit','completed','2026-08-17T01:00:00Z'),
		('goal_advisor','review-b','pulse-b','opportunity','completed','2026-08-17T02:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := ensurePulseReviewLogSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	var canonical, legacy int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pulse_review_log WHERE module='strategic_review'`).Scan(&canonical); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pulse_review_log WHERE module IN ('strategy_auditor','goal_advisor')`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if canonical != 2 || legacy != 0 {
		t.Fatalf("review migration canonical=%d legacy=%d", canonical, legacy)
	}
}

func TestLegacyReviewTableDropsNarrativeColumns(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	db, err := openRunConcernsDB(ctx, ws, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE pulse_review_log (
		_id INTEGER PRIMARY KEY AUTOINCREMENT,
		module TEXT NOT NULL,
		review_run_id TEXT NOT NULL DEFAULT '',
		pulse_run_id TEXT NOT NULL DEFAULT '',
		verdict TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT '',
		artifact_path TEXT NOT NULL DEFAULT '',
		artifact_kind TEXT NOT NULL DEFAULT 'review',
		artifact_markdown TEXT NOT NULL DEFAULT '',
		content_sha256 TEXT NOT NULL DEFAULT '',
		recorded_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO pulse_review_log
		(module, review_run_id, pulse_run_id, verdict, status, artifact_markdown, recorded_at)
		VALUES ('workflow_review','legacy-1','pulse-1','One issue','completed',?,'2026-08-01T00:00:00Z')`,
		"## Verdict\n\nOne issue\n\nCONCERNS: old concern"); err != nil {
		t.Fatal(err)
	}
	if err := ensurePulseReviewLogSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(pulse_review_log)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	for _, removed := range []string{"artifact_path", "artifact_kind", "artifact_markdown", "content_sha256"} {
		if columns[removed] {
			t.Fatalf("legacy narrative column still exists: %s", removed)
		}
	}
	var findingCount int
	if err := db.QueryRowContext(ctx, `SELECT finding_count FROM pulse_review_log WHERE review_run_id='legacy-1'`).Scan(&findingCount); err != nil {
		t.Fatal(err)
	}
	if findingCount != 1 {
		t.Fatalf("migrated finding_count = %d, want 1", findingCount)
	}
}

func TestTypedReviewRequiresCompleteFindingRecord(t *testing.T) {
	ws := concernsWorkspace(t)
	_, err := RecordPulseReviewFinding(context.Background(), ws, "pulse-1", "review-1", PulseReviewFindingInput{Concern: "missing detail", Module: "workflow_review", PulseFindingDetails: PulseFindingDetails{IssueKind: "workflow_issue"}})
	if err == nil || !strings.Contains(err.Error(), "classification") {
		t.Fatalf("expected incomplete finding rejection, got %v", err)
	}
}

func TestRecordPulseReviewForModulesIndexesOneReceiptPerPerspective(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	if err := CompletePulseReview(ctx, ws, []string{"workflow_review", "llm_ops_review"}, "shared-review", "pulse-1", "Both lanes completed.", "completed"); err != nil {
		t.Fatalf("record shared review: %v", err)
	}
	for _, module := range []string{"workflow_review", "llm_ops_review"} {
		record, err := LoadPulseReviewReceiptForRun(ctx, ws, "shared-review", module)
		if err != nil || record.Verdict != "Both lanes completed." {
			t.Fatalf("%s receipt=%#v err=%v", module, record, err)
		}
	}
}

func TestLoadPulseReviewReceiptsNegativeLimitReturnsCompleteHistory(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	for i := 0; i < 27; i++ {
		runID := fmt.Sprintf("pulse-%02d", i)
		if err := CompletePulseReview(ctx, ws, []string{"workflow_review"}, runID, runID, fmt.Sprintf("Review %02d completed.", i), "completed"); err != nil {
			t.Fatalf("record review %d: %v", i, err)
		}
	}
	preview, err := LoadPulseReviewReceipts(ctx, ws, "workflow_review", 10)
	if err != nil {
		t.Fatal(err)
	}
	complete, err := LoadPulseReviewReceipts(ctx, ws, "workflow_review", -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != 10 || len(complete) != 27 {
		t.Fatalf("preview=%d complete=%d", len(preview), len(complete))
	}
}

func TestRequiredBackgroundPulseReviewReceiptControlsCompletion(t *testing.T) {
	ctx := context.Background()
	workspacePath := t.TempDir()
	const reviewRunID = "background-review-session"

	err := requireBackgroundPulseReviewReceipts(ctx, workspacePath, reviewRunID, []string{"llm_ops_review"})
	if err == nil || !strings.Contains(err.Error(), "required llm_ops_review receipt") {
		t.Fatalf("missing receipt error=%v", err)
	}

	if err := CompletePulseReview(ctx, workspacePath, []string{"llm_ops_review"}, reviewRunID, reviewRunID, "Review failed before reconciliation.", "failed"); err != nil {
		t.Fatalf("seed failed review receipt: %v", err)
	}
	err = requireBackgroundPulseReviewReceipts(ctx, workspacePath, reviewRunID, []string{"llm_ops_review"})
	if err == nil || !strings.Contains(err.Error(), "status failed") {
		t.Fatalf("failed receipt error=%v", err)
	}

	if err := CompletePulseReview(ctx, workspacePath, []string{"llm_ops_review"}, reviewRunID, reviewRunID, "Review completed with typed evidence.", "completed"); err != nil {
		t.Fatalf("seed completed review receipt: %v", err)
	}
	if err := requireBackgroundPulseReviewReceipts(ctx, workspacePath, reviewRunID, []string{"llm_ops_review"}); err != nil {
		t.Fatalf("completed receipt rejected: %v", err)
	}

	const strategyRunID = "background-strategy-session"
	if err := CompletePulseReview(ctx, workspacePath, []string{"strategic_review"}, strategyRunID, strategyRunID, "Strategy review completed with typed evidence.", "completed"); err != nil {
		t.Fatalf("seed strategic review receipt: %v", err)
	}
	if err := requireBackgroundPulseReviewReceipts(ctx, workspacePath, strategyRunID, []string{"strategic_review"}); err != nil {
		t.Fatalf("completed strategic receipt rejected: %v", err)
	}
}

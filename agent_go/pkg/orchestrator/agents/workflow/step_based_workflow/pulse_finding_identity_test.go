package step_based_workflow

import (
	"context"
	"testing"
)

func structuredFindingSummary(id, concern string) string {
	return `PULSE_FINDING_JSON: {"concern":"` + concern + `","finding_id":"` + id + `","issue_kind":"workflow_issue","summary":"identity test"}` + "\nCONCERNS: " + concern
}

func TestStructuredFindingIDSurvivesRewordingAndReviewerChange(t *testing.T) {
	ctx := context.Background()
	workspace := concernsWorkspace(t)
	if _, err := RecordRunConcerns(ctx, workspace, "pulse-1", "", "bug_review", ConcernPhaseReview,
		structuredFindingSummary("HARNESS-STABLE-1", "the first wording")); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordRunConcerns(ctx, workspace, "pulse-2", "", "artifact_review", ConcernPhaseReview,
		structuredFindingSummary("HARNESS-STABLE-1", "different words for the same behavior")); err != nil {
		t.Fatal(err)
	}
	concerns, err := LoadOpenRunConcerns(ctx, workspace, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(concerns) != 1 || concerns[0].SeenCount != 2 {
		t.Fatalf("concerns = %+v, want one stable row seen twice", concerns)
	}
}

func TestFindingIdentityMigrationMergesTwinsAndPreservesEvents(t *testing.T) {
	ctx := context.Background()
	workspace := concernsWorkspace(t)
	if _, err := RecordRunConcerns(ctx, workspace, "pulse-1", "", "bug_review", ConcernPhaseReview,
		structuredFindingSummary("HARNESS-MIGRATE-1", "canonical wording")); err != nil {
		t.Fatal(err)
	}
	db, err := openRunConcernsDB(ctx, workspace, false)
	if err != nil || db == nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_pulse_finding_details_finding_id`); err != nil {
		t.Fatal(err)
	}
	old := "legacy-twin-fp"
	if _, err := db.ExecContext(ctx, `INSERT INTO run_concerns
		(fingerprint,step_id,phase,text,first_seen_run,first_seen_at,last_seen_run,last_seen_at,seen_count,status)
		VALUES (?, 'artifact_review', 'review', 'legacy wording', 'pulse-0', '2026-08-01T00:00:00Z', 'pulse-0', '2026-08-01T00:00:00Z', 3, 'open')`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO pulse_finding_details
		(fingerprint,finding_id,issue_kind,target_key,detail_json,source_run_id,updated_at)
		VALUES (?, 'HARNESS-MIGRATE-1', 'workflow_issue', '', '{}', 'pulse-0', '2026-08-01T00:00:00Z')`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO pulse_finding_events
		(fingerprint,pulse_run_id,event_type,summary,recorded_at) VALUES (?, 'pulse-1', 'filed', 'legacy event', '2026-08-01T00:00:00Z')`, old); err != nil {
		t.Fatal(err)
	}
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	var concerns, seen, events int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(seen_count),0) FROM run_concerns`).Scan(&concerns, &seen); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pulse_finding_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if concerns != 1 || seen != 4 || events != 2 {
		t.Fatalf("after migration concerns=%d seen=%d events=%d, want 1/4/2", concerns, seen, events)
	}
}

func TestFindingIdentityMigrationMovesEventsLeftByPartialMigration(t *testing.T) {
	ctx := context.Background()
	workspace := concernsWorkspace(t)
	if _, err := RecordRunConcerns(ctx, workspace, "pulse-1", "", "bug_review", ConcernPhaseReview,
		structuredFindingSummary("HARNESS-PARTIAL-1", "canonical wording")); err != nil {
		t.Fatal(err)
	}
	db, err := openRunConcernsDB(ctx, workspace, false)
	if err != nil || db == nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	var canonical string
	if err := db.QueryRowContext(ctx, `SELECT fingerprint FROM pulse_finding_details WHERE finding_id='HARNESS-PARTIAL-1'`).Scan(&canonical); err != nil {
		t.Fatal(err)
	}
	const orphan = "old-fingerprint-left-after-partial-migration"
	if _, err := db.ExecContext(ctx, `INSERT INTO pulse_finding_events
		(fingerprint,finding_id,pulse_run_id,event_type,summary,recorded_at)
		VALUES (?, 'HARNESS-PARTIAL-1', 'pulse-0', 'external_action_required', 'old lifecycle event', '2026-08-01T00:00:00Z')`, orphan); err != nil {
		t.Fatal(err)
	}
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	var orphanEvents, canonicalEvents int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pulse_finding_events WHERE fingerprint=?`, orphan).Scan(&orphanEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pulse_finding_events WHERE fingerprint=?`, canonical).Scan(&canonicalEvents); err != nil {
		t.Fatal(err)
	}
	if orphanEvents != 0 || canonicalEvents < 2 {
		t.Fatalf("orphan events=%d canonical events=%d, want 0 and at least 2", orphanEvents, canonicalEvents)
	}
}

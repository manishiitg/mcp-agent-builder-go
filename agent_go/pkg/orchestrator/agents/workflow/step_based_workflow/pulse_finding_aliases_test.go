package step_based_workflow

import (
	"database/sql"
	"strings"
	"testing"
)

func legacyHarnessIdentityFixture(t *testing.T) (string, PulseReviewFindingInput, *sql.DB) {
	t.Helper()
	ctx := t.Context()
	workspace := concernsWorkspace(t)
	input := PulseReviewFindingInput{
		Module: "technical_review", StepID: "example-step", Concern: "Original symptom",
		PulseFindingDetails: PulseFindingDetails{
			IssueKind: IssueKindHarness, TargetKey: "test:shared-target",
			Classification: "correctness_bug", Severity: "medium", Summary: "Original description",
			Impact: "Receipt identity cannot be found", Evidence: []string{"isolated fixture"},
			Reproduction: PulseFindingReproduction{Expected: "Stable returned identity", Observed: "Legacy identity shares target"},
		},
	}
	first, err := RecordPulseReviewFinding(ctx, workspace, "pulse-1", "review-1", input)
	if err != nil {
		t.Fatal(err)
	}
	db, err := openRunConcernsDB(ctx, workspace, false)
	if err != nil || db == nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// A pre-canonicalization row has a different fingerprint from today's hash.
	for _, table := range []string{"run_concerns", "pulse_finding_details", "pulse_finding_events"} {
		if _, err := db.ExecContext(ctx, "UPDATE "+table+" SET fingerprint=? WHERE fingerprint=?", "1111111111111111", first.Fingerprint); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE run_concerns SET issue_id='PUL-11111111' WHERE fingerprint='1111111111111111'"); err != nil {
		t.Fatal(err)
	}
	return workspace, input, db
}

func TestRecordHarnessFindingReusesLegacyIdentityBeforeReturning(t *testing.T) {
	workspace, input, db := legacyHarnessIdentityFixture(t)
	input.Concern = "Same target, newly worded observation"
	input.TargetKey = " TEST:SHARED-TARGET "
	record, err := RecordPulseReviewFinding(t.Context(), workspace, "pulse-2", "review-2", input)
	if err != nil {
		t.Fatal(err)
	}
	if record.IssueID != "PUL-11111111" {
		t.Fatalf("returned a new disposable identity: %+v", record)
	}
	for i := 0; i < 2; i++ {
		if _, err := LoadPulseFindingLifecycles(t.Context(), workspace, "", -1); err != nil {
			t.Fatal(err)
		}
		finding, err := ResolvePulseFindingIssueID(t.Context(), workspace, record.IssueID)
		if err != nil || finding.Fingerprint != record.Fingerprint {
			t.Fatalf("receipt no longer resolves: %+v, %v", finding, err)
		}
	}
	for _, table := range []string{"run_concerns", "pulse_finding_details", "pulse_finding_events"} {
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table+" WHERE fingerprint=?", record.Fingerprint).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Fatalf("no persisted %s row for receipt", table)
		}
	}
}

func insertLegacyHarnessTwin(t *testing.T, db *sql.DB, fingerprint, issueID string) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `INSERT INTO run_concerns
		(fingerprint,issue_id,step_id,phase,text,first_seen_run,first_seen_at,last_seen_run,last_seen_at,seen_count,status)
		VALUES (?,?,'example-step','review','Legacy twin','old-run','2026-08-01','old-run','2026-08-01',1,'open')`, fingerprint, issueID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(t.Context(), `INSERT INTO pulse_finding_details
		(fingerprint,finding_id,issue_kind,target_key,detail_json,source_run_id,updated_at)
		VALUES (?,'','harness_issue','test:shared-target','{}','old-run','2026-08-01')`, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(t.Context(), `INSERT INTO pulse_finding_events
		(fingerprint,pulse_run_id,event_type,summary,recorded_at) VALUES (?,'old-run','filed',?,'2026-08-01')`, fingerprint, issueID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticFindingMergePreservesReturnedIDsAcrossFurtherMerges(t *testing.T) {
	workspace, input, db := legacyHarnessIdentityFixture(t)
	insertLegacyHarnessTwin(t, db, "ffffffffffffffff", "PUL-FFFFFFFF")
	for _, expected := range []string{"1111111111111111", "0000000000000000"} {
		if expected == "0000000000000000" {
			insertLegacyHarnessTwin(t, db, expected, "PUL-00000000")
		}
		if _, err := LoadPulseFindingLifecycles(t.Context(), workspace, "", -1); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"pul-ffffffff", "PUL-11111111"} {
			finding, err := ResolvePulseFindingIssueID(t.Context(), workspace, id)
			if err != nil || finding.Fingerprint != expected {
				t.Fatalf("alias %s => %+v, %v; want %s", id, finding, err, expected)
			}
		}
	}
	input.IssueID = "PUL-FFFFFFFF"
	input.Summary = "Updated through an old public ID"
	record, err := RecordPulseReviewFinding(t.Context(), workspace, "pulse-3", "review-3", input)
	if err != nil || record.IssueID != "PUL-00000000" {
		t.Fatalf("update via alias: %+v, %v", record, err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM run_concerns").Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate lifecycle rows: %d, %v", count, err)
	}
	var details string
	if err := db.QueryRowContext(t.Context(), "SELECT detail_json FROM pulse_finding_details WHERE fingerprint=?", record.Fingerprint).Scan(&details); err != nil || !strings.Contains(details, input.Summary) {
		t.Fatalf("alias update not persisted: %s, %v", details, err)
	}
	if _, err := ResolvePulseFindingIssueID(t.Context(), workspace, "PUL-NOT-REAL"); err == nil {
		t.Fatal("unknown ID resolved")
	}
	var eventCount int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM pulse_finding_events WHERE fingerprint=?", record.Fingerprint).Scan(&eventCount); err != nil || eventCount < 4 {
		t.Fatalf("lost event evidence: %d, %v", eventCount, err)
	}
}

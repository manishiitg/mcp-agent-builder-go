package step_based_workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func concernsWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	ws := "Workflow/testing"
	if err := os.MkdirAll(filepath.Join(root, "Workflow", "testing", "db"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return ws
}

func TestParseConcernLinesExtractsPayloadOnly(t *testing.T) {
	summary := "Did the work.\nCONCERNS: step description says 1% but soul.md says 1.5%\nSTATUS: COMPLETED"
	got := ParseConcernLines(summary)
	if len(got) != 1 || got[0] != "step description says 1% but soul.md says 1.5%" {
		t.Fatalf("got %#v", got)
	}
	// A bare marker carries nothing; recording it would create a permanent
	// fingerprinted row with no content.
	if got := ParseConcernLines("CONCERNS:\nSTATUS: COMPLETED"); len(got) != 0 {
		t.Fatalf("empty payload must be ignored, got %#v", got)
	}
	if got := ParseConcernLines("all good\nSTATUS: COMPLETED"); len(got) != 0 {
		t.Fatalf("no marker should yield nothing, got %#v", got)
	}
}

// Magnitude is the interesting part of a recurring concern, so two reports that
// differ only in a number must stay distinct rows.
func TestConcernFingerprintKeepsDigitsButIgnoresFormatting(t *testing.T) {
	a := concernFingerprint("s1", "12 learnings items stale")
	b := concernFingerprint("s1", "3 learnings items stale")
	if a == b {
		t.Fatal("differing counts must not collapse to one fingerprint")
	}
	spaced := concernFingerprint("s1", "  12   LEARNINGS   items stale ")
	if a != spaced {
		t.Fatal("case and whitespace must not create a new fingerprint")
	}
	if concernFingerprint("s1", "same text") == concernFingerprint("s2", "same text") {
		t.Fatal("same text from different steps must not merge")
	}
}

// The whole point of the table: a concern reported every run collapses to one
// row whose seen_count is the signal. Twelve rows would bury it.
func TestRecordRunConcernsDedupesAndCountsRecurrence(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	summary := "CONCERNS: db_health rows have NULL pnl_inr\nSTATUS: COMPLETED"

	for i := 0; i < 3; i++ {
		if _, err := RecordRunConcerns(ctx, ws, "iteration-0/g", "g", "step-a", ConcernPhaseExecution, summary); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	open, err := LoadOpenRunConcerns(ctx, ws, 10)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected 1 deduped row, got %d: %#v", len(open), open)
	}
	if open[0].SeenCount != 3 {
		t.Fatalf("seen_count = %d, want 3", open[0].SeenCount)
	}
	if open[0].Phase != ConcernPhaseExecution || open[0].StepID != "step-a" {
		t.Fatalf("attribution lost: %#v", open[0])
	}
}

// A fix that did not hold is more important than the original report, so a
// resolved concern that comes back reopens. A rejected one does not: someone
// judged it a non-issue and recurrence is not new evidence against that.
func TestRecurrenceReopensResolvedButNotRejected(t *testing.T) {
	ctx := context.Background()
	summary := "CONCERNS: selector drifted again\nSTATUS: COMPLETED"

	for _, tc := range []struct {
		name       string
		status     string
		wantStatus string
	}{
		{"resolved reopens", ConcernStatusResolved, ConcernStatusOpen},
		{"rejected stays closed", ConcernStatusRejected, ConcernStatusRejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := concernsWorkspace(t)
			if _, err := RecordRunConcerns(ctx, ws, "run-1", "g", "step-a", ConcernPhaseExecution, summary); err != nil {
				t.Fatalf("record: %v", err)
			}
			open, _ := LoadOpenRunConcerns(ctx, ws, 10)
			if len(open) != 1 {
				t.Fatalf("setup: expected 1 row, got %d", len(open))
			}
			if err := ResolveRunConcern(ctx, ws, open[0].Fingerprint, tc.status, "pulse_fixer", "note"); err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if _, err := RecordRunConcerns(ctx, ws, "run-2", "g", "step-a", ConcernPhaseExecution, summary); err != nil {
				t.Fatalf("re-record: %v", err)
			}
			reloaded, _ := LoadOpenRunConcerns(ctx, ws, 10)
			if tc.wantStatus == ConcernStatusOpen {
				if len(reloaded) != 1 || reloaded[0].Status != ConcernStatusOpen {
					t.Fatalf("expected reopened row, got %#v", reloaded)
				}
			} else if len(reloaded) != 0 {
				t.Fatalf("rejected concern must not resurface, got %#v", reloaded)
			}
		})
	}
}

func TestLoadOpenRunConcernsRanksByRecurrence(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		_, _ = RecordRunConcerns(ctx, ws, "r", "g", "step-a", ConcernPhaseExecution, "CONCERNS: seen twice")
	}
	_, _ = RecordRunConcerns(ctx, ws, "r", "g", "step-b", ConcernPhaseLearnings, "CONCERNS: seen once")

	open, err := LoadOpenRunConcerns(ctx, ws, 10)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(open) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(open))
	}
	if !strings.Contains(open[0].Text, "seen twice") {
		t.Fatalf("most-recurring must rank first, got %q", open[0].Text)
	}
}

func TestLoadOpenRunConcernsNegativeLimitReturnsCompleteBacklog(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	for i := 0; i < 63; i++ {
		summary := fmt.Sprintf("CONCERNS: distinct backlog concern %02d", i)
		if _, err := RecordRunConcerns(ctx, ws, "run", "g", "step-a", ConcernPhaseExecution, summary); err != nil {
			t.Fatalf("record concern %d: %v", i, err)
		}
	}

	preview, err := LoadOpenRunConcerns(ctx, ws, 25)
	if err != nil {
		t.Fatalf("load preview: %v", err)
	}
	complete, err := LoadOpenRunConcerns(ctx, ws, -1)
	if err != nil {
		t.Fatalf("load complete backlog: %v", err)
	}
	if len(preview) != 25 || len(complete) != 63 {
		t.Fatalf("preview=%d complete=%d, want 25 and 63", len(preview), len(complete))
	}
}

// A workflow that has never raised a concern has no table; that must read as
// "nothing to report", not as an error the Gate has to handle.
func TestLoadOpenRunConcernsQuietWhenNeverUsed(t *testing.T) {
	ws := concernsWorkspace(t)
	got, err := LoadOpenRunConcerns(context.Background(), ws, 10)
	if err != nil {
		t.Fatalf("missing table should not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no concerns, got %#v", got)
	}
}

func TestResolveRunConcernRejectsUnknownStatus(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	_, _ = RecordRunConcerns(ctx, ws, "r", "g", "step-a", ConcernPhaseExecution, "CONCERNS: something")
	open, _ := LoadOpenRunConcerns(ctx, ws, 10)
	if err := ResolveRunConcern(ctx, ws, open[0].Fingerprint, "ignored", "x", ""); err == nil {
		t.Fatal("unknown status must be rejected")
	}
}

// Reviewer artifacts are per-run files that nothing diffs across runs, so a
// finding repeated every cycle reads as new every cycle. Reviewer concerns go
// through the same fingerprinting as step concerns, keyed by module.
func TestReviewerConcernsDedupeByModuleAcrossRuns(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	artifact := "## Findings\nStored-data integrity issue found.\n\nCONCERNS: ASTERDM/ATUL rows have NULL pnl_inr\n"

	for _, run := range []string{"pulse-1", "pulse-2"} {
		if _, err := RecordRunConcerns(ctx, ws, run, "", "db_health", ConcernPhaseReview, artifact); err != nil {
			t.Fatalf("record %s: %v", run, err)
		}
	}
	// A different module reporting the same sentence is a separate finding.
	if _, err := RecordRunConcerns(ctx, ws, "pulse-2", "", "bug_review", ConcernPhaseReview, artifact); err != nil {
		t.Fatalf("record other module: %v", err)
	}

	open, err := LoadOpenRunConcerns(ctx, ws, 10)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(open) != 2 {
		t.Fatalf("expected 2 rows (one per module), got %d: %#v", len(open), open)
	}
	if open[0].SeenCount != 2 || open[0].StepID != "db_health" {
		t.Fatalf("recurring module finding should rank first with count 2, got %#v", open[0])
	}
	if open[0].Phase != ConcernPhaseReview {
		t.Fatalf("phase = %q, want %q", open[0].Phase, ConcernPhaseReview)
	}
}

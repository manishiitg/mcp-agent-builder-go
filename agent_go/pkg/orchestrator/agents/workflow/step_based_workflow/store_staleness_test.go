package step_based_workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// evidenceWorkspace builds a workflow tree with a learnings freshness ledger
// confirmed at the given time, plus optional changelog entries and soul.md.
func evidenceWorkspace(t *testing.T, confirmedAt string, changelogJSON string, withSoul bool) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	ws := "Workflow/testing"
	base := filepath.Join(root, "Workflow", "testing")

	learnDir := filepath.Join(base, "learnings", GlobalLearningID)
	if err := os.MkdirAll(learnDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ledger := `{"store":"learnings","last_confirmed_at":"` + confirmedAt + `","last_confirmed_run":"iteration-0/g","items":{"references/auth-flow.md":{"hash":"x","last_confirmed_at":"` + confirmedAt + `"}}}`
	if err := os.WriteFile(filepath.Join(learnDir, "_freshness.json"), []byte(ledger), 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}

	if changelogJSON != "" {
		clDir := filepath.Join(base, PlanningFolderName, "changelog")
		if err := os.MkdirAll(clDir, 0o755); err != nil {
			t.Fatalf("mkdir changelog: %v", err)
		}
		if err := os.WriteFile(filepath.Join(clDir, "changelog-test.json"), []byte(changelogJSON), 0o644); err != nil {
			t.Fatalf("write changelog: %v", err)
		}
	}
	if withSoul {
		soulDir := filepath.Join(base, SoulFolderName)
		if err := os.MkdirAll(soulDir, 0o755); err != nil {
			t.Fatalf("mkdir soul: %v", err)
		}
		p := filepath.Join(soulDir, SoulFileName)
		if err := os.WriteFile(p, []byte("# Soul\n\n## Objective\nDo the thing.\n"), 0o644); err != nil {
			t.Fatalf("write soul: %v", err)
		}
		// Force an mtime after the confirmation so it counts as a later edit.
		later := time.Now().Add(time.Hour)
		if err := os.Chtimes(p, later, later); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	return ws
}

const changelogAfter = `{"entries":[
 {"timestamp":"2026-07-22T09:00:00Z","tool":"update_regular_step","reason":"retire the regression pass","step_ids":["step-a"],"changes":[{"step_id":"step-a","field":"description","old_value":"old","new_value":"new"}]}
]}`

// The evidence must carry WHAT changed, not just that something did. A reviewer
// cannot tell a cosmetic edit from one that invalidates an auth flow without the
// reason and the changed field names.
func TestCollectStoreEditEvidenceReportsEditsWithDetail(t *testing.T) {
	ws := evidenceWorkspace(t, "2026-07-19T04:01:33Z", changelogAfter, false)
	got := CollectStoreEditEvidence(ws)
	if len(got) != 1 || got[0].Store != "learnings" {
		t.Fatalf("expected one learnings entry, got %#v", got)
	}
	joined := strings.Join(got[0].EditsSince, "\n")
	for _, want := range []string{"update_regular_step", "retire the regression pass", "step-a", "description"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("evidence missing %q:\n%s", want, joined)
		}
	}
	// It must not read as a verdict — the judgement is the reviewer's.
	if !strings.Contains(got[0].Note, "NOT a verdict") {
		t.Fatalf("note should disclaim judgement, got %q", got[0].Note)
	}
}

// Edits made BEFORE the store was confirmed have already been accounted for by
// whoever confirmed it. Reporting them would make the signal permanent noise.
func TestCollectStoreEditEvidenceIgnoresEditsBeforeConfirmation(t *testing.T) {
	ws := evidenceWorkspace(t, "2026-07-25T00:00:00Z", changelogAfter, false)
	if got := CollectStoreEditEvidence(ws); len(got) != 0 {
		t.Fatalf("edits predating confirmation must not surface, got %#v", got)
	}
}

// A workflow whose plan has not changed since its learnings were confirmed is the
// healthy case and must contribute nothing to Pulse's context.
func TestCollectStoreEditEvidenceQuietWhenNothingChanged(t *testing.T) {
	ws := evidenceWorkspace(t, "2026-07-19T04:01:33Z", "", false)
	if got := CollectStoreEditEvidence(ws); len(got) != 0 {
		t.Fatalf("no edits should mean no evidence, got %#v", got)
	}
}

// soul.md has no changelog entry, so its mtime is the only record that the
// objective or constraints may have moved.
func TestCollectStoreEditEvidenceIncludesSoulEdit(t *testing.T) {
	ws := evidenceWorkspace(t, "2026-07-19T04:01:33Z", "", true)
	got := CollectStoreEditEvidence(ws)
	if len(got) != 1 {
		t.Fatalf("expected soul edit to surface, got %#v", got)
	}
	joined := strings.Join(got[0].EditsSince, "\n")
	if !strings.Contains(joined, "soul.md modified") {
		t.Fatalf("missing soul edit:\n%s", joined)
	}
	// mtime is blunt, so the evidence must say so rather than implying certainty.
	if !strings.Contains(joined, "mtime moves on any save") {
		t.Fatalf("soul evidence should flag mtime's bluntness:\n%s", joined)
	}
}

// A store with content but no confirmation baseline has never been reviewed by
// any run. That is worth surfacing in its own right, not skipping silently.
func TestCollectStoreEditEvidenceReportsMissingBaseline(t *testing.T) {
	ws := evidenceWorkspace(t, "", "", false)
	got := CollectStoreEditEvidence(ws)
	if len(got) != 1 || !strings.Contains(got[0].Note, "no confirmation baseline") {
		t.Fatalf("expected a missing-baseline note, got %#v", got)
	}
}

// This evidence ships into Pulse's decision context, so a workflow with a long
// edit history must not flood it.
func TestPlanEditListIsCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"entries":[`)
	for i := 0; i < 15; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"timestamp":"2026-07-20T09:0` + string(rune('0'+i%10)) + `:00Z","tool":"update_step_config","reason":"r"}`)
	}
	b.WriteString(`]}`)

	ws := evidenceWorkspace(t, "2026-07-19T00:00:00Z", b.String(), false)
	got := CollectStoreEditEvidence(ws)
	if len(got) != 1 {
		t.Fatalf("expected evidence, got %#v", got)
	}
	if len(got[0].EditsSince) > maxListedEdits+1 {
		t.Fatalf("edit list not capped: %d entries", len(got[0].EditsSince))
	}
	if !strings.Contains(strings.Join(got[0].EditsSince, "\n"), "planning/changelog/") {
		t.Fatal("truncation should point at the full record")
	}
}

package step_based_workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

// emptyChangesRefHash is sha256("[]") — the placeholder hash every managed
// mutation collapsed to when it left Changes unset (PLAT-033 evidence).
const emptyChangesRefHash = "sha256:4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"

// TestCompletePlanChangelogEntryPrefersSnapshotOverEmptyChanges is the core
// PLAT-033 regression: a caller with no Changes list (the exact shape every
// reproduced offending entry had) must not collapse before_ref/after_ref to
// the same sha256("[]") when it actually supplies real before/after
// artifact content.
func TestCompletePlanChangelogEntryPrefersSnapshotOverEmptyChanges(t *testing.T) {
	entry := PlanChangelogEntry{
		Tool:           "update_step_config",
		BeforeSnapshot: map[string]interface{}{"execution_tier": "medium"},
		AfterSnapshot:  map[string]interface{}{"execution_tier": "high"},
	}
	completePlanChangelogEntry(&entry)

	if entry.BeforeRef == emptyChangesRefHash || entry.AfterRef == emptyChangesRefHash {
		t.Fatalf("refs still collapsed to the empty-array placeholder: before=%q after=%q", entry.BeforeRef, entry.AfterRef)
	}
	if entry.BeforeRef == entry.AfterRef {
		t.Fatalf("before_ref == after_ref (%q) despite different snapshots", entry.BeforeRef)
	}
	if entry.BeforeRef == "" || entry.AfterRef == "" {
		t.Fatalf("expected non-empty refs, got before=%q after=%q", entry.BeforeRef, entry.AfterRef)
	}
}

// TestCompletePlanChangelogEntryMarksNoOpOnlyWhenBothSnapshotsAreReal proves
// the no_op flag is only set when the writer actually knows both states —
// an empty Changes list with no snapshot must stay "unknown," not silently
// become "confirmed no-op" (which would hide the very drift PLAT-033 was
// filed about).
func TestCompletePlanChangelogEntryMarksNoOpOnlyWhenBothSnapshotsAreReal(t *testing.T) {
	t.Run("real identical snapshots are an explicit no-op", func(t *testing.T) {
		entry := PlanChangelogEntry{
			BeforeSnapshot: map[string]interface{}{"a": 1},
			AfterSnapshot:  map[string]interface{}{"a": 1},
		}
		completePlanChangelogEntry(&entry)
		if !entry.NoOp {
			t.Fatal("identical real snapshots should be marked no_op")
		}
	})

	t.Run("no snapshots at all is not claimed as a no-op", func(t *testing.T) {
		entry := PlanChangelogEntry{}
		completePlanChangelogEntry(&entry)
		if entry.NoOp {
			t.Fatal("an entry with no snapshot evidence must not claim no_op")
		}
	})

	t.Run("real different snapshots are not a no-op", func(t *testing.T) {
		entry := PlanChangelogEntry{
			BeforeSnapshot: map[string]interface{}{"a": 1},
			AfterSnapshot:  map[string]interface{}{"a": 2},
		}
		completePlanChangelogEntry(&entry)
		if entry.NoOp {
			t.Fatal("differing snapshots must not be marked no_op")
		}
	})
}

// TestCompletePlanChangelogEntryUnchangedForCallersWithoutSnapshots is a
// backward-compatibility guard: the ~14 existing callers that only ever set
// Changes (never BeforeSnapshot/AfterSnapshot) must keep computing refs
// exactly as before this fix.
func TestCompletePlanChangelogEntryUnchangedForCallersWithoutSnapshots(t *testing.T) {
	entry := PlanChangelogEntry{
		Changes: []PlanFieldChange{{StepID: "step-1", Field: "title", OldValue: "old", NewValue: "new"}},
	}
	completePlanChangelogEntry(&entry)

	want := PlanChangelogEntry{
		Changes: []PlanFieldChange{{StepID: "step-1", Field: "title", OldValue: "old", NewValue: "new"}},
	}
	completePlanChangelogEntry(&want)
	if entry.BeforeRef != want.BeforeRef || entry.AfterRef != want.AfterRef {
		t.Fatal("ref computation for a Changes-only entry is not stable")
	}
	if entry.BeforeRef == entry.AfterRef {
		t.Fatal("a real field change must not produce matching before/after refs")
	}
}

// TestLogCanonicalArtifactChangeUsesExplicitTargetAndSnapshotRefs is the
// PLAT-033 regression for the write_workflow_manifest evidence: a caller
// with the real pre/post artifact bytes on hand (a direct file write) must
// have those hashed for before_ref/after_ref, and the explicit target must
// override the tool-name-based guess (which put write_workflow_manifest
// entries under planning/plan.json instead of workflow.json).
func TestLogCanonicalArtifactChangeUsesExplicitTargetAndSnapshotRefs(t *testing.T) {
	var persisted string
	readFile := func(context.Context, string) (string, error) { return "", errors.New("not found") }
	writeFile := func(_ context.Context, _ string, content string) error {
		persisted = content
		return nil
	}

	LogCanonicalArtifactChange(
		context.Background(), "Workflow/demo", "write_workflow_manifest",
		"workflow.json was written directly",
		nil, readFile, writeFile, loggerv2.NewNoop(),
		"workflow.json", `{"name":"before"}`, `{"name":"after"}`,
	)

	var clog PlanChangelog
	if err := json.Unmarshal([]byte(persisted), &clog); err != nil {
		t.Fatalf("persisted changelog did not parse: %v", err)
	}
	if len(clog.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(clog.Entries))
	}
	got := clog.Entries[0]
	if got.Target != "workflow.json" {
		t.Fatalf("target = %q, want workflow.json (not the planning/plan.json default)", got.Target)
	}
	if got.BeforeRef == emptyChangesRefHash || got.AfterRef == emptyChangesRefHash {
		t.Fatalf("refs collapsed to the empty-array placeholder: before=%q after=%q", got.BeforeRef, got.AfterRef)
	}
	if got.BeforeRef == got.AfterRef {
		t.Fatalf("before_ref == after_ref (%q) despite different manifest content", got.BeforeRef)
	}
}

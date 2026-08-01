package step_based_workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

func changelogWorkspace(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	dir := filepath.Join(root, "Workflow", "testing", PlanningFolderName, "changelog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return "Workflow/testing"
}

const mixedChangelog = `{"entries":[
 {"timestamp":"2026-07-22T09:00:00Z","tool":"update_regular_step","reason":"retire the regression pass","step_ids":["step-a"],"changes":[{"step_id":"step-a","field":"description","old_value":"o","new_value":"n"}]},
 {"timestamp":"2026-07-21T09:00:00Z","tool":"update_step_config","reason":"already reconciled","step_ids":["step-b"],"artifact_review":{"done":true,"reviewed_by":"pulse_fixer"}},
 {"timestamp":"2026-07-20T09:00:00Z","tool":"update_validation_schema","reason":"tighten output","step_ids":["step-c"],"changes":[{"step_id":"step-c","field":"validation_schema"}]}
]}`

// The backlog must be exactly the entries nobody has stamped. A stamped entry is
// reconciled work and re-reporting it would make the count meaningless.
func TestCollectPlanChangeBacklogExcludesReviewedEntries(t *testing.T) {
	ws := changelogWorkspace(t, map[string]string{"changelog-a.json": mixedChangelog})
	got := CollectPlanChangeBacklog(ws)
	if got == nil {
		t.Fatal("expected a backlog")
	}
	if got.UnreviewedCount != 2 {
		t.Fatalf("unreviewed count = %d, want 2 (the third is stamped done)", got.UnreviewedCount)
	}
	for _, c := range got.Changes {
		if c.Reason == "already reconciled" {
			t.Fatalf("stamped entry leaked into the backlog: %#v", c)
		}
	}
}

// Newest first, and each entry must carry the surface a reviewer traces: the
// reason, the steps, and which fields moved.
func TestCollectPlanChangeBacklogCarriesTriageDetail(t *testing.T) {
	ws := changelogWorkspace(t, map[string]string{"changelog-a.json": mixedChangelog})
	got := CollectPlanChangeBacklog(ws)
	if got.Changes[0].At != "2026-07-22T09:00:00Z" {
		t.Fatalf("expected newest first, got %q", got.Changes[0].At)
	}
	first := got.Changes[0]
	if first.Reason != "retire the regression pass" || len(first.StepIDs) != 1 || first.StepIDs[0] != "step-a" {
		t.Fatalf("triage detail missing: %#v", first)
	}
	if len(first.FieldsChanged) != 1 || first.FieldsChanged[0] != "description" {
		t.Fatalf("changed fields missing: %#v", first)
	}
	if first.SourceFile != "changelog-a.json" {
		t.Fatalf("source file missing: %#v", first)
	}
	// It must read as evidence, not as a claim that something is broken.
	if !strings.Contains(got.Note, "not a verdict") {
		t.Fatalf("note should disclaim judgement: %q", got.Note)
	}
}

// A workflow that has reconciled everything is the healthy case and must add
// nothing to Pulse's context.
func TestCollectPlanChangeBacklogNilWhenAllReviewed(t *testing.T) {
	allDone := `{"entries":[{"timestamp":"2026-07-22T09:00:00Z","tool":"update_regular_step","reason":"r","artifact_review":{"done":true}}]}`
	ws := changelogWorkspace(t, map[string]string{"changelog-a.json": allDone})
	if got := CollectPlanChangeBacklog(ws); got != nil {
		t.Fatalf("fully reconciled workflow should produce no backlog, got %#v", got)
	}
}

func TestCollectPlanChangeBacklogNilWhenNoChangelog(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	if got := CollectPlanChangeBacklog("Workflow/testing"); got != nil {
		t.Fatalf("missing changelog should produce no backlog, got %#v", got)
	}
}

// One unreadable file must not hide the outstanding changes recorded in the
// others — the count would silently understate the backlog.
func TestCollectPlanChangeBacklogSurvivesMalformedFile(t *testing.T) {
	ws := changelogWorkspace(t, map[string]string{
		"changelog-a.json": mixedChangelog,
		"changelog-b.json": "{not json",
	})
	got := CollectPlanChangeBacklog(ws)
	if got == nil || got.UnreviewedCount != 2 {
		t.Fatalf("malformed sibling should not suppress the real backlog, got %#v", got)
	}
}

// An entry with an unparseable timestamp is still outstanding work; dropping it
// would lose a real change because of a formatting problem.
func TestCollectPlanChangeBacklogKeepsUndateableEntries(t *testing.T) {
	bad := `{"entries":[{"timestamp":"whenever","tool":"update_regular_step","reason":"no date"}]}`
	ws := changelogWorkspace(t, map[string]string{"changelog-a.json": bad})
	got := CollectPlanChangeBacklog(ws)
	if got == nil || got.UnreviewedCount != 1 {
		t.Fatalf("undateable entry should still count, got %#v", got)
	}
}

// The count is always exact; only the listing is capped, and it must say so.
func TestCollectPlanChangeBacklogCapsListingNotCount(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"entries":[`)
	for i := 0; i < 20; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"timestamp":"2026-07-20T09:00:0` + string(rune('0'+i%10)) + `Z","tool":"update_step_config","reason":"r"}`)
	}
	b.WriteString(`]}`)

	ws := changelogWorkspace(t, map[string]string{"changelog-a.json": b.String()})
	got := CollectPlanChangeBacklog(ws)
	if got.UnreviewedCount != 20 {
		t.Fatalf("count must be exact, got %d", got.UnreviewedCount)
	}
	if len(got.Changes) != maxListedPlanChanges {
		t.Fatalf("listing should cap at %d, got %d", maxListedPlanChanges, len(got.Changes))
	}
	if !strings.Contains(got.Note, "Showing the") {
		t.Fatalf("note must disclose truncation: %q", got.Note)
	}
}

// TestCanonicalArtifactDriftRecordsChangeAndThenStaysQuiet covers the gap
// AR-20260729-2 reported: evaluation_plan.json has no plan-modification tool, so
// every edit arrived by direct write and left nothing in the changelog for
// Artifact Review to see.
func TestCanonicalArtifactDriftRecordsChangeAndThenStaysQuiet(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"

	files := map[string]string{}
	read := func(_ context.Context, path string) (string, error) { return files[path], nil }
	write := func(_ context.Context, path, content string) error {
		files[path] = content
		abs := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path, "")))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		return os.WriteFile(abs, []byte(content), 0o644)
	}
	logger := loggerv2.NewNoop()

	entryCount := func() int {
		total := 0
		for _, hashes := range []map[string]string{lastRecordedArtifactHashes(workspacePath)} {
			total += len(hashes)
		}
		return total
	}

	// A direct write nobody recorded.
	files[workspacePath+"/evaluation/evaluation_plan.json"] = `{"steps":[{"id":"eval-a"}]}`
	RecordCanonicalArtifactDrift(ctx, workspacePath, read, write, logger)
	if entryCount() != 1 {
		t.Fatalf("first write was not recorded; drift review would still see nothing")
	}
	first := lastRecordedArtifactHashes(workspacePath)["evaluation/evaluation_plan.json"]

	// Unchanged: must not append a no-op entry that buries the real ones.
	RecordCanonicalArtifactDrift(ctx, workspacePath, read, write, logger)
	if got := lastRecordedArtifactHashes(workspacePath)["evaluation/evaluation_plan.json"]; got != first {
		t.Fatalf("an unchanged artifact produced a new hash %q, want %q", got, first)
	}

	// Changed again by direct write: recorded, with the new hash.
	files[workspacePath+"/evaluation/evaluation_plan.json"] = `{"steps":[{"id":"eval-a"},{"id":"eval-b"}]}`
	RecordCanonicalArtifactDrift(ctx, workspacePath, read, write, logger)
	if got := lastRecordedArtifactHashes(workspacePath)["evaluation/evaluation_plan.json"]; got == first || got == "" {
		t.Fatalf("a second direct edit was not recorded (hash still %q)", got)
	}
}

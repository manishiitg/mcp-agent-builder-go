package step_based_workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyPulseReviewsConvertsAndRemovesSources(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/demo"
	reviewDir := filepath.Join(root, "Workflow", "demo", "pulse", "reviews", "legacy-run")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatalf("create legacy review dir: %v", err)
	}
	reviewMarkdown := "# Pulse reviewer result\n\n" +
		"- Pulse run: `pulse-1`\n" +
		"- Review run: `legacy-run`\n" +
		"- Module: `knowledgebase_health`\n" +
		"- Status: `completed`\n" +
		"- Completed at: `2026-07-30T10:11:12.123Z`\n\n" +
		"## Verdict\n\nA stale selector still limits discovery.\n\n" +
		"CONCERNS: stale selector still limits discovery\n"
	reviewPath := filepath.Join(reviewDir, "knowledgebase_health.md")
	if err := os.WriteFile(reviewPath, []byte(reviewMarkdown), 0o644); err != nil {
		t.Fatalf("write legacy review: %v", err)
	}
	learningMarkdown := "# Learning health\n\n" +
		"- Pulse run: `pulse-1`\n" +
		"- Review run: `legacy-run`\n" +
		"- Status: `completed`\n" +
		"- Completed at: `2026-07-30T10:12:12.123Z`\n\n" +
		"## Verdict\n\nLearning artifacts are healthy.\n"
	learningPath := filepath.Join(reviewDir, "learning_health.md")
	if err := os.WriteFile(learningPath, []byte(learningMarkdown), 0o644); err != nil {
		t.Fatalf("write legacy learning review: %v", err)
	}
	packetMarkdown := "# Evidence packet\n\nInternal supporting evidence.\n"
	packetPath := filepath.Join(reviewDir, "bug_review.packet.md")
	if err := os.WriteFile(packetPath, []byte(packetMarkdown), 0o644); err != nil {
		t.Fatalf("write legacy packet: %v", err)
	}

	first, err := MigrateLegacyPulseReviews(context.Background(), workspacePath)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.FilesFound != 3 || first.ReviewReceipts != 2 || first.AuxiliaryFiles != 1 ||
		first.ConcernOccurrences != 1 || first.FilesRemoved != 3 {
		t.Fatalf("first import result = %+v", first)
	}
	for _, path := range []string{reviewPath, learningPath, packetPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("legacy source still exists %s: %v", path, err)
		}
	}

	reviews, err := LoadPulseReviewReceipts(context.Background(), workspacePath, "workflow_review", 10)
	if err != nil {
		t.Fatalf("load imported review: %v", err)
	}
	if len(reviews) != 2 {
		t.Fatalf("imported reviews = %+v, want both historical store reviews consolidated under Engineering Review", reviews)
	}
	reviewByVerdict := map[string]PulseReviewReceipt{}
	for _, review := range reviews {
		reviewByVerdict[review.Verdict] = review
	}
	importedKnowledge := reviewByVerdict["A stale selector still limits discovery."]
	importedLearning := reviewByVerdict["Learning artifacts are healthy."]
	if importedKnowledge.Module != "workflow_review" || importedKnowledge.FindingCount != 1 {
		t.Fatalf("knowledge import did not preserve compact receipt metadata: %+v", importedKnowledge)
	}
	if importedLearning.Module != "workflow_review" || importedLearning.FindingCount != 0 {
		t.Fatalf("learning import was overwritten during module consolidation: %+v", importedLearning)
	}

	findings, err := LoadPulseFindingLifecycles(context.Background(), workspacePath, "workflow_review", 10)
	if err != nil {
		t.Fatalf("load imported finding: %v", err)
	}
	if len(findings) != 1 || findings[0].SeenCount != 1 {
		t.Fatalf("imported findings = %+v, want one occurrence", findings)
	}

	second, err := MigrateLegacyPulseReviews(context.Background(), workspacePath)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.FilesFound != 0 || second.ReviewReceipts != 0 ||
		second.AuxiliaryFiles != 0 || second.ConcernOccurrences != 0 || second.FilesRemoved != 0 {
		t.Fatalf("second import result = %+v", second)
	}
	findings, err = LoadPulseFindingLifecycles(context.Background(), workspacePath, "workflow_review", 10)
	if err != nil {
		t.Fatalf("reload imported finding: %v", err)
	}
	if len(findings) != 1 || findings[0].SeenCount != 1 {
		t.Fatalf("idempotent import inflated recurrence: %+v", findings)
	}
}

func TestMigrateLegacyPulseReviewsReportsUnknownMarkdownWithoutDeletingIt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/demo"
	path := filepath.Join(root, "Workflow", "demo", "pulse", "reviews", "legacy-run", "notes.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create legacy review dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("# Notes\n"), 0o644); err != nil {
		t.Fatalf("write unknown legacy file: %v", err)
	}

	result, err := MigrateLegacyPulseReviews(context.Background(), workspacePath)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.UnrecognizedSkipped) != 1 || result.UnrecognizedSkipped[0] != "pulse/reviews/legacy-run/notes.md" {
		t.Fatalf("unrecognized files = %+v", result.UnrecognizedSkipped)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("unknown file should be retained: %v", err)
	}
}

func TestMigrateLegacyPulseReviewsRemovesAlreadyImportedSourceWithoutDuplicatingReceipt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/demo"
	ctx := context.Background()
	if err := CompletePulseReview(ctx, workspacePath, []string{"workflow_review"}, "legacy-run", "pulse-1", "Old issue", "completed"); err != nil {
		t.Fatal(err)
	}
	reviewDir := filepath.Join(root, "Workflow", "demo", "pulse", "reviews", "legacy-run")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reviewPath := filepath.Join(reviewDir, "bug_review.md")
	markdown := "- Review run: `legacy-run`\n- Pulse run: `pulse-1`\n## Verdict\n\nOld issue\n"
	if err := os.WriteFile(reviewPath, []byte(markdown), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, legacyPulseReviewCleanupLedgerSchema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO pulse_review_artifact_imports
		(legacy_path, content_sha256, imported_at) VALUES ('pulse/reviews/legacy-run/bug_review.md','hash','now')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()

	result, err := MigrateLegacyPulseReviews(ctx, workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesRemoved != 1 || result.ReviewReceipts != 0 {
		t.Fatalf("migration result = %+v", result)
	}
	receipts, err := LoadPulseReviewReceipts(ctx, workspacePath, "workflow_review", -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 {
		t.Fatalf("receipts = %+v, want one existing receipt", receipts)
	}
}

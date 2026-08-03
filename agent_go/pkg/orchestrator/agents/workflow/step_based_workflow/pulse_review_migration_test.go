package step_based_workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestImportLegacyPulseReviewArtifactsCopiesExactlyAndIsIdempotent(t *testing.T) {
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

	first, err := ImportLegacyPulseReviewArtifacts(context.Background(), workspacePath)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.FilesFound != 3 || first.ReviewArtifacts != 2 || first.AuxiliaryArtifacts != 1 ||
		first.ConcernOccurrences != 1 || first.FilesRetained != 3 || first.AlreadyImported != 0 {
		t.Fatalf("first import result = %+v", first)
	}
	for _, path := range []string{reviewPath, learningPath, packetPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("compatibility migration removed %s: %v", path, err)
		}
	}

	reviews, err := LoadPulseReviewArtifacts(context.Background(), workspacePath, "workflow_review", true, 10)
	if err != nil {
		t.Fatalf("load imported review: %v", err)
	}
	if len(reviews) != 2 {
		t.Fatalf("imported reviews = %+v, want both historical reviews consolidated under workflow_review", reviews)
	}
	reviewBySource := map[string]PulseReviewArtifactRecord{}
	for _, review := range reviews {
		reviewBySource[review.LegacySourcePath] = review
	}
	importedKnowledge := reviewBySource["pulse/reviews/legacy-run/knowledgebase_health.md"]
	importedLearning := reviewBySource["pulse/reviews/legacy-run/learning_health.md"]
	if importedKnowledge.Module != "workflow_review" || importedKnowledge.Markdown != reviewMarkdown {
		t.Fatalf("knowledge import did not preserve canonical module and exact Markdown: %+v", importedKnowledge)
	}
	if importedLearning.Module != "workflow_review" || importedLearning.Markdown != learningMarkdown {
		t.Fatalf("learning import was overwritten during module consolidation: %+v", importedLearning)
	}

	findings, err := LoadPulseFindingLifecycles(context.Background(), workspacePath, "workflow_review", 10)
	if err != nil {
		t.Fatalf("load imported finding: %v", err)
	}
	if len(findings) != 1 || findings[0].SeenCount != 1 {
		t.Fatalf("imported findings = %+v, want one occurrence", findings)
	}

	second, err := ImportLegacyPulseReviewArtifacts(context.Background(), workspacePath)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.AlreadyImported != 3 || second.FilesRetained != 3 ||
		second.ReviewArtifacts != 0 || second.AuxiliaryArtifacts != 0 || second.ConcernOccurrences != 0 {
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

func TestImportLegacyPulseReviewArtifactsReportsUnknownMarkdownWithoutDeletingIt(t *testing.T) {
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

	result, err := ImportLegacyPulseReviewArtifacts(context.Background(), workspacePath)
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

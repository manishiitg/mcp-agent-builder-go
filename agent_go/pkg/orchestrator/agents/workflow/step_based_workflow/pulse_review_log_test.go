package step_based_workflow

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// Reviewers write markdown and phrase the verdict differently per module; all of
// these shapes appear in real artifacts on disk.
func TestExtractReviewVerdictHandlesRealArtifactShapes(t *testing.T) {
	for name, tc := range map[string]struct{ artifact, want string }{
		"heading form": {
			artifact: "# KB HEALTH\n\n## Verdict\nRelocation is a half-migration; two live consumers read a deleted path.\n",
			want:     "Relocation is a half-migration; two live consumers read a deleted path.",
		},
		"bold inline form": {
			artifact: "**Verdict:** No cost regression — $23 for the cycle.\n",
			want:     "No cost regression — $23 for the cycle.",
		},
		"plain inline form": {
			artifact: "Verdict: NEEDS-ATTENTION — a few durability trims recommended.\n",
			want:     "NEEDS-ATTENTION — a few durability trims recommended.",
		},
		"heading with blank lines before text": {
			artifact: "## Verdict\n\n\nMostly reconciled. One genuine drift remains.\n",
			want:     "Mostly reconciled. One genuine drift remains.",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := extractReviewVerdict(tc.artifact); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// An artifact with no recognizable verdict still means the reviewer ran, which is
// itself the fact Gate lacks. Inventing a verdict would poison the history it is
// meant to trust.
func TestExtractReviewVerdictEmptyRatherThanGuessing(t *testing.T) {
	if got := extractReviewVerdict("# Findings\n\nSome prose with no verdict line.\n"); got != "" {
		t.Fatalf("expected empty verdict, got %q", got)
	}
}

func TestExtractReviewVerdictTruncatesLongText(t *testing.T) {
	long := "Verdict: " + strings.Repeat("x", maxVerdictChars+200)
	got := extractReviewVerdict(long)
	if len(got) > maxVerdictChars+3 {
		t.Fatalf("verdict not truncated: %d chars", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncation should be visible, got tail %q", got[len(got)-10:])
	}
}

// The gap this closes: confida ran seven reviewers that wrote substantive
// artifacts, and every one left no record that it had run. Gate then had nothing
// to distinguish a module that keeps finding breakage from one that never does.
func TestRecordPulseReviewBuildsPerModuleHistory(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()

	for _, r := range []struct{ module, run, artifact string }{
		{"knowledgebase_health", "pulse-1", "## Verdict\nHalf-migration: two live consumers read a deleted path.\n"},
		{"knowledgebase_health", "pulse-2", "## Verdict\nStill broken; same two consumers.\n"},
		{"report_health", "pulse-2", "## Verdict\nLayout intact, no action needed.\n"},
	} {
		if err := RecordPulseReview(ctx, ws, r.module, r.run, r.run, "pulse/reviews/"+r.run+"/"+r.module+".md", r.artifact); err != nil {
			t.Fatalf("record %s: %v", r.module, err)
		}
	}

	history, err := LoadModuleReviewHistory(ctx, ws, 3)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	byModule := map[string]ModuleReviewHistory{}
	for _, h := range history {
		byModule[h.Module] = h
	}
	kb, ok := byModule["stores_health"]
	if !ok || kb.RunCount != 2 {
		t.Fatalf("stores_health should show 2 migrated-alias runs, got %#v", byModule)
	}
	if len(kb.RecentVerdict) != 2 || !strings.Contains(strings.Join(kb.RecentVerdict, " "), "Half-migration") {
		t.Fatalf("verdicts not retained: %#v", kb.RecentVerdict)
	}
	if rh := byModule["report_health"]; rh.RunCount != 1 {
		t.Fatalf("report_health should show 1 run, got %#v", rh)
	}
}

// A reviewer that ran but produced no parseable verdict must still appear in the
// history — "it ran and said nothing useful" is different from "it never ran",
// and only the first tells Gate the module is being exercised.
func TestReviewHistoryDistinguishesRanFromNeverRan(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	if err := RecordPulseReview(ctx, ws, "db_health", "pulse-1", "pulse-1", "p.md", "no verdict here"); err != nil {
		t.Fatalf("record: %v", err)
	}
	history, err := LoadModuleReviewHistory(ctx, ws, 3)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(history) != 1 || history[0].RunCount != 1 {
		t.Fatalf("expected one recorded run, got %#v", history)
	}
	if !strings.Contains(history[0].RecentVerdict[0], "no verdict line found") {
		t.Fatalf("should say it ran without a verdict, got %q", history[0].RecentVerdict[0])
	}
}

// A workflow whose reviewers have never run must read as "nothing to report"
// rather than erroring inside Gate's state read.
func TestLoadModuleReviewHistoryQuietWhenNeverUsed(t *testing.T) {
	ws := concernsWorkspace(t)
	got, err := LoadModuleReviewHistory(context.Background(), ws, 3)
	if err != nil {
		t.Fatalf("missing table should not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no history, got %#v", got)
	}
}

func TestLoadPulseReviewArtifactsNegativeLimitReturnsCompleteHistory(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	for i := 0; i < 27; i++ {
		runID := fmt.Sprintf("pulse-%02d", i)
		if err := RecordPulseReview(
			ctx, ws, "bug_review", runID, runID, "",
			fmt.Sprintf("## Verdict\nReview %02d completed.\n", i),
		); err != nil {
			t.Fatalf("record review %d: %v", i, err)
		}
	}

	preview, err := LoadPulseReviewArtifacts(ctx, ws, "bug_review", false, 10)
	if err != nil {
		t.Fatalf("load preview: %v", err)
	}
	complete, err := LoadPulseReviewArtifacts(ctx, ws, "bug_review", false, -1)
	if err != nil {
		t.Fatalf("load complete history: %v", err)
	}
	if len(preview) != 10 || len(complete) != 27 {
		t.Fatalf("preview=%d complete=%d, want 10 and 27", len(preview), len(complete))
	}
}

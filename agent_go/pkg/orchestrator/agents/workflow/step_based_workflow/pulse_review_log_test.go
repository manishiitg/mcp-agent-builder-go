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

func TestExtractPulseReviewVerificationsReturnsValidatedStructuredTransport(t *testing.T) {
	artifact := `## Verification
PULSE_VERIFICATION_JSON: {"finding_id":"ISS-9","fingerprint":"fp-9","attempt_id":"fix-9","verdict":"passed","expected":"new run contains a populated value","observed":"run-12 row contains 42","evidence":["runs/run-12/result.json"]}

The next producing run proves the repair held.`
	got, err := ExtractPulseReviewVerifications(artifact)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(got) != 1 || got[0].FindingID != "ISS-9" || got[0].AttemptID != "fix-9" || got[0].Verdict != VerificationPassed {
		t.Fatalf("unexpected verifications: %#v", got)
	}
}

func TestExtractPulseReviewVerificationsRejectsInconclusiveWithoutBoundary(t *testing.T) {
	artifact := `PULSE_VERIFICATION_JSON: {"finding_id":"ISS-9","fingerprint":"fp-9","attempt_id":"fix-9","verdict":"inconclusive","expected":"new row","observed":"no new run yet"}`
	if _, err := ExtractPulseReviewVerifications(artifact); err == nil || !strings.Contains(err.Error(), "next_check") {
		t.Fatalf("expected missing next_check rejection, got %v", err)
	}
}

func TestRecordPulseReviewRetainsAndQuarantinesContractFailure(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	invalid := `## Verdict
The review found useful evidence, but its structured marker is incomplete.
PULSE_VERIFICATION_JSON: {"finding_id":"ISS-9","fingerprint":"fp-9","attempt_id":"fix-9","verdict":"inconclusive","expected":"new row","observed":"no new run yet"}`

	err := RecordPulseReview(ctx, ws, "artifact_review", "review-invalid", "pulse-1", "", invalid)
	if err == nil || !strings.Contains(err.Error(), "artifact retained with status contract_failed") {
		t.Fatalf("expected retained contract failure, got %v", err)
	}

	artifact, err := LoadPulseReviewArtifactForRun(ctx, ws, "review-invalid", "artifact_review")
	if err != nil {
		t.Fatalf("load retained artifact: %v", err)
	}
	if artifact.Status != pulseReviewStatusContractFailed || !strings.Contains(artifact.Markdown, invalid) ||
		!strings.Contains(artifact.Markdown, "Pulse review contract failure") || !strings.Contains(artifact.Markdown, "next_check") {
		t.Fatalf("contract-failed artifact was not retained intact: %#v", artifact)
	}
	if len(artifact.Verifications) != 0 {
		t.Fatalf("invalid markers must be quarantined, got %#v", artifact.Verifications)
	}

	valid := `## Verdict
The next run proved the fix.
PULSE_VERIFICATION_JSON: {"finding_id":"ISS-10","fingerprint":"fp-10","attempt_id":"fix-10","verdict":"passed","expected":"new row contains 42","observed":"new row contains 42","evidence":["db row 42"]}`
	if err := recordPulseReviewAt(ctx, ws, "workflow_review", "review-valid", "pulse-1", "review", "", "completed", valid, "2026-08-03T12:00:00Z"); err != nil {
		t.Fatalf("record valid sibling review: %v", err)
	}
	verifications, err := LoadPulseReviewVerificationsForPulseRun(ctx, ws, "pulse-1", "workflow_review")
	if err != nil {
		t.Fatalf("load quarantined verifications: %v", err)
	}
	if len(verifications) != 1 || verifications[0].FindingID != "ISS-10" {
		t.Fatalf("want only the valid sibling marker, got %#v", verifications)
	}
}

func TestPulseReviewVerificationAllowlistComesFromChangedUnverifiedAttempts(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	concern := filedReviewConcern(t, ws, "pulse-fix", "eval_health", "score scale is not pinned")
	recordFindingDispositions(t, ws, "eval_health", "pulse-fix", []PulseFindingDisposition{{
		Fingerprint:  concern.Fingerprint,
		FindingID:    "EVAL-1",
		Disposition:  FindingDispositionChangedUnverified,
		Summary:      "Scale pinned; next evaluator run must prove it.",
		ChangedFiles: []string{"evaluation/evaluation_plan.json"},
		BeforeRefs:   []string{"scale:before"},
		AfterRefs:    []string{"scale:after"},
		NextCheck:    "the next evaluator workflow run emits the pinned max_score",
		Verification: []PulseFindingVerification{{
			Check:    "next evaluator workflow run",
			Verdict:  VerificationInconclusive,
			Expected: "pinned max_score",
			Observed: "no evaluator run yet",
		}},
	}})

	candidates, err := LoadPulseReviewVerificationCandidates(ctx, ws, "workflow_review")
	if err != nil {
		t.Fatalf("load candidates: %v", err)
	}
	if len(candidates) != 1 || strings.TrimSpace(candidates[0].AttemptID) == "" ||
		candidates[0].FindingID != "EVAL-1" || !strings.Contains(candidates[0].NextCheck, "next evaluator") {
		t.Fatalf("unexpected allowlist: %#v", candidates)
	}

	valid := fmt.Sprintf(`## Verdict
The producing run proved the scale.
PULSE_VERIFICATION_JSON: {"finding_id":"EVAL-1","fingerprint":%q,"attempt_id":%q,"verdict":"passed","expected":"pinned max_score","observed":"pinned max_score","evidence":["evaluation row"]}`,
		concern.Fingerprint, candidates[0].AttemptID)
	if err := RecordPulseReview(ctx, ws, "workflow_review", "review-allowed", "pulse-review", "", valid); err != nil {
		t.Fatalf("allowlisted marker rejected: %v", err)
	}

	outside := `## Verdict
Invented verification.
PULSE_VERIFICATION_JSON: {"finding_id":"OTHER","fingerprint":"other-fp","attempt_id":"other-attempt","verdict":"passed","expected":"x","observed":"x"}`
	err = RecordPulseReview(ctx, ws, "workflow_review", "review-outside", "pulse-review", "", outside)
	if err == nil || !strings.Contains(err.Error(), "outside the backend allowlist") {
		t.Fatalf("expected out-of-allowlist quarantine, got %v", err)
	}
	artifact, loadErr := LoadPulseReviewArtifactForRun(ctx, ws, "review-outside", "workflow_review")
	if loadErr != nil || artifact.Status != pulseReviewStatusContractFailed {
		t.Fatalf("outside marker was not retained as contract_failed: artifact=%#v err=%v", artifact, loadErr)
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
	workflow, ok := byModule["workflow_review"]
	if !ok || workflow.RunCount != 3 || !strings.Contains(strings.Join(workflow.RecentVerdict, " "), "Half-migration") {
		t.Fatalf("historical engineering evidence not consolidated under workflow_review: %#v", byModule)
	}
}

func TestRecordPulseReviewForModulesIndexesOneSharedReportUnderEverySelectedPerspective(t *testing.T) {
	ws := concernsWorkspace(t)
	ctx := context.Background()
	artifact := "## Verdict\nArtifact and store evidence were reviewed together.\n"
	if err := RecordPulseReviewForModules(ctx, ws, []string{"workflow_review", "llm_ops_review"}, "shared-review", "pulse-1", "", artifact); err != nil {
		t.Fatalf("record shared review: %v", err)
	}
	for _, module := range []string{"workflow_review", "llm_ops_review"} {
		record, err := LoadPulseReviewArtifactForRun(ctx, ws, "shared-review", module)
		if err != nil {
			t.Fatalf("load %s shared review: %v", module, err)
		}
		if record.Module != module || !strings.Contains(record.Markdown, "reviewed together") {
			t.Fatalf("%s record = %#v", module, record)
		}
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

package step_based_workflow

import (
	"context"
	"strings"
	"testing"
)

// PLAT-055. The point of the tool is bandwidth: a concern must survive with its
// severity, classification, and evidence intact, because the `CONCERNS:` text
// line could not carry them (39 of 103 active Upwork concerns have no detail
// row at all).

func validStepConcern() StepRunConcernInput {
	return StepRunConcernInput{
		Concern: "bid-pick-job's declared enum contract is unreachable from the turn that writes the file",
		PulseFindingDetails: PulseFindingDetails{
			IssueKind:      "workflow_issue",
			Classification: "correctness_bug",
			Severity:       "high",
			Summary:        "select_route is written as 'picked' because the enum is stated upstream of the write.",
			Impact:         "Every run burns a corrective retry on the same pre-validation failure.",
			Evidence:       []string{"runs/iteration-0/daily-bid/execution/bid-pick-job/selected_job.json"},
		},
	}
}

func recordConcernForTest(t *testing.T, workspacePath, runFolder, stepID, phase string, input StepRunConcernInput) StepRunConcernRecord {
	t.Helper()
	record, err := RecordStepRunConcern(context.Background(), workspacePath, runFolder, "default", stepID, phase, input)
	if err != nil {
		t.Fatalf("RecordStepRunConcern: %v", err)
	}
	return record
}

func lifecycleByFingerprint(t *testing.T, workspacePath, fingerprint string) PulseFindingLifecycle {
	t.Helper()
	findings, err := LoadPulseFindingLifecycles(context.Background(), workspacePath, "", -1)
	if err != nil {
		t.Fatalf("LoadPulseFindingLifecycles: %v", err)
	}
	for _, finding := range findings {
		if finding.Fingerprint == fingerprint {
			return finding
		}
	}
	t.Fatalf("fingerprint %s not found among %d findings", fingerprint, len(findings))
	return PulseFindingLifecycle{}
}

func TestRecordStepRunConcernPersistsStructuredDetail(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/example"

	record := recordConcernForTest(t, workspacePath, "iteration-0", "bid-pick-job", ConcernPhaseExecution, validStepConcern())
	if !record.Recorded || record.Fingerprint == "" {
		t.Fatalf("record = %+v, want recorded with a fingerprint", record)
	}

	finding := lifecycleByFingerprint(t, workspacePath, record.Fingerprint)
	if finding.StepID != "bid-pick-job" || finding.Phase != ConcernPhaseExecution {
		t.Fatalf("attribution wrong: step=%q phase=%q", finding.StepID, finding.Phase)
	}

	// The detail row is the whole point — without it this is just the old
	// unstructured text line wearing a tool's clothes.
	if finding.Details == nil {
		t.Fatal("no structured detail recorded for a tool-filed concern")
	}
	if finding.Details.Severity != "high" || finding.Details.Classification != "correctness_bug" {
		t.Fatalf("detail lost fields: %+v", finding.Details)
	}
	if len(finding.Details.Evidence) != 1 {
		t.Fatalf("evidence not preserved: %+v", finding.Details.Evidence)
	}
	if finding.Details.Impact == "" {
		t.Fatal("impact not preserved")
	}
}

func TestRecordStepRunConcernIsIdempotentWithinOneRun(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/example"

	first := recordConcernForTest(t, workspacePath, "iteration-0", "bid-pick-job", ConcernPhaseExecution, validStepConcern())
	second := recordConcernForTest(t, workspacePath, "iteration-0", "bid-pick-job", ConcernPhaseExecution, validStepConcern())

	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("same concern produced two fingerprints: %s vs %s", first.Fingerprint, second.Fingerprint)
	}
	if !second.Duplicate {
		t.Fatal("second filing in the same run was not reported as a duplicate")
	}

	// seen_count is what Pulse Gate weighs when deciding a root cause deserves
	// repair. A replayed tool call must not manufacture recurrence evidence.
	finding := lifecycleByFingerprint(t, workspacePath, first.Fingerprint)
	if finding.SeenCount != 1 {
		t.Fatalf("seen_count = %d after a replay in one run, want 1", finding.SeenCount)
	}
}

func TestRecordStepRunConcernCountsRecurrenceAcrossRuns(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/example"

	first := recordConcernForTest(t, workspacePath, "iteration-1", "bid-pick-job", ConcernPhaseExecution, validStepConcern())
	recordConcernForTest(t, workspacePath, "iteration-2", "bid-pick-job", ConcernPhaseExecution, validStepConcern())

	// The flip side of idempotence: a defect that genuinely recurs on a later
	// run must accumulate, or a chronic root cause looks new every cycle.
	finding := lifecycleByFingerprint(t, workspacePath, first.Fingerprint)
	if finding.SeenCount != 2 {
		t.Fatalf("seen_count = %d across two runs, want 2", finding.SeenCount)
	}
}

func TestRecordStepRunConcernRejectsIncompleteFindings(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/example"

	cases := []struct {
		name    string
		phase   string
		mutate  func(*StepRunConcernInput)
		wantSub string
	}{
		{
			name:    "no evidence",
			phase:   ConcernPhaseExecution,
			mutate:  func(in *StepRunConcernInput) { in.Evidence = nil },
			wantSub: "evidence",
		},
		{
			name:    "unknown issue kind",
			phase:   ConcernPhaseExecution,
			mutate:  func(in *StepRunConcernInput) { in.IssueKind = "something_else" },
			wantSub: "issue_kind",
		},
		{
			name:   "harness issue without reproduction",
			phase:  ConcernPhaseExecution,
			mutate: func(in *StepRunConcernInput) { in.IssueKind = "harness_issue" },
			// A harness issue leaves the workflow, so it must be actionable by
			// someone who never saw the run.
			wantSub: "target_key",
		},
		{
			name:    "review phase is not a step phase",
			phase:   ConcernPhaseReview,
			mutate:  func(in *StepRunConcernInput) {},
			wantSub: "not a step phase",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := validStepConcern()
			tc.mutate(&input)
			_, err := RecordStepRunConcern(context.Background(), workspacePath, "iteration-0", "default", "bid-pick-job", tc.phase, input)
			if err == nil {
				t.Fatalf("expected rejection for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %v, want mention of %q", err, tc.wantSub)
			}
		})
	}
}

func TestRecordStepRunConcernSeparatesStepsByFingerprint(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/example"

	// Identical text from two steps is two defects, not one — which is exactly
	// why the tool takes its step identity from trusted session state instead
	// of a model-supplied argument.
	a := recordConcernForTest(t, workspacePath, "iteration-0", "bid-pick-job", ConcernPhaseExecution, validStepConcern())
	b := recordConcernForTest(t, workspacePath, "iteration-0", "search-save-jobs", ConcernPhaseExecution, validStepConcern())

	if a.Fingerprint == b.Fingerprint {
		t.Fatal("two different steps collapsed onto one fingerprint")
	}
}

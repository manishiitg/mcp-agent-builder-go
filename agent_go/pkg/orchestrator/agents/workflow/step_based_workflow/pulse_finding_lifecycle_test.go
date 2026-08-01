package step_based_workflow

import (
	"context"
	"strings"
	"testing"
)

func filedReviewConcern(t *testing.T, workspacePath, pulseRunID, module, text string) RunConcern {
	t.Helper()
	ctx := context.Background()
	if _, err := RecordRunConcerns(
		ctx, workspacePath, pulseRunID, "", module, ConcernPhaseReview, "CONCERNS: "+text,
	); err != nil {
		t.Fatalf("record concern: %v", err)
	}
	concerns, err := LoadOpenRunConcerns(ctx, workspacePath, 10)
	if err != nil {
		t.Fatalf("load concern: %v", err)
	}
	if len(concerns) != 1 {
		t.Fatalf("concerns = %+v, want one", concerns)
	}
	return concerns[0]
}

func recordFindingDispositions(t *testing.T, workspacePath, module, pulseRunID string, dispositions []PulseFindingDisposition) {
	t.Helper()
	ctx := context.Background()
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		t.Fatalf("open lifecycle db: db=%v err=%v", db, err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lifecycle transaction: %v", err)
	}
	defer tx.Rollback()
	if err := RecordPulseFindingDispositionsTx(ctx, tx, module, pulseRunID, dispositions, "2026-07-31T08:00:00Z"); err != nil {
		t.Fatalf("record dispositions: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit dispositions: %v", err)
	}
}

func TestPulseFindingLifecycleClosesOnlyWithVerifiedFixAndReopensOnRecurrence(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	module := "bug_review"
	pulseRunID := "pulse-1"
	concernText := "stale selector repeats the same accounts"
	concern := filedReviewConcern(t, workspacePath, pulseRunID, module, concernText)

	findings := []PulseFixFindingRef{{Fingerprint: concern.Fingerprint, FindingID: "BUG-1"}}
	attempt, err := StartPulseFixAttempt(
		ctx, workspacePath, pulseRunID, module, "Diversify selector candidates.",
		findings, []string{"planning/step_config.json"}, []string{"selector:before"},
	)
	if err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	retried, err := StartPulseFixAttempt(
		ctx, workspacePath, pulseRunID, module, "Retry after tool response loss.",
		findings, []string{"planning/step_config.json"}, []string{"selector:before"},
	)
	if err != nil {
		t.Fatalf("retry attempt: %v", err)
	}
	if retried.AttemptID != attempt.AttemptID {
		t.Fatalf("retry created a different attempt: %q != %q", retried.AttemptID, attempt.AttemptID)
	}

	recordFindingDispositions(t, workspacePath, module, pulseRunID, []PulseFindingDisposition{{
		Fingerprint:  concern.Fingerprint,
		FindingID:    "BUG-1",
		AttemptID:    attempt.AttemptID,
		Disposition:  FindingDispositionFixedVerified,
		Summary:      "Selector now includes an exploration pool.",
		ChangedFiles: []string{"planning/step_config.json"},
		BeforeRefs:   []string{"selector:before"},
		AfterRefs:    []string{"selector:after"},
		Verification: []PulseFindingVerification{{
			Check:    "selector diversity test",
			Verdict:  VerificationPassed,
			Expected: "new candidates are eligible",
			Observed: "new candidates were selected",
			Evidence: []string{"go test ./... -run TestSelectorDiversity"},
		}},
	}})

	lifecycles, err := LoadPulseFindingLifecycles(ctx, workspacePath, module, 10)
	if err != nil {
		t.Fatalf("load lifecycle: %v", err)
	}
	if len(lifecycles) != 1 || lifecycles[0].Status != ConcernStatusResolved {
		t.Fatalf("verified finding was not closed: %+v", lifecycles)
	}
	if len(lifecycles[0].Attempts) != 1 || lifecycles[0].Attempts[0].Status != "verified" {
		t.Fatalf("verified attempt missing: %+v", lifecycles[0].Attempts)
	}
	if len(lifecycles[0].Verification) != 1 || lifecycles[0].Verification[0].Verdict != VerificationPassed {
		t.Fatalf("verification evidence missing: %+v", lifecycles[0].Verification)
	}

	if _, err := RecordRunConcerns(
		ctx, workspacePath, "pulse-2", "", module, ConcernPhaseReview, "CONCERNS: "+concernText,
	); err != nil {
		t.Fatalf("record recurrence: %v", err)
	}
	lifecycles, err = LoadPulseFindingLifecycles(ctx, workspacePath, module, 10)
	if err != nil {
		t.Fatalf("reload lifecycle: %v", err)
	}
	if lifecycles[0].Status != ConcernStatusOpen || lifecycles[0].SeenCount != 2 {
		t.Fatalf("recurrence did not reopen finding: %+v", lifecycles[0])
	}
	foundReopened := false
	for _, event := range lifecycles[0].Events {
		foundReopened = foundReopened || event.EventType == "reopened"
	}
	if !foundReopened {
		t.Fatalf("reopened event missing: %+v", lifecycles[0].Events)
	}
}

func TestPulseFindingLifecycleKeepsUnverifiedChangeOpenAndFailedProofReopens(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	module := "eval_health"
	pulseRunID := "pulse-1"
	concern := filedReviewConcern(t, workspacePath, pulseRunID, module, "evaluation uses a stale outcome")
	attempt, err := StartPulseFixAttempt(
		ctx, workspacePath, pulseRunID, module, "Update the evaluation query.",
		[]PulseFixFindingRef{{Fingerprint: concern.Fingerprint, FindingID: "EVAL-1"}},
		[]string{"evaluation/plan.json"}, nil,
	)
	if err != nil {
		t.Fatalf("start attempt: %v", err)
	}

	recordFindingDispositions(t, workspacePath, module, pulseRunID, []PulseFindingDisposition{{
		Fingerprint:  concern.Fingerprint,
		FindingID:    "EVAL-1",
		AttemptID:    attempt.AttemptID,
		Disposition:  FindingDispositionChangedUnverified,
		Summary:      "Query changed; the next producing run is required for proof.",
		ChangedFiles: []string{"evaluation/plan.json"},
		NextCheck:    "next producing run",
		Verification: []PulseFindingVerification{{
			Check:   "new outcome appears in a producing run",
			Verdict: VerificationInconclusive,
		}},
	}})
	lifecycles, err := LoadPulseFindingLifecycles(ctx, workspacePath, module, 10)
	if err != nil {
		t.Fatalf("load awaiting lifecycle: %v", err)
	}
	if len(lifecycles) != 1 || lifecycles[0].Status != ConcernStatusAwaitingVerification {
		t.Fatalf("unverified change was incorrectly closed: %+v", lifecycles)
	}

	recordFindingDispositions(t, workspacePath, module, pulseRunID, []PulseFindingDisposition{{
		Fingerprint: concern.Fingerprint,
		FindingID:   "EVAL-1",
		AttemptID:   attempt.AttemptID,
		Disposition: FindingDispositionFailed,
		Summary:     "The next producing run still returned the stale outcome.",
		Verification: []PulseFindingVerification{{
			Check:    "new outcome appears in a producing run",
			Verdict:  VerificationFailed,
			Expected: "new outcome",
			Observed: "stale outcome",
		}},
	}})
	lifecycles, err = LoadPulseFindingLifecycles(ctx, workspacePath, module, 10)
	if err != nil {
		t.Fatalf("reload failed lifecycle: %v", err)
	}
	if lifecycles[0].Status != ConcernStatusOpen {
		t.Fatalf("failed verification did not reopen finding: %+v", lifecycles[0])
	}
	if !strings.Contains(lifecycles[0].ResolutionNote, "stale outcome") {
		t.Fatalf("failed evidence summary missing: %+v", lifecycles[0])
	}
}

func TestPulseFindingLifecycleLoadsStructuredHarnessReproduction(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	module := "bug_review"
	concernText := "plan updater rejects the effective message-sequence step because its saved type is legacy regular"
	summary := strings.Join([]string{
		"## Finding",
		"The plan-editing compatibility boundary is broken.",
		`PULSE_FINDING_JSON: {"concern":"` + concernText + `","finding_id":"HARNESS-PLAN-EDIT-1","target_key":"harness:plan-editor:legacy-agentic-regular","issue_kind":"harness_issue","classification":"correctness_bug","severity":"critical","summary":"Runtime and editing APIs disagree about the step type.","impact":"Pulse can diagnose the workflow defect but cannot apply its repair.","workaround":"Persist the step as message_sequence manually.","evidence":["update_scripted_step rejected the agentic step","update_message_sequence_step rejected the saved regular type"],"reproduction":{"safe":true,"setup":"Use a copied plan with a regular step and declared_execution_mode=agentic.","action":"Call update_message_sequence_step for that step.","expected":"The harness upgrades the type and applies the edit.","observed":"The updater rejects the saved regular type.","limitations":"No production workflow execution is required."}}`,
		"CONCERNS: " + concernText,
	}, "\n")

	if _, err := RecordRunConcerns(
		ctx, workspacePath, "pulse-structured", "", module, ConcernPhaseReview, summary,
	); err != nil {
		t.Fatalf("record structured harness finding: %v", err)
	}
	lifecycles, err := LoadPulseFindingLifecycles(ctx, workspacePath, module, 10)
	if err != nil {
		t.Fatalf("load structured harness finding: %v", err)
	}
	if len(lifecycles) != 1 {
		t.Fatalf("lifecycles = %+v, want one", lifecycles)
	}
	finding := lifecycles[0]
	if finding.FindingID != "HARNESS-PLAN-EDIT-1" || finding.Details == nil {
		t.Fatalf("structured identity missing: %+v", finding)
	}
	if finding.Details.IssueKind != "harness_issue" || finding.Details.TargetKey != "harness:plan-editor:legacy-agentic-regular" {
		t.Fatalf("harness classification missing: %+v", finding.Details)
	}
	if !finding.Details.Reproduction.Safe ||
		!strings.Contains(finding.Details.Reproduction.Expected, "upgrades the type") ||
		!strings.Contains(finding.Details.Reproduction.Observed, "rejects") {
		t.Fatalf("reproduction contract missing: %+v", finding.Details.Reproduction)
	}
	if len(finding.Details.Evidence) != 2 {
		t.Fatalf("evidence missing: %+v", finding.Details.Evidence)
	}
	if finding.Details.Platform == nil ||
		len(finding.Details.Platform.AffectedWorkflows) != 1 ||
		finding.Details.Platform.SeenCount != 1 {
		t.Fatalf("platform registry linkage missing: %+v", finding.Details.Platform)
	}
}

func TestHarnessFindingPlatformRegistryDeduplicatesAcrossWorkflows(t *testing.T) {
	ctx := context.Background()
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	issueKey := "harness:mcp-bridge:authorization-header"
	concernText := "MCP bridge rejects the valid bearer authorization header"
	summary := `PULSE_FINDING_JSON: {"concern":"` + concernText +
		`","finding_id":"HARNESS-MCP-1","target_key":"` + issueKey +
		`","issue_kind":"harness_issue","classification":"correctness_bug","severity":"high","summary":"The bridge rejects valid authorization.","reproduction":{"safe":true,"setup":"Use a local bridge fixture.","action":"Send the documented header.","expected":"Request is authorized.","observed":"Invalid API token."}}` +
		"\nCONCERNS: " + concernText

	for index, workspacePath := range []string{"Workflow/alpha", "Workflow/beta"} {
		if _, err := RecordRunConcerns(
			ctx, workspacePath, "pulse-"+string(rune('1'+index)), "", "bug_review", ConcernPhaseReview, summary,
		); err != nil {
			t.Fatalf("record %s harness finding: %v", workspacePath, err)
		}
	}
	lifecycles, err := LoadPulseFindingLifecycles(ctx, "Workflow/alpha", "bug_review", 10)
	if err != nil {
		t.Fatalf("load linked harness finding: %v", err)
	}
	if len(lifecycles) != 1 || lifecycles[0].Details == nil || lifecycles[0].Details.Platform == nil {
		t.Fatalf("linked platform details missing: %+v", lifecycles)
	}
	platform := lifecycles[0].Details.Platform
	if platform.IssueKey != issueKey || platform.SeenCount != 2 {
		t.Fatalf("platform issue was not deduplicated: %+v", platform)
	}
	if got := strings.Join(platform.AffectedWorkflows, ","); got != "Workflow/alpha,Workflow/beta" {
		t.Fatalf("affected workflow linkage = %q", got)
	}
}

func TestPulseFindingDetailsMarkerCannotCreateUnfiledConcern(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	summary := strings.Join([]string{
		`PULSE_FINDING_JSON: {"concern":"hidden concern","issue_kind":"harness_issue","summary":"Must not be stored."}`,
		"CONCERNS: visible concern",
	}, "\n")
	if _, err := RecordRunConcerns(
		ctx, workspacePath, "pulse-mismatch", "", "bug_review", ConcernPhaseReview, summary,
	); err != nil {
		t.Fatalf("record concern with mismatched details: %v", err)
	}
	lifecycles, err := LoadPulseFindingLifecycles(ctx, workspacePath, "bug_review", 10)
	if err != nil {
		t.Fatalf("load mismatched details lifecycle: %v", err)
	}
	if len(lifecycles) != 1 || lifecycles[0].Text != "visible concern" {
		t.Fatalf("unexpected lifecycle rows: %+v", lifecycles)
	}
	if lifecycles[0].Details != nil {
		t.Fatalf("unfiled marker was attached: %+v", lifecycles[0].Details)
	}
}

func TestFixedVerifiedRejectsInconclusiveEvidence(t *testing.T) {
	err := validateFindingDisposition(PulseFindingDisposition{
		Fingerprint:  "fp",
		FindingID:    "BUG-1",
		AttemptID:    "attempt",
		Disposition:  FindingDispositionFixedVerified,
		Summary:      "Claimed fixed.",
		ChangedFiles: []string{"workflow.json"},
		Verification: []PulseFindingVerification{{
			Check:   "runtime outcome",
			Verdict: VerificationInconclusive,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "no failed/inconclusive") {
		t.Fatalf("inconclusive fixed_verified evidence was accepted: %v", err)
	}
}

func TestFixedVerifiedRejectsUnpairedChangeReferences(t *testing.T) {
	err := validateFindingDisposition(PulseFindingDisposition{
		Fingerprint:  "fp",
		FindingID:    "BUG-REFS",
		AttemptID:    "attempt",
		Disposition:  FindingDispositionFixedVerified,
		Summary:      "Claimed fixed.",
		ChangedFiles: []string{"workflow.json"},
		BeforeRefs:   []string{"sha256:before"},
		Verification: []PulseFindingVerification{{
			Check:   "runtime outcome",
			Verdict: VerificationPassed,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "paired before_refs and after_refs") {
		t.Fatalf("unpaired fixed_verified references were accepted: %v", err)
	}
}

func TestFixedVerifiedDispositionWhitespaceStillClosesFinding(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	module := "bug_review"
	pulseRunID := "pulse-whitespace"
	concern := filedReviewConcern(t, workspacePath, pulseRunID, module, "whitespace routing regression")
	attempt, err := StartPulseFixAttempt(
		ctx, workspacePath, pulseRunID, module, "Fix lifecycle routing.",
		[]PulseFixFindingRef{{Fingerprint: concern.Fingerprint, FindingID: "BUG-SPACE"}},
		[]string{"pulse_finding_lifecycle.go"}, nil,
	)
	if err != nil {
		t.Fatalf("start fix attempt: %v", err)
	}

	recordFindingDispositions(t, workspacePath, module, pulseRunID, []PulseFindingDisposition{{
		Fingerprint:  " " + concern.Fingerprint + " ",
		FindingID:    " BUG-SPACE ",
		AttemptID:    " " + attempt.AttemptID + " ",
		Disposition:  FindingDispositionFixedVerified + " ",
		Summary:      " Lifecycle routing now uses canonical input. ",
		ChangedFiles: []string{" pulse_finding_lifecycle.go "},
		Verification: []PulseFindingVerification{{
			Check:   " lifecycle regression test ",
			Verdict: " " + VerificationPassed + " ",
		}},
	}})

	lifecycles, err := LoadPulseFindingLifecycles(ctx, workspacePath, module, 10)
	if err != nil {
		t.Fatalf("load lifecycle: %v", err)
	}
	if len(lifecycles) != 1 || lifecycles[0].Status != ConcernStatusResolved {
		t.Fatalf("trimmed fixed_verified was not closed: %+v", lifecycles)
	}
	foundClosed := false
	for _, event := range lifecycles[0].Events {
		if event.EventType == "updated" {
			t.Fatalf("trimmed disposition was routed as an update: %+v", lifecycles[0].Events)
		}
		foundClosed = foundClosed || event.EventType == "closed"
	}
	if !foundClosed {
		t.Fatalf("trimmed disposition did not record a closed event: %+v", lifecycles[0].Events)
	}
}

func TestNoAttemptVerificationHistoryAccumulatesAcrossPulseRuns(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	module := "bug_review"
	concern := filedReviewConcern(t, workspacePath, "pulse-1", module, "configuration is already correct")
	disposition := PulseFindingDisposition{
		Fingerprint: concern.Fingerprint,
		FindingID:   "BUG-NO-CHANGE",
		Disposition: FindingDispositionVerifiedNoChange,
		Summary:     "The suspected defect is not present.",
		Verification: []PulseFindingVerification{{
			Check:    "configuration contract",
			Verdict:  VerificationPassed,
			Expected: "setting is enabled",
			Observed: "setting is enabled",
		}},
	}
	recordFindingDispositions(t, workspacePath, module, "pulse-1", []PulseFindingDisposition{disposition})
	recordFindingDispositions(t, workspacePath, module, "pulse-2", []PulseFindingDisposition{disposition})

	lifecycles, err := LoadPulseFindingLifecycles(ctx, workspacePath, module, 10)
	if err != nil {
		t.Fatalf("load lifecycle: %v", err)
	}
	if len(lifecycles) != 1 || len(lifecycles[0].Verification) != 2 {
		t.Fatalf("no-attempt verification history was overwritten: %+v", lifecycles)
	}
}

func TestExternalActionRequiredLeavesActiveQueueAndStaysSuppressedOnRecurrence(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	module := "bug_review"
	concernText := "scheduler marks a truncated background run successful"
	concern := filedReviewConcern(t, workspacePath, "pulse-external-1", module, concernText)

	recordFindingDispositions(t, workspacePath, module, "pulse-external-1", []PulseFindingDisposition{{
		Fingerprint:     concern.Fingerprint,
		FindingID:       "HARNESS-SCHEDULER-1",
		Disposition:     FindingDispositionExternalAction,
		Summary:         "The defect is in the shared scheduler and has no workflow-level repair.",
		ExternalOwner:   "platform",
		ReasonCode:      "missing_platform_tool",
		ReopenCondition: "scheduler completion detection changes or a platform repair tool becomes available",
	}})

	active, err := LoadOpenRunConcerns(ctx, workspacePath, -1)
	if err != nil {
		t.Fatalf("load active concerns: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("externally owned finding remained active: %+v", active)
	}
	suppressed, err := LoadExternallyOwnedRunConcerns(ctx, workspacePath)
	if err != nil {
		t.Fatalf("load suppressed concerns: %v", err)
	}
	if len(suppressed) != 1 || suppressed[0].Fingerprint != concern.Fingerprint {
		t.Fatalf("external finding was not suppressed: %+v", suppressed)
	}

	if _, err := RecordRunConcerns(
		ctx, workspacePath, "pulse-external-2", "", module, ConcernPhaseReview,
		"CONCERNS: "+concernText,
	); err != nil {
		t.Fatalf("record identical recurrence: %v", err)
	}
	active, err = LoadOpenRunConcerns(ctx, workspacePath, -1)
	if err != nil {
		t.Fatalf("reload active concerns: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("identical recurrence reopened external finding: %+v", active)
	}
	lifecycles, err := LoadPulseFindingLifecycles(ctx, workspacePath, module, -1)
	if err != nil {
		t.Fatalf("load external lifecycle: %v", err)
	}
	if len(lifecycles) != 1 || lifecycles[0].Status != ConcernStatusExternalActionRequired || lifecycles[0].SeenCount != 2 {
		t.Fatalf("external lifecycle lost recurrence audit: %+v", lifecycles)
	}
	if lifecycles[0].ExternalOwner != "platform" ||
		lifecycles[0].ReasonCode != "missing_platform_tool" ||
		!strings.Contains(lifecycles[0].ReopenCondition, "scheduler") {
		t.Fatalf("external ownership metadata missing: %+v", lifecycles[0])
	}
}

func TestExternalActionRequiredNeedsOwnershipAndReopenBoundary(t *testing.T) {
	err := validateFindingDisposition(PulseFindingDisposition{
		Fingerprint: "fp",
		FindingID:   "HARNESS-1",
		Disposition: FindingDispositionExternalAction,
		Summary:     "Outside workflow authority.",
	})
	if err == nil || !strings.Contains(err.Error(), "external_owner, reason_code, and reopen_condition") {
		t.Fatalf("incomplete external disposition was accepted: %v", err)
	}
}

// TestFindingBacklogLeadsWithTheLargestCluster covers the query the Fixer
// actually reads.
//
// get_pulse_finding_backlog resolves here, not through LoadOpenRunConcerns.
// Reordering that other function alone left this one sorting by recency, so
// social-media's 39 execute-find-opportunities concerns — one schema mismatch
// filed once per field it named — stayed interleaved with unrelated findings
// and successive Bug Review passes never reached them.
func TestFindingBacklogLeadsWithTheLargestCluster(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)

	file := func(runID, step, text string) {
		t.Helper()
		if _, err := RecordRunConcerns(
			ctx, workspacePath, runID, "", step, ConcernPhaseReview, "CONCERNS: "+text,
		); err != nil {
			t.Fatalf("record concern: %v", err)
		}
	}
	// Filed first, so recency ranks them last.
	for _, field := range []string{"$.status", "$.targets", "$.synthesis_notes"} {
		file("pulse-1", "execute-find-opportunities",
			"prevalidation gate failed at opportunities.json "+field)
	}
	// Filed last, so recency would put this on top.
	file("pulse-2", "execute-digest", "digest was not delivered")

	lifecycles, err := LoadPulseFindingLifecycles(ctx, workspacePath, "", 50)
	if err != nil {
		t.Fatalf("load backlog: %v", err)
	}
	if len(lifecycles) != 4 {
		t.Fatalf("expected 4 findings, got %d", len(lifecycles))
	}
	for i := 0; i < 3; i++ {
		if lifecycles[i].StepID != "execute-find-opportunities" {
			t.Fatalf("position %d is %q; the 3-concern cluster must lead and stay contiguous, "+
				"not be ranked below a single newer finding", i+1, lifecycles[i].StepID)
		}
	}
	if lifecycles[3].StepID != "execute-digest" {
		t.Fatalf("the isolated finding should follow the cluster, got %q", lifecycles[3].StepID)
	}
}

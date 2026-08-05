package step_based_workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"
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

func TestLoadPulseFindingLifecyclesIncludesLegacyAliasesForCurrentLane(t *testing.T) {
	workspacePath := concernsWorkspace(t)
	filedReviewConcern(t, workspacePath, "pulse-1", "knowledgebase_health", "legacy knowledge concern")

	findings, err := LoadPulseFindingLifecycles(context.Background(), workspacePath, "workflow_review", 10)
	if err != nil {
		t.Fatalf("load stores lifecycle: %v", err)
	}
	if len(findings) != 1 || findings[0].StepID != "workflow_review" {
		t.Fatalf("legacy Engineering finding not visible through current lane: %#v", findings)
	}
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

	recordFindingDispositions(t, workspacePath, module, pulseRunID, []PulseFindingDisposition{{
		Fingerprint:  concern.Fingerprint,
		FindingID:    "BUG-1",
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

	recordFindingDispositions(t, workspacePath, module, pulseRunID, []PulseFindingDisposition{{
		Fingerprint:  concern.Fingerprint,
		FindingID:    "EVAL-1",
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
		Disposition:  FindingDispositionFixedVerified,
		Summary:      "Claimed fixed.",
		ChangedFiles: []string{"workflow.json"},
		Verification: []PulseFindingVerification{{
			Check:   "runtime outcome",
			Verdict: VerificationInconclusive,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "no failed or inconclusive check") ||
		!strings.Contains(err.Error(), "inconclusive=1") {
		t.Fatalf("inconclusive fixed_verified evidence was accepted: %v", err)
	}
}

func TestFixedVerifiedRejectsUnpairedChangeReferences(t *testing.T) {
	err := validateFindingDisposition(PulseFindingDisposition{
		Fingerprint:  "fp",
		FindingID:    "BUG-REFS",
		Disposition:  FindingDispositionFixedVerified,
		Summary:      "Claimed fixed.",
		ChangedFiles: []string{"workflow.json"},
		BeforeRefs:   []string{"sha256:before"},
		Verification: []PulseFindingVerification{{
			Check:   "runtime outcome",
			Verdict: VerificationPassed,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "before_refs and after_refs as equal-length positional pairs") ||
		!strings.Contains(err.Error(), "before_refs=1, after_refs=0") {
		t.Fatalf("unpaired fixed_verified references were accepted: %v", err)
	}
}

func TestFixedVerifiedDispositionWhitespaceStillClosesFinding(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	module := "bug_review"
	pulseRunID := "pulse-whitespace"
	concern := filedReviewConcern(t, workspacePath, pulseRunID, module, "whitespace routing regression")

	recordFindingDispositions(t, workspacePath, module, pulseRunID, []PulseFindingDisposition{{
		Fingerprint:  " " + concern.Fingerprint + " ",
		FindingID:    " BUG-SPACE ",
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
// get_pulse_state(view="backlog") resolves here, not through LoadOpenRunConcerns.
// Distinct behavioral findings on one step should remain adjacent so the
// reviewer can reason about their shared execution boundary. Prevalidation
// field failures are consolidated earlier into one step-level finding.
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
	for _, field := range []string{"wrong source", "stale selector", "missing retry"} {
		file("pulse-1", "execute-find-opportunities",
			"execution contract failure: "+field)
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

// TestAwaitingUserRequiresARealPendingQuestion closes the gap that left
// rtslatency with five findings marked awaiting_user and zero pending
// questions: the operator was told five things needed their decision and had
// nothing to answer, with no way to discover why.
func TestAwaitingUserRequiresARealPendingQuestion(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	module := "eval_health"
	pulseRunID := "pulse-1"
	concern := filedReviewConcern(t, workspacePath, pulseRunID, module,
		"eval steps declare no max_score scale")

	disposition := PulseFindingDisposition{
		Fingerprint: concern.Fingerprint,
		FindingID:   "EVAL-1",
		Disposition: FindingDispositionAwaitingUser,
		Summary:     "Needs a decision on the score scale.",
	}

	// No question at all: the label alone must not be accepted.
	if err := validateFindingDisposition(disposition); err == nil ||
		!strings.Contains(err.Error(), "requires human_input_id") {
		t.Fatalf("awaiting_user was accepted with no question to answer: %v", err)
	}

	// A question that was never created must not satisfy it either.
	disposition.HumanInputID = "invented-id"
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		t.Fatalf("open workflow db: %v", err)
	}
	defer db.Close()
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS report_human_inputs (
		id TEXT PRIMARY KEY, workspace_path TEXT, source TEXT, priority TEXT,
		question TEXT, context TEXT, options_json TEXT, allow_free_text INTEGER,
		status TEXT, selected_option_id TEXT, note TEXT, run_id TEXT)`); err != nil {
		t.Fatalf("create human inputs table: %v", err)
	}
	err = RecordPulseFindingDispositionsTx(ctx, db, module, pulseRunID,
		[]PulseFindingDisposition{disposition}, "")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("awaiting_user accepted a human input id that was never created: %v", err)
	}

	// An answered question does not keep a finding waiting either.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO report_human_inputs (id, status) VALUES ('answered-q', 'answered')`); err != nil {
		t.Fatalf("seed answered question: %v", err)
	}
	disposition.HumanInputID = "answered-q"
	if err := RecordPulseFindingDispositionsTx(ctx, db, module, pulseRunID,
		[]PulseFindingDisposition{disposition}, ""); err == nil ||
		!strings.Contains(err.Error(), "only wait on a pending decision") {
		t.Fatalf("a finding was parked on an already-answered question: %v", err)
	}

	// A real pending question is accepted.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO report_human_inputs (id, status) VALUES ('open-q', 'pending')`); err != nil {
		t.Fatalf("seed pending question: %v", err)
	}
	disposition.HumanInputID = "open-q"
	if err := RecordPulseFindingDispositionsTx(ctx, db, module, pulseRunID,
		[]PulseFindingDisposition{disposition}, ""); err != nil {
		t.Fatalf("a genuine pending decision was rejected: %v", err)
	}
}

func TestAdvisorProposalRoutingRequiresEvidenceOrDecision(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	pulseRunID := "pulse-advisor-routing"
	concern := filedReviewConcern(t, workspacePath, pulseRunID, pulsemodules.StrategyAuditorID,
		"current allocation over-concentrates on reciprocal engagement")
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		t.Fatalf("open workflow db: %v", err)
	}
	defer db.Close()
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS report_human_inputs (
		id TEXT PRIMARY KEY, workspace_path TEXT, source TEXT, priority TEXT,
		question TEXT, context TEXT, options_json TEXT, allow_free_text INTEGER,
		status TEXT, selected_option_id TEXT, note TEXT, run_id TEXT)`); err != nil {
		t.Fatalf("create human inputs table: %v", err)
	}

	proposal := PulseFindingDisposition{
		Fingerprint: concern.Fingerprint,
		FindingID:   "STRATEGY-1",
		Disposition: FindingDispositionProposalOnly,
		Summary:     "Reserve more allocation for reach-bearing tactics.",
	}
	if err := RecordPulseFindingDispositionsTx(ctx, db, pulsemodules.StrategyAuditorID, pulseRunID,
		[]PulseFindingDisposition{proposal}, ""); err == nil || !strings.Contains(err.Error(), "without next_check") {
		t.Fatalf("actionable advisor proposal was silently parked without a decision: %v", err)
	}

	proposal.NextCheck = "after three completed outcome-bearing runs, compare follower growth with the current baseline"
	if err := RecordPulseFindingDispositionsTx(ctx, db, pulsemodules.StrategyAuditorID, pulseRunID,
		[]PulseFindingDisposition{proposal}, ""); err != nil {
		t.Fatalf("evidence-waiting advisor proposal was rejected: %v", err)
	}
}

func TestAdvisorAwaitingUserRequiresOwnedDecision(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	pulseRunID := "pulse-goal-decision"
	concern := filedReviewConcern(t, workspacePath, pulseRunID, pulsemodules.GoalAdvisorID,
		"a new distribution channel could materially increase reach")
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		t.Fatalf("open workflow db: %v", err)
	}
	defer db.Close()
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS report_human_inputs (
		id TEXT PRIMARY KEY, workspace_path TEXT, source TEXT, priority TEXT,
		question TEXT, context TEXT, options_json TEXT, allow_free_text INTEGER,
		status TEXT, selected_option_id TEXT, note TEXT, run_id TEXT)`); err != nil {
		t.Fatalf("create human inputs table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO report_human_inputs (id, source, status) VALUES
		('plan-proposal-new-channel', 'strategy_auditor', 'pending'),
		('strategy-proposal-new-channel', 'goal_advisor', 'pending'),
		('plan-proposal-owned-channel', 'goal_advisor', 'pending')`); err != nil {
		t.Fatalf("seed decisions: %v", err)
	}

	disposition := PulseFindingDisposition{
		Fingerprint:  concern.Fingerprint,
		FindingID:    "GOAL-1",
		Disposition:  FindingDispositionAwaitingUser,
		Summary:      "Ask whether to test a new distribution channel.",
		HumanInputID: "plan-proposal-new-channel",
	}
	if err := RecordPulseFindingDispositionsTx(ctx, db, pulsemodules.GoalAdvisorID, pulseRunID,
		[]PulseFindingDisposition{disposition}, ""); err == nil || !strings.Contains(err.Error(), `source "strategy_auditor"`) {
		t.Fatalf("Goal Advisor accepted another module's decision: %v", err)
	}

	disposition.HumanInputID = "strategy-proposal-new-channel"
	if err := RecordPulseFindingDispositionsTx(ctx, db, pulsemodules.GoalAdvisorID, pulseRunID,
		[]PulseFindingDisposition{disposition}, ""); err == nil || !strings.Contains(err.Error(), `must start with "plan-proposal-"`) {
		t.Fatalf("Goal Advisor accepted the wrong decision id namespace: %v", err)
	}

	disposition.HumanInputID = "plan-proposal-owned-channel"
	if err := RecordPulseFindingDispositionsTx(ctx, db, pulsemodules.GoalAdvisorID, pulseRunID,
		[]PulseFindingDisposition{disposition}, ""); err != nil {
		t.Fatalf("Goal Advisor's real pending decision was rejected: %v", err)
	}
}

// TestAwaitingRunSeparatesWaitingFromBlocked covers the distinction rtslatency
// had no way to express.
//
// Four of its nine "blocked" findings were only waiting for data: security rows
// missing because those steps had not run, and an approved experiment unable to
// ship because the digest step had not executed since 2026-07-29. blocked
// absorbed them because changed_unverified demands a fix attempt with changed
// files, and nothing was fixed. Reading those as blockers points the operator at
// decisions that do not exist.
func TestAwaitingRunSeparatesWaitingFromBlocked(t *testing.T) {
	base := PulseFindingDisposition{
		Fingerprint: "fp", FindingID: "SEC-1",
		Disposition: FindingDispositionAwaitingRun,
		Summary:     "security_daily_metrics has no rows; those steps did not run.",
	}

	// Waiting with no stated boundary is indistinguishable from stalling.
	if err := validateFindingDisposition(base); err == nil ||
		!strings.Contains(err.Error(), "requires next_check") {
		t.Fatalf("awaiting_run accepted with no evidence boundary: %v", err)
	}

	// A finding that changed files is a fix awaiting proof, not a wait.
	withFix := base
	withFix.NextCheck = "next scheduled dev collection run"
	withFix.ChangedFiles = []string{"planning/plan.json"}
	if err := validateFindingDisposition(withFix); err == nil ||
		!strings.Contains(err.Error(), "changed_unverified") {
		t.Fatalf("a finding with changes applied was accepted as awaiting_run: %v", err)
	}

	valid := base
	valid.NextCheck = "next scheduled dev collection run"
	if err := validateFindingDisposition(valid); err != nil {
		t.Fatalf("a genuine wait-for-data finding was rejected: %v", err)
	}

	// It must not land in the acknowledged bucket that blocked and awaiting_user
	// share, or the UI cannot tell them apart.
	status, event, _ := lifecycleStatusForDisposition(FindingDispositionAwaitingRun)
	if status != ConcernStatusAwaitingRun || status == ConcernStatusAcknowledged {
		t.Fatalf("awaiting_run mapped to status %q; it must be distinguishable from blocked", status)
	}
	if event != "awaiting_run" {
		t.Fatalf("awaiting_run event = %q", event)
	}
}

// TestVerificationCanCloseAnAttemptFromAnEarlierRun covers the flow the
// lifecycle mandates and used to block.
//
// changed_unverified exists so a fix whose proof needs a future run is recorded
// now and verified later — fix-verification, post-run-monitor and the Fixer
// contract all say to record it with reason awaiting_next_valid_run. Requiring
// the attempt to belong to the closing run made that impossible: the evidence
// arrives a run later, and the disposition carrying it was rejected as
// belonging to a previous Pulse run. social-media hit this on 2026-08-01 and
// preserved the unresolved state rather than forcing a second write.
func TestVerificationCanCloseAnAttemptFromAnEarlierRun(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	module := "bug_review"
	concern := filedReviewConcern(t, workspacePath, "pulse-1", module, "collector writes a null column")

	// Run 1 applies the fix; proof needs the next producing run. The backend
	// opens the attempt from this disposition.
	recordFindingDispositions(t, workspacePath, module, "pulse-1", []PulseFindingDisposition{{
		Fingerprint: concern.Fingerprint, FindingID: "BUG-1",
		Disposition: FindingDispositionChangedUnverified, Summary: "Applied; awaiting next valid run.",
		NextCheck:    "next scheduled dev collection run writes latency_daily_metrics",
		ChangedFiles: []string{"planning/plan.json"},
		BeforeRefs:   []string{"before"}, AfterRefs: []string{"after"},
		Verification: []PulseFindingVerification{{Check: "consumer read", Verdict: VerificationInconclusive}},
	}})
	opened, err := LoadPulseFindingLifecycles(ctx, workspacePath, module, 10)
	if err != nil || len(opened) != 1 || len(opened[0].Attempts) != 1 {
		t.Fatalf("changed_unverified did not open exactly one attempt: %+v err=%v", opened, err)
	}
	attemptID := opened[0].Attempts[0].AttemptID

	// Run 2 has the evidence and closes the same attempt, without naming it.
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := RecordPulseFindingDispositionsTx(ctx, db, module, "pulse-2", []PulseFindingDisposition{{
		Fingerprint: concern.Fingerprint, FindingID: "BUG-1",
		Disposition: FindingDispositionFixedVerified, Summary: "Next run produced a non-null column.",
		ChangedFiles: []string{"planning/plan.json"},
		Verification: []PulseFindingVerification{{Check: "consumer read", Verdict: VerificationPassed}},
	}}, ""); err != nil {
		t.Fatalf("a later run could not verify its own module's earlier attempt: %v", err)
	}
	closed, err := LoadPulseFindingLifecycles(ctx, workspacePath, module, 10)
	if err != nil || len(closed) != 1 || len(closed[0].Attempts) != 1 ||
		closed[0].Attempts[0].AttemptID != attemptID {
		t.Fatalf("the later run invented a second attempt instead of settling %q: %+v err=%v", attemptID, closed, err)
	}

	// Another module still cannot close it, even naming the attempt directly.
	if err := RecordPulseFindingDispositionsTx(ctx, db, "strategy_auditor", "pulse-2", []PulseFindingDisposition{{
		Fingerprint: concern.Fingerprint, FindingID: "BUG-1", AttemptID: attemptID,
		Disposition: FindingDispositionVerifiedNoChange, Summary: "Not mine to close.",
		Verification: []PulseFindingVerification{{Check: "x", Verdict: VerificationPassed}},
	}}, ""); err == nil || !strings.Contains(err.Error(), "belongs to module") {
		t.Fatalf("another module closed an attempt it did not make: %v", err)
	}
}

// TestChangedUnverifiedMustNameWhatWillSettleIt makes the verification loop
// closable.
//
// A fix awaiting proof is only verifiable if the next reviewer can tell whether
// the evidence has arrived. Without a named boundary it cannot, so it re-attempts
// the fix instead of checking it — rtslatency carried a finding at seen_count 4,
// still awaiting_verification, repaired again on every pass because nothing ever
// checked the run that had since produced its evidence.
func TestChangedUnverifiedMustNameWhatWillSettleIt(t *testing.T) {
	base := PulseFindingDisposition{
		Fingerprint: "fp", FindingID: "BUG-1",
		Disposition:  FindingDispositionChangedUnverified,
		Summary:      "Collector now writes the column.",
		ChangedFiles: []string{"planning/plan.json"},
		Verification: []PulseFindingVerification{{Check: "consumer read", Verdict: VerificationInconclusive}},
	}

	if err := validateFindingDisposition(base); err == nil ||
		!strings.Contains(err.Error(), "requires next_check") {
		t.Fatalf("a fix awaiting proof was accepted with nothing naming what would settle it: %v", err)
	}

	settled := base
	settled.NextCheck = "next dev collection run writes latency_daily_metrics"
	if err := validateFindingDisposition(settled); err != nil {
		t.Fatalf("a fix naming its evidence boundary was rejected: %v", err)
	}
}

// The consolidated Fixer is instructed to pass module="pulse_fixer", meaning
// every due module. This read path treats module as a plain step_id filter, so
// the sentinel matched nothing: the backlog view returned 0 rows for
// the one value the Fixer's own contract tells it to send, against 149 for an
// omitted filter on social-media. It only did any work by falling back to
// omitting the module.
func TestPulseFixerSentinelLoadsEveryModulesBacklog(t *testing.T) {
	ctx := context.Background()
	workspacePath := concernsWorkspace(t)
	pulseRunID := "pulse-consolidated"
	// Filed directly: filedReviewConcern asserts a single open concern exists,
	// and this case needs two modules represented at once.
	for module, text := range map[string]string{
		"bug_review":    "reply targets repeat across runs",
		"stores_health": "follower delta writer is missing",
	} {
		if _, err := RecordRunConcerns(
			ctx, workspacePath, pulseRunID, "", module, ConcernPhaseReview, "CONCERNS: "+text,
		); err != nil {
			t.Fatalf("record %s concern: %v", module, err)
		}
	}

	sentinel, err := LoadPulseFindingLifecycles(ctx, workspacePath, "pulse_fixer", -1)
	if err != nil {
		t.Fatalf("load backlog for the fixer sentinel: %v", err)
	}
	everything, err := LoadPulseFindingLifecycles(ctx, workspacePath, "", -1)
	if err != nil {
		t.Fatalf("load complete backlog: %v", err)
	}
	if len(sentinel) != len(everything) {
		t.Fatalf("pulse_fixer returned %d findings, want the complete backlog of %d",
			len(sentinel), len(everything))
	}
	if len(sentinel) != 2 {
		t.Fatalf("sentinel backlog = %d findings, want both modules", len(sentinel))
	}

	// A real perspective must still filter, while its retired artifact aliases
	// remain one Engineering backlog.
	scoped, err := LoadPulseFindingLifecycles(ctx, workspacePath, "workflow_review", -1)
	if err != nil {
		t.Fatalf("load scoped backlog: %v", err)
	}
	if len(scoped) != 2 {
		t.Fatalf("Engineering backlog = %+v, want both retired aliases", scoped)
	}
}

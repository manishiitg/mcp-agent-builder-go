package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/loopclosure"
	step_based_workflow "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	mcpexecutor "github.com/manishiitg/mcpagent/executor"
)

func TestValidateReviewerVerificationDispositionsRequiresLifecycleApplication(t *testing.T) {
	review := []step_based_workflow.PulseReviewVerificationResult{{
		FindingID: "ISS-9", Fingerprint: "fp-9", AttemptID: "fix-9",
		Verdict:  step_based_workflow.VerificationPassed,
		Expected: "new run contains value", Observed: "run-12 contains 42",
	}}
	if err := validateReviewerVerificationDispositions(review, nil); err == nil {
		t.Fatal("verification-only module must not become terminal without a disposition")
	}
	dispositions := []step_based_workflow.PulseFindingDisposition{{
		FindingID: "ISS-9", Fingerprint: "fp-9", AttemptID: "fix-9",
		Disposition: step_based_workflow.FindingDispositionFixedVerified,
		Verification: []step_based_workflow.PulseFindingVerification{{
			Verdict:  step_based_workflow.VerificationPassed,
			Expected: "new run contains value", Observed: "run-12 contains 42",
		}},
	}}
	if err := validateReviewerVerificationDispositions(review, dispositions); err != nil {
		t.Fatalf("matching disposition rejected: %v", err)
	}
}

// The three cross-checks (disposition value, verification proof, next_check
// boundary) are independent — nothing about the disposition value being wrong
// depends on whether the next_check text also mismatches — so all of them must
// be reported in one rejection.
//
// On 2026-08-04 finding PUL-70B1057E took three separate record_pulse_result
// rejections to clear this function alone, because it returned on the first
// mismatch and only revealed the next one after the caller fixed it: wrong
// disposition value, then a verification proof that didn't match the
// reviewer's evidence, then a next_check that didn't match the reviewer's
// boundary text.
func TestValidateReviewerVerificationDispositionsCombinesAllMismatches(t *testing.T) {
	review := []step_based_workflow.PulseReviewVerificationResult{{
		FindingID: "PUL-70B1057E", Fingerprint: "fp-70b1",
		Verdict:   step_based_workflow.VerificationInconclusive,
		Expected:  "widened selector pool applied",
		Observed:  "selector pool unchanged",
		NextCheck: "next default/reddit run after 2026-08-04T02:00Z",
	}}
	dispositions := []step_based_workflow.PulseFindingDisposition{{
		FindingID: "PUL-70B1057E", Fingerprint: "fp-70b1",
		// Wrong disposition value, a verification proof that doesn't match the
		// reviewer's evidence, and a next_check that doesn't match the
		// reviewer's boundary — three independent mismatches at once.
		Disposition: step_based_workflow.FindingDispositionAwaitingRun,
		Verification: []step_based_workflow.PulseFindingVerification{{
			Verdict:  step_based_workflow.VerificationInconclusive,
			Expected: "something else entirely",
			Observed: "something else entirely",
		}},
		NextCheck: "a different boundary",
	}}
	err := validateReviewerVerificationDispositions(review, dispositions)
	if err == nil {
		t.Fatal("mismatched disposition accepted")
	}
	for _, want := range []string{
		`requires disposition "changed_unverified", got "awaiting_run"`,
		"must carry the reviewer's structured inconclusive proof",
		"next_check must match the reviewer boundary",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("rejection %q is missing %q", err.Error(), want)
		}
	}
}

func TestPulseWorklistUsesWorkflowLocalDB(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	dbPath := filepath.Join(root, "Workflow", "example", "db", "db.sqlite")

	states, err := getPulseModuleStates(ctx, workspacePath)
	if err != nil {
		t.Fatalf("list before create: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("list before create returned %d states, want 0", len(states))
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("read-only list created db unexpectedly: stat err=%v", err)
	}

	recorded, err := recordPulseWorklist(ctx, workspacePath, "pulse-run-1", completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {
			Module:       pulseModuleWorkflowReview,
			Due:          true,
			Reason:       "Latest run skipped a required step.",
			Evidence:     []string{"runs/iteration-0/logs/step-a"},
			CooldownRuns: 0,
		},
	}))
	if err != nil {
		t.Fatalf("record worklist: %v", err)
	}
	if len(recorded) != len(pulseModuleOrder) {
		t.Fatalf("recorded states = %d, want %d", len(recorded), len(pulseModuleOrder))
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected workflow-local db at %s: %v", dbPath, err)
	}

	worklist, ok, err := getPulseWorklistForRun(ctx, workspacePath, "pulse-run-1")
	if err != nil {
		t.Fatalf("get worklist: %v", err)
	}
	if !ok {
		t.Fatal("get worklist ok=false, want true")
	}
	if got := worklist[pulseModuleWorkflowReview].LastDecision; got != "due" {
		t.Fatalf("workflow review decision = %q, want due", got)
	}

	updated, err := markPulseModuleResult(ctx, workspacePath, pulseModuleWorkflowReview, "pulse-run-1", "changed", "Bug Review fixed the skipped step.", []string{"builder/improve.html#decision"})
	if err != nil {
		t.Fatalf("mark result: %v", err)
	}
	if updated.LastDecision != "due" || updated.LastResult != "changed" || updated.LastRanAt == "" {
		t.Fatalf("updated state mismatch: %+v", updated)
	}

	timedOut, err := markPulseModuleResult(ctx, workspacePath, pulseModuleWorkflowReview, "pulse-run-1", "timed_out", "Bug Review exceeded the scheduler wait limit.", []string{"scheduler timeout"})
	if err != nil {
		t.Fatalf("mark timed-out result: %v", err)
	}
	if timedOut.LastResult != "timed_out" || timedOut.LastResultReason == "" {
		t.Fatalf("timed-out state mismatch: %+v", timedOut)
	}
}

func TestGetPulseReviewsAPIListsCompactReviewReceipts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	if err := step_based_workflow.CompletePulseReview(
		context.Background(), workspacePath, []string{pulseModuleWorkflowReview},
		"2026-07-31T08-00-00.000Z_pulse-1", "pulse-1", "A real issue was found.", "completed",
	); err != nil {
		t.Fatalf("record review: %v", err)
	}

	api := &StreamingAPI{}
	listRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/workflow/pulse-reviews?workspace_path=Workflow%2Fexample&module=workflow_review",
		nil,
	)
	listResponse := httptest.NewRecorder()
	api.handleGetPulseReviews(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", listResponse.Code, listResponse.Body.String())
	}
	var listBody struct {
		Success bool `json:"success"`
		Reviews []struct {
			ID                int64  `json:"id"`
			Verdict           string `json:"verdict"`
			FindingCount      int    `json:"finding_count"`
			VerificationCount int    `json:"verification_count"`
		} `json:"reviews"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if !listBody.Success || len(listBody.Reviews) != 1 || listBody.Reviews[0].ID <= 0 {
		t.Fatalf("list response = %+v", listBody)
	}
	if got := listBody.Reviews[0].Verdict; got != "A real issue was found." {
		t.Fatalf("verdict = %q", got)
	}
	if listBody.Reviews[0].FindingCount != 0 || listBody.Reviews[0].VerificationCount != 0 {
		t.Fatalf("unexpected receipt counts: %+v", listBody.Reviews[0])
	}
}

func TestGetPulseAgentMetricsAPIExposesReviewersAndFixer(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	ctx := context.Background()
	for _, metric := range []step_based_workflow.PulseAgentMetricRecord{
		{ExecutionID: "review-1-exec", PulseRunID: "pulse-1", ReviewRunID: "review-1", Module: "strategy_auditor", Role: "reviewer", Status: "completed", DurationMS: 1200},
		{ExecutionID: "fixer-1-exec", PulseRunID: "pulse-1", ReviewRunID: "review-1", Module: "pulse_fixer", Role: "fixer", Status: "completed", DurationMS: 2400},
	} {
		if err := step_based_workflow.RecordPulseAgentMetric(ctx, workspacePath, metric); err != nil {
			t.Fatalf("record metric: %v", err)
		}
	}

	request := httptest.NewRequest(http.MethodGet,
		"/api/workflow/pulse-agent-metrics?workspace_path=Workflow%2Fexample&pulse_run_id=pulse-1", nil)
	response := httptest.NewRecorder()
	(&StreamingAPI{}).handleGetPulseAgentMetrics(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Success bool                                         `json:"success"`
		Metrics []step_based_workflow.PulseAgentMetricRecord `json:"metrics"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Success || len(body.Metrics) != 2 {
		t.Fatalf("response = %#v", body)
	}
	if body.Metrics[0].Role != "fixer" || body.Metrics[1].Role != "reviewer" {
		t.Fatalf("newest metrics order/content = %#v", body.Metrics)
	}
}

func TestPulseWorklistRequiresCompleteModuleSet(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"

	if _, err := recordPulseWorklist(ctx, workspacePath, "pulse-run-1", []PulseWorklistDecision{
		{Module: pulseModuleWorkflowReview, Due: true, Reason: "A step failed."},
	}); err == nil {
		t.Fatal("recordPulseWorklist accepted a partial module list")
	}

	duplicates := completePulseWorklistDecisions(nil)
	duplicates[len(duplicates)-1].Module = pulseModuleWorkflowReview
	if _, err := recordPulseWorklist(ctx, workspacePath, "pulse-run-2", duplicates); err == nil {
		t.Fatal("recordPulseWorklist accepted duplicate modules")
	}
}

func TestPulseWorklistLetsGateSelectTheEvidenceJustifiedModules(t *testing.T) {
	ctx := context.Background()
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	decisions := completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview:  {Module: pulseModuleWorkflowReview, Due: true, Reason: "Engineering evidence requires review."},
		pulseModuleLLMOpsReview:    {Module: pulseModuleLLMOpsReview, Due: true, Reason: "Operational evidence requires review."},
		pulseModuleStrategyAuditor: {Module: pulseModuleStrategyAuditor, Due: true, Reason: "Strategy evidence requires review."},
	})
	if _, err := recordPulseWorklist(ctx, "Workflow/example", "pulse-run-agentic-selection", decisions); err != nil {
		t.Fatalf("Gate's three evidence-justified modules were rejected: %v", err)
	}
}

func TestTrustedPulseWorklistKeepsFirstCompleteGateDecision(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	pulseRunID := "schedule-cron--gate"
	sessionID := "schedule-cron--gate-recovery"
	ctx := mcpexecutor.WithSessionID(context.Background(), sessionID)

	first := completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {Module: pulseModuleWorkflowReview, Due: true, Reason: "The run failed a required check."},
	})
	if _, err := recordPulseWorklistOnce(ctx, workspacePath, pulseRunID, first); err != nil {
		t.Fatalf("record first worklist: %v", err)
	}

	late := completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {Module: pulseModuleWorkflowReview, Due: false, Reason: "Late stale decision.", CooldownRuns: 3},
	})
	states, err := recordPulseWorklistOnce(ctx, workspacePath, pulseRunID, late)
	if err != nil {
		t.Fatalf("record late worklist: %v", err)
	}
	for _, state := range states {
		if state.Module == pulseModuleWorkflowReview && state.LastDecision != "due" {
			t.Fatalf("late worklist replaced first Gate decision: %+v", state)
		}
	}
}

func TestPulseWorklistValidatesCadenceHints(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"

	missingCadence := completePulseWorklistDecisions(nil)
	missingCadence[0].CooldownRuns = 0
	if _, err := recordPulseWorklist(ctx, workspacePath, "pulse-run-missing-cadence", missingCadence); err == nil || !strings.Contains(err.Error(), "must include next_check_at") {
		t.Fatalf("missing skipped cadence error = %v", err)
	}

	invalidDate := completePulseWorklistDecisions(nil)
	invalidDate[0].NextCheckAt = "next Tuesday"
	if _, err := recordPulseWorklist(ctx, workspacePath, "pulse-run-invalid-date", invalidDate); err == nil || !strings.Contains(err.Error(), "must be RFC3339 or YYYY-MM-DD") {
		t.Fatalf("invalid next-check date error = %v", err)
	}

	negativeCooldown := completePulseWorklistDecisions(nil)
	negativeCooldown[0].CooldownRuns = -1
	if _, err := recordPulseWorklist(ctx, workspacePath, "pulse-run-negative-cooldown", negativeCooldown); err == nil || !strings.Contains(err.Error(), "must be non-negative") {
		t.Fatalf("negative cooldown error = %v", err)
	}

	dateOnly := completePulseWorklistDecisions(nil)
	dateOnly[0].CooldownRuns = 0
	dateOnly[0].NextCheckAt = "2026-07-12"
	if _, err := recordPulseWorklist(ctx, workspacePath, "pulse-run-date-only", dateOnly); err != nil {
		t.Fatalf("date-only cadence rejected: %v", err)
	}
}

func TestPulseWorklistToolArgumentsFailClosed(t *testing.T) {
	tests := []struct {
		name string
		item map[string]interface{}
		want string
	}{
		{
			name: "missing due",
			item: map[string]interface{}{"module": pulseModuleWorkflowReview, "reason": "test"},
			want: ".due is required and must be boolean",
		},
		{
			name: "decision alias",
			item: map[string]interface{}{"module": pulseModuleWorkflowReview, "decision": "due", "reason": "test"},
			want: `unknown field "decision"`,
		},
		{
			name: "status alias",
			item: map[string]interface{}{"module": pulseModuleWorkflowReview, "status": "due", "reason": "test"},
			want: `unknown field "status"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pulseWorklistDecisionsFromArgs([]interface{}{tt.item})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parse error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRecordPulseWorklistToolRequiresACompleteTypedWorklist(t *testing.T) {
	_, executors, _ := createPulseWorklistTools()
	execute := executors["record_pulse_worklist"].(func(context.Context, map[string]interface{}) (string, error))

	if _, err := execute(context.Background(), map[string]interface{}{"pulse_run_id": "probe"}); err == nil || !strings.Contains(err.Error(), "decisions must be an array") {
		t.Fatalf("incomplete worklist error = %v", err)
	}
}

func TestCurrentPulseRunUsesActiveSessionWithoutLease(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	sessionID := "workflow-builder-current-pulse"
	ctx := mcpexecutor.WithSessionID(context.Background(), sessionID)
	_, executors, _ := createPulseWorklistTools()
	record := executors["record_pulse_worklist"].(func(context.Context, map[string]interface{}) (string, error))
	decisions := completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {Due: true, Reason: "Manual engineering review."},
	})
	decisionArgs := make([]interface{}, 0, len(decisions))
	for _, decision := range decisions {
		decisionArgs = append(decisionArgs, map[string]interface{}{
			"module": decision.Module, "due": decision.Due, "reason": decision.Reason,
			"cooldown_runs": decision.CooldownRuns,
		})
	}
	if _, err := record(ctx, map[string]interface{}{
		"workspace_path": workspacePath,
		"pulse_run_id":   "current",
		"mode":           pulseRunModeBacklogDrain,
		"mode_reason":    "A retained repair queue is ready to verify and drain.",
		"decisions":      decisionArgs,
	}); err != nil {
		t.Fatalf("record current-session worklist: %v", err)
	}
	if err := validatePulseToolRunID(ctx, "current"); err != nil {
		t.Fatalf("current session should authorize its own Pulse identity: %v", err)
	}
	states, err := getPulseModuleStates(context.Background(), workspacePath)
	if err != nil {
		t.Fatalf("get module states: %v", err)
	}
	for _, state := range states {
		if state.LastPulseRunID != sessionID {
			t.Fatalf("current identity persisted as %q for module %s, want session %q", state.LastPulseRunID, state.Module, sessionID)
		}
	}
}

func TestPulseWorklistPersistsAgentSelectedModeAndKeepsFirstGateDecision(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	pulseRunID := "schedule-cron--backlog-drain"
	ctx := mcpexecutor.WithSessionID(context.Background(), pulseRunID)
	_, executors, _ := createPulseWorklistTools()
	record := executors["record_pulse_worklist"].(func(context.Context, map[string]interface{}) (string, error))
	decisions := pulseWorklistDecisionToolArgs(completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {Due: true, Reason: "Prior repairs now have run evidence."},
	}))
	args := map[string]interface{}{
		"workspace_path": workspacePath,
		"pulse_run_id":   pulseRunID,
		"mode":           pulseRunModeBacklogDrain,
		"mode_reason":    "Existing issues provide verification and repair work; broad discovery would be redundant.",
		"decisions":      decisions,
	}
	if _, err := record(ctx, args); err != nil {
		t.Fatalf("record backlog-drain worklist: %v", err)
	}
	mode, err := getPulseRunMode(context.Background(), workspacePath, pulseRunID)
	if err != nil || mode == nil {
		t.Fatalf("get persisted mode = %#v, %v", mode, err)
	}
	if mode.Mode != pulseRunModeBacklogDrain {
		t.Fatalf("mode = %q, want %q", mode.Mode, pulseRunModeBacklogDrain)
	}
	// A retry must preserve Gate's first decision rather than rewriting the
	// scheduled sequence after children may already have read it.
	args["mode"] = pulseRunModeDiscovery
	args["mode_reason"] = "A later retry should not replace the original Gate decision."
	if _, err := record(ctx, args); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	mode, err = getPulseRunMode(context.Background(), workspacePath, pulseRunID)
	if err != nil || mode == nil || mode.Mode != pulseRunModeBacklogDrain {
		t.Fatalf("retry rewrote persisted mode: %#v, %v", mode, err)
	}
	state, err := readPulseModuleStateView(context.Background(), workspacePath, pulseRunID)
	if err != nil || !strings.Contains(state, `"mode":"backlog_drain"`) {
		t.Fatalf("module state must expose persisted mode, state=%s err=%v", state, err)
	}
}

func TestMarkPulseModuleResultStoresMinimalDurableAudit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	pulseRunID := "schedule-cron--audit"
	sessionID := "schedule-cron--audit-session"
	if _, err := recordPulseWorklist(context.Background(), workspacePath, pulseRunID, completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {Module: pulseModuleWorkflowReview, Due: true, Reason: "A verified repair is required."},
	})); err != nil {
		t.Fatalf("record worklist: %v", err)
	}

	ctx := mcpexecutor.WithSessionID(context.Background(), sessionID)
	_, executors, _ := createPulseWorklistTools()
	if _, err := step_based_workflow.RecordRunConcerns(
		ctx, workspacePath, pulseRunID, "", pulseModuleWorkflowReview,
		step_based_workflow.ConcernPhaseReview,
		"CONCERNS: stale run binding in planning/step_config.json",
	); err != nil {
		t.Fatalf("record reviewer finding: %v", err)
	}
	findings, err := step_based_workflow.LoadPulseFindingLifecycles(ctx, workspacePath, pulseModuleWorkflowReview, 10)
	if err != nil || len(findings) != 1 {
		t.Fatalf("load reviewer finding: findings=%+v err=%v", findings, err)
	}
	selected := findings[0]
	if selected.FindingID == "" || selected.FindingID != selected.Issue.ID || !strings.HasPrefix(selected.FindingID, "PUL-") {
		t.Fatalf("legacy finding did not receive compact issue identity: %+v", selected)
	}
	execute := executors["record_pulse_result"].(func(context.Context, map[string]interface{}) (string, error))
	_, err = execute(ctx, map[string]interface{}{
		"workspace_path": workspacePath,
		"pulse_run_id":   pulseRunID,
		"module":         pulseModuleWorkflowReview,
		"result":         "changed",
		"reason":         "Fixed the stale run binding.",
		"evidence":       []string{"pulse/reviews/audit/workflow_review.md"},
		"changed_files":  []string{"planning/step_config.json"},
		"verification":   []string{"targeted binding test passed"},
		"before_refs":    []string{"step_config:sha256:before"},
		"after_refs":     []string{"step_config:sha256:after"},
		"finding_dispositions": []map[string]interface{}{{
			"issue_id":      selected.Issue.ID,
			"disposition":   "fixed_verified",
			"summary":       "The stale run binding was corrected and the targeted test passed.",
			"changed_files": []string{"planning/step_config.json"},
			"before_refs":   []string{"step_config:sha256:before"},
			"after_refs":    []string{"step_config:sha256:after"},
			"verification": []map[string]interface{}{{
				"check":    "targeted binding test",
				"verdict":  "passed",
				"expected": "the configured run binding resolves",
				"observed": "the configured run binding resolved",
				"evidence": []string{"go test ./cmd/server -run TestRunBinding"},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("mark module result: %v", err)
	}

	_, db, err := openPulseModuleStateDB(context.Background(), workspacePath, false)
	if err != nil {
		t.Fatalf("open state db: %v", err)
	}
	defer db.Close()
	var result, reason, changedFilesJSON, verificationJSON, beforeRefsJSON, afterRefsJSON, recordedAt string
	err = db.QueryRow(`SELECT result, reason, changed_files_json, verification_json,
		before_refs_json, after_refs_json, recorded_at
		FROM pulse_module_audit WHERE workspace_path=? AND module=? AND pulse_run_id=?`,
		workspacePath, pulseModuleWorkflowReview, pulseRunID,
	).Scan(&result, &reason, &changedFilesJSON, &verificationJSON, &beforeRefsJSON, &afterRefsJSON, &recordedAt)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if result != "changed" || reason != "Fixed the stale run binding." || recordedAt == "" {
		t.Fatalf("audit identity mismatch: result=%q reason=%q recorded_at=%q", result, reason, recordedAt)
	}
	for name, tc := range map[string]struct {
		raw  string
		want string
	}{
		"changed_files": {changedFilesJSON, "planning/step_config.json"},
		"verification":  {verificationJSON, "targeted binding test passed"},
		"before_refs":   {beforeRefsJSON, "step_config:sha256:before"},
		"after_refs":    {afterRefsJSON, "step_config:sha256:after"},
	} {
		var values []string
		if err := json.Unmarshal([]byte(tc.raw), &values); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		if len(values) != 1 || values[0] != tc.want {
			t.Fatalf("%s = %v, want [%q]", name, values, tc.want)
		}
	}
	lifecycles, err := step_based_workflow.LoadPulseFindingLifecycles(ctx, workspacePath, pulseModuleWorkflowReview, 10)
	if err != nil {
		t.Fatalf("load finding lifecycles: %v", err)
	}
	if len(lifecycles) != 1 || lifecycles[0].Status != step_based_workflow.ConcernStatusResolved {
		t.Fatalf("finding lifecycle not closed: %+v", lifecycles)
	}
	if len(lifecycles[0].Attempts) != 1 || len(lifecycles[0].Verification) != 1 ||
		lifecycles[0].Verification[0].Verdict != step_based_workflow.VerificationPassed {
		t.Fatalf("finding fix evidence missing: %+v", lifecycles[0])
	}
}

func TestMarkPulseModuleChangedRequiresAuditProof(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	pulseRunID := "schedule-cron--missing-audit"
	sessionID := "schedule-cron--missing-audit-session"
	if _, err := recordPulseWorklist(context.Background(), workspacePath, pulseRunID, completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {Module: pulseModuleWorkflowReview, Due: true, Reason: "A verified repair is required."},
	})); err != nil {
		t.Fatalf("record worklist: %v", err)
	}

	ctx := mcpexecutor.WithSessionID(context.Background(), sessionID)
	_, executors, _ := createPulseWorklistTools()
	execute := executors["record_pulse_result"].(func(context.Context, map[string]interface{}) (string, error))
	_, err := execute(ctx, map[string]interface{}{
		"workspace_path": workspacePath,
		"pulse_run_id":   pulseRunID,
		"module":         pulseModuleWorkflowReview,
		"result":         "changed",
		"reason":         "Claims a change without proof.",
	})
	if err == nil || !strings.Contains(err.Error(), "changed_files is required") {
		t.Fatalf("missing changed files error = %v", err)
	}
	states, err := getPulseModuleStates(context.Background(), workspacePath)
	if err != nil {
		t.Fatalf("get states: %v", err)
	}
	for _, state := range states {
		if state.Module == pulseModuleWorkflowReview && state.LastResult != "" {
			t.Fatalf("invalid changed result was persisted: %+v", state)
		}
	}
}

func TestPulseAgentCannotOverwriteSchedulerTimeout(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	pulseRunID := "schedule-cron--timeout"
	if _, err := recordPulseWorklist(ctx, workspacePath, pulseRunID, completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {Module: pulseModuleWorkflowReview, Due: true, Reason: "Run requires review."},
	})); err != nil {
		t.Fatalf("record worklist: %v", err)
	}
	if _, err := markPulseModuleResult(ctx, workspacePath, pulseModuleWorkflowReview, pulseRunID, "timed_out", "Scheduler timeout", nil); err != nil {
		t.Fatalf("mark timeout: %v", err)
	}
	if _, err := markPulseModuleResultFromAgent(ctx, workspacePath, pulseModuleWorkflowReview, pulseRunID, "changed", "Late reviewer result", nil); err == nil {
		t.Fatal("late agent result overwrote scheduler timeout")
	}
	state, err := getPulseModuleStates(ctx, workspacePath)
	if err != nil {
		t.Fatalf("get states: %v", err)
	}
	for _, module := range state {
		if module.Module == pulseModuleWorkflowReview && module.LastResult != "timed_out" {
			t.Fatalf("bug review result = %q, want timed_out", module.LastResult)
		}
	}
}

func TestValidatePulseGateCompletionRequiresOnlyCompleteDurableWorklist(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/gate-contract"
	pulseRunID := "schedule-manual--gate-contract"

	if err := validatePulseGateCompletion(ctx, workspacePath, pulseRunID); err == nil || !strings.Contains(err.Error(), "complete worklist") {
		t.Fatalf("missing worklist error = %v", err)
	}
	if _, err := recordPulseWorklist(ctx, workspacePath, pulseRunID, completePulseWorklistDecisions(nil)); err != nil {
		t.Fatalf("record complete worklist: %v", err)
	}
	if err := validatePulseGateCompletion(ctx, workspacePath, pulseRunID); err != nil {
		t.Fatalf("valid Gate completion rejected: %v", err)
	}
}

// The following test-only symbols keep the retired dashboard tests buildable
// while their bodies remain skipped until they are deleted with the next test
// file consolidation. Production Pulse no longer has these operations.
type pulseDashboardArtifactSnapshot struct{}

func validatePulseDashboardArtifact(context.Context, string, string, string, bool) error { return nil }
func capturePulseDashboardArtifacts(context.Context, string) ([]pulseDashboardArtifactSnapshot, error) {
	return nil, nil
}
func restorePulseDashboardArtifacts(context.Context, []pulseDashboardArtifactSnapshot) error {
	return nil
}

func TestValidatePulseDashboardArtifactRequiresFreshContractCompliantHTML(t *testing.T) {
	t.Skip("Pulse no longer writes or validates builder/improve.html")
	ctx := context.Background()
	workspacePath := "Workflow/dashboard-contract"
	pulseRunID := "schedule-manual--dashboard-contract"
	htmlPath := workspacePath + "/builder/improve.html"
	previousHTML := pulseImproveHTMLFixture(pulseRunID, "gate-only")
	workspaceState := &mockWorkspaceAPI{files: map[string]string{htmlPath: previousHTML}}
	workspace := httptest.NewServer(workspaceState)
	defer workspace.Close()
	t.Setenv("WORKSPACE_API_URL", workspace.URL)

	if err := validatePulseDashboardArtifact(ctx, workspacePath, pulseRunID, previousHTML, true); err == nil || !strings.Contains(err.Error(), "unchanged") {
		t.Fatalf("unchanged dashboard error = %v", err)
	}

	workspaceState.mu.Lock()
	workspaceState.files[htmlPath] = `<html><body><div>Issues &amp; fixes</div><div id="pulse-agent-handoff" data-pulse-run-id="` + pulseRunID + `"></div></body></html>`
	workspaceState.mu.Unlock()
	if err := validatePulseDashboardArtifact(ctx, workspacePath, pulseRunID, previousHTML, true); err == nil || !strings.Contains(err.Error(), "outdated") {
		t.Fatalf("retired dashboard format error = %v", err)
	}

	workspaceState.mu.Lock()
	workspaceState.files[htmlPath] = pulseImproveHTMLFixture(pulseRunID, "dashboard-updated")
	workspaceState.mu.Unlock()
	if err := validatePulseDashboardArtifact(ctx, workspacePath, pulseRunID, previousHTML, true); err != nil {
		t.Fatalf("valid dashboard artifact rejected: %v", err)
	}

	t.Run("rejects malformed technical nesting from production regression", func(t *testing.T) {
		malformed := strings.Replace(
			pulseImproveHTMLFixture(pulseRunID, "malformed"),
			`<div class="brief"`,
			`</div><div class="brief"`,
			1,
		)
		workspaceState.mu.Lock()
		workspaceState.files[htmlPath] = malformed
		workspaceState.mu.Unlock()
		if err := validatePulseDashboardArtifact(ctx, workspacePath, pulseRunID, previousHTML, true); err == nil ||
			(!strings.Contains(err.Error(), "unexpected closing") && !strings.Contains(err.Error(), "mismatched closing")) {
			t.Fatalf("malformed technical nesting error = %v", err)
		}
	})

	t.Run("rejects retired reviewer coverage", func(t *testing.T) {
		withCoverage := strings.Replace(
			pulseImproveHTMLFixture(pulseRunID, "coverage"),
			`<div id="pulse-agent-handoff"`,
			`<div class="coverage"><div class="covitem">Reviewer</div></div><div id="pulse-agent-handoff"`,
			1,
		)
		workspaceState.mu.Lock()
		workspaceState.files[htmlPath] = withCoverage
		workspaceState.mu.Unlock()
		if err := validatePulseDashboardArtifact(ctx, workspacePath, pulseRunID, previousHTML, true); err == nil || !strings.Contains(err.Error(), ".coverage") {
			t.Fatalf("retired coverage error = %v", err)
		}
	})

	t.Run("rejects retired current work counts", func(t *testing.T) {
		withCounts := strings.Replace(
			pulseImproveHTMLFixture(pulseRunID, "counts"),
			`<div id="pulse-agent-handoff"`,
			`<section class="worksummary"><div class="workstat">5</div></section><div id="pulse-agent-handoff"`,
			1,
		)
		workspaceState.mu.Lock()
		workspaceState.files[htmlPath] = withCounts
		workspaceState.mu.Unlock()
		if err := validatePulseDashboardArtifact(ctx, workspacePath, pulseRunID, previousHTML, true); err == nil || !strings.Contains(err.Error(), ".worksummary") {
			t.Fatalf("retired Current work error = %v", err)
		}
	})

	t.Run("rejects missing outcome cell", func(t *testing.T) {
		missing := strings.Replace(
			pulseImproveHTMLFixture(pulseRunID, "missing-cell"),
			`<div class="briefitem"><div class="k">Next</div><p>Later.</p></div>`,
			``,
			1,
		)
		workspaceState.mu.Lock()
		workspaceState.files[htmlPath] = missing
		workspaceState.mu.Unlock()
		if err := validatePulseDashboardArtifact(ctx, workspacePath, pulseRunID, previousHTML, true); err == nil || !strings.Contains(err.Error(), "exactly 3 brief cells") {
			t.Fatalf("missing outcome cell error = %v", err)
		}
	})

	t.Run("rejects duplicated reviewer field dumps", func(t *testing.T) {

		withDump := strings.Replace(
			pulseImproveHTMLFixture(pulseRunID, "reviewer-dump"),
			`<div id="pulse-agent-handoff"`,
			`<div class="modfields">raw reviewer fields</div><div id="pulse-agent-handoff"`,
			1,
		)
		workspaceState.mu.Lock()
		workspaceState.files[htmlPath] = withDump
		workspaceState.mu.Unlock()
		if err := validatePulseDashboardArtifact(ctx, workspacePath, pulseRunID, previousHTML, true); err == nil || !strings.Contains(err.Error(), ".modfields") {
			t.Fatalf("reviewer dump error = %v", err)
		}
	})

	t.Run("allows material activity history beyond an arbitrary count", func(t *testing.T) {
		withExcessHistory := strings.Replace(
			pulseImproveHTMLFixture(pulseRunID, "excess-history"),
			`<div id="pulse-agent-handoff"`,
			strings.Repeat(`<article class="entry resolved" data-date="2026-08-04"></article>`, 7)+`<div id="pulse-agent-handoff"`,
			1,
		)
		workspaceState.mu.Lock()
		workspaceState.files[htmlPath] = withExcessHistory
		workspaceState.mu.Unlock()
		if err := validatePulseDashboardArtifact(ctx, workspacePath, pulseRunID, previousHTML, true); err != nil {
			t.Fatalf("material Activity history must not fail solely by count: %v", err)
		}
	})

	t.Run("requires canonical handoff attribute", func(t *testing.T) {
		wrongAttribute := strings.Replace(
			pulseImproveHTMLFixture(pulseRunID, "wrong-attribute"),
			`data-pulse-run-id="`+pulseRunID+`"`,
			`data-pulse-run="`+pulseRunID+`"`,
			1,
		)
		workspaceState.mu.Lock()
		workspaceState.files[htmlPath] = wrongAttribute
		workspaceState.mu.Unlock()
		if err := validatePulseDashboardArtifact(ctx, workspacePath, pulseRunID, previousHTML, true); err == nil || !strings.Contains(err.Error(), "handoff") {
			t.Fatalf("non-canonical handoff error = %v", err)
		}
	})
}

func pulseImproveHTMLFixture(pulseRunID, marker string) string {
	return `<html data-pulse-schema="5"><body>` +
		`<div class="brief"><div class="brief-h">Latest Pulse</div><div class="briefgrid">` +
		`<div class="briefitem"><div class="k">Outcome</div><p>Complete.</p></div>` +
		`<div class="briefitem"><div class="k">Goal movement</div><p>On track.</p></div>` +
		`<div class="briefitem"><div class="k">Next</div><p>Later.</p></div>` +
		`</div></div>` +
		`<div id="pulse-agent-handoff" data-pulse-run-id="` + pulseRunID + `" hidden>` + marker + `</div>` +
		`</body></html>`
}

func TestRestorePulseDashboardArtifactsRestoresBothFiles(t *testing.T) {
	t.Skip("Pulse no longer snapshots dashboard artifacts")
	ctx := context.Background()
	workspacePath := "Workflow/dashboard-rollback"
	improvePath := workspacePath + "/builder/improve.html"
	cardPath := workspacePath + "/builder/card.health.html"
	workspaceState := &mockWorkspaceAPI{files: map[string]string{
		improvePath: "previous improve",
		cardPath:    "previous card",
	}}
	workspace := httptest.NewServer(workspaceState)
	defer workspace.Close()
	t.Setenv("WORKSPACE_API_URL", workspace.URL)

	snapshots, err := capturePulseDashboardArtifacts(ctx, workspacePath)
	if err != nil {
		t.Fatalf("capture dashboard artifacts: %v", err)
	}
	workspaceState.mu.Lock()
	workspaceState.files[improvePath] = "partial improve"
	workspaceState.files[cardPath] = "partial card"
	workspaceState.mu.Unlock()

	if err := restorePulseDashboardArtifacts(ctx, snapshots); err != nil {
		t.Fatalf("restore dashboard artifacts: %v", err)
	}
	workspaceState.mu.Lock()
	defer workspaceState.mu.Unlock()
	if got := workspaceState.files[improvePath]; got != "previous improve" {
		t.Fatalf("improve after restore = %q", got)
	}
	if got := workspaceState.files[cardPath]; got != "previous card" {
		t.Fatalf("card after restore = %q", got)
	}
}

func TestRestorePulseDashboardArtifactsRemovesNewPartialFiles(t *testing.T) {
	t.Skip("Pulse no longer snapshots dashboard artifacts")
	ctx := context.Background()
	workspacePath := "Workflow/dashboard-new-rollback"
	improvePath := workspacePath + "/builder/improve.html"
	cardPath := workspacePath + "/builder/card.health.html"
	workspaceState := &mockWorkspaceAPI{files: map[string]string{}}
	workspace := httptest.NewServer(workspaceState)
	defer workspace.Close()
	t.Setenv("WORKSPACE_API_URL", workspace.URL)

	snapshots, err := capturePulseDashboardArtifacts(ctx, workspacePath)
	if err != nil {
		t.Fatalf("capture missing dashboard artifacts: %v", err)
	}
	workspaceState.mu.Lock()
	workspaceState.files[improvePath] = "partial improve"
	workspaceState.files[cardPath] = "partial card"
	workspaceState.mu.Unlock()

	if err := restorePulseDashboardArtifacts(ctx, snapshots); err != nil {
		t.Fatalf("restore dashboard artifacts: %v", err)
	}
	workspaceState.mu.Lock()
	defer workspaceState.mu.Unlock()
	if _, exists := workspaceState.files[improvePath]; exists {
		t.Fatal("new partial improve file was not removed")
	}
	if _, exists := workspaceState.files[cardPath]; exists {
		t.Fatal("new partial card file was not removed")
	}
}

func TestHandleGetPulseModuleState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"

	if _, err := recordPulseWorklist(ctx, workspacePath, "pulse-run-1", completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleGoalAdvisor: {
			Module: pulseModuleGoalAdvisor,
			Due:    true,
			Reason: "Goal trend is below target for two runs.",
		},
	})); err != nil {
		t.Fatalf("record worklist: %v", err)
	}
	if err := initializePulseFinalCommandStates(ctx, workspacePath, "pulse-run-1"); err != nil {
		t.Fatalf("initialize final commands: %v", err)
	}
	if _, err := markPulseFinalCommandState(ctx, workspacePath, pulseFinalCommandBackup, "pulse-run-1", "done", "Backup completed"); err != nil {
		t.Fatalf("mark backup: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/workflow/pulse-module-state?workspace_path=Workflow/example", nil)
	rec := httptest.NewRecorder()
	(&StreamingAPI{}).handleGetPulseModuleState(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Success        bool                     `json:"success"`
		Modules        []PulseModuleState       `json:"modules"`
		Commands       []PulseFinalCommandState `json:"commands"`
		GateMode       *PulseRunMode            `json:"gate_mode"`
		ShadowCoverage map[string]string        `json:"shadow_signal_coverage"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatal("success=false")
	}
	if len(payload.Modules) != len(pulseModuleOrder) {
		t.Fatalf("modules = %d, want %d", len(payload.Modules), len(pulseModuleOrder))
	}
	if len(payload.Commands) != len(pulseFinalCommandOrder) {
		t.Fatalf("commands = %d, want %d", len(payload.Commands), len(pulseFinalCommandOrder))
	}
	if payload.GateMode == nil || payload.GateMode.Mode != pulseRunModeDiscovery {
		t.Fatalf("gate mode = %#v, want persisted default discovery", payload.GateMode)
	}
	if payload.Commands[0].Command != pulseFinalCommandBackup || payload.Commands[0].Status != "done" {
		t.Fatalf("backup command mismatch: %+v", payload.Commands[0])
	}
	if payload.ShadowCoverage["status"] != "not_instrumented" {
		t.Fatalf("shadow coverage = %+v, want not_instrumented", payload.ShadowCoverage)
	}
}

func TestHandleGetPulseFindingsReturnsFiledLifecycle(t *testing.T) {
	ctx := context.Background()
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/example"
	if _, err := step_based_workflow.RecordRunConcerns(
		ctx, workspacePath, "pulse-run-1", "", pulseModuleWorkflowReview,
		step_based_workflow.ConcernPhaseReview,
		"CONCERNS: selector keeps targeting the same accounts",
	); err != nil {
		t.Fatalf("record finding: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/workflow/pulse-findings?workspace_path=Workflow/example&module=workflow_review",
		nil,
	)
	rec := httptest.NewRecorder()
	(&StreamingAPI{}).handleGetPulseFindings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Success  bool                                        `json:"success"`
		Findings []step_based_workflow.PulseFindingLifecycle `json:"findings"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || len(payload.Findings) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	finding := payload.Findings[0]
	if finding.Status != step_based_workflow.ConcernStatusOpen || finding.Module != pulseModuleWorkflowReview {
		t.Fatalf("finding identity/status mismatch: %+v", finding)
	}
	if finding.Issue.ID == "" || finding.Issue.Title != "selector keeps targeting the same accounts" ||
		finding.Issue.Status != "backlog" || finding.Issue.Priority != "none" {
		t.Fatalf("compact issue projection missing: %+v", finding.Issue)
	}
	if finding.FindingID != finding.Issue.ID || !strings.HasPrefix(finding.FindingID, "PUL-") {
		t.Fatalf("fixer-compatible finding id missing: finding_id=%q issue=%+v", finding.FindingID, finding.Issue)
	}
	if len(finding.Events) != 1 || finding.Events[0].EventType != "filed" {
		t.Fatalf("filed lifecycle event missing: %+v", finding.Events)
	}
}

func TestPulseFinalCommandStatesTrackAndReconcileOutcomes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	pulseRunID := "pulse-final-1"

	if err := initializePulseFinalCommandStates(ctx, workspacePath, pulseRunID); err != nil {
		t.Fatalf("initialize final commands: %v", err)
	}
	states, err := getPulseFinalCommandStates(ctx, workspacePath)
	if err != nil {
		t.Fatalf("get final commands: %v", err)
	}
	if len(states) != len(pulseFinalCommandOrder) {
		t.Fatalf("states = %d, want %d", len(states), len(pulseFinalCommandOrder))
	}
	for _, state := range states {
		if state.Status != "waiting" || state.PulseRunID != pulseRunID {
			t.Fatalf("unexpected initialized state: %+v", state)
		}
	}

	running, err := markPulseFinalCommandState(ctx, workspacePath, pulseFinalCommandBackup, pulseRunID, "running", "Starting backup")
	if err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if running.StartedAt == "" || running.FinishedAt != "" {
		t.Fatalf("running timestamps mismatch: %+v", running)
	}
	done, err := markPulseFinalCommandState(ctx, workspacePath, pulseFinalCommandBackup, pulseRunID, "done", "Backup completed")
	if err != nil {
		t.Fatalf("mark done: %v", err)
	}
	if done.FinishedAt == "" {
		t.Fatalf("done state missing finished_at: %+v", done)
	}

	if err := finalizeUnresolvedPulseFinalCommands(ctx, workspacePath, pulseRunID, "timed_out", "Finalizer timed out"); err != nil {
		t.Fatalf("reconcile unresolved: %v", err)
	}
	states, err = getPulseFinalCommandStates(ctx, workspacePath)
	if err != nil {
		t.Fatalf("get reconciled commands: %v", err)
	}
	for _, state := range states {
		if state.Command == pulseFinalCommandBackup {
			if state.Status != "done" {
				t.Fatalf("completed backup was overwritten: %+v", state)
			}
			continue
		}
		if state.Status != "timed_out" {
			t.Fatalf("unresolved command not timed out: %+v", state)
		}
	}
}

func TestSuccessfulFinalCommandClosesOnlyStageOwnershipFindings(t *testing.T) {
	t.Skip("dashboard-stage ownership ended with the retired improve.html stage")
	ctx := context.Background()
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/finalizer-reconciliation"
	pulseRunID := "pulse-finalizer-reconciliation"
	if err := initializePulseFinalCommandStates(ctx, workspacePath, pulseRunID); err != nil {
		t.Fatal(err)
	}
	_, db, err := openPulseModuleStateDB(ctx, workspacePath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`CREATE TABLE run_concerns (fingerprint TEXT PRIMARY KEY, status TEXT NOT NULL, resolved_at TEXT NOT NULL DEFAULT '', resolved_by TEXT NOT NULL DEFAULT '', resolution_note TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE pulse_finding_events (_id INTEGER PRIMARY KEY AUTOINCREMENT, fingerprint TEXT NOT NULL, finding_id TEXT NOT NULL DEFAULT '', pulse_run_id TEXT NOT NULL DEFAULT '', attempt_id TEXT NOT NULL DEFAULT '', event_type TEXT NOT NULL, summary TEXT NOT NULL DEFAULT '', metadata_json TEXT NOT NULL DEFAULT '{}', recorded_at TEXT NOT NULL, UNIQUE(fingerprint,pulse_run_id,attempt_id,event_type))`,
		`INSERT INTO run_concerns(fingerprint,status) VALUES ('dashboard-owned','external_action_required'),('real-platform-bug','external_action_required')`,
		`INSERT INTO pulse_finding_events(fingerprint,finding_id,pulse_run_id,event_type,metadata_json,recorded_at) VALUES
			('dashboard-owned','PUL-DASH','review-1','external_action_required','{"reason_code":"dashboard_stage_owned"}','2026-08-07T00:00:00Z'),
			('real-platform-bug','PUL-REAL','review-1','external_action_required','{"reason_code":"missing_platform_tool"}','2026-08-07T00:00:00Z')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if _, err := markPulseFinalCommandStateFromAgent(ctx, workspacePath, pulseFinalCommandBackup, pulseRunID, "running", "Backing up"); err != nil {
		t.Fatal(err)
	}
	if _, err := markPulseFinalCommandStateFromAgent(ctx, workspacePath, pulseFinalCommandBackup, pulseRunID, "done", "Backed up"); err != nil {
		t.Fatal(err)
	}
	for fingerprint, want := range map[string]string{"dashboard-owned": "resolved", "real-platform-bug": "external_action_required"} {
		var status string
		if err := db.QueryRowContext(ctx, `SELECT status FROM run_concerns WHERE fingerprint=?`, fingerprint).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != want {
			t.Fatalf("%s status=%q, want %q", fingerprint, status, want)
		}
	}
	var closed int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pulse_finding_events WHERE fingerprint='dashboard-owned' AND event_type='closed' AND pulse_run_id=?`, pulseRunID).Scan(&closed); err != nil {
		t.Fatal(err)
	}
	if closed != 1 {
		t.Fatalf("closed events=%d, want 1", closed)
	}
}

func TestPulseFinalCommandAgentWritesAreOrderedAndMonotonic(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	pulseRunID := "schedule-cron--final"
	if err := initializePulseFinalCommandStates(ctx, workspacePath, pulseRunID); err != nil {
		t.Fatalf("initialize final commands: %v", err)
	}
	if _, err := markPulseFinalCommandStateFromAgent(ctx, workspacePath, "dashboard", pulseRunID, "running", "Removed dashboard"); err == nil || !strings.Contains(err.Error(), "not a valid") {
		t.Fatalf("removed dashboard was accepted: %v", err)
	}
	if _, err := markPulseFinalCommandStateFromAgent(ctx, workspacePath, pulseFinalCommandBackup, pulseRunID, "done", "Skipped running"); err == nil || !strings.Contains(err.Error(), "marked running") {
		t.Fatalf("direct-done error = %v", err)
	}
	if _, err := markPulseFinalCommandStateFromAgent(ctx, workspacePath, pulseFinalCommandBackup, pulseRunID, "running", "Backing up"); err != nil {
		t.Fatalf("mark backup running: %v", err)
	}
	if _, err := markPulseFinalCommandStateFromAgent(ctx, workspacePath, pulseFinalCommandBackup, pulseRunID, "done", "Backed up"); err != nil {
		t.Fatalf("mark backup done: %v", err)
	}
}

func TestFinalizeAllUnresolvedPulseCommandsAfterRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/example"
	if err := initializePulseFinalCommandStates(ctx, workspacePath, "schedule-cron--old"); err != nil {
		t.Fatalf("initialize final commands: %v", err)
	}
	changed, err := finalizeAllUnresolvedPulseFinalCommands(ctx, workspacePath, "failed", "Server restarted")
	if err != nil {
		t.Fatalf("finalize all: %v", err)
	}
	if changed != int64(len(pulseFinalCommandOrder)) {
		t.Fatalf("changed = %d, want %d", changed, len(pulseFinalCommandOrder))
	}
	states, err := getPulseFinalCommandStates(ctx, workspacePath)
	if err != nil {
		t.Fatalf("get states: %v", err)
	}
	for _, state := range states {
		if state.Status != "failed" || state.FinishedAt == "" {
			t.Fatalf("state was not reconciled: %+v", state)
		}
	}
}

// TestCreatePulseWorklistToolsEveryToolHasACategory reproduces a real production
// failure: resolve_run_concern was added to the returned tool list without a
// matching entry in the categories map, so any orchestrator whose tool set
// included it failed at agent-creation time with "tool resolve_run_concern not
// found in ToolCategories map - category is REQUIRED" — before the orchestrator
// could even start (social-media's "[Route] Execution" todo_task, 2026-07-29).
// The categories map is hand-maintained separately from the tool slice, so
// nothing else catches a new tool silently missing its entry.
func TestCreatePulseWorklistToolsEveryToolHasACategory(t *testing.T) {
	tools, _, categories := createPulseWorklistTools()
	for _, tool := range tools {
		if tool.Function == nil {
			t.Fatalf("tool with nil Function: %+v", tool)
		}
		name := tool.Function.Name
		category, ok := categories[name]
		if !ok {
			t.Fatalf("tool %q has no entry in the categories map returned by createPulseWorklistTools", name)
		}
		if category == "" {
			t.Fatalf("tool %q has an empty category", name)
		}
	}
}

func TestResolveRunConcernToolCannotCloseWithoutVerification(t *testing.T) {
	_, executors, _ := createPulseWorklistTools()
	execute := executors["resolve_run_concern"].(func(context.Context, map[string]interface{}) (string, error))
	_, err := execute(context.Background(), map[string]interface{}{
		"workspace_path": "Workflow/example",
		"fingerprint":    "finding-fingerprint",
		"status":         "resolved",
		"note":           "claimed fixed without proof",
	})
	if err == nil || !strings.Contains(err.Error(), "verified finding_dispositions") {
		t.Fatalf("unverified close error = %v", err)
	}
}

func TestGetPulseModuleStateExposesLoopClosureButNotShadowHistory(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	_, executors, _ := createPulseWorklistTools()
	execute := executors["get_pulse_state"].(func(context.Context, map[string]interface{}) (string, error))

	raw, err := execute(context.Background(), map[string]interface{}{"workspace_path": "Workflow/example", "view": "module"})
	if err != nil {
		t.Fatalf("get_pulse_state(view=module): %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode Gate payload: %v", err)
	}
	loopClosure, exists := payload["loop_closure"].(map[string]interface{})
	if !exists {
		t.Fatalf("Gate payload is missing loop_closure evidence: %s", raw)
	}
	if got := loopClosure["coverage_status"]; got != loopclosure.CoverageNotInstrumented {
		t.Fatalf("loop_closure coverage = %#v, want %q", got, loopclosure.CoverageNotInstrumented)
	}
	note, _ := payload["loop_closure_note"].(string)
	for _, required := range []string{"do not mandate a module", "authorize mutation", "coverage_status"} {
		if !strings.Contains(note, required) {
			t.Fatalf("loop_closure_note missing %q: %q", required, note)
		}
	}
	for _, forbidden := range []string{"stalled_loops", "shadow_signals", "shadow_signal_observations"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("Gate payload exposes internal shadow field %q: %s", forbidden, raw)
		}
	}
}

func TestRecordPulseWorklistPersistsShadowObservationAfterDecision(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/example"
	pulseRunID := "schedule-cron--shadow"
	sessionID := "schedule-cron--shadow-session"
	ctx := mcpexecutor.WithSessionID(context.Background(), sessionID)

	decisions := completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {
			Module:   pulseModuleWorkflowReview,
			Due:      true,
			Reason:   "Gate independently found a failed step.",
			Evidence: []string{"run_metadata.json"},
		},
	})
	_, executors, _ := createPulseWorklistTools()
	execute := executors["record_pulse_worklist"].(func(context.Context, map[string]interface{}) (string, error))
	args := map[string]interface{}{
		"workspace_path": workspacePath,
		"pulse_run_id":   pulseRunID,
		"mode":           pulseRunModeDiscovery,
		"mode_reason":    "The Gate is recording a normal review pass for this test.",
		"decisions":      pulseWorklistDecisionToolArgs(decisions),
	}
	if _, err := execute(ctx, args); err != nil {
		t.Fatalf("record_pulse_worklist: %v", err)
	}
	// An idempotent retry happens after the current Gate state exists. It must
	// not overwrite the pre-decision shadow snapshot with a later observation.
	if _, err := execute(ctx, args); err != nil {
		t.Fatalf("idempotent record_pulse_worklist retry: %v", err)
	}

	observations, err := getPulseShadowSignalObservations(context.Background(), workspacePath, 10)
	if err != nil {
		t.Fatalf("get shadow observations: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("observations = %d, want 1: %+v", len(observations), observations)
	}
	got := observations[0]
	if got.PulseRunID != pulseRunID || got.DetectorVersion != loopclosure.DetectorVersion {
		t.Fatalf("observation identity mismatch: %+v", got)
	}
	if got.CoverageStatus != loopclosure.CoverageNotInstrumented {
		t.Fatalf("coverage = %q, want %q", got.CoverageStatus, loopclosure.CoverageNotInstrumented)
	}
	if len(got.GateDecisions) != len(pulseModuleOrder) {
		t.Fatalf("Gate decisions = %d, want %d", len(got.GateDecisions), len(pulseModuleOrder))
	}
	if len(got.Signals) != 0 {
		t.Fatalf("new workflow should have no shadow findings: %+v", got.Signals)
	}
}

func pulseWorklistDecisionToolArgs(decisions []PulseWorklistDecision) []interface{} {
	out := make([]interface{}, 0, len(decisions))
	for _, decision := range decisions {
		item := map[string]interface{}{
			"module":        decision.Module,
			"due":           decision.Due,
			"reason":        decision.Reason,
			"cooldown_runs": decision.CooldownRuns,
		}
		if len(decision.Evidence) > 0 {
			evidence := make([]interface{}, 0, len(decision.Evidence))
			for _, value := range decision.Evidence {
				evidence = append(evidence, value)
			}
			item["evidence"] = evidence
		}
		if decision.NextCheckAt != "" {
			item["next_check_at"] = decision.NextCheckAt
		}
		if decision.NextCheckAfterRunID != "" {
			item["next_check_after_run_id"] = decision.NextCheckAfterRunID
		}
		out = append(out, item)
	}
	return out
}

func completePulseWorklistDecisions(overrides map[string]PulseWorklistDecision) []PulseWorklistDecision {
	out := make([]PulseWorklistDecision, 0, len(pulseModuleOrder))
	for _, module := range pulseModuleOrder {
		decision := PulseWorklistDecision{
			Module:       module,
			Due:          false,
			Reason:       "No evidence requires this module this run.",
			CooldownRuns: 1,
		}
		if overrides != nil {
			if override, ok := overrides[module]; ok {
				if override.Module == "" {
					override.Module = module
				}
				decision = override
			}
		}
		out = append(out, decision)
	}
	return out
}

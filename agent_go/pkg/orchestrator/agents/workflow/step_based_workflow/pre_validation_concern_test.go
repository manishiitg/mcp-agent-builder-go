package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workspace"
	workspacehandlers "github.com/manishiitg/coding-agent-loop/workspace/handlers"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
	"github.com/spf13/viper"
)

// newPreValidationConcernTestOrchestrator wires a REAL workspace-docs HTTP
// server (for SavePreValidationLog's file write) AND WORKSPACE_DOCS_PATH (for
// RecordRunConcerns' direct sqlite write) at the SAME root, so a test can
// verify both real artifacts SavePreValidationLog produces.
func newPreValidationConcernTestOrchestrator(t *testing.T) (*StepBasedWorkflowOrchestrator, string) {
	t.Helper()

	base, err := orchestrator.NewBaseOrchestrator(
		loggerv2.NewDefault(), nil, orchestrator.OrchestratorTypeWorkflow,
		"", 0, "", nil, nil, false, &orchestrator.LLMConfig{}, 1, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewBaseOrchestrator: %v", err)
	}
	workflowRelPath := "Workflow/pre-validation-concern-test"
	hcpo := &StepBasedWorkflowOrchestrator{BaseOrchestrator: base}
	hcpo.selectedRunFolder = "iteration-0/default"
	hcpo.currentGroupName = "default"

	docsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(docsDir, workflowRelPath, "db"), 0o755); err != nil {
		t.Fatalf("mkdir workflow dir: %v", err)
	}
	t.Setenv("WORKSPACE_DOCS_PATH", docsDir)
	gin.SetMode(gin.TestMode)
	viper.Set("docs-dir", docsDir)
	router := gin.New()
	router.Any("/api/documents/*filepath", workspacehandlers.HandleDocumentRequest)
	wsServer := httptest.NewServer(router)
	t.Cleanup(wsServer.Close)

	hcpo.WorkspaceClient = workspace.NewClient(wsServer.URL)
	hcpo.SetWorkspacePath(workflowRelPath)

	return hcpo, filepath.Join(docsDir, workflowRelPath)
}

func targetingAuditFailure(field, message string) *WorkspaceVerificationResult {
	return &WorkspaceVerificationResult{
		OverallPass: false,
		Summary: ValidationSummary{
			TotalChecks: 1, FailedChecks: 1,
			Errors: []ValidationError{{File: "targeting_audit.json", Path: field, Message: message}},
		},
	}
}

// TestSavePreValidationLogFilesFailureAsRunConcern proves the actual fix: a
// failing gate is now durably recorded in db/db.sqlite via RecordRunConcerns,
// not just (over-writably) in the per-run pre_validation.json. This is the
// exact scenario found live in the twitter automation workflow's
// execute-targeting-audit step: it failed prevalidation twice before passing,
// and once it passed, pre_validation.json on disk showed only the clean
// final attempt with zero trace of the two real failures.
func TestSavePreValidationLogFilesFailureAsRunConcern(t *testing.T) {
	hcpo, _ := newPreValidationConcernTestOrchestrator(t)
	ctx := context.Background()
	stepID := "execute-targeting-audit"
	logPath := hcpo.GetWorkspacePath() + "/runs/" + hcpo.selectedRunFolder

	SavePreValidationLog(ctx, hcpo.BaseOrchestrator, logPath, stepID, stepID,
		targetingAuditFailure("$.our_follower_count", "unknown key our_follower_count"), nil,
		hcpo.GetWorkspacePath(), hcpo.selectedRunFolder, hcpo.currentGroupName)

	concerns, err := LoadOpenRunConcerns(ctx, hcpo.GetWorkspacePath(), 25)
	if err != nil {
		t.Fatalf("LoadOpenRunConcerns: %v", err)
	}
	if len(concerns) != 1 {
		t.Fatalf("got %d open concerns, want 1: %+v", len(concerns), concerns)
	}
	c := concerns[0]
	if c.StepID != stepID {
		t.Errorf("concern StepID = %q, want %q", c.StepID, stepID)
	}
	if c.Phase != ConcernPhasePreValidation {
		t.Errorf("concern Phase = %q, want %q", c.Phase, ConcernPhasePreValidation)
	}
	if c.SeenCount != 1 {
		t.Errorf("SeenCount = %d, want 1", c.SeenCount)
	}
	if c.Status != ConcernStatusOpen {
		t.Errorf("Status = %q, want %q", c.Status, ConcernStatusOpen)
	}
}

// TestSavePreValidationLogConcernRecursAcrossRuns proves the concern survives
// past the run that raised it and its own log-file overwrite -- the same
// field failing on a LATER run (a fresh pre_validation.json, a fresh
// pre-run-folder) still accumulates onto the SAME db row instead of starting
// over, which is exactly the cross-run chronic-defect signal a per-run JSON
// file structurally cannot provide.
func TestSavePreValidationLogConcernRecursAcrossRuns(t *testing.T) {
	hcpo, _ := newPreValidationConcernTestOrchestrator(t)
	ctx := context.Background()
	stepID := "execute-targeting-audit"

	for _, run := range []string{"iteration-0/default", "iteration-0/default-2"} {
		hcpo.selectedRunFolder = run
		logPath := hcpo.GetWorkspacePath() + "/runs/" + run
		SavePreValidationLog(ctx, hcpo.BaseOrchestrator, logPath, stepID, stepID,
			targetingAuditFailure("$.validated_targeting.follower_max", "Expected number, got <nil>"), nil,
			hcpo.GetWorkspacePath(), hcpo.selectedRunFolder, hcpo.currentGroupName)
	}

	concerns, err := LoadOpenRunConcerns(ctx, hcpo.GetWorkspacePath(), 25)
	if err != nil {
		t.Fatalf("LoadOpenRunConcerns: %v", err)
	}
	if len(concerns) != 1 {
		t.Fatalf("got %d open concerns, want 1 (same field, same fingerprint, across runs): %+v", len(concerns), concerns)
	}
	if concerns[0].SeenCount != 2 {
		t.Fatalf("SeenCount = %d, want 2 (recurred across both runs)", concerns[0].SeenCount)
	}
	if concerns[0].LastSeenRun != "iteration-0/default-2" {
		t.Errorf("LastSeenRun = %q, want the second run", concerns[0].LastSeenRun)
	}
}

func TestSavePreValidationLogCollapsesAllFailedChecksForOneStep(t *testing.T) {
	hcpo, _ := newPreValidationConcernTestOrchestrator(t)
	ctx := context.Background()
	stepID := "execute-find-opportunities"
	logPath := hcpo.GetWorkspacePath() + "/runs/" + hcpo.selectedRunFolder

	failure := &WorkspaceVerificationResult{
		OverallPass: false,
		Summary: ValidationSummary{
			TotalChecks:  3,
			FailedChecks: 3,
			Errors: []ValidationError{
				{File: "opportunities.json", Path: "$.action_targets", Message: "must exist"},
				{File: "opportunities.json", Path: "$.coverage_report", Message: "must be an object"},
				{File: "opportunities.json", Path: "$.targets", Message: "must be an array"},
			},
		},
	}
	SavePreValidationLog(ctx, hcpo.BaseOrchestrator, logPath, stepID, stepID,
		failure, nil, hcpo.GetWorkspacePath(), hcpo.selectedRunFolder, hcpo.currentGroupName)

	concerns, err := LoadOpenRunConcerns(ctx, hcpo.GetWorkspacePath(), 25)
	if err != nil {
		t.Fatalf("LoadOpenRunConcerns: %v", err)
	}
	if len(concerns) != 1 {
		t.Fatalf("got %d concerns, want one step-level prevalidation bug: %+v", len(concerns), concerns)
	}
	for _, path := range []string{"$.action_targets", "$.coverage_report", "$.targets"} {
		if !strings.Contains(concerns[0].Text, path) {
			t.Fatalf("one concern must retain field evidence %q: %s", path, concerns[0].Text)
		}
	}
	if concerns[0].SeenCount != 1 {
		t.Fatalf("SeenCount=%d, want one observation for one run", concerns[0].SeenCount)
	}

	// A repair retry in the same run updates the one bug's latest evidence but
	// does not manufacture another recurrence.
	SavePreValidationLog(ctx, hcpo.BaseOrchestrator, logPath, stepID, stepID,
		targetingAuditFailure("$.new_field", "still missing"), nil,
		hcpo.GetWorkspacePath(), hcpo.selectedRunFolder, hcpo.currentGroupName)
	concerns, err = LoadOpenRunConcerns(ctx, hcpo.GetWorkspacePath(), 25)
	if err != nil {
		t.Fatalf("reload concerns: %v", err)
	}
	if len(concerns) != 1 || concerns[0].SeenCount != 1 || !strings.Contains(concerns[0].Text, "$.new_field") {
		t.Fatalf("same-run retry should update one concern without recurrence inflation: %+v", concerns)
	}
}

// TestSavePreValidationLogPassNoConcern proves the common case (a step passes
// on its first attempt) files nothing -- no concern noise for healthy steps.
func TestSavePreValidationLogPassNoConcern(t *testing.T) {
	hcpo, _ := newPreValidationConcernTestOrchestrator(t)
	ctx := context.Background()
	stepID := "clean-step"
	logPath := hcpo.GetWorkspacePath() + "/runs/" + hcpo.selectedRunFolder

	SavePreValidationLog(ctx, hcpo.BaseOrchestrator, logPath, stepID, stepID,
		&WorkspaceVerificationResult{OverallPass: true, Summary: ValidationSummary{TotalChecks: 1, PassedChecks: 1}}, nil,
		hcpo.GetWorkspacePath(), hcpo.selectedRunFolder, hcpo.currentGroupName)

	concerns, err := LoadOpenRunConcerns(ctx, hcpo.GetWorkspacePath(), 25)
	if err != nil {
		t.Fatalf("LoadOpenRunConcerns: %v", err)
	}
	if len(concerns) != 0 {
		t.Fatalf("expected no concerns for a passing result, got %+v", concerns)
	}
}

func TestSavePreValidationLogRetainsEveryScriptedAndMessageSequenceAttempt(t *testing.T) {
	hcpo, _ := newPreValidationConcernTestOrchestrator(t)
	ctx := context.Background()
	logPath := hcpo.GetWorkspacePath() + "/runs/" + hcpo.selectedRunFolder

	scriptedStep := "read-credentials"
	SavePreValidationLog(ctx, hcpo.BaseOrchestrator, logPath, scriptedStep, scriptedStep,
		targetingAuditFailure("$.credentials", "missing credentials"), nil,
		hcpo.GetWorkspacePath(), hcpo.selectedRunFolder, hcpo.currentGroupName,
		PreValidationAttempt{ExecutionMode: "scripted", ValidationPhase: "repair-check", ExecutionAttempt: 1, ValidationAttempt: 1})
	SavePreValidationLog(ctx, hcpo.BaseOrchestrator, logPath, scriptedStep, scriptedStep,
		&WorkspaceVerificationResult{OverallPass: true, Summary: ValidationSummary{TotalChecks: 1, PassedChecks: 1}}, nil,
		hcpo.GetWorkspacePath(), hcpo.selectedRunFolder, hcpo.currentGroupName,
		PreValidationAttempt{ExecutionMode: "scripted", ValidationPhase: "repair-check", ExecutionAttempt: 1, ValidationAttempt: 2})

	messageStep := "collect-input"
	SavePreValidationLog(ctx, hcpo.BaseOrchestrator, logPath, messageStep, messageStep,
		targetingAuditFailure("$.input", "missing input"), nil,
		hcpo.GetWorkspacePath(), hcpo.selectedRunFolder, hcpo.currentGroupName,
		PreValidationAttempt{ExecutionMode: "message_sequence", ValidationPhase: "message-sequence-verify", ExecutionAttempt: 1, ValidationAttempt: 1})
	SavePreValidationLog(ctx, hcpo.BaseOrchestrator, logPath, messageStep, messageStep,
		&WorkspaceVerificationResult{OverallPass: true, Summary: ValidationSummary{TotalChecks: 1, PassedChecks: 1}}, nil,
		hcpo.GetWorkspacePath(), hcpo.selectedRunFolder, hcpo.currentGroupName,
		PreValidationAttempt{ExecutionMode: "message_sequence", ValidationPhase: "message-sequence-verify", ExecutionAttempt: 1, ValidationAttempt: 2})

	read := func(path string) PreValidationLogEntry {
		t.Helper()
		raw, err := hcpo.ReadWorkspaceFile(ctx, path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var entry PreValidationLogEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		return entry
	}

	for _, tc := range []struct {
		step, phase, mode string
	}{
		{scriptedStep, "repair-check", "scripted"},
		{messageStep, "message-sequence-verify", "message_sequence"},
	} {
		base := fmt.Sprintf("%s/runs/%s/logs/%s", hcpo.GetWorkspacePath(), hcpo.selectedRunFolder, tc.step)
		failed := read(fmt.Sprintf("%s/pre_validation_%s_execution_001_attempt_001.json", base, tc.phase))
		passed := read(fmt.Sprintf("%s/pre_validation_%s_execution_001_attempt_002.json", base, tc.phase))
		latest := read(base + "/pre_validation.json")
		if failed.OverallPass || failed.ValidationAttempt != 1 || failed.ExecutionMode != tc.mode || len(failed.Errors) == 0 {
			t.Fatalf("failed %s attempt was not retained accurately: %+v", tc.mode, failed)
		}
		if !passed.OverallPass || passed.ValidationAttempt != 2 || !latest.OverallPass || latest.ValidationAttempt != 2 {
			t.Fatalf("latest %s result should be the passing second attempt: passed=%+v latest=%+v", tc.mode, passed, latest)
		}
	}
}

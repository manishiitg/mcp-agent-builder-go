package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	step_based_workflow "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	mcpexecutor "github.com/manishiitg/mcpagent/executor"
)

func TestTypedPulseReviewerToolsPersistFindingAndCompactReceipt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/typed-review-tools"
	const sessionID = "typed-reviewer-session"
	pulseRunID := sessionID
	reviewRunID := sessionID
	ctx := mcpexecutor.WithSessionID(context.Background(), sessionID)

	_, executors, _ := createPulseWorklistTools()
	recordFinding := executors["record_pulse_finding"].(func(context.Context, map[string]interface{}) (string, error))
	completeReview := executors["complete_pulse_review"].(func(context.Context, map[string]interface{}) (string, error))
	raw, err := recordFinding(ctx, map[string]interface{}{
		"workspace_path": workspacePath, "pulse_run_id": pulseRunID,
		"module": pulseModuleWorkflowReview, "concern": "collector silently drops failed rows",
		"issue_kind": "workflow_issue", "classification": "correctness_bug", "severity": "high",
		"summary": "Failed rows disappear", "impact": "The workflow can report success on incomplete data.",
		"evidence": []interface{}{"runs/iteration-0/result.json"},
	})
	if err != nil {
		t.Fatalf("record_pulse_finding: %v", err)
	}
	var recorded step_based_workflow.PulseReviewFindingRecord
	if err := json.Unmarshal([]byte(raw), &recorded); err != nil || recorded.IssueID == "" || !strings.HasPrefix(recorded.IssueID, "PUL-") {
		t.Fatalf("recorded finding=%#v decode_err=%v raw=%s", recorded, err, raw)
	}
	if strings.Contains(raw, "fingerprint") || strings.Contains(raw, "finding_id") {
		t.Fatalf("public record_pulse_finding response leaked an internal lifecycle identity: %s", raw)
	}
	// A completion retry may replay the tool call. It must not manufacture a
	// second recurrence in the same review identity.
	if _, err := recordFinding(ctx, map[string]interface{}{
		"workspace_path": workspacePath, "pulse_run_id": pulseRunID,
		"module": pulseModuleWorkflowReview, "concern": "collector silently drops failed rows",
		"issue_kind": "workflow_issue", "classification": "correctness_bug", "severity": "high",
		"summary": "Failed rows disappear", "impact": "The workflow can report success on incomplete data.",
		"evidence": []interface{}{"runs/iteration-0/result.json"},
	}); err != nil {
		t.Fatalf("idempotent record_pulse_finding retry: %v", err)
	}

	if _, err := completeReview(ctx, map[string]interface{}{
		"workspace_path": workspacePath, "pulse_run_id": pulseRunID,
		"modules": []interface{}{pulseModuleWorkflowReview}, "verdict": "   ", "status": "completed",
	}); err == nil || !strings.Contains(err.Error(), "non-empty verdict") {
		t.Fatalf("blank verdict error=%v", err)
	}

	if _, err := completeReview(ctx, map[string]interface{}{
		"workspace_path": workspacePath, "pulse_run_id": pulseRunID,
		"modules": []interface{}{pulseModuleWorkflowReview}, "verdict": "One correctness issue.", "status": "completed",
	}); err != nil {
		t.Fatalf("complete_pulse_review: %v", err)
	}

	receipt, err := step_based_workflow.LoadPulseReviewReceiptForRun(context.Background(), workspacePath, reviewRunID, pulseModuleWorkflowReview)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.FindingCount != 1 || receipt.Status != "completed" || receipt.Verdict != "One correctness issue." {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	findings, err := step_based_workflow.LoadPulseFindingLifecycles(context.Background(), workspacePath, pulseModuleWorkflowReview, -1)
	if err != nil || len(findings) != 1 || findings[0].SeenCount != 1 || findings[0].Details == nil || findings[0].Details.Summary != "Failed rows disappear" {
		t.Fatalf("unexpected lifecycle: %#v err=%v", findings, err)
	}
}

func TestTypedPulseReviewerToolsRequireCurrentConversation(t *testing.T) {
	_, executors, _ := createPulseWorklistTools()
	recordFinding := executors["record_pulse_finding"].(func(context.Context, map[string]interface{}) (string, error))
	if _, err := recordFinding(mcpexecutor.WithSessionID(context.Background(), "untrusted-reviewer"), map[string]interface{}{
		"workspace_path": "Workflow/demo", "pulse_run_id": "another-conversation", "module": pulseModuleWorkflowReview,
	}); err == nil {
		t.Fatal("untrusted reviewer write unexpectedly succeeded")
	}
}

package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	mcpexecutor "github.com/manishiitg/mcpagent/executor"
)

func TestPulseFailedResultCorrectionKeepsStateAuditAndHistoryConsistent(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	ctx := context.Background()
	workspace, run := "Workflow/correction", "pulse-parent"
	if _, err := recordPulseWorklist(ctx, workspace, run, completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModulePlanDriftReview: {Module: pulseModulePlanDriftReview, Due: true, Reason: "Drift due."},
	})); err != nil {
		t.Fatal(err)
	}
	_, executors, _ := createPulseWorklistTools()
	execute := executors["record_pulse_result"].(func(context.Context, map[string]interface{}) (string, error))
	args := map[string]interface{}{"workspace_path": workspace, "pulse_run_id": run, "module": pulseModulePlanDriftReview,
		"result": "failed", "reason": "Parent checked before child finished."}
	if _, err := execute(ctx, args); err != nil {
		t.Fatal(err)
	}
	args["result"], args["reason"] = "done", "Child completed; verified its receipt and checkpoint."
	if _, err := execute(ctx, args); err == nil {
		t.Fatal("correction without proof accepted")
	}
	args["evidence"] = []string{"runs/pulse/pulse-parent/plan-drift-review.md"}
	args["verification"] = []string{"Verified child terminal status and persisted review."}
	args["pulse_run_id"] = "other-run"
	if _, err := execute(ctx, args); err == nil {
		t.Fatal("another run replaced the receipt")
	}
	args["pulse_run_id"] = run
	// A failure late in persistence must roll back state and history together.
	_, failureDB, err := openPulseModuleStateDB(ctx, workspace, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failureDB.Exec(`CREATE TRIGGER reject_receipt BEFORE UPDATE ON pulse_module_audit BEGIN SELECT RAISE(ABORT, 'test receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(ctx, args); err == nil {
		t.Fatal("injected receipt failure did not propagate")
	}
	var unchanged string
	var historyCount int
	if err := failureDB.QueryRow(`SELECT last_result FROM pulse_module_state WHERE module=?`, pulseModulePlanDriftReview).Scan(&unchanged); err != nil {
		t.Fatal(err)
	}
	if err := failureDB.QueryRow(`SELECT COUNT(*) FROM pulse_module_result_history`).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if unchanged != "failed" || historyCount != 0 {
		t.Fatalf("partial correction leaked: %s history=%d", unchanged, historyCount)
	}
	if _, err := failureDB.Exec(`DROP TRIGGER reject_receipt`); err != nil {
		t.Fatal(err)
	}
	failureDB.Close()
	response, err := execute(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Module PulseModuleState `json:"module"`
	}
	if err := json.Unmarshal([]byte(response), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Module.LastResult != "done" || payload.Module.LastResultReason != args["reason"] {
		t.Fatalf("stale response: %s", response)
	}
	_, db, err := openPulseModuleStateDB(ctx, workspace, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var state, audit, stateReason, auditReason string
	if err := db.QueryRow(`SELECT s.last_result,a.result,s.last_result_reason,a.reason FROM pulse_module_state s
		JOIN pulse_module_audit a ON a.workspace_path=s.workspace_path AND a.module=s.module AND a.pulse_run_id=s.last_pulse_run_id
		WHERE s.module=?`, pulseModulePlanDriftReview).Scan(&state, &audit, &stateReason, &auditReason); err != nil {
		t.Fatal(err)
	}
	if state != "done" || audit != state || stateReason != auditReason {
		t.Fatalf("inconsistent records: %s %s %s %s", state, audit, stateReason, auditReason)
	}
	var previousState, previousAudit string
	if err := db.QueryRow(`SELECT json_extract(previous_state_json,'$.last_result'),json_extract(previous_audit_json,'$.result') FROM pulse_module_result_history`).Scan(&previousState, &previousAudit); err != nil {
		t.Fatal(err)
	}
	if previousState != "failed" || previousAudit != "failed" {
		t.Fatal("original failure history lost")
	}
	if _, err := execute(ctx, args); err != nil {
		t.Fatalf("same-result retry rejected: %v", err)
	}
	args["result"] = "failed"
	if _, err := execute(ctx, args); err == nil {
		t.Fatal("stale failure downgraded verified success")
	}
}

func TestPulseParentCannotFailReviewWhileChildRuns(t *testing.T) {
	for _, tc := range []struct {
		name, childStatus, childParent, caller string
		blocked                                bool
	}{
		{"active child", trackedExecutionStatusRunning, "root", "parent", true},
		{"active grandchild", trackedExecutionStatusCompleted, "root", "parent", true},
		{"notification while peer runs", trackedExecutionStatusRunning, "root", "parent", true},
		{"notification after peer failed", trackedExecutionStatusFailed, "root", "parent", false},
		{"completed child", trackedExecutionStatusCompleted, "root", "parent", false},
		{"failed child", trackedExecutionStatusFailed, "root", "parent", false},
		{"unrelated child", trackedExecutionStatusRunning, "unrelated-root", "parent", false},
		{"child writes its own failure", trackedExecutionStatusRunning, "root", "child-session", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := lifecycleTestAPI()
			api.trackedWorkflowExecutions["root"] = &TrackedWorkflowExecution{ExecutionID: "root", SessionID: "parent", Source: trackedExecutionSourceConversationTurn, Status: trackedExecutionStatusRunning, StartedAt: time.Now()}
			api.trackedWorkflowExecutions["child"] = &TrackedWorkflowExecution{ExecutionID: "child", SessionID: "parent", Status: tc.childStatus, Metadata: map[string]string{"parent_execution_id": tc.childParent}}
			if strings.HasPrefix(tc.name, "notification") {
				api.trackSyntheticConversationTurnStart("notification", "parent", "root", "Handle completion")
			}
			if tc.name == "active grandchild" {
				api.trackedWorkflowExecutions["grandchild"] = &TrackedWorkflowExecution{ExecutionID: "grandchild", SessionID: "parent", Status: trackedExecutionStatusRunning, Metadata: map[string]string{"parent_execution_id": "child"}}
			}
			if tc.caller == "child-session" {
				api.trackedWorkflowExecutions["child-root"] = &TrackedWorkflowExecution{ExecutionID: "child-root", SessionID: tc.caller, Source: trackedExecutionSourceConversationTurn, Status: trackedExecutionStatusRunning, StartedAt: time.Now()}
			}
			calls := 0
			executors := map[string]interface{}{"record_pulse_result": func(context.Context, map[string]interface{}) (string, error) { calls++; return "recorded", nil }}
			api.guardPulseResultExecutor(executors, "parent")
			execute := executors["record_pulse_result"].(func(context.Context, map[string]interface{}) (string, error))
			_, err := execute(mcpexecutor.WithSessionID(context.Background(), tc.caller), map[string]interface{}{"module": "plan_drift_review", "result": "failed"})
			if tc.blocked {
				if err == nil || !strings.Contains(err.Error(), "active child") || calls != 0 {
					t.Fatalf("premature failure accepted: %v, calls=%d", err, calls)
				}
			} else if err != nil || calls != 1 {
				t.Fatalf("genuine result rejected: %v, calls=%d", err, calls)
			}
		})
	}
}

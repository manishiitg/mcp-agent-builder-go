package workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
)

func TestExecuteShellCommandRejectsWorkflowStepWithoutWorkingDir(t *testing.T) {
	const sessionID = "workflow-step-missing-cwd"
	defer ClearSessionShellConfig(sessionID)
	common.SetSessionFolderGuard(
		sessionID,
		[]string{"Workflow/testing/runs/iteration-0/default/execution"},
		[]string{"Workflow/testing/runs/iteration-0/default/execution/step-one"},
	)
	common.SetSessionShellEnv(sessionID, map[string]string{
		"STEP_OUTPUT_DIR": "/workspace-docs/Workflow/testing/runs/iteration-0/default/execution/step-one",
	})

	ctx := context.WithValue(context.Background(), common.ChatSessionIDKey, sessionID)
	_, err := NewClient("http://127.0.0.1:1").ExecuteShellCommand(ctx, ExecuteShellCommandParams{Command: "pwd"})
	if err == nil {
		t.Fatal("workflow step without cwd unexpectedly fell back to workspace root")
	}
	if !strings.Contains(err.Error(), "no working directory") || !strings.Contains(err.Error(), sessionID) {
		t.Fatalf("unexpected missing-cwd error: %v", err)
	}
}

func TestExecuteShellCommandAllowsExplicitWorkflowStepWorkingDir(t *testing.T) {
	const sessionID = "workflow-step-explicit-cwd"
	defer ClearSessionShellConfig(sessionID)
	common.SetSessionFolderGuard(
		sessionID,
		[]string{"Workflow/testing/runs/iteration-0/default/execution"},
		[]string{"Workflow/testing/runs/iteration-0/default/execution/step-one"},
	)
	common.SetSessionShellEnv(sessionID, map[string]string{
		"STEP_OUTPUT_DIR": "/workspace-docs/Workflow/testing/runs/iteration-0/default/execution/step-one",
	})

	// Reaching the HTTP request (rather than the missing-cwd guard) is enough for
	// this assertion; the deliberately closed port then supplies the final error.
	ctx := context.WithValue(context.Background(), common.ChatSessionIDKey, sessionID)
	_, err := NewClient("http://127.0.0.1:1").ExecuteShellCommand(ctx, ExecuteShellCommandParams{
		Command:          "pwd",
		WorkingDirectory: "Workflow/testing/runs/iteration-0/default/execution",
	})
	if err == nil || strings.Contains(err.Error(), "no working directory") {
		t.Fatalf("explicit workflow cwd was rejected before HTTP: %v", err)
	}
}

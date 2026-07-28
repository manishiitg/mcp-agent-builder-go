package step_based_workflow

import (
	"context"
	"testing"

	virtualtools "github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"
)

func TestMessageSequenceExecutionIDReusesStandaloneStepExecution(t *testing.T) {
	ctx := virtualtools.WithBackgroundAgentID(context.Background(), "exec-math-solver-123")
	if got := messageSequenceExecutionID(ctx, "math-solver"); got != "exec-math-solver-123" {
		t.Fatalf("messageSequenceExecutionID() = %q, want standalone step execution ID", got)
	}
}

func TestMessageSequenceExecutionIDDoesNotReuseWorkflowExecution(t *testing.T) {
	ctx := virtualtools.WithBackgroundAgentID(context.Background(), "workflow-full-123")
	if got := messageSequenceExecutionID(ctx, "math-solver"); got != "" {
		t.Fatalf("messageSequenceExecutionID() = %q, want empty for workflow-level execution", got)
	}
}

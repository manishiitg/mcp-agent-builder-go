package step_based_workflow

import (
	"context"
	"testing"

	virtualtools "github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"
	orchestrator_events "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
)

func TestWithWorkshopExecutionParentPreservesDetachedLifecycleIdentity(t *testing.T) {
	launch := virtualtools.WithBackgroundAgentID(context.Background(), "query-root")
	detached := withWorkshopExecutionParent(context.Background(), launch)

	if got := currentWorkshopParentExecutionID(detached); got != "query-root" {
		t.Fatalf("parent execution id = %q, want query-root", got)
	}
}

func TestCurrentWorkshopParentExecutionIDUsesSemanticParentNotCorrelation(t *testing.T) {
	parentCtx := context.WithValue(context.Background(), orchestrator_events.ParentExecutionIDKey, "parent-node")
	if got := currentWorkshopParentExecutionID(parentCtx); got != "parent-node" {
		t.Fatalf("ParentExecutionIDKey = %q, want parent-node", got)
	}

	correlationCtx := context.WithValue(context.Background(), orchestrator_events.ForceCorrelationIDKey, "workshop-step-display-group")
	if got := currentWorkshopParentExecutionID(correlationCtx); got != "" {
		t.Fatalf("correlation id must not become semantic parent, got %q", got)
	}
}

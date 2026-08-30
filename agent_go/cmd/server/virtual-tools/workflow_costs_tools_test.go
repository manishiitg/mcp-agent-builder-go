package virtualtools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
)

func TestWorkflowCostsToolDefinitionExplainsScopeAndPhaseColumn(t *testing.T) {
	raw, err := json.Marshal(workflowCostsQueryToolDefinition())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"costs/costs.sqlite", "cost_events", "phase", "PLAT-184"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("tool definition missing %q: %s", want, raw)
		}
	}
}

func TestWorkflowCostsToolRegistryExposesQuery(t *testing.T) {
	registry := CreateWorkflowCostsToolRegistry("http://127.0.0.1:1", "", "session")
	if len(registry.Tools) != 1 {
		t.Fatalf("tools=%d, want 1", len(registry.Tools))
	}
	if registry.Executors["query_workflow_costs"] == nil {
		t.Fatal("missing executor query_workflow_costs")
	}
	if registry.Categories["query_workflow_costs"] != WorkflowCostsToolCategory {
		t.Fatalf("category=%q", registry.Categories["query_workflow_costs"])
	}
}

func TestResolveCurrentWorkflowCostsPathScopesToOwnWorkflowOnly(t *testing.T) {
	sessionID := "workflow-costs-path"
	defer common.ClearSessionShellConfig(sessionID)
	common.SetSessionFolderGuard(sessionID, []string{"/tmp/untrusted.sqlite", "Workflow/demo/builder"}, nil)

	path, err := resolveCurrentWorkflowCostsPath(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if path != "Workflow/demo/costs/costs.sqlite" {
		t.Fatalf("resolved path=%q", path)
	}
}

func TestResolveCurrentWorkflowCostsPathFailsClosedOutsideAnyWorkflowFolder(t *testing.T) {
	sessionID := "workflow-costs-path-none"
	defer common.ClearSessionShellConfig(sessionID)
	common.SetSessionFolderGuard(sessionID, []string{"_users/default/Chats"}, nil)

	if _, err := resolveCurrentWorkflowCostsPath(context.Background(), sessionID); err == nil {
		t.Fatal("expected an error outside any Workflow/<name> folder, got none")
	}
}

package virtualtools

import (
	"context"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
)

func TestWorkflowDBToolRegistryExposesQueryAndMutation(t *testing.T) {
	registry := CreateWorkflowDBToolRegistry("http://127.0.0.1:1", "", "session")
	if len(registry.Tools) != 2 {
		t.Fatalf("tools=%d, want 2", len(registry.Tools))
	}
	for _, name := range []string{"query_workflow_db", "mutate_workflow_db"} {
		if registry.Executors[name] == nil {
			t.Fatalf("missing executor %q", name)
		}
		if registry.Categories[name] != WorkflowDBToolCategory {
			t.Fatalf("category[%q]=%q", name, registry.Categories[name])
		}
	}
}

func TestWorkflowDBMutationExecutorDeniesReadOnlySession(t *testing.T) {
	sessionID := "workflow-db-read-only"
	defer common.ClearSessionShellConfig(sessionID)
	common.SetSessionFolderGuard(sessionID, []string{"Workflow/demo/db"}, nil)
	common.SetSessionShellEnv(sessionID, map[string]string{workflowDBAccessEnv: "read"})

	registry := CreateWorkflowDBToolRegistry("http://127.0.0.1:1", "", sessionID)
	_, err := registry.Executors["mutate_workflow_db"](context.Background(), map[string]any{
		"statements": []any{map[string]any{"sql": "DELETE FROM facts"}},
	})
	if err == nil || !strings.Contains(err.Error(), "mutation denied") {
		t.Fatalf("read-only mutation error=%v", err)
	}
}

func TestResolveCurrentWorkflowDBPathNeverAcceptsArbitraryDBPath(t *testing.T) {
	sessionID := "workflow-db-path"
	defer common.ClearSessionShellConfig(sessionID)
	common.SetSessionFolderGuard(sessionID, []string{"/tmp/untrusted.sqlite", "Workflow/demo/db"}, nil)

	path, err := resolveCurrentWorkflowDBPath(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if path != "Workflow/demo/db/db.sqlite" {
		t.Fatalf("resolved path=%q", path)
	}
}

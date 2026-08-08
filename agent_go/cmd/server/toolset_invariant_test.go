package server

import (
	"testing"

	virtualtools "github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// TestToolSetInvariants guards the bug class that hid notify_user: a tool the
// product depends on missing from its registration path.
//
// It used to also reconcile registration against GetToolsForWorkshopMode, via a
// hand-maintained list of names registered outside the workflow pool. That list
// was the band-aid over having two sources of truth, and it went stale twice
// (notify_user, list_llm_capabilities). The mode allow-list is gone, so
// registration is now the only source and there is nothing left to reconcile.
func TestToolSetInvariants(t *testing.T) {
	// 1. The workflow tool pool (workflowMode=true) must register every human tool.
	tools, _, cats := createCustomTools(true, "default", "invariant-test")
	pool := map[string]bool{}
	counts := map[string]int{}
	for _, tl := range tools {
		if tl.Function != nil {
			pool[tl.Function.Name] = true
			counts[tl.Function.Name]++
		}
	}
	for name, count := range counts {
		if count != 1 {
			t.Fatalf("workflow pool contains %d definitions for %q", count, name)
		}
	}
	browserTools := virtualtools.CreateWorkspaceBrowserTools()
	withBrowser := append(append([]llmtypes.Tool(nil), tools...), browserTools...)
	browserExecutors := virtualtools.CreateWorkspaceBrowserToolExecutors()
	executors := map[string]interface{}{}
	_, baseExecutors, _ := createCustomTools(true, "default", "filter-invariant-test")
	for name, executor := range baseExecutors {
		executors[name] = executor
	}
	for name, executor := range browserExecutors {
		executors[name] = executor
	}
	filtered, _ := orchestrator.FilterCustomToolsByCategory(withBrowser, executors, []string{
		"workspace_advanced:*", "human_tools:*", "workspace_browser:*",
		"workflow_db:query_workflow_db", "workflow_db:mutate_workflow_db",
	})
	filteredCounts := map[string]int{}
	for _, tool := range filtered {
		if tool.Function != nil {
			filteredCounts[tool.Function.Name]++
		}
	}
	for name, count := range filteredCounts {
		if count != 1 {
			t.Fatalf("filtered workflow pool contains %d definitions for %q", count, name)
		}
	}
	for _, n := range []string{"human_feedback", "notify_user", "create_human_input_request", "mark_human_input_consumed"} {
		if !pool[n] || cats[n] != "human_tools" {
			t.Fatalf("workflow pool missing human tool %q (in_pool=%v cat=%q)", n, pool[n], cats[n])
		}
	}
	if pool["submit_human_answer"] {
		t.Fatal("workflow pool still exposes removed submit_human_answer tool")
	}

	// Human interaction/notification tools must be registered in the pool.
	for _, n := range virtualtools.WorkshopHumanToolNames() {
		if !pool[n] || cats[n] != "human_tools" {
			t.Fatalf("workflow pool missing workshop human tool %q (in_pool=%v cat=%q)", n, pool[n], cats[n])
		}
	}

	// 4. Pulse worklist tools are registered in the workflow tool pool and must
	//    be visible in workshop mode, otherwise scheduled Pulse turns can ask
	//    for record_pulse_result/get_pulse_state and then fail at
	//    runtime with "not callable in this chat session".
	for _, n := range []string{"get_pulse_state", "begin_pulse_fixer_run", "record_pulse_worklist", "record_pulse_result", "record_pulse_impact", "resolve_run_concern"} {
		if !pool[n] || cats[n] != "workflow" {
			t.Fatalf("workflow pool missing Pulse state tool %q (in_pool=%v cat=%q)", n, pool[n], cats[n])
		}
	}
}

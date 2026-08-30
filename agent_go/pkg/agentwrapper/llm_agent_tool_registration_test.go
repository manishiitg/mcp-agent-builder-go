package agent

import (
	"context"
	"testing"
)

func noopExecutor(context.Context, map[string]interface{}) (string, error) { return "", nil }

// Re-registering a tool name must replace, not accumulate.
//
// This wrapper converts legacy incremental assembly into one immutable
// definition. That assembly registered into a map keyed by tool name, so
// re-registration was idempotent. Appending to a slice instead made it fatal:
// mcpagent's finalizeDefinition rejects a duplicate name and the agent fails to
// construct.
//
// A retained AgentWorks chat can resume the previous turn's thread and
// re-registers delegation tools onto a wrapper that already carries them. Every
// run after the first died before step 1 with `duplicate direct tool name
// "delegate"` — 2026-08-03 09:01:00 and 2026-08-04 09:00:18.
func TestReRegisteringAToolNameReplacesRatherThanDuplicating(t *testing.T) {
	w := &LLMAgentWrapper{}

	if err := w.RegisterCustomTool("delegate", "first", nil, noopExecutor, "delegation"); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if err := w.RegisterCustomTool("delegate", "second", nil, noopExecutor, "delegation"); err != nil {
		t.Fatalf("re-registration: %v", err)
	}

	direct := w.definition.Tools.Direct
	if len(direct) != 1 {
		names := make([]string, 0, len(direct))
		for _, tool := range direct {
			names = append(names, tool.Name)
		}
		t.Fatalf("definition carries %d direct tools %v, want 1 — finalizeDefinition rejects duplicates", len(direct), names)
	}
	if direct[0].Description != "second" {
		t.Errorf("description = %q, want the later registration to win", direct[0].Description)
	}
}

// Replacement must not disturb the tools registered around it.
func TestReRegistrationPreservesTheSurroundingTools(t *testing.T) {
	w := &LLMAgentWrapper{}
	for _, name := range []string{"query_agent", "delegate", "list_agents"} {
		if err := w.RegisterCustomTool(name, "v1", nil, noopExecutor, "delegation"); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	if err := w.RegisterCustomTool("delegate", "v2", nil, noopExecutor, "delegation"); err != nil {
		t.Fatalf("re-register delegate: %v", err)
	}

	direct := w.definition.Tools.Direct
	if len(direct) != 3 {
		t.Fatalf("got %d direct tools, want 3", len(direct))
	}
	want := []string{"query_agent", "delegate", "list_agents"}
	for i, name := range want {
		if direct[i].Name != name {
			t.Errorf("direct[%d] = %q, want %q — replacement moved a tool", i, direct[i].Name, name)
		}
	}
	if direct[1].Description != "v2" {
		t.Errorf("delegate description = %q, want v2", direct[1].Description)
	}
}

// A finalized definition is still closed to registration.
func TestRegistrationAfterFinalizeIsStillRejected(t *testing.T) {
	w := &LLMAgentWrapper{finalized: true}
	if err := w.RegisterCustomTool("delegate", "late", nil, noopExecutor, "delegation"); err == nil {
		t.Fatal("registration succeeded after finalize")
	}
}

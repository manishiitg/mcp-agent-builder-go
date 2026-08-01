package agentsession

import (
	"context"
	"testing"
)

func TestDefinitionFromConfigBuildsIdentityBeforeConstruction(t *testing.T) {
	handler := func(context.Context, map[string]interface{}) (string, error) { return "ok", nil }
	definition := definitionFromConfig(Config{
		SystemPrompt: "family assistant",
		Tools: []Tool{
			{Name: "query_family", Description: "Query", Params: map[string]interface{}{"type": "object"}, Handler: handler},
			{Name: "notify_family", Description: "Notify", Category: "notifications", Handler: handler},
		},
	})

	if definition.Instructions != "family assistant" {
		t.Fatalf("instructions = %q", definition.Instructions)
	}
	if len(definition.Tools.MCP) != 1 || definition.Tools.MCP[0].Name != "exa-search" {
		t.Fatalf("MCP sources = %#v", definition.Tools.MCP)
	}
	if len(definition.Tools.Direct) != 2 {
		t.Fatalf("direct tools = %#v", definition.Tools.Direct)
	}
	if got := definition.Tools.Direct[0].DisplayGroup; got != "family_tools" {
		t.Fatalf("default display group = %q", got)
	}
	if got := definition.Tools.Direct[1].DisplayGroup; got != "notifications" {
		t.Fatalf("explicit display group = %q", got)
	}
}

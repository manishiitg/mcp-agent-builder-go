package orchestrator

import (
	"context"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

func TestPrepareToolDefinitionsForAgentBuildsConstructionIdentity(t *testing.T) {
	bo := &BaseOrchestrator{
		logger:         loggerv2.NewNoop(),
		ToolCategories: map[string]string{"query_records": "database"},
	}
	tool := llmtypes.Tool{Type: "function", Function: &llmtypes.FunctionDefinition{
		Name:        "query_records",
		Description: "Query workflow records",
		Parameters: llmtypes.NewParameters(map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}),
	}}
	executor := func(context.Context, map[string]interface{}) (string, error) { return "ok", nil }

	definitions, err := bo.prepareToolDefinitionsForAgent(
		&agents.OrchestratorAgentConfig{
			FolderGuardReadPaths: []string{"Workflow/demo/db"},
		},
		[]llmtypes.Tool{tool},
		map[string]interface{}{"query_records": executor},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 {
		t.Fatalf("definitions = %#v", definitions)
	}
	definition := definitions[0]
	if definition.Name != "query_records" || definition.DisplayGroup != "database" {
		t.Fatalf("definition = %#v", definition)
	}
	if definition.Execute == nil {
		t.Fatal("prepared definition lost its executor")
	}
}

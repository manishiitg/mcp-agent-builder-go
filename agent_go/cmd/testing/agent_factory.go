package testing

import (
	"context"
	"encoding/json"
	"fmt"

	virtualtools "github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"
	mcpagent "github.com/manishiitg/mcpagent/agent"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

func runAgentText(ctx context.Context, agent *mcpagent.Agent, input string) (string, error) {
	result, err := agent.Run(ctx, mcpagent.Turn{Input: input})
	return result.Text, err
}

func createTestingAgent(ctx context.Context, model llmtypes.Model, configPath, server string, maxTurns int, logger loggerv2.Logger, direct []mcpagent.ToolDefinition) (*mcpagent.Agent, error) {
	definition := mcpagent.AgentDefinition{Tools: mcpagent.ToolSet{Direct: direct}}
	if server != "" {
		definition.Tools.MCP = []mcpagent.MCPToolSource{{Name: server}}
	}
	return mcpagent.NewAgentFromDefinition(ctx, definition, mcpagent.RuntimeConfig{
		Model: model, MCPConfigPath: configPath,
		Generation:    mcpagent.GenerationRuntimeConfig{MaxTurns: maxTurns},
		Observability: mcpagent.ObservabilityRuntimeConfig{Logger: logger},
	})
}

func advancedWorkspaceToolDefinitions() ([]mcpagent.ToolDefinition, error) {
	tools := virtualtools.CreateWorkspaceAdvancedTools()
	executors := virtualtools.CreateWorkspaceToolExecutors()
	definitions := make([]mcpagent.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		if tool.Function == nil {
			continue
		}
		executor, ok := executors[tool.Function.Name]
		if !ok {
			continue
		}
		var parameters map[string]interface{}
		encoded, err := json.Marshal(tool.Function.Parameters)
		if err != nil {
			return nil, fmt.Errorf("encode parameters for %s: %w", tool.Function.Name, err)
		}
		if err := json.Unmarshal(encoded, &parameters); err != nil {
			return nil, fmt.Errorf("decode parameters for %s: %w", tool.Function.Name, err)
		}
		definitions = append(definitions, mcpagent.ToolDefinition{
			Name: tool.Function.Name, Description: tool.Function.Description,
			InputSchema: parameters, Execute: executor, DisplayGroup: "workspace",
		})
	}
	return definitions, nil
}

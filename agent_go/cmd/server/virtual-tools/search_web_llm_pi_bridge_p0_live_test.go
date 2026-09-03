//go:build search_web_llm_pi_bridge_p0_live

package virtualtools

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	mcpagent "github.com/manishiitg/mcpagent/agent"
	"github.com/manishiitg/mcpagent/agentreview"
	"github.com/manishiitg/mcpagent/llm"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

// TestSearchWebLLMPiBridgeP0 proves the cross-repository production path used
// by Pi-dev: Pi loads mcpagent's api-bridge, calls this builder-owned direct
// tool, and receives a real result from Parallel's hosted MCP search.
func TestSearchWebLLMPiBridgeP0(t *testing.T) {
	if os.Getenv("RUN_SEARCH_WEB_LLM_PI_BRIDGE_P0") != "1" {
		t.Skip("set RUN_SEARCH_WEB_LLM_PI_BRIDGE_P0=1 to run the real Pi-dev search_web_llm P0")
	}
	if _, err := exec.LookPath("pi"); err != nil {
		t.Skip("Pi CLI is required for the Pi-dev search_web_llm P0")
	}
	apiKey := strings.TrimSpace(os.Getenv("PI_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	}
	if apiKey == "" {
		t.Skip("PI_API_KEY or GEMINI_API_KEY is required for the Pi-dev search_web_llm P0")
	}

	bridgeBin := buildPiBridgeBinary(t)
	t.Setenv("MCP_BRIDGE_BINARY", bridgeBin)
	apiURL, apiToken := startPiBridgeExecutor(t)

	modelID := strings.TrimSpace(os.Getenv("SEARCH_WEB_LLM_PI_P0_MODEL"))
	if modelID == "" {
		modelID = "google/gemini-3.8-flash"
	}
	model, err := llm.InitializeLLM(llm.Config{
		Provider: llm.ProviderPiCLI,
		ModelID:  modelID,
		APIKeys:  &llm.ProviderAPIKeys{PiCLI: &apiKey},
		Logger:   loggerv2.NewDefault(),
	})
	if err != nil {
		t.Fatalf("initialize Pi CLI: %v", err)
	}

	query := "Find the most recent stable Go release and include the official Go release-notes URL."
	called := false
	agent, err := mcpagent.NewAgentFromDefinition(context.Background(), mcpagent.AgentDefinition{
		Instructions: "You are a web-research agent. Use the search_web_llm tool exactly once with provider parallel to answer the user's question. Do not use any provider-native web-search capability. Include the official Go URL from the tool result in your final answer.",
		Tools: mcpagent.ToolSet{Direct: []mcpagent.ToolDefinition{{
			Name:         "search_web_llm",
			Description:  "Search the live web through hosted MCP. Required arguments: query and provider=parallel.",
			DisplayGroup: "workspace_advanced",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query":    map[string]interface{}{"type": "string"},
					"provider": map[string]interface{}{"type": "string", "enum": []string{"parallel"}},
				},
				"required": []string{"query", "provider"},
			},
			Execute: func(ctx context.Context, args map[string]interface{}) (string, error) {
				called = true
				return CreateSearchWebLLMProviderTestExecutor("")(ctx, map[string]any{
					"query":    args["query"],
					"provider": args["provider"],
				})
			},
		}}},
	}, mcpagent.RuntimeConfig{
		Model: model,
		Generation: mcpagent.GenerationRuntimeConfig{
			Provider: llm.ProviderPiCLI,
			MaxTurns: 6,
			APIKeys:  &mcpagent.AgentAPIKeys{PiCLI: &apiKey},
		},
		Tools:  mcpagent.ToolRuntimeConfig{CodeExecution: true, AdditionalBridge: []string{"search_web_llm"}},
		Coding: mcpagent.CodingRuntimeConfig{AgentToolsMode: "mcp_only"},
		MCP: mcpagent.MCPRuntimeConfig{
			SessionID:  "search-web-llm-pi-p0-" + strings.ReplaceAll(t.Name(), "/", "-"),
			APIBaseURL: apiURL,
			APIToken:   apiToken,
		},
		Workspace:     mcpagent.WorkspaceRuntimeConfig{CodingAgentWorkingDir: t.TempDir(), IsolatedSession: true},
		Observability: mcpagent.ObservabilityRuntimeConfig{Logger: loggerv2.NewDefault()},
	})
	if err != nil {
		t.Fatalf("create Pi-dev mcpagent: %v", err)
	}
	defer agent.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	result, err := agent.Run(ctx, mcpagent.Turn{Input: query})
	if err != nil {
		t.Fatalf("Pi-dev search turn: %v", err)
	}
	if !called {
		t.Fatalf("Pi-dev did not invoke search_web_llm; output: %s", truncateMCPTestResult(result.Text, 1_000))
	}
	if !strings.Contains(strings.ToLower(result.Text), "go") || !strings.Contains(result.Text, "https://go.dev/") {
		t.Fatalf("Pi-dev final answer lacks the expected Go evidence: %s", truncateMCPTestResult(result.Text, 1_000))
	}

	record := agentreview.WriteWithCriteria(t,
		"TestSearchWebLLMPiBridgeP0",
		"Pi-dev invoked builder search_web_llm through mcpagent's api bridge and returned live Go release evidence.",
		[]string{
			"Pi-dev invoked search_web_llm through the MCP bridge rather than a native search tool",
			"the returned answer is grounded in a real Parallel web-search result and includes an official Go URL",
			"the response is coherent and contains no credentials or transport noise",
		},
		map[string]any{"query": query, "response": result.Text},
		map[string]any{"provider": "pi-cli", "tool": "search_web_llm", "backend": "parallel"},
	)
	agentreview.RequireReviewed(t, record)
}

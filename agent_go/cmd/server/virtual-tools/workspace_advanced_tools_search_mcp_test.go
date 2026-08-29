package virtualtools

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMCPWebSearchRequest(t *testing.T) {
	tests := []struct {
		provider string
		wantName string
		wantURL  string
		wantTool string
	}{
		{provider: "parallel", wantName: "parallel", wantURL: "https://search.parallel.ai/mcp", wantTool: "web_search"},
		{provider: "exa-search", wantName: "exa", wantURL: "https://mcp.exa.ai/mcp", wantTool: "web_search_exa"},
		{provider: "firecrawl", wantName: "firecrawl", wantURL: "https://mcp.firecrawl.dev/v2/mcp", wantTool: "firecrawl_search"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			request, ok := buildMCPWebSearchRequest(tt.provider, "current web search")
			if !ok {
				t.Fatal("mcpWebSearchRequest() did not recognize provider")
			}
			if request.provider != tt.wantName || request.url != tt.wantURL || request.tool != tt.wantTool {
				t.Fatalf("mcpWebSearchRequest() = %+v", request)
			}
		})
	}

	if _, ok := buildMCPWebSearchRequest("codex-cli", "current web search"); ok {
		t.Fatal("mcpWebSearchRequest() recognized a published LLM provider")
	}
}

// TestSearchWebLLMToolP0Contract is the inexpensive P0 guard for this special
// virtual tool. It checks the public routing contract before the stricter live
// provider-matrix gate runs against real services. search_web_llm is a hosted
// MCP-only tool; any new provider must be added to this list and the CLI
// provider-matrix runner.
func TestSearchWebLLMToolP0Contract(t *testing.T) {
	mcpProviders := []string{"parallel", "exa", "firecrawl"}
	for _, provider := range mcpProviders {
		if _, ok := buildMCPWebSearchRequest(provider, "P0 search provider matrix"); !ok {
			t.Fatalf("P0 search provider %q is not registered as MCP-backed", provider)
		}
	}

	if got, want := len(mcpProviders), 3; got != want {
		t.Fatalf("P0 search provider matrix has %d providers, want %d", got, want)
	}

	for _, provider := range []string{"claude-code", "codex-cli", "cursor-cli", "pi-cli", "vertex"} {
		_, err := createSearchWebLLMExecutor(" ")(context.Background(), map[string]any{"query": "P0 route", "provider": provider})
		if err == nil || !strings.Contains(err.Error(), "unsupported search_web_llm provider") {
			t.Fatalf("native provider %q was not rejected as unsupported: %v", provider, err)
		}
	}

	_, err := createSearchWebLLMExecutor(" ")(context.Background(), map[string]any{
		"query":    "P0 model validation",
		"provider": "parallel",
		"model_id": "must-not-be-accepted",
	})
	if err == nil || !strings.Contains(err.Error(), "does not accept model_id") {
		t.Fatalf("P0 MCP model_id rejection = %v, want explicit rejection", err)
	}
}

// This is intentionally opt-in: it calls each public keyless MCP service and
// therefore consumes the caller's anonymous free-tier quota.
func TestMCPWebSearchLive(t *testing.T) {
	if os.Getenv("RUN_SEARCH_WEB_LLM_MCP_E2E") != "1" {
		t.Skip("set RUN_SEARCH_WEB_LLM_MCP_E2E=1 to call public MCP search providers")
	}

	query := strings.TrimSpace(os.Getenv("SEARCH_WEB_LLM_MCP_E2E_QUERY"))
	if query == "" {
		query = "What is Example Domain? Answer in one sentence."
	}
	providers := []string{"parallel", "exa", "firecrawl"}
	if configured := strings.TrimSpace(os.Getenv("SEARCH_WEB_LLM_MCP_E2E_PROVIDERS")); configured != "" {
		providers = strings.Split(configured, ",")
	}

	for _, provider := range providers {
		provider = strings.TrimSpace(provider)
		if provider == "" {
			continue
		}
		t.Run(provider, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			result, err := createSearchWebLLMExecutor(" ")(ctx, map[string]any{
				"query":    query,
				"provider": provider,
			})
			if err != nil && (strings.Contains(err.Error(), "free MCP rate limit") || strings.Contains(err.Error(), "Anonymous keyless access is unavailable")) {
				t.Skipf("anonymous MCP quota is unavailable for %s: %v", provider, err)
			}
			if err != nil {
				t.Fatalf("executeMCPWebSearch() error = %v", err)
			}
			if strings.TrimSpace(result) == "" {
				t.Fatal("executeMCPWebSearch() returned an empty result")
			}
			t.Logf("search_web_llm response: %s", truncateMCPTestResult(result, 2_000))
		})
	}
}

func truncateMCPTestResult(result string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(result))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}

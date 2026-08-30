//go:build search_web_llm_tool_p0_live

package virtualtools

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/manishiitg/mcpagent/agentreview"
)

var searchWebLLMToolP0Criteria = []string{
	"the recorded response is a real web-search result, not a mock, auth prompt, rate-limit message, or transport error",
	"the result answers the release query using current information and includes evidence from authoritative web sources",
	"the response contains usable source URLs or source metadata for a follow-up reader",
	"the response is coherent and does not expose credentials, request headers, or internal transport noise",
}

// TestSearchWebLLMToolP0Live is the agentic live contract for this special
// virtual tool. It executes every supported provider through the public
// search_web_llm executor, then stores the exact returned response for an
// agent to inspect and approve before the cheap review gate can pass.
func TestSearchWebLLMToolP0Live(t *testing.T) {
	workspaceURL := strings.TrimSpace(os.Getenv("SEARCH_WEB_LLM_P0_WORKSPACE_URL"))
	if workspaceURL == "" {
		workspaceURL = strings.TrimSpace(os.Getenv("WORKSPACE_API_URL"))
	}
	query := strings.TrimSpace(os.Getenv("SEARCH_WEB_LLM_P0_QUERY"))
	if query == "" {
		query = "Find the most recent stable Go release, including its version and the official release-notes URL."
	}

	providers := []string{"parallel", "exa", "firecrawl"}
	if configured := strings.TrimSpace(os.Getenv("SEARCH_WEB_LLM_P0_PROVIDERS")); configured != "" {
		providers = strings.Split(configured, ",")
	}
	executor := CreateSearchWebLLMProviderTestExecutor(workspaceURL)
	for _, provider := range providers {
		provider = strings.TrimSpace(provider)
		if provider == "" {
			continue
		}
		t.Run(provider, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			result, err := executor(ctx, map[string]any{"query": query, "provider": provider})
			if err != nil {
				t.Fatalf("search_web_llm(%s) failed: %v", provider, err)
			}
			if strings.TrimSpace(result) == "" {
				t.Fatal("search_web_llm returned an empty response")
			}
			if !strings.Contains(strings.ToLower(result), "go") {
				t.Fatalf("search_web_llm response did not contain the expected release marker: %s", truncateMCPTestResult(result, 500))
			}

			record := agentreview.WriteWithCriteria(t,
				"TestSearchWebLLMToolP0Live_"+strings.ReplaceAll(provider, "-", "_"),
				"Real search_web_llm response for "+provider,
				searchWebLLMToolP0Criteria,
				map[string]any{"provider": provider, "query": query, "response": result},
				map[string]any{"provider": provider, "route": searchWebLLMP0Route(provider), "expected_marker": "go"},
			)
			agentreview.RequireReviewed(t, record)
		})
	}
}

func searchWebLLMP0Route(provider string) string {
	if _, ok := buildMCPWebSearchRequest(provider, "P0 route"); ok {
		return "hosted-mcp"
	}
	return "published-native-llm"
}

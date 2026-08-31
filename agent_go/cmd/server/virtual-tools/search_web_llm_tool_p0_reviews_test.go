package virtualtools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manishiitg/mcpagent/agentreview"
)

// TestSearchWebLLMToolP0ReviewsApproved is the cheap, no-network half of the
// tool P0. Capture writes provider receipts; this gate requires agent approval
// for every current receipt before release.
func TestSearchWebLLMToolP0ReviewsApproved(t *testing.T) {
	if os.Getenv("SEARCH_WEB_LLM_TOOL_P0_VERIFY") != "1" {
		t.Skip("set SEARCH_WEB_LLM_TOOL_P0_VERIFY=1 to enforce the release P0 review gate")
	}
	dir := filepath.Join("testdata", "search-web-llm-agent-reviews")
	for _, provider := range []string{"parallel", "exa", "firecrawl"} {
		name := "TestSearchWebLLMToolP0Live_" + strings.ReplaceAll(provider, "-", "_")
		if _, err := os.Stat(filepath.Join(dir, name+".json")); err != nil {
			t.Fatalf("search_web_llm P0 receipt for %q is missing; run scripts/search-web-llm-tool-p0.sh capture", provider)
		}
	}
	agentreview.RequireAllApproved(t, dir)
}

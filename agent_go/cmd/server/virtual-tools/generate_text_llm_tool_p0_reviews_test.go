package virtualtools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/manishiitg/mcpagent/agentreview"
)

// TestGenerateTextLLMToolP0ReviewApproved is the no-network release half of
// the agentic P0. A live Pi bridge capture must be agent-reviewed before this
// special tool can be considered release-ready.
func TestGenerateTextLLMToolP0ReviewApproved(t *testing.T) {
	if os.Getenv("GENERATE_TEXT_LLM_TOOL_P0_VERIFY") != "1" {
		t.Skip("set GENERATE_TEXT_LLM_TOOL_P0_VERIFY=1 to enforce the generate_text_llm P0 review gate")
	}
	dir := filepath.Join("testdata", "generate-text-llm-agent-reviews")
	if _, err := os.Stat(filepath.Join(dir, "TestGenerateTextLLMPiBridgeP0.json")); err != nil {
		t.Fatalf("generate_text_llm Pi bridge P0 receipt is missing; run scripts/generate-text-llm-tool-p0.sh capture")
	}
	agentreview.RequireAllApproved(t, dir)
}

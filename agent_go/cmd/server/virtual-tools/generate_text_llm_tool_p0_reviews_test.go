package virtualtools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manishiitg/mcpagent/agentreview"
)

var generateTextLLMP0RequiredTiers = []string{"low", "medium", "high"}

type generateTextLLMP0ReviewReceipt struct {
	Output struct {
		Runs []struct {
			Tier          string                `json:"tier"`
			ToolCalls     int                   `json:"tool_calls"`
			ToolResult    generateTextLLMResult `json:"tool_result"`
			FinalResponse string                `json:"final_response"`
		} `json:"runs"`
	} `json:"output"`
}

// TestGenerateTextLLMToolP0ReviewApproved is the no-network release half of
// the agentic P0. A live Pi bridge capture must be agent-reviewed before this
// special tool can be considered release-ready.
func TestGenerateTextLLMToolP0ReviewApproved(t *testing.T) {
	if os.Getenv("GENERATE_TEXT_LLM_TOOL_P0_VERIFY") != "1" {
		t.Skip("set GENERATE_TEXT_LLM_TOOL_P0_VERIFY=1 to enforce the generate_text_llm P0 review gate")
	}
	dir := filepath.Join("testdata", "generate-text-llm-agent-reviews")
	receiptPath := filepath.Join(dir, "TestGenerateTextLLMPiBridgeP0.json")
	b, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("generate_text_llm Pi bridge P0 receipt is missing; run scripts/generate-text-llm-tool-p0.sh capture")
	}
	var receipt generateTextLLMP0ReviewReceipt
	if err := json.Unmarshal(b, &receipt); err != nil {
		t.Fatalf("decode generate_text_llm Pi bridge P0 receipt: %v", err)
	}
	if len(receipt.Output.Runs) != len(generateTextLLMP0RequiredTiers) {
		t.Fatalf("%s has %d tier runs, want exactly %d", receiptPath, len(receipt.Output.Runs), len(generateTextLLMP0RequiredTiers))
	}
	seen := make(map[string]bool, len(receipt.Output.Runs))
	for _, run := range receipt.Output.Runs {
		if run.ToolCalls != 1 || run.ToolResult.Tier != run.Tier || run.ToolResult.Provider == "" || run.ToolResult.ModelID == "" {
			t.Fatalf("invalid %q tier evidence in %s: %#v", run.Tier, receiptPath, run)
		}
		canary := "GENERATE_TEXT_LLM_P0_" + strings.ToUpper(run.Tier) + "_OK"
		if !strings.Contains(run.ToolResult.Response, canary) {
			t.Fatalf("%q tier tool result in %s lacks canary %q", run.Tier, receiptPath, canary)
		}
		if !strings.Contains(run.FinalResponse, canary) {
			t.Fatalf("%q tier Pi response in %s does not relay canary %q", run.Tier, receiptPath, canary)
		}
		if seen[run.Tier] {
			t.Fatalf("duplicate %q tier evidence in %s", run.Tier, receiptPath)
		}
		seen[run.Tier] = true
	}
	for _, tier := range generateTextLLMP0RequiredTiers {
		if !seen[tier] {
			t.Fatalf("%s must contain real Pi bridge evidence for tier %q; re-capture it", receiptPath, tier)
		}
	}
	agentreview.RequireAllApproved(t, dir)
}

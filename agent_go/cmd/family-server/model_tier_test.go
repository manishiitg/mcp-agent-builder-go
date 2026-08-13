package main

import (
	"testing"

	"github.com/manishiitg/mcpagent/llm"
	llmproviders "github.com/manishiitg/multi-llm-provider-go"
)

// Fast Mode must change how hard the model thinks, never WHICH model runs.
// Swapping the model changed the tutor itself - different phrasing, different
// judgment on how much to reveal under teaching_mode - which is the one thing
// this app cannot trade for latency.
func TestFastModeChangesEffortNotModel(t *testing.T) {
	for _, p := range []llmproviders.Provider{
		llmproviders.ProviderCodexCLI,
		llmproviders.ProviderClaudeCode,
		llmproviders.ProviderCursorCLI,
	} {
		provider := llm.Provider(p)
		normal, fast := selectedModelID(familyState{}, provider), selectedModelID(familyState{}, provider)
		if normal != fast {
			t.Fatalf("%s: model must not depend on Fast Mode (%q vs %q)", p, normal, fast)
		}
		if got := selectedReasoningEffort(false, provider); got != "high" {
			t.Fatalf("%s: normal effort should be high, got %q", p, got)
		}
		if got := selectedReasoningEffort(true, provider); got == "high" || got == "" {
			t.Fatalf("%s: fast mode should lower effort, got %q", p, got)
		}
	}
}

func TestCursorDropsOneStepOnly(t *testing.T) {
	// Cursor's honoring of reasoning_effort is the least established of the
	// three, so it steps to medium rather than assuming a floor it may ignore.
	if got := selectedReasoningEffort(true, llm.Provider(llmproviders.ProviderCursorCLI)); got != "medium" {
		t.Fatalf("cursor fast effort = %q, want medium", got)
	}
	if got := selectedReasoningEffort(true, llm.Provider(llmproviders.ProviderCodexCLI)); got != "low" {
		t.Fatalf("codex fast effort = %q, want low", got)
	}
}

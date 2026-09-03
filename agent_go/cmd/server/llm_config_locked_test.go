package server

import (
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
)

const cursorOnlyPublishedList = `[{"id":"agentworks-default","name":"cursor-cli (cursor-cli)","provider":"cursor-cli","model_id":"cursor-cli"}]`

// With an explicitly published list, a locked deployment offers exactly that
// list: a workflow's saved OpenAI config must not keep running on OpenAI just
// because the platform knows the model.
func TestLockedLLMRestrictsToThePublishedListWhenOneIsConfigured(t *testing.T) {
	t.Setenv("LLM_CONFIG_LOCKED", "true")
	t.Setenv("DEFAULT_PUBLISHED_LLMS", cursorOnlyPublishedList)
	t.Setenv("DEFAULT_PUBLISHED_LLMS_PATH", "")

	if !isAllowedDefaultLLM("cursor-cli", "cursor-cli") {
		t.Fatal("the published entry itself must be allowed")
	}
	if isAllowedDefaultLLM("openai", "gpt-5.2") {
		t.Fatal("a known model of another provider must be refused when a published list is configured")
	}
	if isAllowedDefaultLLM("cursor-cli", "gpt-5") {
		t.Fatal("another model of the published provider must be refused too")
	}
}

// Without an explicit list the historical behaviour stands: any model the
// platform knows for a locked provider is accepted.
func TestLockedLLMWithoutPublishedListKeepsKnownModels(t *testing.T) {
	t.Setenv("LLM_CONFIG_LOCKED", "true")
	t.Setenv("DEFAULT_PUBLISHED_LLMS", "")
	t.Setenv("DEFAULT_PUBLISHED_LLMS_PATH", "")

	if !isAllowedDefaultLLM("cursor-cli", "gpt-5") {
		t.Fatal("a curated cursor-cli model must stay allowed in the legacy mode")
	}
}

// A product profile's own binding (Video Studio pins claude-code) is operator
// configuration and wins under the lock; a user's or a workflow's choice that
// is not published falls back to the server default.
func TestResolveLockedLLMHonoursProfileBindingsAndFallsBackOtherwise(t *testing.T) {
	t.Setenv("LLM_CONFIG_LOCKED", "true")
	t.Setenv("DEFAULT_PUBLISHED_LLMS", cursorOnlyPublishedList)
	t.Setenv("DEFAULT_PUBLISHED_LLMS_PATH", "")

	profileBound := &orchestrator.LLMConfig{Primary: orchestrator.LLMModel{Provider: "claude-code", ModelID: "claude-sonnet-5"}}
	if p, m := resolveLockedLLM(profileBound, llmConfigSourceAgentProfile); p != "claude-code" || m != "claude-sonnet-5" {
		t.Fatalf("profile binding must win under the lock, got %s/%s", p, m)
	}

	userChoice := &orchestrator.LLMConfig{Primary: orchestrator.LLMModel{Provider: "openai", ModelID: "gpt-5.2"}}
	p, m := resolveLockedLLM(userChoice, "")
	if p == "openai" {
		t.Fatalf("an unpublished user choice must not run under the lock, got %s/%s", p, m)
	}
	dp, dm := getPrimaryProviderAndModelFromDefaults()
	if p != dp || m != dm {
		t.Fatalf("unpublished choice must fall back to the server default %s/%s, got %s/%s", dp, dm, p, m)
	}

	published := &orchestrator.LLMConfig{Primary: orchestrator.LLMModel{Provider: "cursor-cli", ModelID: "cursor-cli"}}
	if p, m := resolveLockedLLM(published, ""); p != "cursor-cli" || m != "cursor-cli" {
		t.Fatalf("the published entry must be accepted, got %s/%s", p, m)
	}
}

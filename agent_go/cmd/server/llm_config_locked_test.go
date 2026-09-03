package server

import (
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workflowtypes"
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

// A workflow's saved llm_config is what the Builder chat, scheduled runs and
// step tiers actually use, so the lock must rewrite it: every role becomes
// the published default (a role already on the list is kept), fallbacks go,
// and the provider-profile shortcut is expanded so nothing derives models
// from an unlocked provider. Off-lock the config passes through untouched.
func TestLockedPresetLLMConfigRewritesEveryRoleToThePublishedDefault(t *testing.T) {
	t.Setenv("LLM_CONFIG_LOCKED", "true")
	t.Setenv("DEFAULT_PUBLISHED_LLMS", cursorOnlyPublishedList)
	t.Setenv("DEFAULT_PUBLISHED_LLMS_PATH", "")

	saved := &workflowtypes.PresetLLMConfig{
		SchemaVersion: 2,
		Mode:          workflowtypes.LLMConfigModeExplicit,
		BuilderLLM:    &workflowtypes.AgentLLMConfig{Provider: "claude-code", ModelID: "claude-sonnet-5", Fallbacks: []workflowtypes.AgentLLMFallback{{Provider: "openai", ModelID: "gpt-5.2"}}},
		PulseLLM:      &workflowtypes.AgentLLMConfig{Provider: "cursor-cli", ModelID: "cursor-cli", Fallbacks: []workflowtypes.AgentLLMFallback{{Provider: "openai", ModelID: "gpt-5.2"}}},
		TieredConfig:  &workflowtypes.TieredLLMConfig{Tier1: &workflowtypes.AgentLLMConfig{Provider: "openai", ModelID: "gpt-5.2"}},
	}
	got := lockedPresetLLMConfig(saved)
	if got == saved {
		t.Fatal("must return a rewritten copy, not the saved config")
	}
	for name, role := range map[string]*workflowtypes.AgentLLMConfig{"builder": got.BuilderLLM, "pulse": got.PulseLLM, "tier1": got.TieredConfig.Tier1, "tier2": got.TieredConfig.Tier2, "tier3": got.TieredConfig.Tier3} {
		if role == nil || role.Provider != "cursor-cli" || role.ModelID != "cursor-cli" || len(role.Fallbacks) != 0 {
			t.Fatalf("%s = %+v, want cursor-cli/cursor-cli with no fallbacks", name, role)
		}
	}
	if got.Mode != workflowtypes.LLMConfigModeExplicit || got.Provider != "" {
		t.Fatalf("mode/provider = %q/%q, want explicit with no provider profile", got.Mode, got.Provider)
	}
	if saved.BuilderLLM.Provider != "claude-code" {
		t.Fatal("the saved config must not be mutated")
	}

	profile := &workflowtypes.PresetLLMConfig{SchemaVersion: 2, Mode: workflowtypes.LLMConfigModeProviderProfile, Provider: "claude-code"}
	if got := lockedPresetLLMConfig(profile); got.BuilderLLM == nil || got.BuilderLLM.Provider != "cursor-cli" {
		t.Fatalf("provider-profile config must be expanded to the published default, got %+v", got.BuilderLLM)
	}
	if lockedPresetLLMConfig(nil) == nil {
		t.Fatal("a workflow with no llm_config still gets the published default under the lock")
	}

	t.Setenv("LLM_CONFIG_LOCKED", "false")
	if got := lockedPresetLLMConfig(saved); got != saved {
		t.Fatal("off-lock the saved config must pass through untouched")
	}
}

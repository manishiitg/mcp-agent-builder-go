package main

import (
	"testing"

	"github.com/manishiitg/mcpagent/llm"
	llmproviders "github.com/manishiitg/multi-llm-provider-go"
)

func TestAvailableModelsComeFromTheRealCatalog(t *testing.T) {
	for _, tc := range []struct {
		provider llmproviders.Provider
		want     string
	}{
		{llmproviders.ProviderCodexCLI, "gpt-5.6-sol"},
		{llmproviders.ProviderClaudeCode, "claude-opus-5"},
		{llmproviders.ProviderCursorCLI, "composer-2.5"},
	} {
		models := availableModelsFor(tc.provider)
		if len(models) == 0 {
			t.Fatalf("%s: no models offered", tc.provider)
		}
		found := false
		for _, m := range models {
			if m.ID == tc.want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: expected %q in the catalog, got %v", tc.provider, tc.want, models)
		}
	}
	// A provider this app doesn't drive must yield no picker rather than a
	// wrong or empty-looking one.
	if got := availableModelsFor(llmproviders.Provider("openai")); got != nil {
		t.Fatalf("unsupported provider should offer nothing, got %v", got)
	}
}

func TestSelectedModelFallsBackToTheTunedDefault(t *testing.T) {
	provider := llm.Provider(llmproviders.ProviderClaudeCode)

	// No choice recorded -> this app's tuned default, not empty.
	if got := selectedModelID(familyState{}, provider); got != mediumTierModelID(provider) {
		t.Fatalf("empty state should use the tuned default, got %q", got)
	}
	// A choice wins.
	s := familyState{SelectedModels: map[string]string{string(provider): "claude-opus-5"}}
	if got := selectedModelID(s, provider); got != "claude-opus-5" {
		t.Fatalf("chosen model should win, got %q", got)
	}
	// A choice for a DIFFERENT provider must not leak into this one.
	other := familyState{SelectedModels: map[string]string{"codex-cli": "gpt-5.6-luna"}}
	if got := selectedModelID(other, provider); got != mediumTierModelID(provider) {
		t.Fatalf("another provider's choice leaked, got %q", got)
	}
}

package guidance

import (
	"strings"
	"testing"
)

// modeManagedConfigurationTools are the provider-configuration tools that only
// modes managing provider configuration actually register. Recommending them in
// any other mode produces `tools_unavailable` at runtime.
var modeManagedConfigurationTools = []string{
	"list_published_llms",
	"list_provider_models",
	"test_llm",
	"save_published_llm",
	"set_provider_auth",
}

// TestConfigurationAccessGuidanceRecommendsToolsOnlyWhereTheyAreRegistered
// pins guidance to the registry that already governs it. `llm-provider-config`
// declares the modes that manage provider configuration; the tool
// recommendation must appear in exactly those modes and no others.
//
// Before the fix the recommendation was appended unconditionally, so `run`
// mode — every scheduled workflow run and every Pulse pass — was instructed to
// call tools it never receives.
func TestConfigurationAccessGuidanceRecommendsToolsOnlyWhereTheyAreRegistered(t *testing.T) {
	for _, mode := range allGuidanceModes(t) {
		guidanceText := buildConfigurationAccessGuidance(mode)
		manages := modeAllowedIn("llm-provider-config", mode, referenceKinds)

		for _, tool := range modeManagedConfigurationTools {
			mentioned := strings.Contains(guidanceText, tool)
			if mentioned && !manages {
				t.Errorf("mode %q does not manage provider configuration (llm-provider-config is not registered for it), "+
					"yet the guidance recommends %q; that tool is not registered in this mode and the call fails with tools_unavailable.\nguidance: %s",
					mode, tool, guidanceText)
			}
			if !mentioned && manages {
				t.Errorf("mode %q manages provider configuration but the guidance never names %q, so the agent cannot discover it.\nguidance: %s",
					mode, tool, guidanceText)
			}
		}
	}
}

// TestConfigurationAccessGuidanceKeepsConfigProhibitionInEveryMode guards the
// other half of the split: gating the tool recommendation must not take the
// "do not hand-edit config/" guardrail down with it. That prohibition is not
// mode-specific — a run-mode agent that cannot call the tools is exactly the
// one that must not fall back to editing config files directly.
func TestConfigurationAccessGuidanceKeepsConfigProhibitionInEveryMode(t *testing.T) {
	for _, mode := range allGuidanceModes(t) {
		guidanceText := buildConfigurationAccessGuidance(mode)
		if !strings.Contains(guidanceText, "config/") {
			t.Errorf("mode %q lost the config/ prohibition entirely: %s", mode, guidanceText)
		}
	}
}

// allGuidanceModes derives the mode list from the registry itself so a newly
// introduced mode is covered without editing this test.
func allGuidanceModes(t *testing.T) []string {
	t.Helper()
	seen := map[string]struct{}{}
	var modes []string
	for _, meta := range referenceKinds {
		for _, mode := range meta.Modes {
			if _, ok := seen[mode]; ok {
				continue
			}
			seen[mode] = struct{}{}
			modes = append(modes, mode)
		}
	}
	if len(modes) == 0 {
		t.Fatal("no modes discovered from referenceKinds")
	}
	return modes
}

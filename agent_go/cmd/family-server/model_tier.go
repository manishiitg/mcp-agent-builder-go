package main

import (
	"github.com/manishiitg/mcpagent/llm"
	llmproviders "github.com/manishiitg/multi-llm-provider-go"
)

// mediumTierModelID resolves the coding-agent model ID for a provider from
// multi-llm-provider-go's shared tier defaults (e.g. "claude-sonnet-5" for
// Claude Code), instead of leaving ModelID empty — which silently defers to
// whatever model the user's own coding-agent CLI happens to be set to via its
// own /model command, an ambient setting unrelated to this app. Falls back to
// "" (agentsession's own default) if the provider has no published tier
// defaults.
//
// Normally this is the "medium" tier. Two deliberate per-provider exceptions:
//   - Cursor CLI: its medium tier defaults to composer-2.5, but this app wants
//     Cursor's high tier (grok-4.5) instead — composer-2.5 wasn't strong enough
//     for family tutoring use, so we pin the stronger model for Cursor specifically.
//   - Codex CLI: pinned to gpt-5.6-sol rather than the catalog's medium-tier
//     default (gpt-5.6-terra) — a deliberate choice, not a tier lookup, so it's
//     a literal here rather than routed through GetCodingAgentDefaultTierModels.
func mediumTierModelID(provider llm.Provider) string {
	if llmproviders.Provider(provider) == llmproviders.ProviderCodexCLI {
		return "gpt-5.6-sol"
	}
	tiers, ok := llmproviders.GetCodingAgentDefaultTierModels(llmproviders.Provider(provider))
	if !ok {
		return ""
	}
	if llmproviders.Provider(provider) == llmproviders.ProviderCursorCLI {
		return tiers.High.ModelID
	}
	return tiers.Medium.ModelID
}

// A lowTierModelID() used to live here, resolving each provider's FAST tier
// (haiku for Claude Code, composer-2.5 for Cursor) for CHILD Mode, on the theory
// that short tutoring turns want latency over depth. Child Mode now uses
// mediumTierModelID like the parent — the cheaper tier was costing accuracy on
// the judgment that matters most in a tutor (see child.go) — so it went rather
// than lingering unused. Both surfaces resolving through one function is also
// one fewer thing to keep in sync.

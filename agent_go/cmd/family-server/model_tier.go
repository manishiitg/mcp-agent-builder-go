package main

import (
	"encoding/json"
	"net/http"

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
//     Cursor's high tier (grok-4.6) instead — composer-2.5 wasn't strong enough
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

// lowTierModelID resolves the FAST/cheap model for a provider — used only
// when the parent has explicitly turned on Fast Mode (familyState.FastMode,
// see selectedModelID), trading accuracy for latency across every surface
// (parent, child, WhatsApp, Pulse). This is the same tradeoff Child Mode
// itself deliberately moved away from by DEFAULT (see mediumTierModelID's own
// comment) — Fast Mode exists so a parent can opt back into it explicitly
// when speed matters more than depth for a given session, not as a silent
// default.
//
//   - Codex CLI: the catalog's own low tier, gpt-5.6-luna.
//   - Claude Code: the catalog's own low tier, claude-haiku-4-5.
//   - Cursor CLI: the catalog's low tier is "auto" (its own internal router),
//     which testing found too unpredictable for a "fast but still reliable"
//     mode — composer-2.5 (the catalog's MEDIUM tier, deliberately not used
//     as Cursor's normal-mode model — see mediumTierModelID's own override to
//     grok-4.6) is the actual fast, cheap, always-available model, so Fast
//     Mode uses that instead of the catalog's nominal "low".
func lowTierModelID(provider llm.Provider) string {
	tiers, ok := llmproviders.GetCodingAgentDefaultTierModels(llmproviders.Provider(provider))
	if !ok {
		return ""
	}
	if llmproviders.Provider(provider) == llmproviders.ProviderCursorCLI {
		return tiers.Medium.ModelID
	}
	return tiers.Low.ModelID
}

// selectedModelID is mediumTierModelID, unless Fast Mode is on — the single
// place every call site (chat.go, child.go, pulse.go, whatsapp_bot.go) should
// go through, so the toggle applies uniformly rather than needing to be
// threaded into each one individually.
func selectedModelID(fastMode bool, provider llm.Provider) string {
	if fastMode {
		return lowTierModelID(provider)
	}
	return mediumTierModelID(provider)
}

// GET /api/fast-mode
func handleGetFastMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": s.FastMode})
}

type setFastModeRequest struct {
	Enabled bool `json:"enabled"`
}

// POST /api/fast-mode
func handleSetFastMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req setFastModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	stateMu.Lock()
	s := loadState()
	s.FastMode = req.Enabled
	err := saveState(s)
	stateMu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": req.Enabled})
}

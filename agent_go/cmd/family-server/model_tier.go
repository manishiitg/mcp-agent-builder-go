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

// defaultReasoningEffort is what every surface uses normally. Family tutoring
// wants the model to actually think — this is not a latency-sensitive chat.
const defaultReasoningEffort = "high"

// selectedReasoningEffort is where Fast Mode now acts.
//
// Fast Mode used to swap the MODEL for a cheaper one, which changed the
// tutoring itself: a different model reasons differently, phrases differently,
// and is worse at the thing the app exists to do. Reducing reasoning effort on
// the SAME model is the better trade — the parent keeps the model they chose
// and gets a faster, shallower answer from it, rather than a different tutor.
//
// Per provider, because the useful floor differs:
//   - Codex CLI exposes real low/medium/high effort; "low" is a genuine,
//     large latency win.
//   - Claude Code maps effort onto thinking budget; "low" likewise.
//   - Cursor CLI's honoring of reasoning_effort is the least established of
//     the three, so it drops one step to "medium" rather than assuming a floor
//     that may not be respected.
//
// A provider that ignores reasoning_effort entirely makes Fast Mode a no-op
// for that provider rather than doing something surprising — which is the
// right failure, but worth knowing when picking an engine.
func selectedReasoningEffort(fastMode bool, provider llm.Provider) string {
	if !fastMode {
		return defaultReasoningEffort
	}
	if llmproviders.Provider(provider) == llmproviders.ProviderCursorCLI {
		return "medium"
	}
	return "low"
}

// selectedModelID is the single place every call site (chat.go, child.go,
// pulse.go, whatsapp_bot.go) resolves the model, so a change applies uniformly
// rather than being threaded into each one. No longer depends on Fast Mode —
// see selectedReasoningEffort.
func selectedModelID(provider llm.Provider) string {
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

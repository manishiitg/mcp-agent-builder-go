package main

import (
	"encoding/json"
	"net/http"
	"strings"

	llmproviders "github.com/manishiitg/multi-llm-provider-go"
	"github.com/manishiitg/multi-llm-provider-go/pkg/adapters/claudecode"
	"github.com/manishiitg/multi-llm-provider-go/pkg/adapters/codexcli"
	"github.com/manishiitg/multi-llm-provider-go/pkg/adapters/cursorcli"
)

// Model choice per coding agent.
//
// The catalog is read from the provider adapters rather than listed here, so
// this cannot drift from what the agent actually accepts. That matters more
// than it sounds: the shorthand this feature was requested with ("haiku 4.6",
// "gpt4.5") did not match reality — the real ids are claude-haiku-4-5-...
// and grok-4.5 — and a hardcoded list would have shipped both mistakes.

type modelOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// availableModelsFor returns the models a provider actually accepts. Only the
// three coding agents this app supports are listed; anything else returns nil
// so the UI simply shows no picker rather than an empty or wrong one.
func availableModelsFor(provider llmproviders.Provider) []modelOption {
	var ids []string
	switch provider {
	case llmproviders.ProviderCodexCLI:
		for _, m := range codexcli.GetAllCodexCLIModels() {
			ids = append(ids, m.ModelID)
		}
	case llmproviders.ProviderClaudeCode:
		for _, m := range claudecode.GetAllClaudeCodeModels() {
			ids = append(ids, m.ModelID)
		}
	case llmproviders.ProviderCursorCLI:
		for _, m := range cursorcli.GetAllCursorCLIModels() {
			ids = append(ids, m.ModelID)
		}
	default:
		return nil
	}
	out := make([]modelOption, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out = append(out, modelOption{ID: id, Label: id})
	}
	return out
}

type modelsResponse struct {
	Provider string        `json:"provider"`
	Selected string        `json:"selected"`
	Default  string        `json:"default"`
	Models   []modelOption `json:"models"`
}

// GET /api/models — what this family can pick from for the engine it is
// currently using, plus what is in force right now.
func handleGetModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()

	provider, ok := engineToProvider(s.Engine)
	if !ok {
		// No engine chosen yet — report an empty catalog rather than guessing.
		writeJSON(w, http.StatusOK, modelsResponse{})
		return
	}
	writeJSON(w, http.StatusOK, modelsResponse{
		Provider: string(provider),
		// Reported separately from Default so the UI can show which one is
		// actually running without pretending the family chose it.
		Selected: strings.TrimSpace(s.SelectedModels[string(provider)]),
		Default:  mediumTierModelID(provider),
		Models:   availableModelsFor(llmproviders.Provider(provider)),
	})
}

type setModelRequest struct {
	// Empty clears the choice and returns to this app's tuned default.
	ModelID string `json:"model_id"`
}

// POST /api/models — choose a model for the engine currently in use.
func handleSetModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req setModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	stateMu.Lock()
	s := loadState()
	provider, ok := engineToProvider(s.Engine)
	if !ok {
		stateMu.Unlock()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no coding agent selected yet"})
		return
	}
	chosen := strings.TrimSpace(req.ModelID)

	// Validate on the way in. An id that the agent does not accept would
	// otherwise fail on every turn, long after the setting was changed and with
	// nothing connecting the two.
	if chosen != "" {
		valid := false
		for _, m := range availableModelsFor(llmproviders.Provider(provider)) {
			if m.ID == chosen {
				valid = true
				break
			}
		}
		if !valid {
			stateMu.Unlock()
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "that model isn't offered by " + string(provider),
			})
			return
		}
	}

	if s.SelectedModels == nil {
		s.SelectedModels = map[string]string{}
	}
	if chosen == "" {
		delete(s.SelectedModels, string(provider))
	} else {
		s.SelectedModels[string(provider)] = chosen
	}
	err := saveState(s)
	stateMu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"selected": chosen})
}

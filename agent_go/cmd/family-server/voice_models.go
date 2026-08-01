package main

import (
	"encoding/json"
	"net/http"
	"sync"
)

// modelInstallState is live progress for one background install, polled by
// the settings UI. A multi-hundred-MB-to-multi-GB download with no visible
// progress reads as a hang, so bytes/total are tracked as it runs rather than
// only reported at the end.
type modelInstallState struct {
	Installing bool   `json:"installing"`
	GotBytes   int64  `json:"got_bytes"`
	TotalBytes int64  `json:"total_bytes"`
	Error      string `json:"error,omitempty"`
}

var (
	modelInstallMu     sync.Mutex
	modelInstallStates = map[string]*modelInstallState{}
)

func installStateFor(id string) modelInstallState {
	modelInstallMu.Lock()
	defer modelInstallMu.Unlock()
	if s, ok := modelInstallStates[id]; ok {
		return *s
	}
	return modelInstallState{}
}

// POST /api/voice/model/install {"id":"parakeet"} — starts a background
// install and returns immediately; the settings UI polls /api/voice/status
// for progress.
func handleVoiceModelInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.ID != "parakeet" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown model"})
		return
	}
	installMlxVoiceEnv()
	writeJSON(w, http.StatusOK, map[string]string{"status": "installing"})
}

// POST /api/voice/model/remove {"id":"parakeet"} — deletes the MLX voice
// environment.
func handleVoiceModelRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.ID != "parakeet" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown model"})
		return
	}
	if err := removeMlxVoiceEnv(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

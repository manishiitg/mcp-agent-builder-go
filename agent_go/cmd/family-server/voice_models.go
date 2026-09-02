package main

import (
	"encoding/json"
	"net/http"
)

func readTierRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return false
	}
	if req.ID != voiceTierID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown model"})
		return false
	}
	return true
}

// POST /api/voice/model/install {"id":"standard"} — starts the shared model
// download (and load) in the background and returns immediately; the
// settings UI polls /api/voice/status for progress.
func handleVoiceModelInstall(w http.ResponseWriter, r *http.Request) {
	if !readTierRequest(w, r) {
		return
	}
	if !familyVoice.Status().Available {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "This build was made without the speech engine"})
		return
	}
	familyVoice.Warm()
	writeJSON(w, http.StatusOK, map[string]string{"status": "installing"})
}

// POST /api/voice/model/remove {"id":"standard"} — unloads the engine and
// deletes the model files. Note the directory is shared with every other
// AgentWorks app on this machine, so they would re-download on next use.
func handleVoiceModelRemove(w http.ResponseWriter, r *http.Request) {
	if !readTierRequest(w, r) {
		return
	}
	if err := familyVoice.Remove(); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// voiceTranscriptionStatus is what the WhatsApp settings UI needs to render
// the voice-transcription toggle: whether it's on, whether the speech model is
// actually on disk, whether a download is in flight, and the last error if
// any.
type voiceTranscriptionStatus struct {
	Enabled     bool   `json:"enabled"`
	Installed   bool   `json:"installed"`
	Installing  bool   `json:"installing"`
	ModelSizeMB int    `json:"model_size_mb"`
	Available   bool   `json:"available"`
	Error       string `json:"error,omitempty"`
}

// voiceModelInstalled reports whether the shared speech model is on disk.
func voiceModelInstalled() bool {
	return familyVoice.Status().Installed
}

// whatsAppVoiceEnabled resolves the parent's effective choice — see the
// WhatsAppVoiceEnabled field doc comment in state.go for the nil-vs-false
// distinction this depends on.
func whatsAppVoiceEnabled(s familyState) bool {
	if s.WhatsAppVoiceEnabled != nil {
		return *s.WhatsAppVoiceEnabled
	}
	return voiceModelInstalled()
}

func currentVoiceTranscriptionStatus(s familyState) voiceTranscriptionStatus {
	st := familyVoice.Status()
	return voiceTranscriptionStatus{
		Enabled:     whatsAppVoiceEnabled(s),
		Installed:   st.Installed,
		Installing:  st.Downloading,
		ModelSizeMB: st.SizeMB,
		Available:   st.Available,
		Error:       st.Error,
	}
}

// handleWhatsAppVoiceToggle turns on-device WhatsApp voice-note transcription
// on or off. Turning it on starts the shared model download if it isn't on
// disk yet and returns immediately; the frontend polls for progress. Turning
// it off does NOT delete anything — that's a deliberate, separate action via
// the "Remove" button on the tier's own Settings card, not a side effect of
// this toggle.
func handleWhatsAppVoiceToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.Enabled && !familyVoice.Status().Available {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Voice transcription isn't included in this build"})
		return
	}

	stateMu.Lock()
	s := loadState()
	enabled := req.Enabled
	s.WhatsAppVoiceEnabled = &enabled
	err := saveState(s)
	stateMu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if enabled {
		familyVoice.Warm()
	} else {
		log.Printf("[voice] WhatsApp voice transcription disabled (engine left installed — remove it separately from Settings if you want it gone)")
	}

	stateMu.Lock()
	s = loadState()
	stateMu.Unlock()
	writeJSON(w, http.StatusOK, currentVoiceTranscriptionStatus(s))
}

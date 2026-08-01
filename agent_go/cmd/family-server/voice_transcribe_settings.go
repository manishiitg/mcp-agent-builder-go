package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// voiceTranscriptionStatus is what the WhatsApp settings UI needs to render
// the voice-transcription toggle: whether it's on, whether Parakeet is
// actually installed, whether an install is in flight, and the last install
// error if any.
type voiceTranscriptionStatus struct {
	Enabled     bool   `json:"enabled"`
	Installed   bool   `json:"installed"`
	Installing  bool   `json:"installing"`
	ModelSizeMB int    `json:"model_size_mb"`
	Available   bool   `json:"available"`
	Error       string `json:"error,omitempty"`
}

// voiceModelInstalled reports whether Parakeet (the shared MLX voice
// environment, see voice_mlx_env.go) is ready to transcribe.
func voiceModelInstalled() bool {
	return mlxVoiceInstalled()
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
	st := installStateFor(mlxVoiceInstallID)
	return voiceTranscriptionStatus{
		Enabled:     whatsAppVoiceEnabled(s),
		Installed:   voiceModelInstalled(),
		Installing:  st.Installing,
		ModelSizeMB: mlxVoiceTotalSizeMB,
		Available:   detectVoiceHardware().IsAppleSilicon,
		Error:       st.Error,
	}
}

// handleWhatsAppVoiceToggle turns on-device WhatsApp voice-note transcription
// on or off. Turning it on kicks off the MLX voice environment install if it
// isn't already there (Parakeet — Apple Silicon only) and returns
// immediately; the frontend polls for progress. Turning it off does NOT
// delete anything — that's a deliberate, separate action via the "Remove"
// button on the tier's own Settings card, not a side effect of this toggle.
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
	if req.Enabled && !detectVoiceHardware().IsAppleSilicon {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Voice transcription needs a newer Mac (2020 or later)"})
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
		installMlxVoiceEnv()
	} else {
		log.Printf("[voice] WhatsApp voice transcription disabled (engine left installed — remove it separately from Settings if you want it gone)")
	}

	stateMu.Lock()
	s = loadState()
	stateMu.Unlock()
	writeJSON(w, http.StatusOK, currentVoiceTranscriptionStatus(s))
}

// lastLines trims a subprocess's (often noisy) output down to a short tail
// for a readable error message/log line.
func lastLines(s string, maxChars int) string {
	if len(s) > maxChars {
		return s[len(s)-maxChars:]
	}
	return s
}

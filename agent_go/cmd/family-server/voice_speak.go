package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// defaultTTSVoice is macOS's own natural-sounding default voice — the
// zero-setup TTS tier needs no download and no config to sound reasonable out
// of the box. The picker (once built) lets a parent switch this, e.g. to
// "Aman" (en_IN) for an Indian-English accent that may read more naturally to
// a CBSE/ICSE family than a US voice.
const defaultTTSVoice = "Samantha"

// speakMaxChars bounds one request — this is for a chat reply, not reading a
// whole file aloud; `say` itself has no hard limit, but a runaway request
// (or an accidental whole-page paste) shouldn't tie up a synthesis call for
// minutes.
const speakMaxChars = 4000

type voiceSpeakRequest struct {
	Text  string `json:"text"`
	Voice string `json:"voice,omitempty"`
}

// POST /api/voice/speak — synthesizes text to speech entirely on-device via
// macOS's built-in `say` (AVSpeechSynthesizer under the hood): no download,
// no API key, works offline, available on every Mac. This is the zero-setup
// "Built-in" TTS tier — see skills/_shared voice tier design (in progress).
// Returns audio/wav bytes directly; the frontend plays it in an <audio> tag.
func handleVoiceSpeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req voiceSpeakRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}
	if len(text) > speakMaxChars {
		text = text[:speakMaxChars]
	}
	voice := strings.TrimSpace(req.Voice)
	if voice == "" {
		voice = defaultTTSVoice
	}

	// The more natural voice wins automatically when it's installed — same
	// rule as the speech models: installing an upgrade IS choosing it, with no
	// separate "make it active" step to forget.
	var audio []byte
	var err error
	if piperInstalled() {
		audio, err = speakWithPiper(text)
		if err != nil {
			// Never leave the parent with silence because the optional upgrade
			// broke — fall back to the always-present system voice.
			log.Printf("[voice] piper failed, falling back to the system voice: %v", err)
			audio, err = synthesizeSpeech(text, voice)
		}
	} else {
		audio, err = synthesizeSpeech(text, voice)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-store") // per-reply text, never worth caching
	_, _ = w.Write(audio)
}

// synthesizeSpeech shells out to `say`, writing to a temp WAV file (say has no
// "write to stdout" mode) and reading it back. WAVE/LEI16 output, not the
// AIFF `say` defaults to, because AIFF playback in <audio> is unreliable on
// Chromium (this app's own webview) even though Safari handles it fine — WAV
// is universally supported.
func synthesizeSpeech(text, voice string) ([]byte, error) {
	if _, err := exec.LookPath("say"); err != nil {
		return nil, fmt.Errorf("'say' is not available on this system (macOS only)")
	}
	tmpFile, err := os.CreateTemp("", "sq-tts-*.wav")
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "say",
		"-v", voice,
		"-o", tmpPath,
		"--file-format=WAVE",
		"--data-format=LEI16@22050",
		text,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("say failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return os.ReadFile(tmpPath)
}

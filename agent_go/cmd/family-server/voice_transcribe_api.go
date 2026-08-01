package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// maxVoiceClipBytes bounds one mic recording. Generous for speech (a few
// minutes of webm/opus) while still refusing an accidental large upload to
// this endpoint.
const maxVoiceClipBytes = 25 << 20 // 25 MB

// POST /api/voice/transcribe — multipart form with an "audio" file part.
// Transcribes a mic recording from the app's own composer entirely on-device,
// reusing the SAME Parakeet pipeline that already handles WhatsApp voice
// notes (transcribeAudioFile in voice_transcribe.go): identical engine,
// identical model, just a different entry point. Parakeet reads whatever
// container MediaRecorder produced (webm/opus in Chromium, mp4 in Safari)
// directly, so no format conversion is needed on either side.
func handleVoiceTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(maxVoiceClipBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read the recording"})
		return
	}
	file, hdr, err := r.FormFile("audio")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no audio in the request"})
		return
	}
	defer file.Close()

	// Keep the browser's own extension so ffmpeg can sniff the container it
	// actually got, rather than us guessing wrong and failing the convert.
	ext := filepath.Ext(hdr.Filename)
	if ext == "" {
		ext = ".webm"
	}
	tmp, err := os.CreateTemp("", "sq-mic-*"+ext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save the recording"})
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, io.LimitReader(file, maxVoiceClipBytes)); err != nil {
		tmp.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save the recording"})
		return
	}
	if err := tmp.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save the recording"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	text, err := transcribeAudioFile(ctx, tmpPath)
	if err != nil {
		// Surfaced verbatim rather than swallowed: the likely cause is "not
		// installed yet", which the UI can only tell the parent about if it's
		// actually told.
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprint(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}

// POST /api/voice/warm — fire-and-forget: starts loading Parakeet into the
// persistent worker's memory if it isn't warm already. The frontend calls
// this the MOMENT recording starts, not after it ends, so a cold worker
// (unloaded after voiceWorkerIdleTimeout — see voice_worker.go — 15 minutes
// of no voice use) pays its load cost while the parent is still talking
// instead of on their first live-preview tick, where it previously showed up
// as several seconds of dead air with no captions and no visible reason why.
func handleVoiceWarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if sharedVoiceWorker.IsWarm(parakeetModel) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "warm"})
		return
	}
	go func() {
		if err := warmParakeet(context.Background()); err != nil {
			log.Printf("[voice] on-demand warm-up failed: %v", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "warming"})
}

package main

import (
	"context"
	"fmt"
	"io"
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
// reusing the SAME whisper.cpp pipeline that already handles WhatsApp voice
// notes (transcribeAudioFile in voice_transcribe.go): identical engine,
// identical model, just a different entry point. ffmpeg normalizes whatever
// container MediaRecorder produced (webm/opus in Chromium, mp4 in Safari)
// into the 16kHz mono WAV whisper.cpp needs, so no format negotiation is
// needed on the frontend.
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
		// Surfaced verbatim rather than swallowed: the likely causes here are
		// "model not downloaded yet" / "whisper-cli not installed", which the
		// UI can only tell the parent about if it's actually told.
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprint(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}

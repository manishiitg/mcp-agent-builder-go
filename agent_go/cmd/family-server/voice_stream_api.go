package main

import (
	"encoding/base64"
	"io"
	"log"
	"net/http"
)

// Incremental dictation endpoints, backed by the native helper
// (see docs/refactor/native_streaming_stt.md).
//
// Deliberately three small POSTs rather than a WebSocket. The audio only ever
// travels over loopback to a server on the same machine, at ~6 chunks/second;
// a socket would buy nothing here and would add a framing protocol, a new
// dependency, and its own backpressure failure modes. Each request also stays
// independently debuggable with curl, which the existing voice endpoints are.
//
// Flow:
//	POST /api/voice/stream/start            -> {"ok":true}
//	POST /api/voice/stream/chunk  (raw PCM) -> {"partial":"..."}    (repeat)
//	POST /api/voice/stream/finish           -> {"text":"..."}
//
// Chunk bodies are raw little-endian Float32 samples, 16kHz mono — the format
// FluidAudio wants, and what an AudioWorklet produces natively. No container,
// so unlike the MediaRecorder path each chunk stands alone and only the NEW
// audio is ever sent.

// maxVoiceChunkBytes caps a single chunk. 160ms of 16kHz mono float32 is
// ~10KB; this leaves generous headroom for a slow client batching several
// together while still refusing anything absurd.
const maxVoiceChunkBytes = 4 << 20

func handleVoiceStreamStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !nativeVoiceAvailable() {
		http.Error(w, "native voice helper not available", http.StatusServiceUnavailable)
		return
	}
	// load is idempotent in the helper and cheap once weights are cached; the
	// first call downloads them, which is why this worker carries a long call
	// timeout.
	if _, err := sharedNativeVoiceWorker.call(r.Context(), map[string]any{"cmd": "load"}); err != nil {
		log.Printf("[voice-native] load failed: %v", err)
		http.Error(w, "could not start the voice engine", http.StatusInternalServerError)
		return
	}
	if _, err := sharedNativeVoiceWorker.call(r.Context(), map[string]any{"cmd": "start"}); err != nil {
		log.Printf("[voice-native] start failed: %v", err)
		http.Error(w, "could not start dictation", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleVoiceStreamChunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pcm, err := io.ReadAll(io.LimitReader(r.Body, maxVoiceChunkBytes))
	if err != nil {
		http.Error(w, "could not read audio", http.StatusBadRequest)
		return
	}
	resp, err := sharedNativeVoiceWorker.call(r.Context(), map[string]any{
		"cmd": "audio",
		"pcm": base64.StdEncoding.EncodeToString(pcm),
	})
	if err != nil {
		// A dropped preview tick is not worth surfacing to the speaker — the
		// next one carries the same audio's transcript anyway, since the
		// helper keeps the utterance. Mirrors the frontend's existing
		// treatment of a failed preview refresh.
		log.Printf("[voice-native] chunk failed: %v", err)
		writeJSON(w, http.StatusOK, map[string]any{"partial": ""})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"partial": resp["partial"]})
}

func handleVoiceStreamFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, err := sharedNativeVoiceWorker.call(r.Context(), map[string]any{"cmd": "finish"})
	if err != nil {
		log.Printf("[voice-native] finish failed: %v", err)
		http.Error(w, "could not finish transcribing", http.StatusInternalServerError)
		return
	}
	// "text" is the batch pass — punctuated and capitalised. "streamed" is the
	// raw streaming transcript, returned too so the client can fall back to it
	// if the batch stage ever comes back empty.
	writeJSON(w, http.StatusOK, map[string]any{"text": resp["text"], "streamed": resp["streamed"]})
}

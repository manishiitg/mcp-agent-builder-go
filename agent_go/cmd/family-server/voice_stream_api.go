package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

// Per-chunk diagnostics: one line per 160ms of speech. Invaluable while
// bringing this path up — it is what identified the frozen-preview bug — and
// far too noisy now that it works, so opt IN with SPARKQUILL_VOICE_DEBUG=1.
func voiceStreamDebug() bool { return os.Getenv("SPARKQUILL_VOICE_DEBUG") == "1" }

var voiceChunkSeq atomic.Int64

// One dictation at a time. The helper keeps a single utterance buffer, so two
// overlapping sessions interleave their audio into one transcript — observed
// live on 2026-08-04, when a cold-start model load left the mic button looking
// idle, the parent clicked repeatedly, and six concurrent capture sessions
// produced 541 chunks for 15 seconds of speech and a garbage transcript. The
// client now blocks that (see MicState 'preparing'), but the invariant belongs
// here too: this is the only place that can actually enforce it, and a second
// client, a stale tab, or a retry would otherwise reintroduce it.
var voiceStreamActive atomic.Bool

// pcmStats reports loudness of a raw little-endian Float32 buffer, plus the
// sample count — enough to tell silence from real speech, and to catch a
// client sending the wrong sample rate or a truncated chunk.
func pcmStats(b []byte) (rms, peak float64, samples int) {
	samples = len(b) / 4
	if samples == 0 {
		return 0, 0, 0
	}
	var sum float64
	for i := 0; i+4 <= len(b); i += 4 {
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(b[i:])))
		sum += v * v
		if a := math.Abs(v); a > peak {
			peak = a
		}
	}
	return math.Sqrt(sum / float64(samples)), peak, samples
}

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
	// Claim the session BEFORE loading. The cold-start load is the long wait,
	// and therefore precisely the window in which repeat clicks arrive — a
	// guard taken after it would be useless for the case it exists to stop.
	if !voiceStreamActive.CompareAndSwap(false, true) {
		log.Printf("[voice-native] rejecting overlapping stream start")
		http.Error(w, "a dictation session is already in progress", http.StatusConflict)
		return
	}
	// load is idempotent in the helper and cheap once weights are cached; the
	// first call downloads them, which is why this worker carries a long call
	// timeout.
	if _, err := sharedNativeVoiceWorker.call(r.Context(), map[string]any{"cmd": "load"}); err != nil {
		voiceStreamActive.Store(false)
		log.Printf("[voice-native] load failed: %v", err)
		http.Error(w, "could not start the voice engine", http.StatusInternalServerError)
		return
	}
	if _, err := sharedNativeVoiceWorker.call(r.Context(), map[string]any{"cmd": "start"}); err != nil {
		voiceStreamActive.Store(false)
		log.Printf("[voice-native] start failed: %v", err)
		http.Error(w, "could not start dictation", http.StatusInternalServerError)
		return
	}
	voiceChunkSeq.Store(0)
	log.Printf("[voice-native] stream start (helper=%s)", nativeVoiceHelperPath())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// The model costs 15-20s to load and ~600MB resident, so its lifetime is tied
// to whether the app is actually on screen rather than to a timer. The desktop
// shell calls these when the window is hidden to the menu bar and shown again
// (see desktop-sparkquill/main.js) — a timer alone either wasted memory while
// the app sat in the background, or made the mic slow after a gap.
func handleVoiceNativeWarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !nativeVoiceAvailable() {
		writeJSON(w, http.StatusOK, map[string]any{"warm": false})
		return
	}
	// Returns immediately: the caller is a window event, and nothing should
	// wait on a 20s load.
	go func() {
		started := time.Now()
		if err := warmNativeVoice(context.Background()); err != nil {
			log.Printf("[voice-native] warm on foreground failed: %v", err)
			return
		}
		log.Printf("[voice-native] warm and ready in %s", time.Since(started).Round(time.Millisecond))
	}()
	writeJSON(w, http.StatusOK, map[string]any{"warming": true})
}

func handleVoiceNativeUnload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Never mid-dictation: that would kill the utterance the user is speaking.
	if voiceStreamActive.Load() {
		writeJSON(w, http.StatusOK, map[string]any{"unloaded": false, "reason": "dictation in progress"})
		return
	}
	sharedNativeVoiceWorker.Stop()
	writeJSON(w, http.StatusOK, map[string]any{"unloaded": true})
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
	started := time.Now()
	seq := voiceChunkSeq.Add(1)
	resp, err := sharedNativeVoiceWorker.call(r.Context(), map[string]any{
		"cmd": "audio",
		"pcm": base64.StdEncoding.EncodeToString(pcm),
	})
	// Diagnostic line per chunk. rms/peak are computed from the bytes actually
	// received, which separates the failure modes that otherwise look alike
	// from the UI: silence or a wrong sample rate (rms≈0, or a sample count
	// that isn't 2560) versus real audio the model simply transcribes poorly
	// (healthy rms, empty or wrong partial).
	if voiceStreamDebug() {
		rms, peak, n := pcmStats(pcm)
		partial, _ := resp["partial"].(string)
		log.Printf("[voice-native] chunk#%d samples=%d rms=%.4f peak=%.4f took=%s partial=%q err=%v",
			seq, n, rms, peak, time.Since(started).Round(time.Millisecond), partial, err)
	}
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
	// Released even when the transcription below fails: a stuck flag would
	// wedge dictation for the rest of the process's life.
	defer voiceStreamActive.Store(false)
	resp, err := sharedNativeVoiceWorker.call(r.Context(), map[string]any{"cmd": "finish"})
	if err != nil {
		log.Printf("[voice-native] finish failed: %v", err)
		http.Error(w, "could not finish transcribing", http.StatusInternalServerError)
		return
	}
	streamed, _ := resp["streamed"].(string)
	final, _ := resp["text"].(string)
	log.Printf("[voice-native] finish after %d chunks\n  streamed=%q\n  final=%q",
		voiceChunkSeq.Load(), streamed, final)
	// "text" is the batch pass — punctuated and capitalised. "streamed" is the
	// raw streaming transcript, returned too so the client can fall back to it
	// if the batch stage ever comes back empty.
	writeJSON(w, http.StatusOK, map[string]any{"text": resp["text"], "streamed": resp["streamed"]})
}

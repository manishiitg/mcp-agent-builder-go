package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// Exercises the real path — handler -> voiceWorker -> Swift helper process —
// rather than a mock, because the things most likely to break are exactly the
// seams a mock would paper over: locating the binary, the readiness handshake,
// and the JSON line protocol agreeing on both sides.
//
// Opt-in: the helper's first run downloads CoreML weights (~96s), which has no
// place in a default test run. Enable with:
//
//	SPARKQUILL_VOICE_STREAM_TEST=1 go test ./cmd/family-server/ -run VoiceStream
//
// Asserts plumbing, not transcript content: driving it with synthetic audio
// would only prove what the standalone check in
// docs/refactor/native_streaming_stt.md already established against real
// speech.
func TestVoiceStreamRoundTrip(t *testing.T) {
	if os.Getenv("SPARKQUILL_VOICE_STREAM_TEST") != "1" {
		t.Skip("set SPARKQUILL_VOICE_STREAM_TEST=1 to run (downloads models on first use)")
	}
	if !nativeVoiceAvailable() {
		t.Skip("native voice helper not built; run swift build -c release in desktop-sparkquill/voice-helper")
	}

	post := func(t *testing.T, h http.HandlerFunc, body []byte) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("bad JSON %q: %v", rec.Body.String(), err)
		}
		return out
	}

	if got := post(t, handleVoiceStreamStart, nil); got["ok"] != true {
		t.Fatalf("start did not report ok: %v", got)
	}

	// A quiet 160ms chunk, matching what the AudioWorklet sends: little-endian
	// float32, 16kHz mono. Real recordings open on near-silence anyway — and a
	// clip that starts at full volume on sample 0 is exactly what produced a
	// spurious dropped-word finding earlier (see the refactor doc).
	const samples = 16000 * 160 / 1000
	chunk := new(bytes.Buffer)
	for i := 0; i < samples; i++ {
		_ = binary.Write(chunk, binary.LittleEndian, float32(0))
	}
	for i := 0; i < 5; i++ {
		got := post(t, handleVoiceStreamChunk, chunk.Bytes())
		if _, ok := got["partial"]; !ok {
			t.Fatalf("chunk %d returned no partial field: %v", i, got)
		}
	}

	got := post(t, handleVoiceStreamFinish, nil)
	if _, ok := got["text"]; !ok {
		t.Fatalf("finish returned no text field: %v", got)
	}
	if _, ok := got["text"].(string); !ok {
		t.Fatalf("finish text was not a string: %T", got["text"])
	}
}

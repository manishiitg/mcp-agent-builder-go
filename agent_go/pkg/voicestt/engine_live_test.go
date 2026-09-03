package voicestt

import (
	"os"
	"strings"
	"testing"
)

// TestEngineTranscribesRealAudioLive is a live integration test against a real
// downloaded model and a real WAV file — set VOICESTT_TEST_MODEL_DIR to run it.
// This is deliberately opt-in like the mcpagent RUN_MCPAGENT_REAL_BRIDGE_E2E
// tests: it needs a ~630MB model on disk and is not something CI should fetch
// on every run.
func TestEngineTranscribesRealAudioLive(t *testing.T) {
	modelDir := os.Getenv("VOICESTT_TEST_MODEL_DIR")
	wavPath := os.Getenv("VOICESTT_TEST_WAV")
	if modelDir == "" || wavPath == "" {
		t.Skip("set VOICESTT_TEST_MODEL_DIR and VOICESTT_TEST_WAV to run this live test")
	}

	engine, err := NewEngine(modelDir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	samples, sr, err := ReadWAV(wavPath)
	if err != nil {
		t.Fatalf("read wav: %v", err)
	}
	if sr != SampleRate {
		t.Fatalf("test wav is %dHz, want %d (this test does not resample)", sr, SampleRate)
	}

	stream := engine.NewStream()
	defer stream.Close()

	// Feed in small chunks, exactly like a live mic would deliver audio, and
	// require that TEXT ACTUALLY GROWS as more audio arrives -- the property
	// that makes this "streaming" rather than "decode the whole clip and
	// pretend it arrived incrementally". A test that only checks the final
	// transcript cannot tell those apart.
	chunk := SampleRate / 10
	var sawGrowth bool
	var lastLen int
	for i := 0; i < len(samples); i += chunk {
		end := i + chunk
		if end > len(samples) {
			end = len(samples)
		}
		res := stream.AcceptWaveform(samples[i:end])
		if len(res.Text) > lastLen {
			sawGrowth = true
			lastLen = len(res.Text)
		}
	}
	final := stream.Finish()

	if !sawGrowth {
		t.Fatal("transcript never grew across chunks — this decoded as one batch, not a stream")
	}
	expect := os.Getenv("VOICESTT_TEST_EXPECT")
	if expect == "" {
		expect = "yellow lamps"
	}
	lower := strings.ToLower(final.Text)
	if !strings.Contains(lower, strings.ToLower(expect)) {
		t.Fatalf("final transcript did not contain %q: %q", expect, final.Text)
	}
	t.Logf("final transcript: %q", final.Text)
	t.Logf("punctuated: %q", engine.Punctuate(final.Text))
}

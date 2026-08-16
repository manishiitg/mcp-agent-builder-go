package voicestt

import (
	"encoding/binary"
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

	samples, sr, err := readWav16Mono(wavPath)
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
	lower := strings.ToLower(final.Text)
	if !strings.Contains(lower, "yellow lamps") {
		t.Fatalf("final transcript did not contain expected words from the known test clip: %q", final.Text)
	}
	t.Logf("final transcript: %q", final.Text)
}

func readWav16Mono(path string) (samples []float32, sampleRate int, err error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: test-only, path comes from an env var the test author controls.
	if err != nil {
		return nil, 0, err
	}
	sampleRate = int(binary.LittleEndian.Uint32(b[24:28]))
	off := 12
	var dataOff, dataLen int
	for off+8 <= len(b) {
		id := string(b[off : off+4])
		sz := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		if id == "data" {
			dataOff, dataLen = off+8, sz
			break
		}
		off += 8 + sz + sz%2
	}
	n := dataLen / 2
	samples = make([]float32, n)
	for i := 0; i < n; i++ {
		v := int16(binary.LittleEndian.Uint16(b[dataOff+i*2 : dataOff+i*2+2]))
		samples[i] = float32(v) / 32768.0
	}
	return samples, sampleRate, nil
}

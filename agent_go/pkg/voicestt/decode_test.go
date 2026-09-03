package voicestt

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// writeWAV writes 16-bit PCM with the given channels/rate; a 440Hz tone.
func writeWAV(t *testing.T, path string, channels, rate, frames int) {
	t.Helper()
	dataLen := frames * channels * 2
	b := make([]byte, 44+dataLen)
	copy(b[0:], "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+dataLen))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], uint16(channels))
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*channels*2))
	binary.LittleEndian.PutUint16(b[32:], uint16(channels*2))
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(dataLen))
	for i := 0; i < frames; i++ {
		v := int16(math.Sin(2*math.Pi*440*float64(i)/float64(rate)) * 16000)
		for c := 0; c < channels; c++ {
			binary.LittleEndian.PutUint16(b[44+(i*channels+c)*2:], uint16(v))
		}
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadWAVDownmixesToMono(t *testing.T) {
	dir := t.TempDir()
	stereo := filepath.Join(dir, "stereo.wav")
	writeWAV(t, stereo, 2, SampleRate, 1600)
	samples, rate, err := ReadWAV(stereo)
	if err != nil {
		t.Fatal(err)
	}
	if rate != SampleRate || len(samples) != 1600 {
		t.Fatalf("rate=%d samples=%d", rate, len(samples))
	}
	if RMS(samples) < 0.3 {
		t.Fatalf("tone should be loud, rms=%.3f", RMS(samples))
	}
}

// A 16kHz WAV needs no converter at all; that path must work on any OS.
func TestDecodeFileParsesNativeWAVDirectly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mono16k.wav")
	writeWAV(t, path, 1, SampleRate, 3200)
	samples, err := DecodeFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 3200 {
		t.Fatalf("got %d samples", len(samples))
	}
}

func TestDecodePCM16RoundTrip(t *testing.T) {
	b := make([]byte, 4)
	half, floor := int16(16384), int16(-32768)
	binary.LittleEndian.PutUint16(b[0:], uint16(half))  //nolint:gosec // G115: intentional PCM16 bit reinterpretation.
	binary.LittleEndian.PutUint16(b[2:], uint16(floor)) //nolint:gosec // G115: intentional PCM16 bit reinterpretation.
	s := DecodePCM16(b)
	if len(s) != 2 || math.Abs(float64(s[0])-0.5) > 1e-6 || s[1] != -1 {
		t.Fatalf("got %v", s)
	}
}

func TestModelDirIsDeterministic(t *testing.T) {
	// The map-ordered hash this replaced produced a different directory on
	// nearly every server start, and four identical 631MB copies on one dev
	// machine. Same URLs must always mean the same directory.
	if DefaultModelDir() != DefaultModelDir() || modelDirFingerprint(fingerprintSources()) != modelDirFingerprint(fingerprintSources()) {
		t.Fatal("DefaultModelDir must be stable across calls")
	}
	if filepath.Base(DefaultModelDir()) != "nemotron-streaming-"+modelDirFingerprint(fingerprintSources()) {
		t.Fatalf("unexpected dir %s", DefaultModelDir())
	}
}

func TestManagerStatusBeforeAnyLoad(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "model"))
	st := m.Status()
	if st.Available != Available || st.Installed || st.Ready || st.Downloading || st.Loading || st.ActiveStreams != 0 {
		t.Fatalf("unexpected fresh status %+v", st)
	}
	if !m.Unload() {
		t.Fatal("unloading a never-loaded manager should be a no-op success")
	}
}

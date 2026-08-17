// Package voicestt is the shared AgentWorks streaming speech-to-text
// capability (see agentprofiles.RuntimeCapabilities.Voice). A product opts in
// by declaring the capability in its product.yaml; it never bundles its own
// STT engine, model, or websocket handling.
//
// Engine wraps sherpa-onnx-go (github.com/k2-fsa/sherpa-onnx-go), running
// NVIDIA's Nemotron cache-aware streaming ASR model. Verified live (2026-08-16):
// 88ms to first partial, 17.3x realtime decode on CPU, word-for-word accurate
// against ground truth. sherpa-onnx-go ships prebuilt native libraries as
// platform-specific Go modules (sherpa-onnx-go-{linux,macos,windows}) — no
// local C/C++ toolchain or manual linking required.
package voicestt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// ModelFiles names the three ONNX sessions a transducer streaming model
// decomposes into (encoder / decoder / joiner) plus its token vocabulary.
// This is the shape sherpa-onnx's OnlineTransducerModelConfig expects, and the
// shape the Nemotron export at csukuangfj/sherpa-onnx-nemotron-speech-streaming-en-0.6b-int8
// ships in.
type ModelFiles struct {
	Encoder string
	Decoder string
	Joiner  string
	Tokens  string
}

// DefaultModelURLs is the maintainer's own (k2-fsa/sherpa-onnx author)
// int8-quantized export of NVIDIA's Nemotron-3.5 streaming ASR model.
// Chosen over other community re-exports specifically because sherpa-onnx's
// own author publishes it, and because int8 keeps download + memory bounded
// (~630MB total) without giving up the accuracy verified in the spike.
var DefaultModelURLs = map[string]string{
	"encoder.int8.onnx": "https://huggingface.co/csukuangfj/sherpa-onnx-nemotron-speech-streaming-en-0.6b-int8-2026-01-14/resolve/main/encoder.int8.onnx",
	"decoder.int8.onnx": "https://huggingface.co/csukuangfj/sherpa-onnx-nemotron-speech-streaming-en-0.6b-int8-2026-01-14/resolve/main/decoder.int8.onnx",
	"joiner.int8.onnx":  "https://huggingface.co/csukuangfj/sherpa-onnx-nemotron-speech-streaming-en-0.6b-int8-2026-01-14/resolve/main/joiner.int8.onnx",
	"tokens.txt":        "https://huggingface.co/csukuangfj/sherpa-onnx-nemotron-speech-streaming-en-0.6b-int8-2026-01-14/resolve/main/tokens.txt",
}

// SampleRate the model was trained on. AcceptWaveform expects audio already at
// this rate; resampling is the caller's job (browser mic capture is usually
// 48kHz).
const SampleRate = 16000

// Engine holds ONE loaded recognizer shared across every session. Loading is
// the expensive part (encoder.onnx is ~620MB and takes ~1.7s to initialize in
// the spike); a live OnlineStream, by contrast, is cheap per-connection state
// (a ring buffer of feature frames), so one Engine safely serves many
// concurrent sessions each with their own Stream.
type Engine struct {
	mu         sync.RWMutex
	recognizer *sherpa.OnlineRecognizer
	modelDir   string
}

// NewEngine loads the recognizer from modelDir, downloading any missing model
// file from DefaultModelURLs first. Loading blocks the caller (~1-2s); do this
// once at server startup or lazily behind a sync.Once, not per request.
func NewEngine(modelDir string) (*Engine, error) {
	if err := ensureModelFiles(modelDir); err != nil {
		return nil, fmt.Errorf("voicestt: prepare model: %w", err)
	}
	files := ModelFiles{
		Encoder: filepath.Join(modelDir, "encoder.int8.onnx"),
		Decoder: filepath.Join(modelDir, "decoder.int8.onnx"),
		Joiner:  filepath.Join(modelDir, "joiner.int8.onnx"),
		Tokens:  filepath.Join(modelDir, "tokens.txt"),
	}
	config := &sherpa.OnlineRecognizerConfig{
		FeatConfig: sherpa.FeatureConfig{SampleRate: SampleRate, FeatureDim: 80},
		ModelConfig: sherpa.OnlineModelConfig{
			Transducer: sherpa.OnlineTransducerModelConfig{
				Encoder: files.Encoder,
				Decoder: files.Decoder,
				Joiner:  files.Joiner,
			},
			Tokens: files.Tokens,
			// 2 threads matched the spike's 17.3x-realtime measurement; higher
			// thread counts did not meaningfully improve single-stream latency
			// and would multiply badly across concurrent sessions.
			NumThreads: 2,
			Provider:   "cpu",
			// nemo_transducer is the ModelType sherpa-onnx uses for NeMo-family
			// (Parakeet/Nemotron) exports — a plain "transducer" ModelType expects
			// a different token/blank-id convention and silently mis-decodes.
			ModelType: "nemo_transducer",
		},
		DecodingMethod: "greedy_search",
		// Endpoint detection lets a caller know a phrase has settled (silence
		// after speech) without needing its own VAD.
		EnableEndpoint:             1,
		Rule1MinTrailingSilence:    2.4,
		Rule2MinTrailingSilence:    1.2,
		Rule3MinUtteranceLength:    20,
	}
	recognizer := sherpa.NewOnlineRecognizer(config)
	if recognizer == nil {
		return nil, fmt.Errorf("voicestt: sherpa.NewOnlineRecognizer returned nil (check model files under %s)", modelDir)
	}
	return &Engine{recognizer: recognizer, modelDir: modelDir}, nil
}

// Close releases the native recognizer. Safe to call once at server shutdown.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.recognizer != nil {
		sherpa.DeleteOnlineRecognizer(e.recognizer)
		e.recognizer = nil
	}
}

// Result is one decode step's output.
type Result struct {
	Text string
	// EndOfUtterance is true when the recognizer's endpoint rule fires
	// (trailing silence after speech) — the caller's cue to treat Text as
	// settled and reset for the next phrase, mirroring how a push-to-talk UI
	// commits a line.
	EndOfUtterance bool
}

// Stream is one caller's live microphone session against the shared Engine.
// Not safe for concurrent use by multiple goroutines — one stream is one
// sequential audio connection, matching how a single websocket delivers audio
// in order.
type Stream struct {
	engine *Engine
	stream *sherpa.OnlineStream
}

// NewStream opens a fresh streaming session. Cheap: does not reload the model.
func (e *Engine) NewStream() *Stream {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return &Stream{engine: e, stream: sherpa.NewOnlineStream(e.recognizer)}
}

// AcceptWaveform feeds one chunk of mono float32 PCM at voicestt.SampleRate and
// returns the current partial/final transcript. Call this once per received
// audio chunk — sherpa-onnx accumulates internally, so chunk size only affects
// latency (smaller = more frequent partials), never correctness.
func (s *Stream) AcceptWaveform(samples []float32) Result {
	s.engine.mu.RLock()
	defer s.engine.mu.RUnlock()
	s.stream.AcceptWaveform(SampleRate, samples)
	for s.engine.recognizer.IsReady(s.stream) {
		s.engine.recognizer.Decode(s.stream)
	}
	res := s.engine.recognizer.GetResult(s.stream)
	isEndpoint := s.engine.recognizer.IsEndpoint(s.stream)
	if isEndpoint {
		s.engine.recognizer.Reset(s.stream)
	}
	return Result{Text: res.Text, EndOfUtterance: isEndpoint}
}

// Finish signals no more audio is coming and returns the final transcript for
// whatever is still buffered. Call once, at the end of the connection.
func (s *Stream) Finish() Result {
	s.engine.mu.RLock()
	defer s.engine.mu.RUnlock()
	s.stream.InputFinished()
	for s.engine.recognizer.IsReady(s.stream) {
		s.engine.recognizer.Decode(s.stream)
	}
	res := s.engine.recognizer.GetResult(s.stream)
	return Result{Text: res.Text, EndOfUtterance: true}
}

// Close releases this stream's native state. The shared Engine is unaffected.
func (s *Stream) Close() {
	sherpa.DeleteOnlineStream(s.stream)
}

// ensureModelFiles downloads any file in DefaultModelURLs missing from dir,
// verifying each download is non-empty before accepting it (an interrupted
// download silently truncated is worse than a clear "missing" error).
func ensureModelFiles(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, url := range DefaultModelURLs {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			continue
		}
		if err := downloadFile(url, path); err != nil {
			return fmt.Errorf("download %s: %w", name, err)
		}
	}
	return nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Get(url) //nolint:gosec // G107: fixed, hardcoded HuggingFace URLs, not user input.
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	tmp := dest + ".partial"
	f, err := os.Create(tmp) //nolint:gosec // G304: dest is derived from a fixed internal file list, not user input.
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// modelDirFingerprint is a short, filesystem-safe identifier for a model
// directory derived from its source URLs — used so a shared cache path stays
// stable across process restarts without needing a config file to name it.
func modelDirFingerprint(urls map[string]string) string {
	h := sha256.New()
	for name, url := range urls {
		h.Write([]byte(name))
		h.Write([]byte(url))
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// DefaultModelDir returns a stable, user-scoped cache location for the model
// files, keyed by DefaultModelURLs so a change to the pinned model version
// naturally lands in a fresh directory instead of colliding with stale files.
func DefaultModelDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".agentworks", "voice-models", "nemotron-streaming-"+modelDirFingerprint(DefaultModelURLs))
}

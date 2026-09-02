//go:build cgo

// Package voicestt is the ONE AgentWorks speech-to-text engine. The agent
// server (AgentWorks' composer and every product that declares
// agentprofiles.RuntimeCapabilities.Voice in its product.yaml), SparkQuill's
// family-server, and the desktop builds of both all embed this package: the
// same model, the same Manager (download/load/warm/unload), the same
// /api/voice/stream WebSocket (ServeStream), the same file decoding
// (DecodeFile). No product bundles its own STT engine, model, or transport.
//
// It needs a CGO build; engine_nocgo.go is the stub for builds without one,
// and Available tells a UI which it got.
//
// Engine wraps sherpa-onnx-go (github.com/k2-fsa/sherpa-onnx-go), running
// NVIDIA's Nemotron cache-aware streaming ASR model. Verified live (2026-08-16):
// 88ms to first partial, 17.3x realtime decode on CPU, word-for-word accurate
// against ground truth. sherpa-onnx-go ships prebuilt native libraries as
// platform-specific Go modules (sherpa-onnx-go-{linux,macos,windows}) — no
// local C/C++ toolchain or manual linking required.
package voicestt

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

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

// SampleRate the model was trained on. AcceptWaveform expects audio already at
// this rate; resampling is the caller's job (browser mic capture is usually
// 48kHz).
const SampleRate = 16000

// Available reports whether this build carries the native recognizer. The
// non-CGO build (engine_nocgo.go) flips it to false so /api/capabilities can
// hide the mic control up front instead of the first click discovering a 503.
const Available = true

// Engine holds ONE loaded recognizer shared across every session. Loading is
// the expensive part (encoder.onnx is ~620MB and takes ~1.7s to initialize in
// the spike); a live OnlineStream, by contrast, is cheap per-connection state
// (a ring buffer of feature frames), so one Engine safely serves many
// concurrent sessions each with their own Stream.
type Engine struct {
	mu         sync.RWMutex
	recognizer *sherpa.OnlineRecognizer
	punct      *sherpa.OnlinePunctuation
	modelDir   string
}

// NewEngine loads the recognizer from modelDir, downloading any missing model
// file from DefaultModelURLs first. Loading blocks the caller (~1-2s); do this
// once at server startup or lazily behind a sync.Once, not per request.
func NewEngine(modelDir string) (*Engine, error) {
	if err := EnsureModelFiles(modelDir, nil); err != nil {
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
		EnableEndpoint:          1,
		Rule1MinTrailingSilence: 2.4,
		Rule2MinTrailingSilence: 1.2,
		Rule3MinUtteranceLength: 20,
	}
	recognizer := sherpa.NewOnlineRecognizer(config)
	if recognizer == nil {
		return nil, fmt.Errorf("voicestt: sherpa.NewOnlineRecognizer returned nil (check model files under %s)", modelDir)
	}
	e := &Engine{recognizer: recognizer, modelDir: modelDir}
	// The punctuation model is small and optional: a directory that has the
	// speech model but not this (an install interrupted between the two)
	// still dictates, just without punctuation, rather than failing outright.
	if punctuationInstalled(modelDir) {
		e.punct = sherpa.NewOnlinePunctuation(&sherpa.OnlinePunctuationConfig{
			Model: sherpa.OnlinePunctuationModelConfig{
				CnnBilstm:  filepath.Join(modelDir, PunctuationDirName, "model.int8.onnx"),
				BpeVocab:   filepath.Join(modelDir, PunctuationDirName, "bpe.vocab"),
				NumThreads: 1,
				Provider:   "cpu",
			},
		})
	}
	return e, nil
}

// Punctuate adds capitalization and punctuation to a raw transcript. The
// model is English-only, so text with any non-ASCII letters is returned as
// is rather than mangled; empty input stays empty.
//
// Input is lower-cased first: the model does its own casing (names, "I",
// sentence starts) and reads existing capitals as proper nouns — measured
// live, the recognizer's capitalized first word turned "fox" into "Fox".
func (e *Engine) Punctuate(text string) string {
	if text == "" || e.punct == nil || !isASCIIText(text) {
		return text
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.punct == nil {
		return text
	}
	if out := e.punct.AddPunct(strings.ToLower(text)); out != "" {
		return out
	}
	return text
}

// Close releases the native recognizer. Safe to call once at server shutdown.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.recognizer != nil {
		sherpa.DeleteOnlineRecognizer(e.recognizer)
		e.recognizer = nil
	}
	if e.punct != nil {
		sherpa.DeleteOnlinePunctuation(e.punct)
		e.punct = nil
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

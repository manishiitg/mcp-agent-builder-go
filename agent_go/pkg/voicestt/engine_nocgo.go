//go:build !cgo

package voicestt

import (
	"fmt"
	"os"
	"path/filepath"
)

// ModelFiles describes the files used by the native recognizer. It remains
// available in non-CGO builds so callers retain the same package API.
type ModelFiles struct {
	Encoder string
	Decoder string
	Joiner  string
	Tokens  string
}

// DefaultModelURLs documents the pinned native model assets. They are not
// downloaded in a non-CGO build because the recognizer is unavailable.
var DefaultModelURLs = map[string]string{
	"encoder.int8.onnx": "https://huggingface.co/csukuangfj/sherpa-onnx-nemotron-speech-streaming-en-0.6b-int8-2026-01-14/resolve/main/encoder.int8.onnx",
	"decoder.int8.onnx": "https://huggingface.co/csukuangfj/sherpa-onnx-nemotron-speech-streaming-en-0.6b-int8-2026-01-14/resolve/main/decoder.int8.onnx",
	"joiner.int8.onnx":  "https://huggingface.co/csukuangfj/sherpa-onnx-nemotron-speech-streaming-en-0.6b-int8-2026-01-14/resolve/main/joiner.int8.onnx",
	"tokens.txt":        "https://huggingface.co/csukuangfj/sherpa-onnx-nemotron-speech-streaming-en-0.6b-int8-2026-01-14/resolve/main/tokens.txt",
}

const SampleRate = 16000

// Engine and Stream deliberately preserve the public API. NewEngine returns
// the diagnostic below before either can be used.
type Engine struct{}
type Stream struct{}

type Result struct {
	Text           string
	EndOfUtterance bool
}

// NewEngine makes the capability failure explicit instead of preventing an
// otherwise rootless Linux deployment from starting. The HTTP route turns this
// into a 503 only for profiles that opt into voice input.
func NewEngine(string) (*Engine, error) {
	return nil, fmt.Errorf("voicestt requires a CGO-enabled build with the sherpa-onnx native runtime")
}

func (*Engine) Close()                          {}
func (*Engine) NewStream() *Stream              { return &Stream{} }
func (*Stream) AcceptWaveform([]float32) Result { return Result{} }
func (*Stream) Finish() Result                  { return Result{EndOfUtterance: true} }
func (*Stream) Close()                          {}

// DefaultModelDir remains stable for configuration and diagnostics even when
// this particular build cannot load the native recognizer.
func DefaultModelDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".agentworks", "voice-models", "nemotron-streaming")
}

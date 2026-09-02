package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Native streaming speech-to-text — see docs/refactor/native_streaming_stt.md.
//
// This drives the Swift helper (desktop-sparkquill/voice-helper), which keeps
// decoder state across appends instead of re-transcribing the whole recording
// on every preview refresh the way the Python/MLX worker must. It reuses
// voiceWorker wholesale — same readiness handshake, call timeout, idle unload,
// and crash reaping — differing only in how the process is started.
//
// The Python path remains the default and is untouched; nothing here runs
// unless the helper binary is actually present.

// nativeVoiceCallTimeout is generous because the FIRST load downloads two
// CoreML model sets from HuggingFace (measured ~96s cold, ~0.6s once cached).
// The default 30s would kill the process mid-download and never recover.
const nativeVoiceCallTimeout = 6 * time.Minute

// nativeVoiceIdleTimeout is far longer than the Python worker's 15 minutes.
// Reloading costs 15-17s of CoreML model load (measured live: "worker started"
// to "stream start"), which is paid in full the next time the mic is clicked —
// so a 15-minute timeout meant a parent who dictates a few times an hour waits
// 15s nearly every time. The tradeoff is one resident ~600MB process while the
// app is open, which is the right trade for a desktop app whose whole point is
// that talking to it is instant. warmNativeVoice below removes the first wait
// of a session too.
const nativeVoiceIdleTimeout = 3 * time.Hour

var sharedNativeVoiceWorker = &voiceWorker{
	name:        "voice-native",
	launch:      nativeVoiceWorkerCmd,
	callTimeout: nativeVoiceCallTimeout,
	idleTimeout: nativeVoiceIdleTimeout,
}

// warmNativeVoice loads the model before anyone asks for it, so the first mic
// click of a session doesn't pay for it. Safe to call when the helper is
// missing: it simply reports that and returns.
func warmNativeVoice(ctx context.Context) error {
	if !nativeVoiceAvailable() {
		return fmt.Errorf("native voice helper not available")
	}
	_, err := sharedNativeVoiceWorker.call(ctx, map[string]any{"cmd": "load"})
	return err
}

// nativeVoiceHelperPath resolves the helper binary, or "" when unavailable.
//
// Apple Silicon only: FluidAudio runs CoreML on the Neural Engine, and the
// existing voice tier is already gated the same way (see voice_hardware.go),
// so this is not a new restriction.
func nativeVoiceHelperPath() string {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		return ""
	}
	candidates := []string{}
	// Explicit override wins — used by tests and by anyone running a locally
	// built helper against a packaged server.
	if p := os.Getenv("SPARKQUILL_VOICE_HELPER"); p != "" {
		candidates = append(candidates, p)
	}
	// Packaged: main.js spawns family-server with cwd set to the app's
	// Resources directory, and electron-builder stages the helper there
	// alongside the server binary itself.
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "voice-helper"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "voice-helper"))
		// Development: the helper built in place by `swift build -c release`.
		// Walk up rather than assuming a fixed depth — cwd differs between
		// `go run ./cmd/family-server` (agent_go/) and `go test` (the package
		// directory), and both should find it.
		dir := wd
		for i := 0; i < 6; i++ {
			candidates = append(candidates, filepath.Join(dir,
				"desktop-sparkquill", "voice-helper", ".build", "release", "voice-helper"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

func nativeVoiceWorkerCmd() (*exec.Cmd, error) {
	path := nativeVoiceHelperPath()
	if path == "" {
		return nil, fmt.Errorf("native voice helper not available on this machine")
	}
	return exec.Command(path), nil
}

// nativeVoiceAvailable reports whether the streaming path can be used at all.
// Callers fall back to the Python path when this is false, so a machine
// without the helper keeps working exactly as before.
func nativeVoiceAvailable() bool { return nativeVoiceHelperPath() != "" }

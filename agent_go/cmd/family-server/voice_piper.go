package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Piper is the optional "more natural voice" TTS tier: a small neural
// text-to-speech model that runs on any Mac (Intel included), noticeably
// warmer than the built-in system voice. It's a Python package rather than a
// single binary, so installing it means creating an isolated virtualenv —
// deliberately isolated so this never touches the user's own Python setup.
//
// Everything lives under the app's data dir and is deleted wholesale on
// uninstall, so "turn it off" genuinely reclaims the space.

// piperVoiceURL is the voice this tier ships with. One good default rather
// than a voice picker: the tier's promise is "sounds more natural", and
// choosing between a dozen near-identical American voices is a decision a
// parent has no basis to make.
const piperVoiceBase = "https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_US/amy/medium"

// piperTotalSizeMB is what the install actually costs end to end: the Python
// package tree (onnxruntime/numpy dominate) plus the ~63MB voice model.
// Measured, not guessed.
const piperTotalSizeMB = 210

func piperDir() string    { return filepath.Join(familyDataDir(), "piper") }
func piperPython() string { return filepath.Join(piperDir(), ".venv", "bin", "python") }
func piperModel() string  { return filepath.Join(piperDir(), "voice.onnx") }

// piperInstalled reports whether BOTH halves are present. Checking only one
// would let a half-finished install look ready and then fail at speak time.
func piperInstalled() bool {
	if fi, err := os.Stat(piperPython()); err != nil || fi.Size() == 0 {
		return false
	}
	fi, err := os.Stat(piperModel())
	return err == nil && fi.Size() > 0
}

// installPiper builds the isolated venv and downloads the voice, reporting
// progress into the same modelInstallStates the whisper tiers use so the UI
// needs no special case.
func installPiper() {
	const id = "piper"
	modelInstallMu.Lock()
	if st, running := modelInstallStates[id]; running && st.Installing {
		modelInstallMu.Unlock()
		return
	}
	modelInstallStates[id] = &modelInstallState{Installing: true, TotalBytes: int64(piperTotalSizeMB) * 1024 * 1024}
	modelInstallMu.Unlock()

	setErr := func(err error) {
		modelInstallMu.Lock()
		modelInstallStates[id] = &modelInstallState{Error: err.Error()}
		modelInstallMu.Unlock()
		log.Printf("[voice] piper install failed: %v", err)
	}

	go func() {
		if err := os.MkdirAll(piperDir(), 0o700); err != nil {
			setErr(err)
			return
		}
		if err := buildPiperVenv(); err != nil {
			setErr(err)
			return
		}
		// The package tree is the bulk of the install and finishes with no
		// byte-level progress available, so credit it as roughly done before
		// the voice download takes over the bar.
		modelInstallMu.Lock()
		if st := modelInstallStates[id]; st != nil {
			st.GotBytes = int64(float64(st.TotalBytes) * 0.7)
		}
		modelInstallMu.Unlock()

		if err := downloadFileWithProgress(piperVoiceBase+"/en_US-amy-medium.onnx", piperModel(), id, 0.7); err != nil {
			setErr(err)
			return
		}
		if err := downloadFileWithProgress(piperVoiceBase+"/en_US-amy-medium.onnx.json", piperModel()+".json", id, 1.0); err != nil {
			setErr(err)
			return
		}
		modelInstallMu.Lock()
		delete(modelInstallStates, id)
		modelInstallMu.Unlock()
		log.Printf("[voice] piper installed")
	}()
}

// buildPiperVenv creates the isolated environment. Prefers uv (much faster),
// falling back to the stdlib venv module so this still works on a machine
// without uv rather than failing outright.
func buildPiperVenv() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	venv := filepath.Join(piperDir(), ".venv")

	if uv, err := exec.LookPath("uv"); err == nil {
		if out, err := exec.CommandContext(ctx, uv, "venv", "--python", "3.12", venv).CombinedOutput(); err != nil {
			return fmt.Errorf("could not set up the voice environment: %s", lastLines(string(out), 200))
		}
		out, err := exec.CommandContext(ctx, uv, "pip", "install", "--python", piperPython(), "piper-tts").CombinedOutput()
		if err != nil {
			return fmt.Errorf("could not install the voice: %s", lastLines(string(out), 200))
		}
		return nil
	}

	py, err := exec.LookPath("python3")
	if err != nil {
		return fmt.Errorf("python3 is not available on this computer")
	}
	if out, err := exec.CommandContext(ctx, py, "-m", "venv", venv).CombinedOutput(); err != nil {
		return fmt.Errorf("could not set up the voice environment: %s", lastLines(string(out), 200))
	}
	if out, err := exec.CommandContext(ctx, piperPython(), "-m", "pip", "install", "piper-tts").CombinedOutput(); err != nil {
		return fmt.Errorf("could not install the voice: %s", lastLines(string(out), 200))
	}
	return nil
}

// removePiper deletes the whole thing — venv and voice together — so turning
// the tier off actually reclaims its ~210MB rather than leaving the bulk of it
// behind.
func removePiper() error {
	if err := os.RemoveAll(piperDir()); err != nil {
		return err
	}
	log.Printf("[voice] piper removed")
	return nil
}

// speakWithPiper renders text to a WAV via the installed Piper voice.
func speakWithPiper(text string) ([]byte, error) {
	tmp, err := os.CreateTemp("", "sq-piper-*.wav")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, piperPython(), "-m", "piper", "-m", piperModel(), "-f", tmpPath)
	cmd.Stdin = strings.NewReader(text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("piper failed: %w (%s)", err, lastLines(string(out), 200))
	}
	return os.ReadFile(tmpPath)
}

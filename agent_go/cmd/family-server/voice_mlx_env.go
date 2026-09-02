package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// mlxVoiceDir/mlxVoicePython is the Python environment for the Apple-
// Silicon-only Parakeet speech-to-text feature (voice_parakeet.go), which
// runs on Apple's MLX framework via the mlx-audio package.
func mlxVoiceDir() string    { return filepath.Join(familyDataDir(), "mlx-voice") }
func mlxVoicePython() string { return filepath.Join(mlxVoiceDir(), ".venv", "bin", "python") }

const mlxVoiceInstallID = "mlx-voice"

// mlxVoiceTotalSizeMB is the REAL end-to-end cost, measured rather than
// guessed: ~750MB of Python packages (mlx-audio and its deps) plus a
// ~2.36GB Parakeet checkpoint, warmed during install (see installMlxVoiceEnv)
// so the feature's first real use isn't an unannounced multi-GB download.
const mlxVoiceTotalSizeMB = 3110

// mlxVoiceInstalled checks the interpreter AND that the package set actually
// imports. A bare venv directory left behind by a half-finished install is
// not "installed" — the feature depending on it would otherwise report ready
// and then silently do nothing.
func mlxVoiceInstalled() bool {
	if fi, err := os.Stat(mlxVoicePython()); err != nil || fi.Size() == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, mlxVoicePython(), "-c", "import mlx_audio").Run() == nil
}

// installMlxVoiceEnv builds the environment and warms the Parakeet
// checkpoint, reporting progress into modelInstallStates so the Settings
// card shows live progress. Idempotent: a second call while an install is
// already running is a no-op.
//
// Progress is coarse rather than per-byte during the model download (unlike
// the old whisper-tier downloads, which streamed through Go and could report
// exact bytes) — this runs inside the Python subprocess via mlx-audio's own
// huggingface_hub fetch, which this code doesn't intercept. The bar still
// moves at each real milestone; it just doesn't creep smoothly during the
// heaviest step.
func installMlxVoiceEnv() {
	modelInstallMu.Lock()
	if st, running := modelInstallStates[mlxVoiceInstallID]; running && st.Installing {
		modelInstallMu.Unlock()
		return
	}
	modelInstallStates[mlxVoiceInstallID] = &modelInstallState{Installing: true, TotalBytes: int64(mlxVoiceTotalSizeMB) * 1024 * 1024}
	modelInstallMu.Unlock()

	setErr := func(err error) {
		modelInstallMu.Lock()
		modelInstallStates[mlxVoiceInstallID] = &modelInstallState{Error: err.Error()}
		modelInstallMu.Unlock()
		log.Printf("[voice] mlx voice install failed: %v", err)
	}
	bump := func(frac float64) {
		modelInstallMu.Lock()
		if st := modelInstallStates[mlxVoiceInstallID]; st != nil {
			st.GotBytes = int64(frac * float64(st.TotalBytes))
		}
		modelInstallMu.Unlock()
	}

	go func() {
		if err := os.MkdirAll(mlxVoiceDir(), 0o700); err != nil {
			setErr(err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()

		venvPy, pipArgs, err := pythonEnvBuilder(ctx, filepath.Join(mlxVoiceDir(), ".venv"))
		if err != nil {
			setErr(err)
			return
		}
		bump(0.05)
		if out, err := exec.CommandContext(ctx, venvPy, append(append([]string{}, pipArgs...), "mlx-audio")...).CombinedOutput(); err != nil {
			setErr(fmt.Errorf("could not install the voice engine: %s", lastLines(string(out), 200)))
			return
		}
		bump(0.25)

		// Warm the Parakeet checkpoint (~2.36GB) into the persistent worker's
		// memory — this is what makes the FIRST mic/WhatsApp transcription
		// instant instead of hanging on an unannounced download AND a cold
		// model load.
		if err := warmParakeet(ctx); err != nil {
			setErr(fmt.Errorf("could not finish setting up speech recognition: %w", err))
			return
		}

		modelInstallMu.Lock()
		delete(modelInstallStates, mlxVoiceInstallID)
		modelInstallMu.Unlock()
		log.Printf("[voice] mlx voice engine installed (Parakeet)")
	}()
}

// removeMlxVoiceEnv deletes the whole environment.
func removeMlxVoiceEnv() error {
	if err := os.RemoveAll(mlxVoiceDir()); err != nil {
		return err
	}
	log.Printf("[voice] mlx voice engine removed (Parakeet)")
	return nil
}

// pythonEnvBuilder creates an isolated virtualenv at venvPath and returns the
// command + leading args to install packages into it.
//
// Prefers uv (dramatically faster) but falls back to Python's own stdlib venv
// module, because uv is a developer tool most Macs don't have — and a voice
// feature that only works for people who already develop software isn't
// "works on any MacBook". python3 itself ships with macOS's command line
// tools and is present on any machine that can run this app's other tooling.
func pythonEnvBuilder(ctx context.Context, venvPath string) (cmd string, pipArgs []string, err error) {
	venvPy := filepath.Join(venvPath, "bin", "python")
	if uv, lookErr := exec.LookPath("uv"); lookErr == nil {
		if out, e := exec.CommandContext(ctx, uv, "venv", "--python", "3.12", venvPath).CombinedOutput(); e != nil {
			return "", nil, fmt.Errorf("could not set up the voice engine: %s", lastLines(string(out), 200))
		}
		// uv installs INTO a venv via --python, so it stays the driver.
		return uv, []string{"pip", "install", "--python", venvPy}, nil
	}
	py, lookErr := exec.LookPath("python3")
	if lookErr != nil {
		return "", nil, fmt.Errorf("this needs Python, which isn't set up on this computer yet")
	}
	if out, e := exec.CommandContext(ctx, py, "-m", "venv", venvPath).CombinedOutput(); e != nil {
		return "", nil, fmt.Errorf("could not set up the voice engine: %s", lastLines(string(out), 200))
	}
	return venvPy, []string{"-m", "pip", "install"}, nil
}

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

// mlxVoiceDir/mlxVoicePython is the ONE shared Python environment for every
// Apple-Silicon-only voice feature: Kokoro (read-aloud, voice_kokoro.go) and
// Parakeet (speech-to-text, voice_parakeet.go) both run on Apple's MLX
// framework via the same mlx-audio package, so they share ONE install rather
// than duplicating ~1GB of Python packages twice. Concretely: turning on the
// "most natural" voice and turning on Parakeet transcription install (and
// uninstall) the exact same thing.
func mlxVoiceDir() string    { return filepath.Join(familyDataDir(), "mlx-voice") }
func mlxVoicePython() string { return filepath.Join(mlxVoiceDir(), ".venv", "bin", "python") }

const mlxVoiceInstallID = "mlx-voice"

// mlxVoiceTotalSizeMB is the REAL end-to-end cost, measured rather than
// guessed: ~1.07GB of Python packages (mlx-audio + misaki + a spaCy model),
// plus a ~312MB Kokoro checkpoint and a ~2.36GB Parakeet checkpoint — both
// warmed during install (see installMlxVoiceEnv) so neither feature's first
// real use is an unannounced multi-hundred-MB-to-multi-GB download.
const mlxVoiceTotalSizeMB = 3750

// spacyModelSpec pins the English spaCy model misaki (Kokoro's text
// processor) needs. Pinned to a wheel URL rather than `spacy download`,
// because that shells out to pip/uv itself — and when misaki tried to fetch
// this model lazily at speak time instead, it failed SILENTLY (no
// virtualenv env-var is set when Go execs the interpreter directly).
// Installing it explicitly here avoids that failure mode entirely.
const spacyModelSpec = "en_core_web_sm @ https://github.com/explosion/spacy-models/releases/download/en_core_web_sm-3.8.0/en_core_web_sm-3.8.0-py3-none-any.whl"

// mlxVoiceInstalled checks the interpreter AND that the shared package set
// actually imports. A bare venv directory left behind by a half-finished
// install is not "installed" — every feature depending on it would otherwise
// report ready and then silently do nothing.
func mlxVoiceInstalled() bool {
	if fi, err := os.Stat(mlxVoicePython()); err != nil || fi.Size() == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, mlxVoicePython(), "-c", "import mlx_audio, en_core_web_sm").Run() == nil
}

// installMlxVoiceEnv builds the environment and warms BOTH checkpoints
// (Kokoro + Parakeet), reporting progress into modelInstallStates so either
// feature's card in Settings shows the same live progress. Idempotent: a
// second call while an install is already running is a no-op.
//
// Progress is coarse rather than per-byte during the two model downloads
// (unlike the old whisper-tier downloads, which streamed through Go and
// could report exact bytes) — these run inside the Python subprocess via
// mlx-audio's own huggingface_hub fetch, which this code doesn't intercept.
// The bar still moves at each real milestone; it just doesn't creep smoothly
// during the two heaviest steps.
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
		bump(0.15)
		if out, err := exec.CommandContext(ctx, venvPy, append(append([]string{}, pipArgs...), "misaki[en]")...).CombinedOutput(); err != nil {
			setErr(fmt.Errorf("could not install the voice engine: %s", lastLines(string(out), 200)))
			return
		}
		bump(0.2)
		if out, err := exec.CommandContext(ctx, venvPy, append(append([]string{}, pipArgs...), spacyModelSpec)...).CombinedOutput(); err != nil {
			setErr(fmt.Errorf("could not install the voice engine: %s", lastLines(string(out), 200)))
			return
		}
		bump(0.25)

		// Warm the Kokoro checkpoint (~312MB) — this is what makes the FIRST
		// "read this aloud" instant instead of hanging on an unannounced
		// download.
		if _, err := speakWithKokoro("Ready.", ""); err != nil {
			setErr(fmt.Errorf("could not finish setting up the voice: %w", err))
			return
		}
		bump(0.35)

		// Warm the Parakeet checkpoint (~2.36GB) the same way — same reason,
		// for the mic/WhatsApp transcription side.
		if err := warmParakeet(ctx); err != nil {
			setErr(fmt.Errorf("could not finish setting up speech recognition: %w", err))
			return
		}

		modelInstallMu.Lock()
		delete(modelInstallStates, mlxVoiceInstallID)
		modelInstallMu.Unlock()
		log.Printf("[voice] mlx voice engine installed (Kokoro + Parakeet)")
	}()
}

// removeMlxVoiceEnv deletes the whole shared environment — Kokoro AND
// Parakeet together, since they are one install. Whichever Settings card
// triggers this (the read-aloud tier or the speech-to-text tier), both
// features go back to needing a fresh install; both cards' copy discloses
// this rather than leaving it as a surprise.
func removeMlxVoiceEnv() error {
	if err := os.RemoveAll(mlxVoiceDir()); err != nil {
		return err
	}
	log.Printf("[voice] mlx voice engine removed (Kokoro + Parakeet)")
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

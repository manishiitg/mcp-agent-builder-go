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

// Kokoro (via mlx-audio) is the top "most natural voice" tier — the closest
// to a real person reading aloud. It runs on Apple's MLX framework, which
// only exists on Apple Silicon, so it is genuinely unavailable on an Intel
// Mac rather than merely slower.
//
// kokoroTotalSizeMB is MEASURED, not estimated: ~1.07GB of Python packages
// plus a ~349MB model. An earlier version of this tier advertised "350MB",
// counting only the model and hiding three quarters of the real cost — the
// exact kind of promise-small-then-pull-large this file's sizes exist to
// avoid.
const kokoroTotalSizeMB = 1450

// kokoroModel is the published Kokoro checkpoint mlx-audio downloads on first
// use (into the shared HuggingFace cache, which is why removal below is
// honest about only reclaiming the venv).
const kokoroModel = "prince-canuma/Kokoro-82M"

// kokoroVoice — one warm default rather than a picker. The tier's promise is
// "sounds most like a person"; choosing between near-identical voices is a
// decision a parent has no basis to make.
const kokoroVoice = "af_heart"

// spacyModelSpec pins the English model misaki needs. Pinned to a wheel URL
// rather than `spacy download`, because that command shells out to pip/uv
// itself — reintroducing exactly the runtime-install failure this avoids.
const spacyModelSpec = "en_core_web_sm @ https://github.com/explosion/spacy-models/releases/download/en_core_web_sm-3.8.0/en_core_web_sm-3.8.0-py3-none-any.whl"

func kokoroDir() string    { return filepath.Join(familyDataDir(), "kokoro") }
func kokoroPython() string { return filepath.Join(kokoroDir(), ".venv", "bin", "python") }

// kokoroInstalled checks the interpreter AND that mlx_audio is importable.
// A bare venv directory is not "installed": a half-finished install left one
// behind, the tier advertised itself as ready, and every play button silently
// did nothing.
func kokoroInstalled() bool {
	if fi, err := os.Stat(kokoroPython()); err != nil || fi.Size() == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, kokoroPython(), "-c", "import mlx_audio, en_core_web_sm").Run() == nil
}

// installKokoro builds the isolated environment. Reports progress into the
// same modelInstallStates the other tiers use, so the UI needs no special case.
func installKokoro() {
	const id = "kokoro"
	modelInstallMu.Lock()
	if st, running := modelInstallStates[id]; running && st.Installing {
		modelInstallMu.Unlock()
		return
	}
	modelInstallStates[id] = &modelInstallState{Installing: true, TotalBytes: int64(kokoroTotalSizeMB) * 1024 * 1024}
	modelInstallMu.Unlock()

	setErr := func(err error) {
		modelInstallMu.Lock()
		modelInstallStates[id] = &modelInstallState{Error: err.Error()}
		modelInstallMu.Unlock()
		log.Printf("[voice] kokoro install failed: %v", err)
	}

	go func() {
		if err := os.MkdirAll(kokoroDir(), 0o700); err != nil {
			setErr(err)
			return
		}
		if err := buildKokoroVenv(id); err != nil {
			setErr(err)
			return
		}
		// The model itself downloads on first use, inside mlx-audio, with no
		// byte-level progress to surface — so warm it up HERE rather than
		// making the parent's first "read this aloud" hang for a 349MB fetch
		// they were given no warning about.
		modelInstallMu.Lock()
		if st := modelInstallStates[id]; st != nil {
			st.GotBytes = int64(float64(st.TotalBytes) * 0.75)
		}
		modelInstallMu.Unlock()
		if _, err := speakWithKokoro("Ready."); err != nil {
			setErr(fmt.Errorf("could not finish setting up the voice: %w", err))
			return
		}
		modelInstallMu.Lock()
		delete(modelInstallStates, id)
		modelInstallMu.Unlock()
		log.Printf("[voice] kokoro installed")
	}()
}

// buildKokoroVenv installs mlx-audio plus misaki, which Kokoro needs for text
// processing and which mlx-audio does NOT pull in itself — installing only
// mlx-audio produces a package that imports fine and then fails at the moment
// someone actually asks it to speak.
func buildKokoroVenv(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	venv := filepath.Join(kokoroDir(), ".venv")

	// uv is much faster, but most Macs won't have it — fall back to the
	// stdlib venv module so this works on a plain machine rather than
	// demanding a developer tool the parent has never heard of.
	venvPy, pipArgs, err := pythonEnvBuilder(ctx, venv)
	if err != nil {
		return err
	}

	bump := func(frac float64) {
		modelInstallMu.Lock()
		if st := modelInstallStates[id]; st != nil {
			st.GotBytes = int64(frac * float64(st.TotalBytes))
		}
		modelInstallMu.Unlock()
	}
	bump(0.1)
	if out, err := exec.CommandContext(ctx, venvPy, append(append([]string{}, pipArgs...), "mlx-audio")...).CombinedOutput(); err != nil {
		return fmt.Errorf("could not install the voice: %s", lastLines(string(out), 200))
	}
	bump(0.55)
	if out, err := exec.CommandContext(ctx, venvPy, append(append([]string{}, pipArgs...), "misaki[en]")...).CombinedOutput(); err != nil {
		return fmt.Errorf("could not install the voice: %s", lastLines(string(out), 200))
	}
	bump(0.65)
	// misaki needs this spaCy model, and if it's missing it tries to fetch it
	// ITSELF at speak time by shelling out to uv — which fails when the server
	// runs the interpreter directly (no virtualenv env-var set), and fails
	// SILENTLY: the tier reported "Installed", every play button did nothing,
	// and the only clue was buried in a subprocess's stderr. Installing it here
	// means nothing has to be fetched the first time someone presses play.
	if out, err := exec.CommandContext(ctx, venvPy, append(append([]string{}, pipArgs...), spacyModelSpec)...).CombinedOutput(); err != nil {
		return fmt.Errorf("could not install the voice: %s", lastLines(string(out), 200))
	}
	return nil
}

// removeKokoro deletes the environment. The model itself lives in the shared
// HuggingFace cache used by other tools too, so it is deliberately left alone
// rather than deleting something this app doesn't exclusively own.
func removeKokoro() error {
	if err := os.RemoveAll(kokoroDir()); err != nil {
		return err
	}
	log.Printf("[voice] kokoro removed")
	return nil
}

// speakWithKokoro renders text to WAV. mlx-audio writes to <prefix>.wav in the
// working directory rather than taking an output path, so this runs in a temp
// dir and reads the result back.
func speakWithKokoro(text string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "sq-kokoro-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	// First run also downloads the model; later runs are fast.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	script := `import sys
from mlx_audio.tts.generate import generate_audio
generate_audio(text=sys.argv[1], model=sys.argv[2], voice=sys.argv[3],
               file_prefix="out", audio_format="wav", join_audio=True, verbose=False)`
	cmd := exec.CommandContext(ctx, kokoroPython(), "-c", script, text, kokoroModel, kokoroVoice)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("kokoro failed: %w (%s)", err, lastLines(string(out), 200))
	}
	data, err := os.ReadFile(filepath.Join(dir, "out.wav"))
	if err != nil {
		return nil, fmt.Errorf("kokoro produced no audio")
	}
	return data, nil
}

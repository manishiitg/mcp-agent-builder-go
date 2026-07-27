package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Kokoro is the top "most natural voice" read-aloud tier — the closest to a
// real person reading aloud. It runs via mlx-audio on Apple's MLX framework,
// which only exists on Apple Silicon, so it is genuinely unavailable on an
// Intel Mac rather than merely slower. It shares its Python environment with
// Parakeet (speech-to-text) — see voice_mlx_env.go for the install/remove
// logic both features go through together.

// kokoroModel is the published Kokoro checkpoint mlx-audio downloads on first
// use (into the shared HuggingFace cache). It also carries voices for other
// languages, including Hindi (hf_alpha/hf_beta) — see voice_choices.go.
const kokoroModel = "prince-canuma/Kokoro-82M"

// kokoroVoice — the default when nothing else was picked.
const kokoroVoice = "af_heart"

// speakWithKokoro renders text to WAV via the shared MLX voice environment.
// mlx-audio writes to <prefix>.wav in the working directory rather than
// taking an output path, so this runs in a temp dir and reads the result back.
func speakWithKokoro(text, voice string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "sq-kokoro-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	// First run also downloads the model; installMlxVoiceEnv warms it ahead
	// of time so this is fast in normal use.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	script := `import sys
from mlx_audio.tts.generate import generate_audio
generate_audio(text=sys.argv[1], model=sys.argv[2], voice=sys.argv[3], lang_code=sys.argv[4],
               file_prefix="out", audio_format="wav", join_audio=True, verbose=False)`
	if strings.TrimSpace(voice) == "" {
		voice = kokoroVoice
	}
	// Kokoro's own convention: a voice ID's first letter IS its language code
	// (af_/am_ -> American English "a", hf_/hm_ -> Hindi "h", etc.) — deriving
	// it here means adding a language to the picker never risks drifting out
	// of sync with a separately-stored field.
	langCode := voice[:1]
	cmd := exec.CommandContext(ctx, mlxVoicePython(), "-c", script, text, kokoroModel, voice, langCode)
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

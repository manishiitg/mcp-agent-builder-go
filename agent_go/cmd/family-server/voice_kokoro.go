package main

import (
	"fmt"
	"os"
	"strings"
)

// Kokoro is the top "most natural voice" read-aloud tier — the closest to a
// real person reading aloud. It runs via mlx-audio on Apple's MLX framework,
// which only exists on Apple Silicon, so it is genuinely unavailable on an
// Intel Mac rather than merely slower. It shares its Python environment (and
// the persistent voice worker — see voice_worker.go) with Parakeet
// (speech-to-text) — see voice_mlx_env.go for the install/remove logic both
// features go through together.

// kokoroModel is the published Kokoro checkpoint mlx-audio downloads on first
// use (into the shared HuggingFace cache). It also carries voices for other
// languages, including Hindi (hf_alpha/hf_beta) — see voice_choices.go.
const kokoroModel = "prince-canuma/Kokoro-82M"

// kokoroVoice — the default when nothing else was picked.
const kokoroVoice = "af_heart"

// speakWithKokoro renders text to WAV via the persistent voice worker (see
// voice_worker.go), which keeps the Kokoro model loaded in memory rather than
// re-importing mlx_audio and reloading it fresh on every single reply.
func speakWithKokoro(text, voice string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "sq-kokoro-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	if strings.TrimSpace(voice) == "" {
		voice = kokoroVoice
	}
	// Kokoro's own convention: a voice ID's first letter IS its language code
	// (af_/am_ -> American English "a", hf_/hm_ -> Hindi "h", etc.) — deriving
	// it here means adding a language to the picker never risks drifting out
	// of sync with a separately-stored field.
	langCode := voice[:1]

	resp, err := sharedVoiceWorker.call(map[string]any{
		"cmd":       "speak",
		"model":     kokoroModel,
		"text":      text,
		"voice":     voice,
		"lang_code": langCode,
		"out_dir":   dir,
	})
	if err != nil {
		return nil, fmt.Errorf("kokoro failed: %w", err)
	}
	path, _ := resp["path"].(string)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("kokoro produced no audio")
	}
	return data, nil
}

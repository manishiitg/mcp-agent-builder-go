package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// parakeetModel is the published MLX checkpoint used for speech-to-text.
// English only — the well-benchmarked 0.6B model (HF's Open ASR Leaderboard
// shows it beating Whisper on English specifically), not the smaller 110M
// variant, which was measured to trade meaningfully more accuracy for a
// smaller download.
//
// This REPLACED whisper.cpp as the only speech-to-text engine in this app —
// a decision made explicitly with the family after being told plainly what
// it gives up: whisper.cpp covered ~100 languages (including Hindi) and ran
// on any Mac; Parakeet is English-only and Apple-Silicon-only. In exchange:
// materially better English accuracy/speed, and mlx_audio.stt exposes a
// genuine streaming API (unlike whisper.cpp's batch-only CLI), which is what
// makes real-time "see it as you speak" transcription actually buildable
// later.
const parakeetModel = "mlx-community/parakeet-tdt-0.6b-v2"

// transcribeWithParakeet runs one audio file through Parakeet via mlx-audio's
// own STT module — the SAME shared environment used for Kokoro (see
// voice_mlx_env.go), so there's no second Python environment to install.
// Unlike whisper.cpp, Parakeet reads compressed audio directly (webm/opus,
// mp4, etc.) with no separate ffmpeg conversion step.
func transcribeWithParakeet(ctx context.Context, audioPath string) (string, error) {
	if !mlxVoiceInstalled() {
		return "", fmt.Errorf("speech recognition isn't set up yet")
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	script := `import sys
from mlx_audio.stt.generate import generate_transcription
result = generate_transcription(model=sys.argv[1], audio=sys.argv[2], verbose=False)
sys.stdout.write(result.text)`
	cmd := exec.CommandContext(ctx, mlxVoicePython(), "-c", script, parakeetModel, audioPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("parakeet failed: %w (%s)", err, lastLines(stderr.String(), 200))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// warmParakeet triggers the model's own one-time ~2.36GB download during
// install (see installMlxVoiceEnv), rather than leaving it for a parent's
// first real recording to hang on.
func warmParakeet(ctx context.Context) error {
	script := `import sys
from mlx_audio.stt.utils import load_model
load_model(sys.argv[1])`
	cmd := exec.CommandContext(ctx, mlxVoicePython(), "-c", script, parakeetModel)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", lastLines(string(out), 200))
	}
	return nil
}

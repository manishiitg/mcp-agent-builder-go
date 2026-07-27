package main

import (
	"context"
	"fmt"
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

// transcribeWithParakeet runs one audio file through the persistent voice
// worker (see voice_worker.go), which keeps Parakeet loaded in memory rather
// than re-importing mlx_audio and reloading the model on every single call —
// measured directly at ~1.9s of pure import overhead per cold process,
// before any real work happens. Unlike whisper.cpp, Parakeet reads compressed
// audio directly (webm/opus, mp4, etc.), with no separate ffmpeg conversion.
func transcribeWithParakeet(_ context.Context, audioPath string) (string, error) {
	if !mlxVoiceInstalled() {
		return "", fmt.Errorf("speech recognition isn't set up yet")
	}
	resp, err := sharedVoiceWorker.call(map[string]any{
		"cmd":        "transcribe",
		"model":      parakeetModel,
		"audio_path": audioPath,
	})
	if err != nil {
		return "", fmt.Errorf("parakeet failed: %w", err)
	}
	text, _ := resp["text"].(string)
	return text, nil
}

// warmParakeet triggers the model's own one-time ~2.36GB download during
// install (see installMlxVoiceEnv) AND loads it into the worker's memory,
// rather than leaving either for a parent's first real recording to hang on.
func warmParakeet(_ context.Context) error {
	_, err := sharedVoiceWorker.call(map[string]any{
		"cmd":   "load_stt",
		"model": parakeetModel,
	})
	return err
}

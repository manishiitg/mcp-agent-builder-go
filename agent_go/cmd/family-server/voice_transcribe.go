package main

import (
	"context"
)

// transcribeAudioFile transcribes a local audio file to text entirely
// on-device via Parakeet (mlx_audio.stt, see voice_parakeet.go) — no cloud API
// key, no per-use cost. Powers both the desktop mic and WhatsApp voice notes,
// so the same engine and behavior apply everywhere speech comes in.
//
// Apple Silicon only (Parakeet runs on Apple's MLX framework, which does not
// exist on Intel) and English only. whisper.cpp — which covered ~100
// languages including Hindi and ran on any Mac — was deliberately replaced by
// this, a decision made explicitly with the family after being told exactly
// what it gives up, in exchange for materially better English accuracy and
// speed and a real path to on-device live transcription later
// (mlx_audio.stt exposes a genuine streaming API; whisper.cpp did not).
func transcribeAudioFile(ctx context.Context, audioPath string) (string, error) {
	return transcribeWithParakeet(ctx, audioPath)
}

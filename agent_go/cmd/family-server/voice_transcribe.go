package main

import (
	"context"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/voicestt"
)

// transcribeAudioFile transcribes a local audio file to text entirely
// on-device through the shared AgentWorks engine (familyVoice) — no cloud API
// key, no per-use cost. Powers both an uploaded mic recording and WhatsApp
// voice notes, so the same engine and behavior apply everywhere speech comes
// in. English only (the pinned model's language); runs on any Mac, Intel
// included, which the previous Apple-Silicon-only engine could not.
//
// Decoding a container (WhatsApp's ogg/opus, a browser's webm) is
// voicestt.DecodeFile's job: macOS's own afconvert first, ffmpeg if present.
func transcribeAudioFile(ctx context.Context, audioPath string) (string, error) {
	samples, err := voicestt.DecodeFile(ctx, audioPath)
	if err != nil {
		return "", err
	}
	return familyVoice.Transcribe(ctx, samples)
}

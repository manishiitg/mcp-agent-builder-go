package main

import (
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// GET /api/voice/hardware — architecture + RAM, for the voice tier picker.
func handleVoiceHardware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, detectVoiceHardware())
}

// voiceTier is one option in the STT or TTS picker. Available is a hard
// hardware gate (e.g. Apple-Silicon-only tiers on an Intel Mac); ComingSoon is
// honest labeling for a tier that's designed into the catalog but has no real
// install/run path wired up yet — never show a tier as pickable unless it
// genuinely does something when picked.
type voiceTier struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Description    string `json:"description"`
	SizeMB         int    `json:"size_mb,omitempty"`
	Languages      string `json:"languages"`
	Available      bool   `json:"available"`
	UnavailableWhy string `json:"unavailable_reason,omitempty"`
	Installed      bool   `json:"installed"`
	ComingSoon     bool   `json:"coming_soon,omitempty"`
}

type voiceStatusResponse struct {
	Hardware voiceHardware `json:"hardware"`
	STTTiers []voiceTier   `json:"stt_tiers"`
	TTSTiers []voiceTier   `json:"tts_tiers"`
}

// GET /api/voice/status — the full tier catalog for the Settings -> Voice
// picker: hardware facts plus every STT/TTS tier, each with real
// availability/installed state computed against THIS machine and the
// WhatsApp voice pipeline's own existing install state (voiceModelInstalled)
// — not a second, disconnected notion of "installed".
func handleVoiceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hw := detectVoiceHardware()
	_, sayErr := exec.LookPath("say")

	stt := []voiceTier{
		{
			ID:          "builtin",
			Label:       "Built-in",
			Description: "whisper.cpp, on-device — the same engine already used for WhatsApp voice notes.",
			SizeMB:      whisperModelSizeMB,
			Languages:   "100+ languages, including Hindi",
			Available:   true,
			Installed:   voiceModelInstalled(),
		},
		{
			ID:          "better",
			Label:       "Better, still universal",
			Description: "whisper.cpp with a larger model — more accurate, still runs on any Mac.",
			SizeMB:      1500,
			Languages:   "100+ languages, including Hindi",
			Available:   true,
			ComingSoon:  true,
		},
		{
			ID:          "fastest",
			Label:       "Fastest (English only)",
			Description: "Parakeet via Apple's MLX framework — faster and more accurate than Whisper for English specifically, but MLX only runs on Apple Silicon.",
			SizeMB:      600,
			Languages:   "English only",
			Available:   hw.IsAppleSilicon,
			ComingSoon:  true,
		},
	}
	if !hw.IsAppleSilicon {
		stt[2].UnavailableWhy = "Requires an Apple Silicon Mac (M-series chip)"
	}

	tts := []voiceTier{
		{
			ID:          "builtin",
			Label:       "Built-in",
			Description: "macOS's own voice (AVSpeechSynthesizer) — no download, works instantly offline.",
			Languages:   "English, Hindi, and every language macOS itself supports",
			Available:   sayErr == nil,
			Installed:   sayErr == nil,
		},
		{
			ID:          "better",
			Label:       "Better, still universal",
			Description: "Piper — a small, fast neural voice, noticeably more natural than the system voice, on any Mac.",
			SizeMB:      60,
			Languages:   "English (per downloaded voice)",
			Available:   true,
			ComingSoon:  true,
		},
		{
			ID:          "fastest",
			Label:       "Fastest, most natural (Apple Silicon only)",
			Description: "mlx-audio with a Kokoro voice — the most natural-sounding option, but only runs on Apple Silicon.",
			SizeMB:      350,
			Languages:   "English only",
			Available:   hw.IsAppleSilicon,
			ComingSoon:  true,
		},
	}
	if sayErr != nil {
		tts[0].UnavailableWhy = "'say' not found on this system"
	}
	if !hw.IsAppleSilicon {
		tts[2].UnavailableWhy = "Requires an Apple Silicon Mac (M-series chip)"
	}

	writeJSON(w, http.StatusOK, voiceStatusResponse{Hardware: hw, STTTiers: stt, TTSTiers: tts})
}

// voiceHardware is what the voice settings UI needs to decide which STT/TTS
// tiers are actually offerable on this machine: Apple-Silicon-only tiers
// (Parakeet via MLX, mlx-audio TTS) need IsAppleSilicon. Nothing gates on RAM
// today — whisper.cpp's tiers and Piper are all light enough on any Mac this
// app targets — but TotalRAMBytes is reported so a future heavier tier has a
// real number to gate on instead of guessing.
type voiceHardware struct {
	Arch           string `json:"arch"` // Go's GOARCH: "arm64" | "amd64"
	IsAppleSilicon bool   `json:"is_apple_silicon"`
	TotalRAMBytes  int64  `json:"total_ram_bytes"`
}

// detectVoiceHardware reports this machine's architecture and RAM. Best-effort:
// a RAM read failure just reports 0 rather than erroring the whole request —
// architecture is what actually gates tier availability, RAM is informational.
func detectVoiceHardware() voiceHardware {
	return voiceHardware{
		Arch:           runtime.GOARCH,
		IsAppleSilicon: runtime.GOARCH == "arm64",
		TotalRAMBytes:  totalSystemRAMBytes(),
	}
}

// totalSystemRAMBytes reads the real, total installed system RAM via sysctl —
// NOT this process's own memory usage (which is what Go's runtime/memstats
// reports, a different and much smaller number that would be useless for a
// hardware-tier decision).
func totalSystemRAMBytes() int64 {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

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
	// Live download progress, when this tier is being installed right now.
	Installing bool   `json:"installing,omitempty"`
	GotBytes   int64  `json:"got_bytes,omitempty"`
	TotalBytes int64  `json:"total_bytes,omitempty"`
	InstallErr string `json:"install_error,omitempty"`
	// Whether the UI may offer install / remove for this tier.
	CanInstall bool `json:"can_install,omitempty"`
	CanRemove  bool `json:"can_remove,omitempty"`
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

	stt := make([]voiceTier, 0, len(whisperTierOrder))
	// Presented weakest-first so the list reads as a ladder; whisperTierOrder
	// is best-first because it answers a different question (which model to
	// actually USE), so don't reuse it for display order.
	installedCount := 0
	for _, id := range []string{"builtin", "better", "best"} {
		if whisperTierInstalled(id) {
			installedCount++
		}
	}
	for _, meta := range []struct{ id, label, desc string }{
		{"builtin", "Built-in", "whisper.cpp base model — the same engine already used for WhatsApp voice notes. Fast, good enough for clear speech."},
		{"better", "Better", "whisper.cpp small model — noticeably more accurate on names, numbers and quieter speech. Still runs on any Mac."},
		{"best", "Most accurate", "whisper.cpp medium model — the most accurate option that runs on any Mac. Slower, and a big download."},
	} {
		wt := whisperTiers[meta.id]
		st := installStateFor(meta.id)
		installed := whisperTierInstalled(meta.id)
		stt = append(stt, voiceTier{
			ID:          meta.id,
			Label:       meta.label,
			Description: meta.desc,
			SizeMB:      wt.SizeMB,
			Languages:   "100+ languages, including Hindi",
			Available:   true,
			Installed:   installed,
			Installing:  st.Installing,
			GotBytes:    st.GotBytes,
			TotalBytes:  st.TotalBytes,
			InstallErr:  st.Error,
			CanInstall:  !installed && !st.Installing,
			// Never offer to remove the last one — that would silently turn
			// speech input off rather than just downgrading it.
			CanRemove: installed && installedCount > 1,
		})
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

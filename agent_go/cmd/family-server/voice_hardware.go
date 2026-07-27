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

	// One tier: Parakeet, English-only, Apple Silicon only. This app used to
	// offer three universal whisper.cpp tiers (any Mac, ~100 languages
	// including Hindi) — replaced deliberately, after being told plainly what
	// that gives up, for materially better English accuracy/speed and a real
	// path to live "see it as you speak" transcription later (mlx_audio.stt
	// exposes a genuine streaming API; whisper.cpp's CLI was batch-only).
	stt := []voiceTier{func() voiceTier {
		st := installStateFor(mlxVoiceInstallID)
		// While the shared install is still running, report NOT installed
		// even once the Python packages import successfully — the packages
		// finish well before the two model warm-ups do, and showing
		// "Installed" alongside a live progress bar would look broken.
		installed := mlxVoiceInstalled() && !st.Installing
		return voiceTier{
			ID:          "parakeet",
			Label:       "Standard",
			Description: "Understands clear English speech, quickly and accurately.",
			SizeMB:      mlxVoiceTotalSizeMB,
			Languages:   "English",
			Available:   hw.IsAppleSilicon,
			Installed:   installed,
			Installing:  st.Installing,
			GotBytes:    st.GotBytes,
			TotalBytes:  st.TotalBytes,
			InstallErr:  st.Error,
			CanInstall:  hw.IsAppleSilicon && !installed && !st.Installing,
			// Removing this ALSO removes the "most natural" read-aloud voice
			// below — they are the same shared install (see voice_mlx_env.go).
			CanRemove: installed,
		}
	}()}
	if !hw.IsAppleSilicon {
		stt[0].UnavailableWhy = "Needs a newer Mac (2020 or later)"
	}

	tts := []voiceTier{
		{
			ID:          "builtin",
			Label:       "Standard",
			Description: "Your Mac's built-in voice. Ready right now — nothing to download.",
			Languages:   "English, Hindi, and many more",
			Available:   sayErr == nil,
			Installed:   sayErr == nil,
		},
		func() voiceTier {
			st := installStateFor(mlxVoiceInstallID)
			// Same reasoning as the STT tier above: not "installed" while the
			// shared install is still running its model warm-ups.
			installed := mlxVoiceInstalled() && !st.Installing
			return voiceTier{
				ID:          "kokoro",
				Label:       "Most natural",
				Description: "The most life-like voice — closest to a real person reading aloud. Also includes Hindi and several other languages.",
				SizeMB:      mlxVoiceTotalSizeMB,
				Languages:   "English, Hindi, and more",
				Available:   hw.IsAppleSilicon,
				Installed:   installed,
				Installing:  st.Installing,
				GotBytes:    st.GotBytes,
				TotalBytes:  st.TotalBytes,
				InstallErr:  st.Error,
				CanInstall:  hw.IsAppleSilicon && !installed && !st.Installing,
				// Removing this ALSO removes speech-to-text above — they are
				// the same shared install (see voice_mlx_env.go).
				CanRemove: installed,
			}
		}(),
	}
	if sayErr != nil {
		tts[0].UnavailableWhy = "Not available on this computer"
	}
	if !hw.IsAppleSilicon {
		tts[1].UnavailableWhy = "Needs a newer Mac (2020 or later)"
	}

	writeJSON(w, http.StatusOK, voiceStatusResponse{Hardware: hw, STTTiers: stt, TTSTiers: tts})
}

// voiceHardware is what the voice settings UI needs to decide which STT/TTS
// tiers are actually offerable on this machine: the shared MLX voice
// environment (Parakeet STT + Kokoro TTS) needs IsAppleSilicon. Nothing gates
// on RAM today — macOS's own voice is light on any Mac this app targets — but
// TotalRAMBytes is reported so a future heavier tier has a real number to
// gate on instead of guessing.
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

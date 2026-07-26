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
		{"builtin", "Standard", "Understands clear speech well. This one is set up for you already."},
		{"better", "More accurate", "Better with names, numbers, and quieter or faster talking."},
		{"best", "Most accurate", "The most accurate at understanding speech. A big download, and a little slower to think."},
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
			// The Standard model is the baseline everything falls back to, so
			// it isn't removable at all — offering to delete the floor is
			// confusing even when another model happens to be installed. The
			// upgrades above it can be removed freely, and never the last one
			// standing (which would silently turn speech input off rather
			// than just downgrading it).
			CanRemove: installed && meta.id != "builtin" && installedCount > 1,
		})
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
			st := installStateFor("piper")
			installed := piperInstalled()
			return voiceTier{
				ID:          "piper",
				Label:       "More natural",
				Description: "A warmer, more human-sounding voice. Works on any Mac.",
				SizeMB:      piperTotalSizeMB,
				Languages:   "English",
				Available:   true,
				Installed:   installed,
				Installing:  st.Installing,
				GotBytes:    st.GotBytes,
				TotalBytes:  st.TotalBytes,
				InstallErr:  st.Error,
				CanInstall:  !installed && !st.Installing,
				CanRemove:   installed,
			}
		}(),
		func() voiceTier {
			st := installStateFor("kokoro")
			installed := kokoroInstalled()
			return voiceTier{
				ID:          "kokoro",
				Label:       "Most natural",
				Description: "The most life-like voice — closest to a real person reading aloud.",
				SizeMB:      kokoroTotalSizeMB,
				Languages:   "English",
				Available:   hw.IsAppleSilicon,
				Installed:   installed,
				Installing:  st.Installing,
				GotBytes:    st.GotBytes,
				TotalBytes:  st.TotalBytes,
				InstallErr:  st.Error,
				CanInstall:  hw.IsAppleSilicon && !installed && !st.Installing,
				CanRemove:   installed,
			}
		}(),
	}
	if sayErr != nil {
		tts[0].UnavailableWhy = "Not available on this computer"
	}
	if !hw.IsAppleSilicon {
		tts[2].UnavailableWhy = "Needs a newer Mac (2020 or later)"
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

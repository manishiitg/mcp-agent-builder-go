package main

import (
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/voicestt"
)

// GET /api/voice/hardware — architecture + RAM, for the voice tier picker.
func handleVoiceHardware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, detectVoiceHardware())
}

// voiceTier is one option in the STT picker. Available is a hard gate (a
// build without the native speech runtime); ComingSoon is
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
	// Warm is distinct from Installed: a model can be fully installed on
	// disk yet still cold (not loaded into memory yet this session, or
	// unloaded while the window was hidden). The UI should only claim
	// "ready — instant" when Warm is true; Installed-but-not-Warm means the
	// FIRST use this session pays a ~1-2s load.
	Warm       bool `json:"warm"`
	ComingSoon bool `json:"coming_soon,omitempty"`
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
}

// GET /api/voice/status — the tier catalog for the Settings -> Voice picker:
// hardware facts plus the STT tier, with real installed/warm state read from
// the shared engine — the same state the WhatsApp voice pipeline reports, not
// a second, disconnected notion of "installed".
func handleVoiceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, voiceStatusResponse{Hardware: detectVoiceHardware(), STTTiers: []voiceTier{voiceStandardTier()}})
}

// voiceStandardTier is the one speech tier: the shared AgentWorks engine
// (pkg/voicestt), English, ~690MB, on any Mac. Its states all come from the
// engine's own Status so the card, the WhatsApp toggle, and the mic never
// disagree about what is installed or warm.
func voiceStandardTier() voiceTier {
	st := familyVoice.Status()
	// While the download is still running, report NOT installed even if some
	// files have landed: "Installed" beside a live progress bar looks broken.
	installed := st.Installed && !st.Downloading
	tier := voiceTier{
		ID:          voiceTierID,
		Label:       "Standard",
		Description: "Understands clear English speech, quickly and accurately — right on this computer.",
		SizeMB:      st.SizeMB,
		Languages:   voicestt.ModelLanguages,
		Available:   st.Available,
		Installed:   installed,
		Warm:        installed && st.Ready,
		Installing:  st.Downloading,
		GotBytes:    st.GotBytes,
		TotalBytes:  st.TotalBytes,
		InstallErr:  st.Error,
		CanInstall:  st.Available && !installed && !st.Downloading,
		CanRemove:   installed && st.ActiveStreams == 0,
	}
	if !st.Available {
		tier.UnavailableWhy = "This build was made without the speech engine"
	}
	return tier
}

// voiceHardware describes this machine for the voice settings UI. Nothing
// gates on it any more — the shared engine runs on Intel and Apple Silicon
// alike — but architecture and RAM are still reported so a future heavier
// tier has real numbers to gate on instead of guessing.
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

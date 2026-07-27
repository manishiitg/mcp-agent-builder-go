package main

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// voiceChoice is one selectable voice within a read-aloud tier.
type voiceChoice struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Accent string `json:"accent,omitempty"`
	Gender string `json:"gender,omitempty"` // "male" | "female"
}

// systemVoiceAllowlist keeps the built-in tier to voices worth reading a
// lesson in. macOS ships ~41 English voices, but most are novelty effects
// ("Bahh", "Boing", "Bad News", "Bubbles") that would be absurd reading a
// child her homework — listing all of them would make the picker useless.
// Indian English (Aman, Rishi) is deliberately included and surfaced: this
// app is aimed at CBSE/ICSE families, for whom a US voice is the less
// natural default.
var systemVoiceAllowlist = []struct{ name, accent, gender string }{
	{"Samantha", "American", "female"},
	{"Alex", "American", "male"},
	{"Ava", "American", "female"},
	{"Allison", "American", "female"},
	{"Tom", "American", "male"},
	{"Aman", "Indian", "male"},
	{"Rishi", "Indian", "male"},
	{"Veena", "Indian", "female"},
	{"Daniel", "British", "male"},
	{"Serena", "British", "female"},
	{"Kate", "British", "female"},
	{"Oliver", "British", "male"},
	{"Karen", "Australian", "female"},
	{"Lee", "Australian", "male"},
	{"Moira", "Irish", "female"},
	{"Fiona", "Scottish", "female"},
}

// Name, then the locale code. Most rows are column-aligned with 2+ spaces
// before the locale, but longer Siri-style names ("Aman (English (India))
// en_IN") are followed by just one space — a \s{2,} pattern skipped those
// rows entirely, which is what made Indian English disappear from the picker.
var sayVoiceLineRE = regexp.MustCompile(`^(.+?)\s+([a-z]{2}_[A-Z]{2})\s`)

// systemVoices returns the allowlisted voices actually present on THIS Mac —
// availability varies by macOS version and which voices the user has
// downloaded, so this is discovered rather than assumed.
func systemVoices() []voiceChoice {
	out, err := exec.Command("say", "-v", "?").Output()
	if err != nil {
		return nil
	}
	installed := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		m := sayVoiceLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// Match on the BASE name: macOS lists some voices with a locale
		// suffix — "Aman (English (India))" rather than plain "Aman" — and
		// these are Siri voices that register dynamically, so the same voice
		// can appear one way on one call and be absent on the next. Exact
		// matching made Indian English silently vanish from the picker.
		// `say -v` accepts either form, so the short name stays the ID.
		name := strings.TrimSpace(m[1])
		if i := strings.Index(name, " ("); i > 0 {
			name = name[:i]
		}
		installed[name] = true
	}
	var voices []voiceChoice
	for _, v := range systemVoiceAllowlist {
		if installed[v.name] {
			voices = append(voices, voiceChoice{ID: v.name, Label: v.name, Accent: v.accent, Gender: v.gender})
		}
	}
	return voices
}

// kokoroVoices are the published Kokoro voices. "a"/"b" prefixes are the
// model's own American/British naming; expanded here so the picker reads as
// names and accents rather than as model internals.
var kokoroVoices = []voiceChoice{
	{ID: "af_heart", Label: "Heart", Accent: "American"},
	{ID: "af_bella", Label: "Bella", Accent: "American"},
	{ID: "af_nicole", Label: "Nicole", Accent: "American"},
	{ID: "af_sarah", Label: "Sarah", Accent: "American"},
	{ID: "am_michael", Label: "Michael", Accent: "American"},
	{ID: "am_adam", Label: "Adam", Accent: "American"},
	{ID: "bf_emma", Label: "Emma", Accent: "British"},
	{ID: "bf_isabella", Label: "Isabella", Accent: "British"},
	{ID: "bm_george", Label: "George", Accent: "British"},
	{ID: "bm_lewis", Label: "Lewis", Accent: "British"},
	// Hindi — verified working (real audio generated, checked by ear against
	// the English voices' distinct output) with the SAME checkpoint already
	// installed for English; no separate download.
	{ID: "hf_alpha", Label: "Alpha", Accent: "Hindi"},
	{ID: "hf_beta", Label: "Beta", Accent: "Hindi"},
	{ID: "hm_omega", Label: "Omega", Accent: "Hindi"},
	{ID: "hm_psi", Label: "Psi", Accent: "Hindi"},
}

func init() {
	// Kokoro's own naming convention: the letter right after the language
	// code is the voice's gender — af_/bf_/hf_ = female, am_/bm_/hm_ = male.
	// Derived rather than hand-entered per voice, so it can't drift out of
	// sync with the ID list above.
	for i, v := range kokoroVoices {
		if len(v.ID) > 1 && v.ID[1] == 'f' {
			kokoroVoices[i].Gender = "female"
		} else if len(v.ID) > 1 && v.ID[1] == 'm' {
			kokoroVoices[i].Gender = "male"
		}
	}
}

// voicesForTier lists what a given read-aloud tier can speak as.
func voicesForTier(tier string) []voiceChoice {
	switch tier {
	case "builtin":
		return systemVoices()
	case "kokoro":
		return kokoroVoices
	default:
		return nil
	}
}

// GET /api/voice/voices?tier=builtin — the selectable voices for one tier.
func handleVoiceVoices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tier := r.URL.Query().Get("tier")
	voices := voicesForTier(tier)
	sort.SliceStable(voices, func(i, j int) bool { return voices[i].Accent < voices[j].Accent })
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tier":     tier,
		"voices":   voices,
		"selected": selectedVoiceFor(tier),
	})
}

// POST /api/voice/voices {"tier":"builtin","voice":"Aman"} — remembers the
// choice, so it survives restarts rather than resetting to the default every
// session.
func handleSetVoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Tier  string `json:"tier"`
		Voice string `json:"voice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	valid := false
	for _, v := range voicesForTier(req.Tier) {
		if v.ID == req.Voice {
			valid = true
			break
		}
	}
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "that voice isn't available"})
		return
	}
	stateMu.Lock()
	s := loadState()
	if s.VoiceChoices == nil {
		s.VoiceChoices = map[string]string{}
	}
	s.VoiceChoices[req.Tier] = req.Voice
	err := saveState(s)
	stateMu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// selectedVoiceFor resolves the parent's saved choice, falling back to each
// tier's default.
func selectedVoiceFor(tier string) string {
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	if v, ok := s.VoiceChoices[tier]; ok && v != "" {
		return v
	}
	switch tier {
	case "kokoro":
		return kokoroVoice
	default:
		return defaultTTSVoice
	}
}

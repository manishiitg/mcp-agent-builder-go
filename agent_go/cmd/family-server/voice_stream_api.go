package main

import (
	"net/http"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/voicestt"
)

// Live dictation: GET /api/voice/stream upgrades to the shared AgentWorks
// dictation WebSocket (voicestt.ServeStream) — raw PCM16 16kHz mono in as
// binary frames, partial/final transcripts out as JSON. Identical to the
// agent server's /api/voice/stream, so the frontend hook is shared too.
//
// This replaced three loopback POSTs (start/chunk/finish) that drove the
// Swift helper. One protocol everywhere outweighs the "POSTs are curl-able"
// convenience: the transcript now streams back word by word as the engine
// emits it, instead of one running-preview string per chunk request.
func handleVoiceStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	voicestt.ServeStream(familyVoice, &voiceUpgrader, w, r)
}

// POST /api/voice/warm — fire-and-forget: starts loading the engine (and on a
// first run, downloading the model) if it isn't ready already. The desktop
// shell calls this when the window is shown, and the frontend the moment a
// mic session starts, so a cold engine warms while the parent is still
// talking instead of on their first spoken word. Returns the live status.
func handleVoiceWarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	familyVoice.Warm()
	st := familyVoice.Status()
	code := http.StatusAccepted
	if st.Ready {
		code = http.StatusOK
	}
	writeJSON(w, code, st)
}

// POST /api/voice/unload — releases the engine's memory (~1GB resident). The
// desktop shell calls this when the window is hidden to the menu bar, so an
// app sitting in the background isn't holding the model. Never mid-dictation:
// that would kill the utterance the user is speaking.
func handleVoiceUnload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !familyVoice.Unload() {
		writeJSON(w, http.StatusOK, map[string]any{"unloaded": false, "reason": "dictation in progress"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unloaded": true})
}

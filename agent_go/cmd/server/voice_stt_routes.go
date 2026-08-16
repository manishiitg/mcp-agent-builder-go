package server

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/voicestt"
)

// The shared streaming STT engine (agentprofiles.RuntimeCapabilities.Voice).
// One process-wide Engine backs every session's Stream — see voicestt.Engine's
// doc comment for why that split is safe: loading the model is the expensive
// part (~1.7s, ~620MB resident), a live Stream is cheap per-connection state.
//
// Loaded lazily behind voiceEngineOnce rather than at server startup so a
// deployment that never enables voice for any profile never pays the download
// or the resident memory.
var (
	voiceEngineOnce sync.Once
	voiceEngine     *voicestt.Engine
	voiceEngineErr  error
)

func getVoiceEngine() (*voicestt.Engine, error) {
	voiceEngineOnce.Do(func() {
		voiceEngine, voiceEngineErr = voicestt.NewEngine(voicestt.DefaultModelDir())
		if voiceEngineErr != nil {
			log.Printf("[VOICE] engine load failed: %v", voiceEngineErr)
		} else {
			log.Printf("[VOICE] engine ready, model dir=%s", voicestt.DefaultModelDir())
		}
	})
	return voiceEngine, voiceEngineErr
}

// profileDeclaresVoice reports whether the named agent profile opted into the
// shared voice capability. Products never get a mic control (or this
// endpoint's functionality) merely by existing — same gate as Browser/Secrets
// in agent_profile_runtime.go.
func (api *StreamingAPI) profileDeclaresVoice(profileID, userID string) bool {
	if api.agentProfiles == nil || profileID == "" {
		return false
	}
	profile, err := api.agentProfiles.Resolve(profileID, 0, userID)
	if err != nil {
		return false
	}
	req := profile.Runtime.Capabilities.Voice
	return req == agentprofiles.CapabilityRequired || req == agentprofiles.CapabilityPreferred || req == agentprofiles.CapabilityOptional
}

// CheckOrigin must be set explicitly: gorilla/websocket's DEFAULT Upgrader
// rejects any request whose Origin differs from Host, which is exactly what a
// browser sends when the frontend (Vite dev server, port 52733) and this API
// server (port 19743) are on different ports — the ordinary case for this
// codebase's dev setup, and true of a separately-deployed frontend generally.
// Caught live: the browser's mic button silently hung for 60+ seconds (model
// load time) and then failed with "request origin not allowed by
// Upgrader.CheckOrigin", because this was left as the zero-value Upgrader.
// Reuses the same CORS allow-list check the terminal live-attach websocket
// upgrader uses (checkLiveAttachOrigin).
var voiceUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 4 * 1024,
}

func (api *StreamingAPI) voiceUpgraderFor() *websocket.Upgrader {
	up := voiceUpgrader
	up.CheckOrigin = api.checkLiveAttachOrigin
	return &up
}

// voiceStreamMessage is one server->client update. Kept minimal and JSON
// rather than reusing the polling-event envelope: this is a narrow-purpose
// transport (audio in, text out), not a session's general event stream.
type voiceStreamMessage struct {
	Type           string `json:"type"` // "partial" | "final" | "error"
	Text           string `json:"text"`
	EndOfUtterance bool   `json:"end_of_utterance,omitempty"`
	Error          string `json:"error,omitempty"`
}

// handleVoiceStream is GET /api/voice/stream?profile_id=...&token=... — a
// WebSocket that accepts raw PCM16 mono 16kHz audio as binary frames and
// streams back partial/final transcripts as JSON text frames.
//
// profile_id gates functionality per agentprofiles.RuntimeCapabilities.Voice,
// mirroring how Browser/Secrets are resolved once per profile rather than
// hardcoded per product (agent_profile_runtime.go). token is the same
// query-param JWT fallback AuthMiddleware and the terminal live-attach
// websocket already use, because browsers cannot set a custom Authorization
// header on a WebSocket upgrade request.
func (api *StreamingAPI) handleVoiceStream(w http.ResponseWriter, r *http.Request) {
	currentUserID := GetUserIDFromContext(r.Context())
	profileID := r.URL.Query().Get("profile_id")
	if !api.profileDeclaresVoice(profileID, currentUserID) {
		http.Error(w, `{"error":"voice capability not enabled for this profile"}`, http.StatusForbidden)
		return
	}

	engine, err := getVoiceEngine()
	if err != nil {
		http.Error(w, `{"error":"voice engine unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	conn, err := api.voiceUpgraderFor().Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[VOICE] upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	stream := engine.NewStream()
	defer stream.Close()

	// Chunk-arrival telemetry, same reasoning as the [STREAM]/[TOOL] logs
	// elsewhere in this codebase: without it, "no text appeared" is
	// indistinguishable between "no audio ever arrived" (mic/permission/silence
	// problem, entirely a frontend concern) and "audio arrived but the model
	// heard nothing to transcribe" (a real STT problem). Logging RMS, not just a
	// byte count, is what separates near-silence (likely a muted/wrong input
	// device) from real speech reaching the engine.
	var audioChunks int
	var totalSamples int
	logFirstChunk := true

	send := func(msg voiceStreamMessage) {
		if writeErr := conn.WriteJSON(msg); writeErr != nil {
			log.Printf("[VOICE] write failed: %v", writeErr)
		}
	}

	for {
		msgType, data, readErr := conn.ReadMessage()
		if readErr != nil {
			// A clean close from the client is not an error worth logging;
			// anything else is diagnosable from this one line without needing
			// to reproduce interactively.
			if websocket.IsUnexpectedCloseError(readErr, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[VOICE] read failed: %v", readErr)
			}
			result := stream.Finish()
			if result.Text != "" {
				send(voiceStreamMessage{Type: "final", Text: result.Text, EndOfUtterance: true})
			}
			log.Printf("[VOICE] session ended: chunks=%d audio_seconds=%.1f final_text=%q",
				audioChunks, float64(totalSamples)/float64(voicestt.SampleRate), result.Text)
			return
		}

		switch msgType {
		case websocket.BinaryMessage:
			samples, decodeErr := decodePCM16(data)
			if decodeErr != nil {
				send(voiceStreamMessage{Type: "error", Error: decodeErr.Error()})
				continue
			}
			audioChunks++
			totalSamples += len(samples)
			if logFirstChunk {
				logFirstChunk = false
				log.Printf("[VOICE] first audio chunk received: %d samples, rms=%.4f", len(samples), pcmRMS(samples))
			} else if audioChunks%50 == 0 {
				log.Printf("[VOICE] %d chunks received, %.1fs of audio, last-chunk rms=%.4f",
					audioChunks, float64(totalSamples)/float64(voicestt.SampleRate), pcmRMS(samples))
			}
			result := stream.AcceptWaveform(samples)
			if result.Text == "" {
				continue
			}
			if result.EndOfUtterance {
				send(voiceStreamMessage{Type: "final", Text: result.Text, EndOfUtterance: true})
			} else {
				send(voiceStreamMessage{Type: "partial", Text: result.Text})
			}
		case websocket.TextMessage:
			// A control message. "finish" is the only one so far — the client
			// says "I'm done talking" without closing the socket, e.g. on
			// release of a push-to-talk button.
			var ctrl struct {
				Action string `json:"action"`
			}
			if json.Unmarshal(data, &ctrl) == nil && ctrl.Action == "finish" {
				result := stream.Finish()
				if result.Text != "" {
					send(voiceStreamMessage{Type: "final", Text: result.Text, EndOfUtterance: true})
				}
			}
		}
	}
}

// decodePCM16 converts little-endian 16-bit PCM bytes (the format
// AudioWorklet/MediaRecorder-derived raw audio arrives in) to the float32
// samples voicestt.Stream.AcceptWaveform expects.
// pcmRMS is a cheap, decisive signal-vs-silence check: real speech at a
// reasonable mic level sits well above ~0.01; a muted input, a wrong/virtual
// device, or a permission grant that returns a synthetic empty stream all read
// near zero. This is what turns "no text appeared" from a guess into a fact.
func pcmRMS(samples []float32) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sumSquares float64
	for _, s := range samples {
		sumSquares += float64(s) * float64(s)
	}
	return math.Sqrt(sumSquares / float64(len(samples)))
}

func decodePCM16(data []byte) ([]float32, error) {
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}
	n := len(data) / 2
	samples := make([]float32, n)
	for i := 0; i < n; i++ {
		v := int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2])) //nolint:gosec // G115: intentional PCM16 bit reinterpretation.
		samples[i] = float32(v) / 32768.0
	}
	return samples, nil
}

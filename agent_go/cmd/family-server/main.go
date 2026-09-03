// Command family-server is a small standalone HTTP server that exposes
// coding-agent engine detection and validation over JSON. It is separate from
// the main AgentWorks server and listens on its own port (default 8010).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/enginedetect"
)

var allowedOrigins = map[string]bool{
	"http://127.0.0.1:5174": true,
	"http://localhost:5174": true,
}

func main() {
	// execute_shell_command runs through security.BuildSafeEnvironment(), which
	// only preserves the real login-shell PATH/HOME (so host CLIs like node,
	// homebrew, aws resolve normally) when NATIVE_WORKSPACE=true — otherwise
	// it silently falls back to a Docker-style minimal PATH that can't find
	// anything outside /usr/bin. family-server, like AgentWorks' own desktop
	// launcher (desktop/main.js), always runs natively on the user's machine,
	// never in Docker, so this is unconditional rather than an env the operator
	// has to remember to set.
	os.Setenv("NATIVE_WORKSPACE", "true")

	// Real mid-turn content streaming (assistant TEXT, not just tool-call
	// observability — those are separate: EnableStreaming covers tool calls
	// and is auto-on for every coding-agent provider regardless) used to be
	// believed to be an env-var opt-in here (CODEX_CLI_STREAM_TRANSCRIPT=1,
	// CURSOR_CLI_STREAM_TRANSCRIPT=1) — confirmed dead: nothing in mcpagent or
	// multi-llm-provider-go ever read those names, so agentsession.Config.
	// StreamCallback never actually fired (ttft=none on every real turn,
	// confirmed live). The real API is a CallOption (e.g.
	// codexcli.WithStreamTranscript(true)), which mcpagent now appends
	// automatically whenever a StreamCallback is registered (see
	// mcpagent/agent/coding_agent_integrations.go's StreamingCallback check) —
	// nothing to set here anymore.

	// Give every interactive coding-agent tmux session ITS OWN naming
	// namespace, distinct from multi-llm-provider-go's shared "mlp-*" default
	// that AgentWorks also uses (unmodified, confirmed — it sets none of these
	// overrides). Without this, family-server's orphan-tmux sweep (see
	// tmux_sweep.go) would match by prefix alone and could kill an AgentWorks
	// session that happens to be alive on the same machine — a real, harmful
	// collision this app must never risk. All four providers already support
	// this via env var; set them for all four regardless of which engine is
	// actually selected, since the family can switch engines at any time.
	os.Setenv("CURSOR_CLI_INTERACTIVE_SESSION_PREFIX", "sq-cursor-cli-int")
	os.Setenv("CLAUDE_CODE_TMUX_SESSION_PREFIX", "sq-claude-code")
	os.Setenv("CODEX_CLI_INTERACTIVE_SESSION_PREFIX", "sq-codex-cli-int")
	os.Setenv("PI_CLI_INTERACTIVE_SESSION_PREFIX", "sq-pi-cli-int")

	// Clean up any coding-agent tmux sessions a PAST process left behind
	// (crash, redeploy, or a plain restart mid-session all orphan whatever was
	// warm at that moment — confirmed live: 32 accumulated in one day of
	// development restarts), then keep sweeping hourly for the rest of this
	// process's own life. See tmux_sweep.go.
	startTmuxSweepLoop()

	// Manual test knobs (see experiment_flags.go) — announce them loudly at
	// startup since neither has any other visible effect until a turn runs.
	if t := experimentCodingAgentTransport(); t != "" {
		log.Printf("[experiment] FAMILY_CODING_TRANSPORT=%s — every turn (parent + child) runs the structured/JSON transport instead of tmux", t)
	}
	if experimentSteeringDisabled() {
		log.Printf("[experiment] FAMILY_DISABLE_STEER set — live mid-turn steering is refused for every conversation")
	}

	defaultPort := "8010"
	if envPort := strings.TrimSpace(os.Getenv("FAMILY_PORT")); envPort != "" {
		defaultPort = envPort
	}

	port := flag.String("port", defaultPort, "port to listen on (or set FAMILY_PORT)")
	flag.Parse()

	// Ensure the workspace folder layout + app-provided skills exist on startup,
	// so existing families pick up newly-added folders (inbox, reports) and skill
	// updates that ship with the binary. Both are idempotent.
	_ = scaffoldFamilyFolders()
	seedSkills()
	// One-time migration from the old shared/parent/child layout to the
	// activity-folder layout (see migrate.go), run BEFORE seedWorkspace so a
	// real migrated reports/academic-map.html or progress.html always lands
	// before seedWorkspace's own "only if missing" placeholder check runs.
	runWorkspaceMigrationIfNeeded()
	seedWorkspace(loadState().Child)          // idempotent: only fills in files that don't exist yet
	mirrorChildSchedule(loadState().Schedule) // keep memory/child-schedule.json in sync, same reasoning as the profile mirror above
	if err := initWhatsAppBot(context.Background()); err != nil {
		log.Printf("whatsapp: failed to initialize (real WhatsApp connection disabled): %v", err)
	} else {
		// If already paired, establish the live connection now so incoming
		// messages arrive without waiting for the frontend to poll status.
		whatsAppBot.EnsureConnecting(context.Background())
	}
	go pulseRunner.Start(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/engines", handleEngines)
	mux.HandleFunc("/api/engines/validate", handleValidate)
	mux.HandleFunc("/api/setup", handleSetup)
	mux.HandleFunc("/api/engine/selection", handleSelectEngine)
	mux.HandleFunc("/api/child", handleCreateChild)
	mux.HandleFunc("/api/parent/pin", handleSetPin)
	mux.HandleFunc("/api/parent/pin/verify", handleVerifyPin)
	mux.HandleFunc("/api/parent/handoff", handleHandoff)
	mux.HandleFunc("/api/activities", handleActivities)
	mux.HandleFunc("/api/child/activity", handleChildActivity)
	mux.HandleFunc("/api/parent/message", handleParentMessage)
	mux.HandleFunc("/api/parent/status", handleParentStatusStream)
	mux.HandleFunc("/api/parent/steer", handleParentSteer)
	mux.HandleFunc("/api/child/message", handleChildMessage)
	mux.HandleFunc("/api/child/steer", handleChildSteer)
	mux.HandleFunc("/api/child/status", handleChildStatusStream)
	mux.HandleFunc("/api/whatsapp/message", handleWhatsAppMessage)
	mux.HandleFunc("/api/whatsapp/status", handleWhatsAppStatus)
	mux.HandleFunc("/api/whatsapp/pair", handleWhatsAppPair)
	mux.HandleFunc("/api/whatsapp/unpair", handleWhatsAppUnpair)
	mux.HandleFunc("/api/whatsapp/voice", handleWhatsAppVoiceToggle)
	mux.HandleFunc("/api/voice/hardware", handleVoiceHardware)
	mux.HandleFunc("/api/voice/status", handleVoiceStatus)
	mux.HandleFunc("/api/voice/transcribe", handleVoiceTranscribe)
	mux.HandleFunc("/api/voice/warm", handleVoiceWarm)
	mux.HandleFunc("/api/voice/unload", handleVoiceUnload)
	// Live dictation: the shared AgentWorks WebSocket (pkg/voicestt), the
	// same endpoint and protocol the agent server exposes.
	mux.HandleFunc("/api/voice/stream", handleVoiceStream)
	mux.HandleFunc("/api/voice/model/install", handleVoiceModelInstall)
	mux.HandleFunc("/api/voice/model/remove", handleVoiceModelRemove)
	mux.HandleFunc("/api/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleSetModel(w, r)
			return
		}
		handleGetModels(w, r)
	})
	mux.HandleFunc("/api/browser/status", handleBrowserStatus)
	mux.HandleFunc("/api/pulse/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleSetPulseConfig(w, r)
			return
		}
		handleGetPulseConfig(w, r)
	})
	mux.HandleFunc("/api/pulse/run", handlePulseRunNow)
	mux.HandleFunc("/api/pulse/status", handlePulseStatus)
	mux.HandleFunc("/api/child-schedule", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleSetChildSchedule(w, r)
			return
		}
		handleGetChildSchedule(w, r)
	})
	mux.HandleFunc("/api/week", handleGetWeek)
	mux.HandleFunc("/api/fast-mode", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleSetFastMode(w, r)
			return
		}
		handleGetFastMode(w, r)
	})
	mux.HandleFunc("/api/workspace/tree", handleWorkspaceTree)
	mux.HandleFunc("/api/workspace/file", handleWorkspaceFile)
	mux.HandleFunc("/api/workspace/raw", handleWorkspaceRaw)
	mux.HandleFunc("/api/workspace/state", handleWorkspaceState)
	mux.HandleFunc("/api/upload", handleUpload)
	mux.HandleFunc("/api/secrets", handleSecrets)
	mux.HandleFunc("/api/reset", handleReset)

	// In the packaged (Electron) app, serve the built SparkQuill frontend from the
	// same origin as the API — so no CORS, and the Electron main just loads
	// http://127.0.0.1:<port>/. FAMILY_WEB_DIR points at the built dist. In dev
	// (Vite on :5174) this is unset and the CORS allow-list handles cross-origin.
	if webDir := strings.TrimSpace(os.Getenv("FAMILY_WEB_DIR")); webDir != "" {
		files := http.FileServer(http.Dir(webDir))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Serve index.html for any path that isn't a real file, so a
			// reload or a stray deep link lands on the app instead of a bare
			// 404. Missing ASSETS still 404 honestly (they're requested with
			// an Accept that doesn't ask for HTML), which keeps a genuinely
			// broken build visible instead of silently serving the shell.
			clean := filepath.Join(webDir, filepath.Clean("/"+r.URL.Path))
			if _, err := os.Stat(clean); err != nil && strings.Contains(r.Header.Get("Accept"), "text/html") {
				http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
				return
			}
			files.ServeHTTP(w, r)
		})
		log.Printf("serving frontend from %s", webDir)
	}

	// If the speech model was already downloaded in an earlier session, load
	// it in the background right away — a parent's FIRST use each session
	// should find it ready rather than paying the ~1-2s load on whatever
	// happens to be the first click. A machine that never enabled voice is
	// not made to download 630MB just by launching the app.
	if familyVoice.Status().Installed {
		familyVoice.Warm()
	}

	addr := ":" + strings.TrimPrefix(strings.TrimSpace(*port), ":")
	log.Printf("family-server listening on %s", addr)
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		log.Fatalf("family-server failed: %v", err)
	}
}

// withCORS applies permissive CORS for the local dev frontend and handles
// OPTIONS preflight requests.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("family-server: failed to encode response: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleEngines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engines := enginedetect.Detect(r.Context())
	writeJSON(w, http.StatusOK, engines)
}

type validateRequest struct {
	Provider string `json:"provider"`
	ModelID  string `json:"model_id"`
}

type validateResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

func handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, validateResponse{
			Valid:   false,
			Message: "invalid JSON body",
		})
		return
	}
	if strings.TrimSpace(req.Provider) == "" {
		writeJSON(w, http.StatusBadRequest, validateResponse{
			Valid:   false,
			Message: "provider is required",
		})
		return
	}

	valid, message := enginedetect.Validate(r.Context(), req.Provider, req.ModelID)
	writeJSON(w, http.StatusOK, validateResponse{Valid: valid, Message: message})
}

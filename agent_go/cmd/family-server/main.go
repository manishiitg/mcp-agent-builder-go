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

	// Opt into real content streaming for codex-cli's and cursor-cli's
	// persistent-interactive (tmux) sessions — off by default in
	// mcpagent/multi-llm-provider-go (codex tails its JSONL rollout file;
	// cursor polls its own sqlite store.db — same idea, different source).
	// Without these, agentsession.Config.StreamCallback never fires and every
	// turn falls back to "reply only available once the whole turn finishes",
	// exactly like before streaming existed — which is also why tool-call
	// events never appear mid-turn for that provider (they ride the same
	// transcript tailer as the text stream).
	os.Setenv("CODEX_CLI_STREAM_TRANSCRIPT", "1")
	os.Setenv("CURSOR_CLI_STREAM_TRANSCRIPT", "1")

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
	seedWorkspace(loadState().Child) // idempotent: only fills in files that don't exist yet
	if err := initWhatsAppBot(context.Background()); err != nil {
		log.Printf("whatsapp: failed to initialize (real WhatsApp connection disabled): %v", err)
	} else {
		// If already paired, establish the live connection now so incoming
		// messages arrive without waiting for the frontend to poll status.
		whatsAppBot.EnsureConnecting(context.Background())
	}
	go startPulseTicker(context.Background())

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
	mux.HandleFunc("/api/voice/speak", handleVoiceSpeak)
	mux.HandleFunc("/api/voice/transcribe", handleVoiceTranscribe)
	mux.HandleFunc("/api/voice/model/install", handleVoiceModelInstall)
	mux.HandleFunc("/api/voice/model/remove", handleVoiceModelRemove)
	mux.HandleFunc("/api/voice/voices", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleSetVoice(w, r)
			return
		}
		handleVoiceVoices(w, r)
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

	// If voice was already installed in an earlier session, start warming the
	// persistent worker in the background right away — a parent's FIRST use
	// each session should find it already loaded rather than paying the
	// ~2-3s cold-start cost on whatever happens to be the first click.
	if mlxVoiceInstalled() {
		go func() {
			if err := warmParakeet(context.Background()); err != nil {
				log.Printf("[voice] background warm-up (speech recognition) failed: %v", err)
			}
			if _, err := sharedVoiceWorker.call(map[string]any{"cmd": "load_tts", "model": kokoroModel}); err != nil {
				log.Printf("[voice] background warm-up (read-aloud voice) failed: %v", err)
			}
		}()
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

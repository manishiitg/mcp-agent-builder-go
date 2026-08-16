package server

import (
	"net/http"
	"os"
	"strings"
)

// runtimeDiagnosticsEnabled controls developer-only terminal inspection APIs.
// Internal terminal/session state remains available to the orchestrator and CLI
// lifecycle code; this flag only prevents the product UI (and any other HTTP
// caller) from enumerating panes, captures, or execution trees by default.
func runtimeDiagnosticsEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENTWORKS_RUNTIME_DEBUG"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (api *StreamingAPI) runtimeDiagnosticsHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !runtimeDiagnosticsEnabled() {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

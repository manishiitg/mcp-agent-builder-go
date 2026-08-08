package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

const maxAgentProfileRequestBytes = 2 << 20

func AgentProfileRoutes(router *mux.Router, registry *agentprofiles.Registry) {
	router.HandleFunc("/agent-profiles", listAgentProfilesHandler(registry)).Methods(http.MethodGet, http.MethodOptions)
	router.HandleFunc("/agent-profiles/validate", validateAgentProfileHandler()).Methods(http.MethodPost, http.MethodOptions)
	router.HandleFunc("/agent-profiles/{id}", getAgentProfileHandler(registry)).Methods(http.MethodGet, http.MethodOptions)
}

func listAgentProfilesHandler(registry *agentprofiles.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		userID := GetUserIDFromContext(r.Context())
		writeAgentProfileJSON(w, http.StatusOK, map[string]interface{}{
			"profiles": registry.List(userID),
		})
	}
}

func getAgentProfileHandler(registry *agentprofiles.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		version := 0
		if rawVersion := strings.TrimSpace(r.URL.Query().Get("version")); rawVersion != "" {
			parsed, err := strconv.Atoi(rawVersion)
			if err != nil || parsed < 1 {
				writeAgentProfileError(w, http.StatusBadRequest, "version must be a positive integer")
				return
			}
			version = parsed
		}
		profile, err := registry.Resolve(mux.Vars(r)["id"], version, GetUserIDFromContext(r.Context()))
		if err != nil {
			writeAgentProfileError(w, http.StatusNotFound, "agent profile not found")
			return
		}
		writeAgentProfileJSON(w, http.StatusOK, profile)
	}
}

func validateAgentProfileHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, maxAgentProfileRequestBytes))
		decoder.DisallowUnknownFields()
		var profile agentprofiles.Profile
		if err := decoder.Decode(&profile); err != nil {
			writeAgentProfileError(w, http.StatusBadRequest, "invalid agent profile: "+err.Error())
			return
		}
		// Validation is the user-profile contract. A client cannot claim built-in
		// authority or choose a different owner through this endpoint.
		profile.BuiltIn = false
		profile.OwnerID = GetUserIDFromContext(r.Context())
		if err := agentprofiles.Validate(profile); err != nil {
			writeAgentProfileError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeAgentProfileJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   true,
			"profile": profile,
		})
	}
}

func writeAgentProfileError(w http.ResponseWriter, status int, message string) {
	writeAgentProfileJSON(w, status, map[string]string{"error": message})
}

func writeAgentProfileJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

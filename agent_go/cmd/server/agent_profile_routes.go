package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

const maxAgentProfileRequestBytes = 2 << 20

// AgentProfileChatRequest is the intentionally small public contract for a
// product-owned chat. Prompt, model, tools, skills, permissions and workspace
// all come from the registered profile rather than the browser.
type AgentProfileChatRequest struct {
	Message         string `json:"message"`
	ConversationKey string `json:"conversation_key,omitempty"`
}

type AgentProfileConversationRequest struct {
	ConversationKey string `json:"conversation_key,omitempty"`
}

type AgentProfileConversationResponse struct {
	ConversationID  string `json:"conversation_id"`
	ConversationKey string `json:"conversation_key"`
	SessionID       string `json:"session_id"`
}

func queryRequestForAgentProfileChat(profile agentprofiles.Profile, input AgentProfileChatRequest, conversation ProductConversationRecord) (QueryRequest, error) {
	if strings.TrimSpace(conversation.SessionID) == "" || strings.TrimSpace(conversation.WorkspacePath) == "" {
		return QueryRequest{}, fmt.Errorf("product conversation has no runtime binding")
	}
	return QueryRequest{
		Query:               input.Message,
		SessionTitle:        firstNonEmptyTrimmed(conversation.Title, profile.Name),
		AgentMode:           "multi-agent",
		AgentProfileID:      profile.ID,
		AgentProfileVersion: profile.Version,
		AgentProfileContext: agentprofiles.PromptContext{
			ProjectTitle:         firstNonEmptyTrimmed(conversation.Title, profile.Name),
			WorkspaceDescription: conversation.Description,
		},
		SelectedFolder:           conversation.WorkspacePath,
		DisableLiveInputDelivery: true,
	}, nil
}

func AgentProfileRoutes(router *mux.Router, registry *agentprofiles.Registry) {
	router.HandleFunc("/agent-profiles", listAgentProfilesHandler(registry)).Methods(http.MethodGet, http.MethodOptions)
	router.HandleFunc("/agent-profiles/validate", validateAgentProfileHandler()).Methods(http.MethodPost, http.MethodOptions)
	router.HandleFunc("/agent-profiles/{id}", getAgentProfileHandler(registry)).Methods(http.MethodGet, http.MethodOptions)
}

func (api *StreamingAPI) handleAgentProfileChatQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	decoder := json.NewDecoder(io.LimitReader(r.Body, maxAgentProfileRequestBytes))
	decoder.DisallowUnknownFields()
	var input AgentProfileChatRequest
	if err := decoder.Decode(&input); err != nil {
		writeAgentProfileError(w, http.StatusBadRequest, "invalid profile chat request: "+err.Error())
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeAgentProfileError(w, http.StatusBadRequest, "invalid profile chat request: expected one JSON object")
		return
	}
	if strings.TrimSpace(input.Message) == "" {
		writeAgentProfileError(w, http.StatusBadRequest, "message is required")
		return
	}
	if api.agentProfiles == nil {
		writeAgentProfileError(w, http.StatusServiceUnavailable, "agent profiles are unavailable")
		return
	}

	profileID := strings.TrimSpace(mux.Vars(r)["id"])
	profile, err := api.agentProfiles.Resolve(profileID, 0, GetUserIDFromContext(r.Context()))
	if err != nil {
		writeAgentProfileError(w, http.StatusNotFound, "agent profile not found")
		return
	}
	conversation, err := api.resolveAgentProfileConversation(r, profile, input.ConversationKey)
	if err != nil {
		writeAgentProfileError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	query, err := queryRequestForAgentProfileChat(profile, input, conversation)
	if err != nil {
		writeAgentProfileError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	encoded, err := json.Marshal(query)
	if err != nil {
		writeAgentProfileError(w, http.StatusInternalServerError, "encode profile chat request")
		return
	}

	// Preserve authentication, cancellation and X-Session-ID, but replace the
	// broad browser-authored QueryRequest with the server-authored profile turn.
	// This is the migration seam: product clients now have a stable minimal API;
	// the shared turn runner can be extracted from handleQuery behind this seam
	// without another frontend migration.
	forwarded := r.Clone(r.Context())
	forwarded.Body = io.NopCloser(bytes.NewReader(encoded))
	forwarded.ContentLength = int64(len(encoded))
	forwarded.Header = r.Header.Clone()
	forwarded.Header.Set("Content-Type", "application/json")
	forwarded.Header.Set("X-Session-ID", conversation.SessionID)
	api.handleQuery(w, forwarded)
}

func (api *StreamingAPI) handleResolveAgentProfileConversation(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxAgentProfileRequestBytes))
	decoder.DisallowUnknownFields()
	var input AgentProfileConversationRequest
	if err := decoder.Decode(&input); err != nil {
		writeAgentProfileError(w, http.StatusBadRequest, "invalid product conversation request: "+err.Error())
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeAgentProfileError(w, http.StatusBadRequest, "invalid product conversation request: expected one JSON object")
		return
	}
	if api.agentProfiles == nil {
		writeAgentProfileError(w, http.StatusServiceUnavailable, "agent profiles are unavailable")
		return
	}
	profile, err := api.agentProfiles.Resolve(strings.TrimSpace(mux.Vars(r)["id"]), 0, GetUserIDFromContext(r.Context()))
	if err != nil {
		writeAgentProfileError(w, http.StatusNotFound, "agent profile not found")
		return
	}
	conversation, err := api.resolveAgentProfileConversation(r, profile, input.ConversationKey)
	if err != nil {
		writeAgentProfileError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeAgentProfileJSON(w, http.StatusOK, AgentProfileConversationResponse{
		ConversationID:  conversation.ConversationID,
		ConversationKey: conversation.ConversationKey,
		SessionID:       conversation.SessionID,
	})
}

func (api *StreamingAPI) resolveAgentProfileConversation(r *http.Request, profile agentprofiles.Profile, requestedKey string) (ProductConversationRecord, error) {
	userID := GetUserIDFromContext(r.Context())
	if userID == "" {
		userID = "default"
	}
	binding, err := resolveProductConversationBinding(r.Context(), userID, profile, requestedKey)
	if err != nil {
		return ProductConversationRecord{}, err
	}
	preferredSessionID := ""
	if binding.AuthoritativeSessionID == "" {
		candidate := strings.TrimSpace(r.Header.Get("X-Session-ID"))
		if candidate != "" && api.canUseSessionIDForQuery(r, candidate) {
			if active, ok := api.getActiveSession(candidate); ok && (active.UserID == "" || active.UserID == userID) {
				preferredSessionID = candidate
			} else if _, found, findErr := FindChatHistoryConversationPathForSession(userID, candidate, ""); findErr != nil {
				return ProductConversationRecord{}, fmt.Errorf("find existing product conversation: %w", findErr)
			} else if found {
				preferredSessionID = candidate
			}
		}
	}
	record, err := defaultProductConversationRegistryStore().resolveOrCreate(r.Context(), userID, profile, binding, preferredSessionID)
	if err != nil {
		return ProductConversationRecord{}, err
	}
	if !api.canUseSessionIDForQuery(r, record.SessionID) {
		return ProductConversationRecord{}, fmt.Errorf("product conversation session belongs to another user")
	}
	return record, nil
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

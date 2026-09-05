package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// AuthMiddleware supplies the user; a binding additionally requires ownership
// of the live chat and current read access to its server-registered workspace.
// Shared read access to somebody else's workflow is NOT chat ownership.
func (api *StreamingAPI) handleUIControl(w http.ResponseWriter, r *http.Request) {
	session := mux.Vars(r)["session_id"]
	user := GetUserIDFromContext(r.Context())
	active, exists := api.getActiveSession(session)
	if !exists || user == "" || active.UserID != user {
		http.Error(w, "inactive_scope", http.StatusForbidden)
		return
	}
	b := api.uiBroker()
	workspace := b.scope(session)
	if !strings.HasPrefix(workspace, "Workflow/") {
		http.Error(w, "unsupported_surface", http.StatusConflict)
		return
	}
	level, manifest := workflowAccessForWorkspacePath(r.Context(), GetUserFromContext(r.Context()), workspace)
	if manifest == nil || level == WorkflowAccessNone || !userAllowedWorkflowID(GetUserFromContext(r.Context()), manifest.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Operation string     `json:"operation"`
		Version   int        `json:"version"`
		Binding   string     `json:"binding"`
		Token     string     `json:"token"`
		Request   string     `json:"request_id"`
		Status    string     `json:"status"`
		Code      string     `json:"code"`
		State     uiSnapshot `json:"state"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil {
		http.Error(w, "invalid_request", http.StatusBadRequest)
		return
	}
	if req.Version != uiControlContract.Version {
		http.Error(w, "version_mismatch", http.StatusConflict)
		return
	}
	var out interface{} = map[string]bool{"ok": true}
	var err error
	switch req.Operation {
	case "bind":
		var client *uiBinding
		client, err = b.bind(session)
		if err == nil {
			out = map[string]interface{}{"binding": client.id, "token": client.token, "workspace": workspace, "version": uiControlContract.Version}
		}
	case "sync":
		out, err = b.syncClient(session, req.Binding, req.Token, req.State)
	case "ack":
		err = b.ack(session, req.Binding, req.Token, req.Request, req.Status, req.Code, req.State)
	case "unbind":
		err = b.unbind(session, req.Binding, req.Token)
	default:
		http.Error(w, "unsupported_operation", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}

package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/services"
)

func (api *StreamingAPI) handleGetOrgDashboardNotifications(w http.ResponseWriter, r *http.Request) {
	if !setupCORS(w, r, http.MethodGet) {
		return
	}
	rawPaths := strings.TrimSpace(r.URL.Query().Get("workspace_paths"))
	if rawPaths == "" {
		http.Error(w, "workspace_paths is required", http.StatusBadRequest)
		return
	}
	parts := strings.Split(rawPaths, ",")
	if len(parts) > 200 {
		http.Error(w, "workspace_paths supports at most 200 workflows", http.StatusBadRequest)
		return
	}
	recentLimit := 10
	if raw := strings.TrimSpace(r.URL.Query().Get("recent_limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 50 {
			recentLimit = parsed
		}
	}
	results := make([]services.OrgDashboardWorkflowNotifications, 0, len(parts))
	for _, rawPath := range parts {
		workspacePath := strings.TrimSpace(rawPath)
		if workspacePath == "" {
			continue
		}
		item, err := services.ListOrgDashboardNotifications(r.Context(), workspacePath, recentLimit)
		if err != nil {
			item = services.OrgDashboardWorkflowNotifications{WorkspacePath: workspacePath, Error: err.Error()}
		}
		results = append(results, item)
	}
	writeAIJSON(w, map[string]interface{}{
		"success":   true,
		"workflows": results,
	})
}

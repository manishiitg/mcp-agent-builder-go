package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manishiitg/mcpagent/llm"
	llmproviders "github.com/manishiitg/multi-llm-provider-go"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/claudeauth"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/cursorauth"
)

const (
	claudeCodeProviderID = "claude-code"
	cursorCLIProviderID  = "cursor-cli"
)

// workflowCredentialProvider carries the handful of things that differ between
// one coding-CLI provider's stored credential and another's. Everything else —
// encryption, per-user/per-workflow scoping, the busy-session guard, the masked
// preview — is identical, and keeping it identical is the point: the Claude
// Code path is the one that has been through review, so Cursor inherits it
// rather than growing a parallel implementation that can drift.
type workflowCredentialProvider struct {
	id string
	// label names the credential in user-facing errors ("Claude Code token").
	label string
	// validate proves the credential works before it is stored. Skipping it
	// would turn a typo into a failed turn much later, with no hint why.
	validate func(context.Context, string) error
	// closeIdleSessions drops idle CLI sessions still holding the previous
	// credential, so the next turn picks up the new one.
	closeIdleSessions func(api *StreamingAPI, userID, workflowPath, reason string)
}

func claudeCodeCredentialProvider() workflowCredentialProvider {
	return workflowCredentialProvider{
		id:       claudeCodeProviderID,
		label:    "Claude Code token",
		validate: validateClaudeCodeOAuthToken,
		closeIdleSessions: func(api *StreamingAPI, userID, workflowPath, reason string) {
			api.closeIdleWorkflowClaudeCodeSessions(userID, workflowPath, reason)
		},
	}
}

func cursorCLICredentialProvider() workflowCredentialProvider {
	return workflowCredentialProvider{
		id:       cursorCLIProviderID,
		label:    "Cursor API key",
		validate: validateCursorCLIAPIKey,
		closeIdleSessions: func(api *StreamingAPI, userID, workflowPath, reason string) {
			api.closeIdleWorkflowCursorCLISessions(userID, workflowPath, reason)
		},
	}
}

type workflowProviderCredentialRequest struct {
	WorkspacePath  string `json:"workspace_path"`
	EncryptedValue string `json:"encrypted_value"`
}

type workflowProviderCredentialStatus struct {
	Configured bool       `json:"configured"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
	// Preview is a masked "first4...last4" rendering of the stored token, e.g.
	// "sk-a...DQAA" — never the full value. It exists purely so a user can
	// visually confirm which token is saved (distinguish it from another one,
	// notice a stale/expired one) without the full secret ever leaving the
	// server. Omitted whenever decryption fails or the token is too short to
	// mask meaningfully.
	Preview string `json:"preview,omitempty"`
}

// maskCredentialPreview renders "first4...last4" for a token, or "" when the
// token is too short to mask without revealing most of it.
func maskCredentialPreview(token string) string {
	const affixLen = 4
	if len(token) < affixLen*2+4 {
		return ""
	}
	return token[:affixLen] + "..." + token[len(token)-affixLen:]
}

func (api *StreamingAPI) handleGetWorkflowClaudeCodeCredential(w http.ResponseWriter, r *http.Request) {
	api.getWorkflowProviderCredential(w, r, claudeCodeCredentialProvider())
}

func (api *StreamingAPI) handleStoreWorkflowClaudeCodeCredential(w http.ResponseWriter, r *http.Request) {
	api.storeWorkflowProviderCredential(w, r, claudeCodeCredentialProvider())
}

func (api *StreamingAPI) handleDeleteWorkflowClaudeCodeCredential(w http.ResponseWriter, r *http.Request) {
	api.deleteWorkflowProviderCredential(w, r, claudeCodeCredentialProvider())
}

func (api *StreamingAPI) handleGetWorkflowCursorCLICredential(w http.ResponseWriter, r *http.Request) {
	api.getWorkflowProviderCredential(w, r, cursorCLICredentialProvider())
}

func (api *StreamingAPI) handleStoreWorkflowCursorCLICredential(w http.ResponseWriter, r *http.Request) {
	api.storeWorkflowProviderCredential(w, r, cursorCLICredentialProvider())
}

func (api *StreamingAPI) handleDeleteWorkflowCursorCLICredential(w http.ResponseWriter, r *http.Request) {
	api.deleteWorkflowProviderCredential(w, r, cursorCLICredentialProvider())
}

func (api *StreamingAPI) getWorkflowProviderCredential(w http.ResponseWriter, r *http.Request, provider workflowCredentialProvider) {
	workspacePath := strings.TrimSpace(r.URL.Query().Get("workspace_path"))
	if workspacePath == "" {
		http.Error(w, "workspace_path is required", http.StatusBadRequest)
		return
	}
	userID := GetUserIDFromContext(r.Context())
	credential, err := api.chatStore.GetWorkflowProviderCredential(r.Context(), userID, workspacePath, provider.id)
	if err != nil {
		http.Error(w, "Failed to load workflow provider credential", http.StatusInternalServerError)
		return
	}
	status := workflowProviderCredentialStatus{Configured: credential != nil}
	if credential != nil {
		updatedAt := credential.UpdatedAt
		status.UpdatedAt = &updatedAt
		// Decrypted only to build the masked preview; the plaintext is never
		// stored, logged, or returned — only the two short affixes below.
		if token, decErr := decryptSecretValue(credential.EncryptedValue, userID); decErr == nil {
			status.Preview = maskCredentialPreview(token)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (api *StreamingAPI) storeWorkflowProviderCredential(w http.ResponseWriter, r *http.Request, provider workflowCredentialProvider) {
	var req workflowProviderCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.WorkspacePath = strings.TrimSpace(req.WorkspacePath)
	req.EncryptedValue = strings.TrimSpace(req.EncryptedValue)
	if req.WorkspacePath == "" || req.EncryptedValue == "" {
		http.Error(w, "workspace_path and encrypted_value are required", http.StatusBadRequest)
		return
	}

	userID := GetUserIDFromContext(r.Context())
	if api.workflowHasBusySession(userID, req.WorkspacePath) {
		http.Error(w, fmt.Sprintf("Stop the active workflow before changing its %s", provider.label), http.StatusConflict)
		return
	}
	token, err := decryptSecretValue(req.EncryptedValue, userID)
	if err != nil {
		http.Error(w, "Invalid encrypted credential", http.StatusBadRequest)
		return
	}
	if err := provider.validate(r.Context(), token); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := api.chatStore.UpsertWorkflowProviderCredential(r.Context(), userID, req.WorkspacePath, provider.id, req.EncryptedValue); err != nil {
		http.Error(w, "Failed to store workflow provider credential", http.StatusInternalServerError)
		return
	}
	provider.closeIdleSessions(api, userID, req.WorkspacePath, fmt.Sprintf("workflow %s changed", provider.label))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (api *StreamingAPI) deleteWorkflowProviderCredential(w http.ResponseWriter, r *http.Request, provider workflowCredentialProvider) {
	workspacePath := strings.TrimSpace(r.URL.Query().Get("workspace_path"))
	if workspacePath == "" {
		http.Error(w, "workspace_path is required", http.StatusBadRequest)
		return
	}
	userID := GetUserIDFromContext(r.Context())
	if api.workflowHasBusySession(userID, workspacePath) {
		http.Error(w, fmt.Sprintf("Stop the active workflow before removing its %s", provider.label), http.StatusConflict)
		return
	}
	if err := api.chatStore.DeleteWorkflowProviderCredential(r.Context(), userID, workspacePath, provider.id); err != nil {
		http.Error(w, "Failed to delete workflow provider credential", http.StatusInternalServerError)
		return
	}
	provider.closeIdleSessions(api, userID, workspacePath, fmt.Sprintf("workflow %s removed", provider.label))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// validateClaudeCodeOAuthToken defers to the shared checker so workflows and
// Video Studio cannot drift apart on what counts as a valid token.
func validateClaudeCodeOAuthToken(parent context.Context, token string) error {
	return claudeauth.ValidateOAuthToken(parent, token)
}

// validateCursorCLIAPIKey mirrors validateClaudeCodeOAuthToken: Cursor has no
// `setup-token` equivalent, so the pasted dashboard key is checked against the
// CLI before it is stored.
func validateCursorCLIAPIKey(parent context.Context, key string) error {
	return cursorauth.ValidateAPIKey(parent, key)
}

func (api *StreamingAPI) loadWorkflowClaudeCodeOAuthToken(ctx context.Context, userID, workflowPath string) (*string, error) {
	return api.loadWorkflowProviderSecret(ctx, userID, workflowPath, claudeCodeProviderID, "Claude Code")
}

func (api *StreamingAPI) loadWorkflowCursorCLIAPIKey(ctx context.Context, userID, workflowPath string) (*string, error) {
	return api.loadWorkflowProviderSecret(ctx, userID, workflowPath, cursorCLIProviderID, "Cursor")
}

func (api *StreamingAPI) loadWorkflowProviderSecret(ctx context.Context, userID, workflowPath, providerID, label string) (*string, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(workflowPath) == "" {
		return nil, nil
	}
	credential, err := api.chatStore.GetWorkflowProviderCredential(ctx, userID, workflowPath, providerID)
	if err != nil || credential == nil {
		return nil, err
	}
	token, err := decryptSecretValue(credential.EncryptedValue, userID)
	if err != nil {
		return nil, fmt.Errorf("decrypt workflow %s credential: %w", label, err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	return &token, nil
}

// workflowProviderAPIKeys clones the normal provider configuration and adds
// private credentials for exactly one user/workflow. It is the only Builder
// path allowed to populate ClaudeCodeOAuthToken or a workflow-scoped CursorCLI
// key.
func (api *StreamingAPI) workflowProviderAPIKeys(ctx context.Context, userID, workflowPath string, base *llm.ProviderAPIKeys) (*llm.ProviderAPIKeys, error) {
	keys := base.Clone()
	if keys == nil {
		keys = &llm.ProviderAPIKeys{}
	}
	token, err := api.loadWorkflowClaudeCodeOAuthToken(ctx, userID, workflowPath)
	if err != nil {
		// Preserve unrelated provider credentials while failing closed for the
		// workflow token itself. Callers can then decide whether this workflow
		// operation should stop or continue with Claude Code's saved login.
		keys.ClaudeCodeOAuthToken = nil
		return keys, err
	}
	keys.ClaudeCodeOAuthToken = token

	// Cursor differs from Claude Code in one way that matters here: the base
	// keys may already carry a server-wide CURSOR_API_KEY. A workflow-scoped
	// key overrides it, but its absence must leave the shared key intact —
	// clearing it would break every workflow that relies on the server default.
	cursorKey, cursorErr := api.loadWorkflowCursorCLIAPIKey(ctx, userID, workflowPath)
	if cursorErr != nil {
		// Fail closed for this provider only: an unreadable scoped key must not
		// silently downgrade to the shared account.
		keys.CursorCLI = nil
		return keys, cursorErr
	}
	if cursorKey != nil {
		keys.CursorCLI = cursorKey
	}
	return keys, nil
}

func normalizeWorkflowCredentialPath(workflowPath string) string {
	return strings.Trim(strings.ReplaceAll(strings.TrimSpace(workflowPath), "\\", "/"), "/")
}

func (api *StreamingAPI) workflowSessionIDs(userID, workflowPath string) []string {
	wantedUser := strings.TrimSpace(userID)
	wanted := normalizeWorkflowCredentialPath(workflowPath)
	if wantedUser == "" || wanted == "" {
		return nil
	}
	api.activeSessionsMux.RLock()
	defer api.activeSessionsMux.RUnlock()
	ids := make([]string, 0)
	for sessionID, session := range api.activeSessions {
		if session == nil || strings.TrimSpace(session.UserID) != wantedUser || normalizeWorkflowCredentialPath(session.WorkspacePath) != wanted {
			continue
		}
		ids = append(ids, sessionID)
	}
	return ids
}

func (api *StreamingAPI) workflowHasBusySession(userID, workflowPath string) bool {
	for _, sessionID := range api.workflowSessionIDs(userID, workflowPath) {
		hasBackgroundAgents := api.bgAgentRegistry != nil && api.bgAgentRegistry.HasRunningAgents(sessionID)
		if api.isSessionBusy(sessionID) || api.canSteerSession(sessionID) || api.hasRunningTrackedExecutionForSession(sessionID) || hasBackgroundAgents {
			return true
		}
	}
	return false
}

func (api *StreamingAPI) closeIdleWorkflowClaudeCodeSessions(userID, workflowPath, reason string) {
	for _, sessionID := range api.workflowSessionIDs(userID, workflowPath) {
		llmproviders.CloseClaudeCodeInteractiveSessionForOwner(sessionID, reason)
		if api.terminalStore == nil {
			continue
		}
		for _, snapshot := range api.terminalStore.ListRaw(sessionID) {
			if !strings.HasPrefix(strings.TrimSpace(snapshot.TmuxSession), "mlp-claude-code") {
				continue
			}
			llmproviders.CloseClaudeCodeInteractiveSessionByTmux(snapshot.TmuxSession, reason)
			api.terminalStore.MarkProcessClosed(snapshot.TerminalID, reason)
		}
	}
}

// closeIdleWorkflowCursorCLISessions is the Cursor counterpart. Video Studio
// drives Cursor over the structured (-p) transport, which starts a fresh
// process per turn and so picks the new key up on its own; this covers the
// interactive tmux sessions, which outlive a turn and would otherwise keep
// running under the replaced credential.
func (api *StreamingAPI) closeIdleWorkflowCursorCLISessions(userID, workflowPath, reason string) {
	for _, sessionID := range api.workflowSessionIDs(userID, workflowPath) {
		llmproviders.CloseCursorCLIInteractiveSessionForOwner(sessionID, reason)
		if api.terminalStore == nil {
			continue
		}
		for _, snapshot := range api.terminalStore.ListRaw(sessionID) {
			if !strings.HasPrefix(strings.TrimSpace(snapshot.TmuxSession), "mlp-cursor") {
				continue
			}
			llmproviders.CloseCursorCLIInteractiveSessionByTmux(snapshot.TmuxSession, reason)
			api.terminalStore.MarkProcessClosed(snapshot.TerminalID, reason)
		}
	}
}

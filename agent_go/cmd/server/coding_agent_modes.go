package server

import (
	"path/filepath"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"

	"github.com/manishiitg/mcpagent/llm"
)

func codingAgentPersistentInteractiveFlags(provider string, allowPersistentInteractive bool) (claudeCode bool, codexCLI bool, cursorCLI bool, piCLI bool) {
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	if !allowPersistentInteractive ||
		!llm.IsTmuxCodingAgentProvider(llm.Provider(normalizedProvider), "") {
		return false, false, false, false
	}

	switch normalizedProvider {
	case strings.ToLower(string(llm.ProviderClaudeCode)):
		return true, false, false, false
	case strings.ToLower(string(llm.ProviderCodexCLI)):
		return false, true, false, false
	case strings.ToLower(string(llm.ProviderCursorCLI)):
		return false, false, true, false
	case strings.ToLower(string(llm.ProviderPiCLI)):
		return false, false, false, true
	default:
		return false, false, false, false
	}
}

func codingAgentRequestAllowsPersistentInteractive(req *QueryRequest, sessionID string) bool {
	if req == nil {
		return false
	}
	// Backend-created children and typed runtime stages have durable outputs and
	// completion notifications, not a user who can continue their native CLI.
	if strings.TrimSpace(req.ParentSessionID) != "" || strings.TrimSpace(req.SessionKind) != "" || req.IsAutoNotification {
		return false
	}
	// "Make interactive" deliberately keeps the schedule session ID. This
	// explicit promotion therefore outranks its historical trigger/ID shape.
	if req.UserInteractiveContinuation {
		return true
	}
	// Workflow Builder chats are represented internally as workflow_phase, but
	// they are still ordinary user-interactive main chats. Classify by origin
	// and ownership instead of agent mode so their conversation tmux survives.
	return !isScheduledSessionIdentity(sessionID, req.TriggeredBy)
}

func codingAgentClaudeCodeChatTransport(provider string) string {
	if strings.ToLower(strings.TrimSpace(provider)) == strings.ToLower(string(llm.ProviderClaudeCode)) {
		return llm.ClaudeCodeTransportTmux
	}
	return ""
}

func codingAgentWorkspaceWorkingDir(workspaceRelativeFolder string) string {
	rel := strings.TrimSpace(workspaceRelativeFolder)
	if rel == "" {
		rel = perUserChatsFolderFor("")
	}
	return filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(rel))
}

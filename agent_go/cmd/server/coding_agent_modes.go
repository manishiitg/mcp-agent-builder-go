package server

import (
	"path/filepath"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"

	"github.com/manishiitg/mcpagent/llm"
)

func codingAgentPersistentInteractiveFlags(provider string) (claudeCode bool, codexCLI bool, cursorCLI bool, piCLI bool) {
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	if codingAgentUsesStructuredTransport(normalizedProvider) {
		return false, false, false, false
	}
	if !llm.IsTmuxCodingAgentProvider(llm.Provider(normalizedProvider), "") {
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

// codingAgentUsesStructuredTransport is the product-level default for coding
// providers whose native CLI JSON protocol is more reliable than terminal UI
// automation. Cursor conversations retain continuity with the provider's
// native --resume session ID; they do not use a persistent tmux pane.
func codingAgentUsesStructuredTransport(provider string) bool {
	return strings.EqualFold(strings.TrimSpace(provider), string(llm.ProviderCursorCLI))
}

// codingAgentUsesStructuredTransportForPolicy resolves the product/profile
// transport setting before the legacy provider default. A profile's explicit
// policy is authoritative; "auto" retains the shared application default.
func codingAgentUsesStructuredTransportForPolicy(provider, policy string) bool {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "structured":
		return true
	case "tmux":
		return false
	default:
		return codingAgentUsesStructuredTransport(provider)
	}
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

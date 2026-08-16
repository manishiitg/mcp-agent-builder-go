package server

import (
	"fmt"
	"strings"
	"time"

	mcpagent "github.com/manishiitg/mcpagent/agent"
	"github.com/manishiitg/mcpagent/llm"
	"github.com/manishiitg/multi-llm-provider-go/pkg/adapters/claudecode"
	"github.com/manishiitg/multi-llm-provider-go/pkg/adapters/codexcli"
)

// diagnoseAndCleanupStalledConversationTurn runs once waitForConversationTurnTree's
// idle-wait has already given up on a turn (PLAT-116, reproduced live on a
// social-media contract-upgrade turn: Codex genuinely completed and stamped
// its work two minutes in, but the platform's own bridge from that
// completion back to its caller never observed it, and the turn sat orphaned
// until this 10-minute safety net gave up 17+ minutes after the real work
// had already succeeded). Two independent things happen here:
//
//  1. Diagnosis: ask the coding-agent provider's OWN completion signal
//     (Codex's task_complete, Claude Code's committed end_turn) whether the
//     turn actually finished after the platform stopped observing progress.
//     That signal already IS the source of truth interactive completion
//     trusts (PLAT-108) — this reuses it purely as a post-hoc diagnostic,
//     never as a live control-flow decision. A wrong or missing read only
//     makes the error message less precise; it can never claim a stuck turn
//     succeeded, since the platform genuinely never processed its result
//     (history, persistence, and downstream steps never ran for it either
//     way).
//  2. Cleanup: regardless of what the diagnosis finds, the session this turn
//     was running in is now orphaned — the goroutine that owns its
//     active-session tracking, HTTP session, and browser resources is
//     permanently blocked on the provider response and will never run its
//     own completion cleanup. Tear all of that down here so the session
//     stops presenting as "running" forever, the exact symptom PLAT-116
//     reproduced live (a browser tab still polling a session's terminal
//     minutes after the underlying tmux pane was already gone).
func (api *StreamingAPI) diagnoseAndCleanupStalledConversationTurn(sessionID, rootExecutionID string, since time.Time, baseErr error) error {
	if api == nil || baseErr == nil {
		return baseErr
	}
	sessionID = strings.TrimSpace(sessionID)
	if evidence := api.diagnoseStalledConversationTurn(sessionID, since); evidence != "" {
		baseErr = fmt.Errorf("%w; provider-side completion evidence: %s", baseErr, evidence)
	}
	api.cleanupOrphanedConversationTurnSession(sessionID, rootExecutionID)
	return baseErr
}

// diagnoseStalledConversationTurn is a best-effort read, not a live
// completion oracle. It reads api.runningAgents without releasing/awaiting
// anything the stalled goroutine holds, and every provider lookup below is
// the same passive file read the interactive adapters already treat as safe
// to run concurrently with a live turn.
func (api *StreamingAPI) diagnoseStalledConversationTurn(sessionID string, since time.Time) string {
	if api == nil || sessionID == "" {
		return ""
	}
	api.runningAgentsMux.RLock()
	agent := api.runningAgents[sessionID]
	api.runningAgentsMux.RUnlock()
	if agent == nil {
		return ""
	}
	handle := mcpagent.SnapshotAgentSession(agent)
	if handle == nil || handle.Provider.Empty() {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(handle.Provider.Provider))
	workingDir := strings.TrimSpace(handle.Provider.WorkingDir)
	nativeSessionID := strings.TrimSpace(handle.Provider.NativeSessionID)

	switch provider {
	case strings.ToLower(string(llm.ProviderCodexCLI)):
		if workingDir == "" {
			return ""
		}
		found, completedAt, message := codexcli.DiagnoseTurnCompletion(workingDir, since)
		if !found {
			return ""
		}
		message = strings.TrimSpace(message)
		if message == "" {
			message = "(no final message recorded)"
		}
		return fmt.Sprintf(
			"Codex CLI's own rollout shows this turn completed at %s: %q — the platform's bridge never observed it (PLAT-116)",
			completedAt.Format(time.RFC3339), message,
		)
	case strings.ToLower(string(llm.ProviderClaudeCode)):
		if nativeSessionID == "" {
			return ""
		}
		found, text := claudecode.DiagnoseTurnCompletion(nativeSessionID, workingDir, since)
		if !found {
			return ""
		}
		text = strings.TrimSpace(text)
		if text == "" {
			text = "(no final message recorded)"
		}
		return fmt.Sprintf(
			"Claude Code's own transcript shows this turn committed a final response: %q — the platform's bridge never observed it (PLAT-116)",
			text,
		)
	default:
		// Pi CLI's marker file has no OS-standard location to read from outside
		// its live session registry (no export exists yet); Cursor's default
		// transport never reaches this interactive wait path at all.
		return ""
	}
}

// cleanupOrphanedConversationTurnSession mirrors the teardown the normal
// completion path runs (server.go's StreamWithEvents goroutine, ~6600-6620
// and ~4090-4110) — status, query index, MCP HTTP session, and browser
// resources. Idempotent: every step here is safe to run again if the
// orphaned goroutine ever does unblock and runs its own cleanup afterward.
func (api *StreamingAPI) cleanupOrphanedConversationTurnSession(sessionID, rootExecutionID string) {
	if api == nil || sessionID == "" {
		return
	}
	api.updateSessionStatus(sessionID, "error")
	api.removeSessionQueryID(sessionID, strings.TrimSpace(rootExecutionID))
	mcpagent.CloseHTTPSession(sessionID)
	api.cleanupBrowserSessions(sessionID)
}

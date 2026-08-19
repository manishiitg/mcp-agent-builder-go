package server

import (
	"strings"
	"time"

	storeevents "github.com/manishiitg/coding-agent-loop/agent_go/internal/events"
	mcpagent "github.com/manishiitg/mcpagent/agent"
	"github.com/manishiitg/mcpagent/llm"
	"github.com/manishiitg/multi-llm-provider-go/pkg/adapters/claudecode"
)

// installToolResultRecovery lets the event store recover tool calls the live
// stream never completed.
//
// PLAT-141: some tool calls produce no tool_call_end at all — one measured
// completing in 41ms with no end event under any session — so the chat showed a
// finished command as unresolved. The provider keeps its own complete record
// (the same session's transcript holds 215 tool_use and 215 tool_result), and
// this reads the real output and runtime back out of it.
//
// Wired here rather than inside the event store so that package keeps no
// provider knowledge: it asks for a result, this decides how to find one.
func (api *StreamingAPI) installToolResultRecovery() {
	storeevents.SetToolResultResolver(api.recoverToolResult)
}

// recoverToolResult resolves one tool call against its session's provider
// record. Returns ok=false whenever the answer would be a guess.
func (api *StreamingAPI) recoverToolResult(sessionID, toolCallID string) (string, time.Duration, bool) {
	if api == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(toolCallID) == "" {
		return "", 0, false
	}
	api.runningAgentsMux.RLock()
	agent := api.runningAgents[sessionID]
	api.runningAgentsMux.RUnlock()
	if agent == nil {
		// The session has already been torn down. Its transcript still exists,
		// but nothing here knows which one it is — the native id lives on the
		// live handle.
		return "", 0, false
	}
	handle := mcpagent.SnapshotAgentSession(agent)
	if handle == nil || handle.Provider.Empty() {
		return "", 0, false
	}
	provider := strings.ToLower(strings.TrimSpace(handle.Provider.Provider))
	if provider != strings.ToLower(string(llm.ProviderClaudeCode)) && provider != "claudecode" {
		// Only Claude Code's transcript shape is understood here. Other
		// providers fall back to an empty settle rather than a wrong one.
		return "", 0, false
	}
	nativeSessionID := strings.TrimSpace(handle.Provider.NativeSessionID)
	workingDir := strings.TrimSpace(handle.Provider.WorkingDir)
	if nativeSessionID == "" {
		return "", 0, false
	}
	entry, ok := claudecode.ToolResultsFromTranscript(nativeSessionID, workingDir)[toolCallID]
	if !ok || strings.TrimSpace(entry.Result) == "" {
		return "", 0, false
	}
	return entry.Result, entry.Duration(), true
}

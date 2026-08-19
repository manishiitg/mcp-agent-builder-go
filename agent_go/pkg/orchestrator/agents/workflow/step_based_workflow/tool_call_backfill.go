package step_based_workflow

import (
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents"
	"github.com/manishiitg/multi-llm-provider-go/pkg/adapters/claudecode"
)

// backfillMissingToolResults recovers a step's orphaned tool calls from Claude
// Code's own transcript before they are written into the durable
// execution-attempt-*.json evidence Pulse reads.
//
// PLAT-142. No interactive (tmux) adapter emits a tool-call-end event, on any
// provider — a terminal pane carries no structured tool events, so there is
// nothing to emit one from. Measured across 745 real execution logs: 15,601
// tool calls, 2,563 (16.4%) with no recorded result, up to 88% on some
// workflows. This is not a display gap; it is Pulse reviewing evidence with
// holes in it.
//
// A same-shaped recovery already exists for the chat UI
// (internal/events.EventStore's settle, keyed by SetToolResultResolver). This
// is deliberately NOT a second implementation of transcript reading — it calls
// the exact same claudecode.ToolResultsFromTranscript the UI path uses, at the
// one place in this package that already holds both the step's raw tool_calls
// slice and its executing agent's session handle, right before that slice is
// serialized. The ideal fix is one reconciliation at mcpagent's event fan-out
// point (Agent.emitTypedEvent), reaching every consumer identically instead of
// once per consumer; that is cross-repo, touches the coding-agent turn
// lifecycle, and needs a live run to verify, so it is recorded as follow-up
// rather than attempted blind here.
func backfillMissingToolResults(toolCalls []orchestrator.ToolCallEntry, executionAgent agents.OrchestratorAgent) {
	if len(toolCalls) == 0 || executionAgent == nil {
		return
	}
	base := executionAgent.GetBaseAgent()
	if base == nil {
		return
	}
	handle := base.SessionHandle()
	if handle == nil || handle.Provider.Empty() {
		return
	}
	provider := strings.ToLower(strings.TrimSpace(handle.Provider.Provider))
	if provider != "claude-code" && provider != "claudecode" {
		// Only Claude Code's transcript shape is understood here. The other
		// interactive providers share the same underlying gap and are recorded
		// as follow-up in PLAT-142, not silently assumed fixed.
		return
	}
	nativeSessionID := strings.TrimSpace(handle.Provider.NativeSessionID)
	if nativeSessionID == "" {
		return
	}
	workingDir := strings.TrimSpace(handle.Provider.WorkingDir)

	var recovered map[string]claudecode.TranscriptToolResult
	for i := range toolCalls {
		entry := &toolCalls[i]
		if !toolCallEntryLooksOrphaned(*entry) {
			continue
		}
		if recovered == nil {
			recovered = claudecode.ToolResultsFromTranscript(nativeSessionID, workingDir)
		}
		found, ok := recovered[entry.ToolCallID]
		if !ok {
			continue
		}
		entry.Result = found.Result
		entry.Duration = found.Duration()
		if !found.EndedAt.IsZero() {
			entry.CompletedAt = found.EndedAt
		}
	}
}

// toolCallEntryLooksOrphaned reports whether an entry is Start-with-no-End —
// the shape a missing tool_call_end event leaves behind — rather than a call
// that genuinely failed or genuinely returned an empty string. Only that exact
// shape is safe to overwrite; anything else already has a real outcome.
func toolCallEntryLooksOrphaned(entry orchestrator.ToolCallEntry) bool {
	return entry.ToolCallID != "" && entry.Result == "" && entry.Error == "" && entry.Duration == 0
}

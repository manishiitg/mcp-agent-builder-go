package step_based_workflow

import (
	"testing"
	"time"

	"github.com/manishiitg/mcpagent/toolcalllog"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
)

// TestOrphanedEntryIsTheOnlyShapeTouched is the guard on the whole recovery: a
// call that genuinely failed, or genuinely returned an empty string, already
// has a real outcome, and overwriting it would replace true evidence with a
// guess. Only Start-with-nothing-at-all is safe to backfill.
func TestOrphanedEntryIsTheOnlyShapeTouched(t *testing.T) {
	cases := []struct {
		name   string
		entry  orchestrator.ToolCallEntry
		orphan bool
	}{
		{"start with nothing at all", orchestrator.ToolCallEntry{ToolCallID: "a"}, true},
		{"has a result", orchestrator.ToolCallEntry{ToolCallID: "a", Result: "ok"}, false},
		{"has an error", orchestrator.ToolCallEntry{ToolCallID: "a", Error: "boom"}, false},
		{"has a duration but no result (fast empty-output call)", orchestrator.ToolCallEntry{ToolCallID: "a", Duration: time.Millisecond}, false},
		{"no id at all", orchestrator.ToolCallEntry{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := toolCallEntryLooksOrphaned(c.entry); got != c.orphan {
				t.Errorf("orphaned = %v, want %v", got, c.orphan)
			}
		})
	}
}

// TestBackfillDoesNothingWithoutALiveClaudeCodeHandle. Every unanswerable path
// must be silent: no agent, no session handle, or a non-Claude-Code provider,
// since PLAT-142 explicitly leaves the other interactive providers as a named
// follow-up rather than guessing at their transcript shape.
func TestBackfillDoesNothingWithoutALiveClaudeCodeHandle(t *testing.T) {
	calls := []orchestrator.ToolCallEntry{{ToolCallID: "a"}}
	backfillMissingToolResults("some-session", calls, nil)
	if calls[0].Result != "" {
		t.Error("a nil executionAgent produced a result out of nowhere")
	}
	backfillMissingToolResults("some-session", nil, nil)
}

// TestBackfillRecoversFromToolcalllogWithoutNeedingAnAgentAtAll is PLAT-143:
// toolcalllog is keyed by the same session id the orphaned entry already
// carries, so a nil executionAgent — no live handle, no transcript needed —
// must not block this source the way it blocks the Claude-transcript fallback.
func TestBackfillRecoversFromToolcalllogWithoutNeedingAnAgentAtAll(t *testing.T) {
	const sessionID = "backfill-toolcalllog-test"
	toolcalllog.Clear(sessionID)
	t.Cleanup(func() { toolcalllog.Clear(sessionID) })

	started := time.Now()
	id := toolcalllog.RecordStart(sessionID, "execute_shell_command", `{}`)
	toolcalllog.RecordEnd(sessionID, id, "execute_shell_command", `{}`, "real output", started)

	calls := []orchestrator.ToolCallEntry{{
		ToolCallID: "toolu_01someRealClaudeID", ToolName: "execute_shell_command", StartedAt: started,
	}}
	backfillMissingToolResults(sessionID, calls, nil)

	if calls[0].Result != "real output" {
		t.Errorf("result = %q, want the toolcalllog-recovered output, no agent handle needed", calls[0].Result)
	}
}

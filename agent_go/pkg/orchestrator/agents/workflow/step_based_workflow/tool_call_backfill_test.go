package step_based_workflow

import (
	"testing"
	"time"

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
	backfillMissingToolResults(calls, nil)
	if calls[0].Result != "" {
		t.Error("a nil executionAgent produced a result out of nowhere")
	}
	backfillMissingToolResults(nil, nil)
}

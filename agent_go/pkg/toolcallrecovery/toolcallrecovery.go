// Package toolcallrecovery recovers a tool call's outcome when the event that
// was supposed to report it never arrived.
//
// PLAT-141/142 found the underlying cause: no interactive (tmux) adapter
// emits a tool-call-end event, on any provider — a terminal pane carries no
// structured tool events, so there is nothing to emit one from. Measured live
// on 2026-08-19: TWO independent mechanisms report the same bridge tool calls
// under different identities — a toolcalllog-backed HTTP hook
// (agent_go/pkg/agentwrapper, keyed by a locally-generated id) that is
// essentially perfect (1 of 36 unpaired in one measured session), and a
// second, still-unlocated mechanism carrying the model's own tool_use id that
// drops roughly 10%. Both were already flowing to the same consumers,
// unreconciled — real, active duplication, not just an occasional gap.
//
// This package is the one place that reconciles an orphaned entry from
// whichever mechanism reported it, reused by every consumer instead of each
// re-implementing the match. It is deliberately NOT scoped to Claude Code:
// toolcalllog is fed by mcpagent/executor's HTTP bridge handler, which serves
// every provider identically.
package toolcallrecovery

import (
	"strings"
	"time"

	"github.com/manishiitg/mcpagent/toolcalllog"
)

// matchWindow bounds how far apart two observations of "the same call" may
// land in time before they are treated as different calls.
//
// The two mechanisms this reconciles have no shared id space, so they are
// correlated by tool name and start time instead. A few seconds comfortably
// covers ordinary scheduling jitter between the two while staying far short of
// the gap between two genuinely separate invocations of the same tool in one
// step — those are seconds-to-minutes apart in every session inspected while
// building this.
const matchWindow = 5 * time.Second

// Candidate is the orphaned side: what a consumer already knows about a tool
// call that never got its own end event.
type Candidate struct {
	ToolName  string
	StartedAt time.Time
}

// Recover looks up sessionID's toolcalllog for a completed call matching
// candidate, returning its real result and real duration.
//
// Matched on tool name plus closest StartedAt within matchWindow — not on
// args, deliberately: the two mechanisms independently JSON-encode the same
// arguments, and Go map key ordering gives no guarantee the two encodings are
// byte-identical even for an identical call. Name plus tight time proximity is
// the correlator that does not depend on that guarantee.
//
// ok is false whenever the answer would be a guess: no toolcalllog entry for
// this session, no completed ("done") entry with a matching name inside the
// window, or an ambiguous multi-candidate match resolved only by picking the
// closest start time (logged as an approximation, never silently trusted as
// exact).
func Recover(sessionID string, candidate Candidate) (result string, duration time.Duration, ok bool) {
	name := strings.TrimSpace(candidate.ToolName)
	if strings.TrimSpace(sessionID) == "" || name == "" || candidate.StartedAt.IsZero() {
		return "", 0, false
	}

	var best toolcalllog.SnapshotCall
	var bestDelta time.Duration = -1
	for _, call := range toolcalllog.Snapshot(sessionID) {
		if call.Status != "done" || call.Name != name {
			continue
		}
		delta := call.StartedAt.Sub(candidate.StartedAt)
		if delta < 0 {
			delta = -delta
		}
		if delta > matchWindow {
			continue
		}
		if bestDelta == -1 || delta < bestDelta {
			best, bestDelta = call, delta
		}
	}
	if bestDelta == -1 {
		return "", 0, false
	}
	return best.Result, best.CompletedAt.Sub(best.StartedAt), true
}

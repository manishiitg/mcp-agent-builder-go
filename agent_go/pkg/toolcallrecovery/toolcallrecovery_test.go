package toolcallrecovery

import (
	"testing"
	"time"

	"github.com/manishiitg/mcpagent/toolcalllog"
)

// TestRecoverMatchesByNameAndTimeNotByID is the reason this package exists:
// the two mechanisms it reconciles between have no shared id space, so the
// correlator has to be name plus proximity in time.
func TestRecoverMatchesByNameAndTimeNotByID(t *testing.T) {
	const sessionID = "recovery-test-basic"
	toolcalllog.Clear(sessionID)
	t.Cleanup(func() { toolcalllog.Clear(sessionID) })

	logID := toolcalllog.RecordStart(sessionID, "execute_shell_command", `{"command":"ls"}`)
	started := time.Now()
	time.Sleep(5 * time.Millisecond)
	toolcalllog.RecordEnd(sessionID, logID, "execute_shell_command", `{"command":"ls"}`, "file1\nfile2", started)

	// The orphaned side names a different, unrelated id — that is the whole
	// point: toolcalllog's own id (a small local counter) is never the same
	// value as the model's real tool_use id.
	result, duration, ok := Recover(sessionID, Candidate{ToolName: "execute_shell_command", StartedAt: started})
	if !ok {
		t.Fatal("did not recover a call that toolcalllog actually completed")
	}
	if result != "file1\nfile2" {
		t.Errorf("result = %q, want the real recorded output", result)
	}
	if duration <= 0 {
		t.Errorf("duration = %v, want a real positive runtime", duration)
	}
}

// TestRecoverIgnoresAToolThatIsStillRunning. A "running" entry has not
// finished yet; returning it would present an incomplete or absent result as
// if it were the tool's own answer.
func TestRecoverIgnoresAToolThatIsStillRunning(t *testing.T) {
	const sessionID = "recovery-test-running"
	toolcalllog.Clear(sessionID)
	t.Cleanup(func() { toolcalllog.Clear(sessionID) })

	started := time.Now()
	toolcalllog.RecordStart(sessionID, "execute_shell_command", `{}`)

	if _, _, ok := Recover(sessionID, Candidate{ToolName: "execute_shell_command", StartedAt: started}); ok {
		t.Error("recovered a call that toolcalllog still shows as running")
	}
}

// TestRecoverRejectsAMatchOutsideTheWindow. A tool call minutes away in time
// is a different invocation of the same tool, not the one being asked about —
// name-only matching without a time bound would silently attribute the wrong
// call's output.
func TestRecoverRejectsAMatchOutsideTheWindow(t *testing.T) {
	const sessionID = "recovery-test-far"
	toolcalllog.Clear(sessionID)
	t.Cleanup(func() { toolcalllog.Clear(sessionID) })

	far := time.Now().Add(-time.Hour)
	logID := toolcalllog.RecordStart(sessionID, "execute_shell_command", `{}`)
	toolcalllog.RecordEnd(sessionID, logID, "execute_shell_command", `{}`, "stale output", far)

	if _, _, ok := Recover(sessionID, Candidate{ToolName: "execute_shell_command", StartedAt: time.Now()}); ok {
		t.Error("matched a call an hour away in time")
	}
}

// TestRecoverPicksTheClosestOfSeveralCandidates. Two calls to the same tool in
// one session is ordinary (a step running execute_shell_command twice); the
// closer one in time is the more defensible match, not the first or last in
// registration order.
func TestRecoverPicksTheClosestOfSeveralCandidates(t *testing.T) {
	const sessionID = "recovery-test-closest"
	toolcalllog.Clear(sessionID)
	t.Cleanup(func() { toolcalllog.Clear(sessionID) })

	base := time.Now()
	early := base.Add(-3 * time.Second)
	near := base.Add(-1 * time.Second)

	id1 := toolcalllog.RecordStart(sessionID, "execute_shell_command", `{}`)
	toolcalllog.RecordEnd(sessionID, id1, "execute_shell_command", `{}`, "early output", early)
	id2 := toolcalllog.RecordStart(sessionID, "execute_shell_command", `{}`)
	toolcalllog.RecordEnd(sessionID, id2, "execute_shell_command", `{}`, "near output", near)

	result, _, ok := Recover(sessionID, Candidate{ToolName: "execute_shell_command", StartedAt: base})
	if !ok {
		t.Fatal("expected a match among the two candidates")
	}
	if result != "near output" {
		t.Errorf("result = %q, want the closer candidate's output", result)
	}
}

// TestRecoverIsSilentWhenItCannotAnswer. Every unanswerable path must return
// ok=false rather than a guess: an empty session id, an empty tool name, a
// zero StartedAt, or a session toolcalllog has never heard of.
func TestRecoverIsSilentWhenItCannotAnswer(t *testing.T) {
	if _, _, ok := Recover("", Candidate{ToolName: "x", StartedAt: time.Now()}); ok {
		t.Error("empty session id produced a match")
	}
	if _, _, ok := Recover("some-session", Candidate{StartedAt: time.Now()}); ok {
		t.Error("empty tool name produced a match")
	}
	if _, _, ok := Recover("some-session", Candidate{ToolName: "x"}); ok {
		t.Error("zero StartedAt produced a match")
	}
	if _, _, ok := Recover("session-nobody-ever-recorded-anything-for", Candidate{ToolName: "x", StartedAt: time.Now()}); ok {
		t.Error("an unknown session produced a match")
	}
}

package server

import "testing"

// Four runs of ONE schedule are four separate sessions with four separate
// conversation files and four separate outcomes. Collapsing them to a single
// row hid both a failed run and, because the newest run crossed midnight in
// the viewer's timezone, an entire day of history.
func TestScheduleRunsEachGetTheirOwnHistoryRow(t *testing.T) {
	scheduleIDBySessionID := map[string]string{
		"schedule-cron--9db4dc39_1785164105108836000": "9db4dc39",
		"schedule-cron--9db4dc39_1785171305134374000": "9db4dc39",
		"schedule-cron--9db4dc39_1785172603962262000": "9db4dc39",
		"schedule-cron--9db4dc39_1785178505153359000": "9db4dc39",
	}

	keys := make(map[string]struct{})
	for sessionID := range scheduleIDBySessionID {
		keys[chatHistoryDisplayKey(sessionID, scheduleIDBySessionID)] = struct{}{}
	}
	if len(keys) != len(scheduleIDBySessionID) {
		t.Fatalf("got %d display keys for %d runs; runs of one schedule must not collapse into one row",
			len(keys), len(scheduleIDBySessionID))
	}
}

// The dedupe still has a job: several files can be written for a SINGLE
// session when a run resumes the same CLI thread. Those must stay one row.
func TestSameSessionStillCollapsesToOneRow(t *testing.T) {
	sessionID := "schedule-cron--9db4dc39_1785164105108836000"
	first := chatHistoryDisplayKey(sessionID, map[string]string{sessionID: "9db4dc39"})
	second := chatHistoryDisplayKey(sessionID, map[string]string{sessionID: "9db4dc39"})
	if first != second {
		t.Fatalf("same session produced different keys: %q vs %q", first, second)
	}
}

// A schedule session and an ordinary chat with the same id must never share a
// row, and neither may key off a schedule id that other runs also carry.
func TestOrdinaryAndScheduleSessionsKeyIndependently(t *testing.T) {
	scheduleSession := "schedule-cron--9db4dc39_1785164105108836000"
	plainSession := "1d293408-2e3a-4964-9b77-3530dd84e6ec"
	ids := map[string]string{scheduleSession: "9db4dc39"}

	if chatHistoryDisplayKey(scheduleSession, ids) == chatHistoryDisplayKey(plainSession, ids) {
		t.Fatal("a schedule run and an ordinary chat collapsed into the same row")
	}
}

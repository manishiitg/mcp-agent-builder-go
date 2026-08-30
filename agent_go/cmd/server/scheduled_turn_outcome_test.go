package server

import (
	"testing"
	"time"

	storeevents "github.com/manishiitg/coding-agent-loop/agent_go/internal/events"
)

func errEvent(kind, detail string, at time.Time) storeevents.Event {
	return storeevents.Event{Type: kind, Timestamp: at, Error: detail}
}

// TestScheduledTurnFailureCatchesATurnThatProducedNothing is the hetzner-ssh
// case from 2026-08-18 20:06: both turns failed with "all LLMs failed ...
// [quota_exhausted]", the run lasted 8.1 seconds, executed no step, and was
// recorded a successful security audit. startSessionInternal returned no error
// because dispatch worked — only the generation failed, and that is recorded as
// events, not as a return value.
func TestScheduledTurnFailureCatchesATurnThatProducedNothing(t *testing.T) {
	store := storeevents.NewEventStore(200)
	const sessionID = "sched-session"
	turnStart := time.Now().UTC()

	store.AddEvent(sessionID, errEvent("conversation_error",
		"all LLMs failed (primary + 0 fallbacks): claudecode/ [quota_exhausted]: claude code usage limit reached", turnStart.Add(time.Second)))

	got := scheduledTurnFailure(store, sessionID, turnStart)
	if got == "" {
		t.Fatal("a turn whose every LLM attempt failed was reported as fine — the run would be recorded success")
	}
	if !contains(got, "quota_exhausted") {
		t.Errorf("failure text lost the cause: %q", got)
	}
}

// TestScheduledTurnFailureIgnoresAnEarlierTurnsFailure. Scheduled runs are many
// turns in one session; a failure from turn 1 must not condemn turn 2, or a
// recovered run is filed as failed.
func TestScheduledTurnFailureIgnoresAnEarlierTurnsFailure(t *testing.T) {
	store := storeevents.NewEventStore(200)
	const sessionID = "sched-session-recovered"
	base := time.Now().UTC()

	store.AddEvent(sessionID, errEvent("conversation_error", "earlier turn blew up", base))
	laterTurnStart := base.Add(time.Minute)
	store.AddEvent(sessionID, storeevents.Event{Type: "agent_end", Timestamp: laterTurnStart.Add(time.Second)})

	if got := scheduledTurnFailure(store, sessionID, laterTurnStart); got != "" {
		t.Errorf("a previous turn's failure was attributed to this turn: %q", got)
	}
}

// TestScheduledTurnFailureStaysQuietForAHealthyTurn guards the direction that
// costs the most: inventing failures would fail runs that worked.
func TestScheduledTurnFailureStaysQuietForAHealthyTurn(t *testing.T) {
	store := storeevents.NewEventStore(200)
	const sessionID = "sched-session-ok"
	turnStart := time.Now().UTC()
	store.AddEvent(sessionID, storeevents.Event{Type: "agent_end", Timestamp: turnStart.Add(time.Second)})

	if got := scheduledTurnFailure(store, sessionID, turnStart); got != "" {
		t.Errorf("healthy turn reported as failed: %q", got)
	}
	if got := scheduledTurnFailure(nil, sessionID, turnStart); got != "" {
		t.Errorf("nil store reported a failure: %q", got)
	}
	if got := scheduledTurnFailure(store, "unknown-session", turnStart); got != "" {
		t.Errorf("unknown session reported a failure: %q", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

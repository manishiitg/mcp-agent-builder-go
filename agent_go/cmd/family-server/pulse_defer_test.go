package main

import (
	"testing"
	"time"
)

func TestSinceInteractiveTurnTreatsNeverAsLongAgo(t *testing.T) {
	lastInteractiveTurn.at = time.Time{}
	// A fresh process must not read "nothing has run" as "active just now",
	// or Pulse would defer forever after every restart.
	if got := sinceInteractiveTurn(); got < pulseQuietPeriod {
		t.Fatalf("expected a long idle time when nothing has run, got %s", got)
	}
}

func TestNoteInteractiveTurnMarksActive(t *testing.T) {
	noteInteractiveTurn()
	if got := sinceInteractiveTurn(); got > time.Minute {
		t.Fatalf("expected the family to read as just-active, got %s", got)
	}
}

// Pulse turns must NOT count as activity, or Pulse would keep deferring
// itself: each check would mark the family active for the next one.
func TestPulseTurnsDoNotCountAsInteractive(t *testing.T) {
	lastInteractiveTurn.at = time.Time{}
	done := markAgentTurnStart("pulse")
	done()
	if got := sinceInteractiveTurn(); got < pulseQuietPeriod {
		t.Fatalf("a pulse turn should not mark the family active, got %s", got)
	}

	done = markAgentTurnStart("parent")
	done()
	if got := sinceInteractiveTurn(); got > time.Minute {
		t.Fatalf("a parent turn should mark the family active, got %s", got)
	}
}

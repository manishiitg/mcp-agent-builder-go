package server

import (
	"context"
	"testing"
	"time"
)

// TestDrainSyntheticTurnStreamReturnsWhenTheProducerGoesSilent is the zombie
// case, reproduced.
//
// A coding-agent CLI parked on a usage-limit wall stops producing without ever
// closing its channel. The previous `for range textChan` had no exit for that
// — it blocked forever, and the deferred cleanup behind it never cleared
// session-busy, never released the input lane, and never recorded the tracked
// execution. The session reported "running" indefinitely with nothing running.
//
// The channel here is deliberately left open and never written to, which is
// exactly what the stalled pane presents.
func TestDrainSyntheticTurnStreamReturnsWhenTheProducerGoesSilent(t *testing.T) {
	t.Setenv(envSyntheticTurnIdleSeconds, "1")
	silent := make(chan string) // never written, never closed

	done := make(chan struct{})
	var reason string
	var stalled bool
	go func() {
		reason, stalled = drainSyntheticTurnStream(context.Background(), silent, nil)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("drain never returned on a silent producer — the turn would zombie")
	}
	if !stalled {
		t.Error("a silent producer must be reported as stalled, not as a cancellation")
	}
	if reason == "" {
		t.Error("expected a reason naming the idle bound")
	}
}

// TestDrainSyntheticTurnStreamStopsOnCancellation covers the other bound.
// Cancelling a turn must actually end the consume loop: cancellation reaches
// the producer, but the old loop was waiting on the channel, not the context,
// so a stop request could leave the consumer parked anyway.
func TestDrainSyntheticTurnStreamStopsOnCancellation(t *testing.T) {
	silent := make(chan string)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	var stalled bool
	go func() {
		_, stalled = drainSyntheticTurnStream(ctx, silent, nil)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("drain ignored context cancellation")
	}
	if stalled {
		t.Error("a cancelled turn must not be reported as stalled — that files a user stop as a defect")
	}
}

// TestDrainSyntheticTurnStreamConsumesEveryChunkThenCloses pins the normal
// path: the idle timer must be reset by activity, and a closed channel is a
// clean end with no reason and no stall.
func TestDrainSyntheticTurnStreamConsumesEveryChunkThenCloses(t *testing.T) {
	t.Setenv(envSyntheticTurnIdleSeconds, "2")
	chunks := make(chan string)
	go func() {
		defer close(chunks)
		// Spread across more than one idle bound in total, so a timer that is
		// not reset on activity would fire mid-stream.
		for i := 0; i < 3; i++ {
			chunks <- "chunk"
			time.Sleep(900 * time.Millisecond)
		}
	}()

	seen := 0
	reason, stalled := drainSyntheticTurnStream(context.Background(), chunks, func(string) { seen++ })

	if reason != "" || stalled {
		t.Errorf("clean close reported as reason=%q stalled=%v", reason, stalled)
	}
	if seen != 3 {
		t.Errorf("consumed %d chunks, want 3", seen)
	}
}

// TestSyntheticTurnIdleTimeoutCannotBeExtended: this is a backstop against a
// turn that can never finish, and a backstop configuration can lengthen
// arbitrarily stops being one. Shortening stays allowed.
func TestSyntheticTurnIdleTimeoutCannotBeExtended(t *testing.T) {
	t.Setenv(envSyntheticTurnIdleSeconds, "999999")
	if got := syntheticTurnIdleTimeout(); got != defaultSyntheticTurnIdleTimeout {
		t.Errorf("timeout = %v, want it clamped to %v", got, defaultSyntheticTurnIdleTimeout)
	}
	t.Setenv(envSyntheticTurnIdleSeconds, "30")
	if got := syntheticTurnIdleTimeout(); got != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", got)
	}
}

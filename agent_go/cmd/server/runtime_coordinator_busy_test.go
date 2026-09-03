package server

import "testing"

// The deploy drain waits on BusyCount: a session whose generation has
// started but not reached a terminal boundary must count as busy, and a
// finished one must not.
func TestRuntimeCoordinatorBusyCount(t *testing.T) {
	c := NewRuntimeCoordinator()
	if got := c.BusyCount(); got != 0 {
		t.Fatalf("fresh coordinator busy = %d, want 0", got)
	}
	c.StartGeneration("s1", "new user turn started")
	c.StartGeneration("s2", "steered turn")
	if got := c.BusyCount(); got != 2 {
		t.Fatalf("two generations in flight, busy = %d", got)
	}
	c.MarkTerminalBoundary("s1", runtimePhaseCompleted, "done")
	if got := c.BusyCount(); got != 1 {
		t.Fatalf("one completed, busy = %d, want 1", got)
	}
	c.Evict("s2")
	if got := c.BusyCount(); got != 0 {
		t.Fatalf("all gone, busy = %d, want 0", got)
	}
	var nilCoordinator *RuntimeCoordinator
	if nilCoordinator.BusyCount() != 0 {
		t.Fatal("nil coordinator must report 0")
	}
}

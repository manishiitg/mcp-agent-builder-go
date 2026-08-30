package step_based_workflow

import (
	"context"
	"testing"
	"time"

	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

func seedWindow(t *testing.T, key string, used float64, resetsAt time.Time, observedAt time.Time) {
	t.Helper()
	llmtypes.ResetAccountRateLimitWindowsForTest()
	llmtypes.RecordAccountRateLimitWindows(key,
		[]llmtypes.RateLimitWindow{{Name: "five_hour", UsedPercent: used, ResetsAt: resetsAt}}, observedAt)
	t.Cleanup(llmtypes.ResetAccountRateLimitWindowsForTest)
}

// TestPacingCostsNothingWhileQuotaIsHealthy is the property that makes this
// opt-in feature safe to leave on: a run nowhere near the wall must not pay a
// second of wall-clock. That is the whole argument for waiting-until-reset over
// a fixed delay between steps.
func TestPacingCostsNothingWhileQuotaIsHealthy(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	key := llmtypes.AccountRateLimitKey("token-A")
	seedWindow(t, key, 40, now.Add(2*time.Hour), now)

	if d := decideQuotaPacing(key, 85, now); d.Wait != 0 || d.Suspend {
		t.Errorf("healthy quota paced the run: %+v", d)
	}
}

// TestPacingWaitsInPlaceWhenTheResetIsClose. The run holds its sessions for a
// few minutes and then continues across the reset — cheaper than a full
// suspend/resume cycle for a short gap.
func TestPacingWaitsInPlaceWhenTheResetIsClose(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	key := llmtypes.AccountRateLimitKey("token-A")
	seedWindow(t, key, 92, now.Add(5*time.Minute), now)

	d := decideQuotaPacing(key, 85, now)
	if d.Suspend {
		t.Fatal("a five-minute wait was escalated to a suspension")
	}
	if d.Wait != 5*time.Minute {
		t.Errorf("wait = %v, want 5m", d.Wait)
	}
}

// TestPacingSuspendsRatherThanIdlingForALongWait.
//
// Waiting in place for hours is the wrong shape: the run holds its tmux
// sessions, its schedule lock and its queue position while doing nothing.
// Beyond the inline cap it becomes a real suspension, which releases all of
// that and resumes at the reset (PLAT-101).
func TestPacingSuspendsRatherThanIdlingForALongWait(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	key := llmtypes.AccountRateLimitKey("token-A")
	reset := now.Add(4 * time.Hour)
	seedWindow(t, key, 97, reset, now)

	d := decideQuotaPacing(key, 85, now)
	if !d.Suspend {
		t.Fatalf("a four-hour wait was taken in place: %+v", d)
	}
	if !d.ResetsAt.Equal(reset) {
		t.Errorf("ResetsAt = %v, want %v", d.ResetsAt, reset)
	}
}

// TestPacingProceedsWhenItCannotKnow. Pacing is an optimization; one that
// blocks work on missing or stale data is worse than none at all.
func TestPacingProceedsWhenItCannotKnow(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	key := llmtypes.AccountRateLimitKey("token-A")

	// Not opted in.
	seedWindow(t, key, 99, now.Add(time.Hour), now)
	if d := decideQuotaPacing(key, 0, now); d.Wait != 0 || d.Suspend {
		t.Error("a workflow that did not opt in was paced")
	}
	// No account identity.
	if d := decideQuotaPacing("", 85, now); d.Wait != 0 || d.Suspend {
		t.Error("a run with no account key was paced")
	}
	// Reading too old to trust.
	seedWindow(t, key, 99, now.Add(time.Hour), now.Add(-3*time.Hour))
	if d := decideQuotaPacing(key, 85, now); d.Wait != 0 || d.Suspend {
		t.Error("a three-hour-old reading paced the run")
	}
	// Exhausted but no stated reset — nothing to wait for.
	llmtypes.ResetAccountRateLimitWindowsForTest()
	llmtypes.RecordAccountRateLimitWindows(key, []llmtypes.RateLimitWindow{{Name: "five_hour", UsedPercent: 99}}, now)
	if d := decideQuotaPacing(key, 85, now); d.Wait != 0 || d.Suspend {
		t.Error("a window with no stated reset paced the run")
	}
}

// TestPacingWaitStopsWhenTheRunIsCancelled: a paced run is still a running run,
// and Stop must mean stop even while it is idling (PLAT-130).
func TestPacingWaitStopsWhenTheRunIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- waitForQuotaPacing(ctx, time.Hour) }()
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("a cancelled pacing wait reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pacing wait ignored cancellation — Stop would not take during a pause")
	}
}

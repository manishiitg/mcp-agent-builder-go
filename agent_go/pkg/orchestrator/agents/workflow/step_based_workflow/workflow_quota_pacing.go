package step_based_workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

const (
	// quotaPacingMaxInlineWait is the longest a run will sit idle inside itself
	// waiting for a window to reopen.
	//
	// Past this, waiting in place is the wrong shape: the run is holding its
	// tmux sessions, its schedule lock and its place in the queue while doing
	// nothing. A longer wait becomes a suspension instead, which releases all
	// of that and resumes at the reset (PLAT-101). Fifteen minutes is short
	// enough that holding on is cheaper than a full suspend/resume cycle.
	quotaPacingMaxInlineWait = 15 * time.Minute

	// quotaPacingMaxReadingAge matches the schedule-level gate: a quota reading
	// older than this is treated as no information at all, because a stale
	// number reads as confidently as a fresh one and would pace a run for a
	// window that reopened hours ago.
	quotaPacingMaxReadingAge = 30 * time.Minute
)

// quotaPacingDecision is what to do before starting a step.
type quotaPacingDecision struct {
	// Wait is a bounded pause to take in place. Zero means proceed.
	Wait time.Duration
	// Suspend is set when the wait is too long to hold in place; the run should
	// record a capacity wait and resume at ResetsAt.
	Suspend  bool
	ResetsAt time.Time
	Window   string
	// UsedPercent is the reading the decision was made on, for the log line.
	UsedPercent float64
}

// decideQuotaPacing decides whether the next step should start now, pause, or
// suspend the run.
//
// The rule the user actually asked for: spread a run out rather than racing
// into a wall. It is not a fixed delay between steps — padding every step costs
// wall-clock when quota is healthy and still does not save a run whose next
// step is the one that exhausts the window. Waiting for the reset does, because
// it moves consumption across the boundary rather than merely slowing it.
//
// Unknown always means proceed. No account key, no cached reading, a stale
// reading, or a window with no stated reset all fall through — pacing is an
// optimization, and one that blocks work on missing data is worse than none.
func decideQuotaPacing(accountKey string, thresholdPercent int, now time.Time) quotaPacingDecision {
	if accountKey == "" || thresholdPercent <= 0 {
		return quotaPacingDecision{}
	}
	windows, _, ok := llmtypes.AccountRateLimitWindows(accountKey, now, quotaPacingMaxReadingAge)
	if !ok {
		return quotaPacingDecision{}
	}
	resetsAt, window := llmtypes.MostConstrainedReset(windows, now)
	if resetsAt.IsZero() {
		return quotaPacingDecision{}
	}
	used := float64(0)
	for _, w := range windows {
		if w.Name == window {
			used = w.UsedPercent
			break
		}
	}
	if used < float64(thresholdPercent) {
		return quotaPacingDecision{}
	}
	wait := resetsAt.Sub(now)
	if wait <= 0 {
		return quotaPacingDecision{}
	}
	if wait > quotaPacingMaxInlineWait {
		return quotaPacingDecision{Suspend: true, ResetsAt: resetsAt, Window: window, UsedPercent: used}
	}
	return quotaPacingDecision{Wait: wait, ResetsAt: resetsAt, Window: window, UsedPercent: used}
}

// waitForQuotaPacing sleeps out a pacing decision, or returns early if the run
// is cancelled. A paced run must still stop when Stop is clicked (PLAT-130).
func waitForQuotaPacing(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// describeQuotaPacing renders a decision for the run log.
func describeQuotaPacing(d quotaPacingDecision) string {
	if d.Suspend {
		return fmt.Sprintf("%s window at %.0f%%; reset %s is too far off to wait in place — suspending",
			d.Window, d.UsedPercent, d.ResetsAt.UTC().Format(time.RFC3339))
	}
	return fmt.Sprintf("%s window at %.0f%%; pausing %s until it resets at %s",
		d.Window, d.UsedPercent, d.Wait.Round(time.Second), d.ResetsAt.UTC().Format(time.RFC3339))
}

// applyQuotaPacingBeforeStep pauses or suspends the run before a step when the
// provider window is close to exhausted. Returns nil to proceed.
func (hcpo *StepBasedWorkflowOrchestrator) applyQuotaPacingBeforeStep(
	ctx context.Context,
	stepNumber, totalSteps int,
	step PlanStep,
	stepPath string,
) error {
	if hcpo == nil || hcpo.executionOptions == nil {
		return nil
	}
	decision := decideQuotaPacing(hcpo.executionOptions.CapacityAccountKey, hcpo.executionOptions.PaceThresholdPercent, time.Now().UTC())
	if decision.Wait <= 0 && !decision.Suspend {
		return nil
	}
	stepID := step.GetID()
	if stepID == "" {
		stepID = fmt.Sprintf("step-%d", stepNumber)
	}
	hcpo.GetLogger().Info(fmt.Sprintf("⏳ Quota pacing before step %d/%d (%q): %s",
		stepNumber, totalSteps, step.GetTitle(), describeQuotaPacing(decision)))

	if decision.Suspend {
		return hcpo.recordWorkflowCapacityWaitForPacing(ctx, decision.ResetsAt, decision.Window,
			describeQuotaPacing(decision), stepNumber, totalSteps, stepID, stepPath, step.GetTitle())
	}
	return waitForQuotaPacing(ctx, decision.Wait)
}

package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/terminals"
)

// PLAT-054. The scheduler's idle wait runs AFTER startSessionInternal has
// already returned the main agent's completion, so the only thing it can be
// waiting on is child work the turn launched. Silence therefore does not imply
// a stall: a browser step pacing itself, or a run_full_workflow child running
// for hours, legitimately emits nothing for longer than the inactivity window.
// These tests pin the distinction — live child work suppresses the timeout,
// absence of it does not.

func withFastSchedulerIdleTimings(t *testing.T) {
	t.Helper()
	oldInterval := schedulerWorkshopIdlePollInterval
	oldMaxInactivity := schedulerWorkshopMaxInactivity
	schedulerWorkshopIdlePollInterval = time.Millisecond
	schedulerWorkshopMaxInactivity = 5 * time.Millisecond
	t.Cleanup(func() {
		schedulerWorkshopIdlePollInterval = oldInterval
		schedulerWorkshopMaxInactivity = oldMaxInactivity
	})
}

func newSchedulerWithRunningExecution(sessionID string, status string) *SchedulerService {
	api := &StreamingAPI{
		terminalStore:             terminals.NewStore(),
		trackedWorkflowExecutions: map[string]*TrackedWorkflowExecution{},
	}
	api.setSessionBusy(sessionID, true)
	api.trackedWorkflowExecutions["exec-1"] = &TrackedWorkflowExecution{
		ExecutionID: "exec-1",
		SessionID:   sessionID,
		Status:      status,
	}
	return &SchedulerService{api: api}
}

func TestWaitForWorkshopIdleKeepsWaitingWhileChildExecutionRuns(t *testing.T) {
	withFastSchedulerIdleTimings(t)

	sessionID := "session-live-child"
	svc := newSchedulerWithRunningExecution(sessionID, trackedExecutionStatusRunning)

	// The inactivity window elapses many times over inside this context. Before
	// PLAT-054 that produced errWorkshopIdleWaitTimeout; now the running child
	// must hold the wait open until the caller's own deadline instead.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	err := svc.waitForWorkshopIdle(ctx, sessionID)
	if errors.Is(err, errWorkshopIdleWaitTimeout) {
		t.Fatalf("wait expired while child execution was still running: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the caller's context deadline", err)
	}
}

func TestWaitForWorkshopIdleStillTimesOutWithNoLiveChildWork(t *testing.T) {
	withFastSchedulerIdleTimings(t)

	sessionID := "session-no-live-child"
	// A completed execution is not live work — a genuinely stalled turn must
	// still expire, otherwise the fix would only trade one failure for another.
	svc := newSchedulerWithRunningExecution(sessionID, "completed")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := svc.waitForWorkshopIdle(ctx, sessionID)
	if !errors.Is(err, errWorkshopIdleWaitTimeout) {
		t.Fatalf("err = %v, want errWorkshopIdleWaitTimeout", err)
	}
	if !strings.Contains(err.Error(), "live_child_work=false") {
		t.Fatalf("err = %v, want the liveness value recorded in the message", err)
	}
}

func TestWaitForWorkshopIdleExpiresLiveChildAfterAbsoluteCeiling(t *testing.T) {
	withFastSchedulerIdleTimings(t)
	oldCeiling := schedulerWorkshopLiveChildCeiling
	schedulerWorkshopLiveChildCeiling = 10 * time.Millisecond
	t.Cleanup(func() { schedulerWorkshopLiveChildCeiling = oldCeiling })

	sessionID := "session-immortal-child"
	svc := newSchedulerWithRunningExecution(sessionID, trackedExecutionStatusRunning)

	// A child that never finishes must not block its schedule forever, so the
	// suppression is bounded rather than unconditional.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := svc.waitForWorkshopIdle(ctx, sessionID)
	if !errors.Is(err, errWorkshopIdleWaitTimeout) {
		t.Fatalf("err = %v, want the absolute ceiling to expire a hung child", err)
	}
}

func TestSessionHasLiveChildWorkReadsExecutionStatus(t *testing.T) {
	sessionID := "session-liveness-predicate"

	running := newSchedulerWithRunningExecution(sessionID, trackedExecutionStatusRunning)
	if !running.sessionHasLiveChildWork(sessionID) {
		t.Fatal("running execution not reported as live child work")
	}
	if running.sessionHasLiveChildWork("some-other-session") {
		t.Fatal("liveness leaked across sessions")
	}

	done := newSchedulerWithRunningExecution(sessionID, "completed")
	if done.sessionHasLiveChildWork(sessionID) {
		t.Fatal("completed execution reported as live child work")
	}
}

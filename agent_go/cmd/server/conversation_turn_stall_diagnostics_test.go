package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDiagnoseStalledConversationTurnReturnsEmptyWithoutARunningAgent(t *testing.T) {
	api := lifecycleTestAPI()
	if evidence := api.diagnoseStalledConversationTurn("no-such-session", time.Now()); evidence != "" {
		t.Fatalf("evidence = %q, want empty when there is no running agent to inspect", evidence)
	}
}

func TestCleanupOrphanedConversationTurnClearsExactTurnAndKeepsConversationReusable(t *testing.T) {
	api := lifecycleTestAPI()
	const sessionID = "session-orphaned"
	cancelCalls := 0
	api.agentCancelFuncs = map[string]context.CancelFunc{
		sessionID: func() { cancelCalls++ },
	}
	api.activeSessions = map[string]*ActiveSessionInfo{
		sessionID: {SessionID: sessionID, Status: "running"},
	}
	api.sessionQueryIDs = map[string][]string{
		sessionID: {"orphaned-query", "other-query"},
	}
	api.trackedWorkflowExecutions["orphaned-query"] = &TrackedWorkflowExecution{
		ExecutionID: "orphaned-query", SessionID: sessionID, Source: trackedExecutionSourceConversationTurn,
		Status: trackedExecutionStatusRunning, StartedAt: time.Now().UTC(),
	}
	api.setSessionBusy(sessionID, true)

	api.cleanupOrphanedConversationTurn(sessionID, "orphaned-query")
	if cancelCalls != 1 {
		t.Fatalf("turn cancel calls = %d, want exactly one", cancelCalls)
	}

	api.activeSessionsMux.RLock()
	status := api.activeSessions[sessionID].Status
	api.activeSessionsMux.RUnlock()
	if status != "error" {
		t.Fatalf("session status = %q, want error after orphaned cleanup", status)
	}

	// PLAT-116 follow-up: the workflow selector's spinner and Global Monitor's
	// runtime_state key off sessionBusy and the tracked execution record, not
	// the coarse active-session status — both were left uncleared originally,
	// which kept a stalled turn showing "running" in the UI for hours.
	if api.isSessionBusy(sessionID) {
		t.Fatal("sessionBusy must be cleared so the workflow selector and Global Monitor stop showing a spinner")
	}
	api.trackedWorkflowExecutionsMux.RLock()
	execStatus := api.trackedWorkflowExecutions["orphaned-query"].Status
	api.trackedWorkflowExecutionsMux.RUnlock()
	if execStatus != trackedExecutionStatusFailed {
		t.Fatalf("tracked execution status = %q, want failed so runtime_state.phase reports terminal", execStatus)
	}

	api.sessionQueryIDMux.RLock()
	remaining := api.sessionQueryIDs[sessionID]
	api.sessionQueryIDMux.RUnlock()
	if len(remaining) != 1 || remaining[0] != "other-query" {
		t.Fatalf("remaining query ids = %v, want only the unrelated query left", remaining)
	}
}

func TestCleanupOrphanedConversationTurnIsIdempotent(t *testing.T) {
	api := lifecycleTestAPI()
	const sessionID = "session-double-cleanup"
	api.activeSessions = map[string]*ActiveSessionInfo{
		sessionID: {SessionID: sessionID, Status: "running"},
	}
	api.sessionQueryIDs = map[string][]string{}

	// Simulates the orphaned goroutine eventually unblocking and running its
	// own normal completion cleanup after the safety net already ran this one.
	api.cleanupOrphanedConversationTurn(sessionID, "query-id")
	api.cleanupOrphanedConversationTurn(sessionID, "query-id")

	api.activeSessionsMux.RLock()
	status := api.activeSessions[sessionID].Status
	api.activeSessionsMux.RUnlock()
	if status != "error" {
		t.Fatalf("session status = %q, want error to survive a second cleanup pass", status)
	}
}

func TestWaitForConversationTurnTreeIdleTimeoutRunsOrphanCleanup(t *testing.T) {
	api := lifecycleTestAPI()
	const sessionID = "session-idle-timeout"
	longAgo := time.Now().Add(-time.Hour)
	api.trackedWorkflowExecutions["stuck-turn"] = &TrackedWorkflowExecution{
		ExecutionID: "stuck-turn", SessionID: sessionID, Source: trackedExecutionSourceConversationTurn,
		Status: trackedExecutionStatusRunning, StartedAt: longAgo,
	}
	api.activeSessions = map[string]*ActiveSessionInfo{
		sessionID: {SessionID: sessionID, Status: "running"},
	}
	api.sessionQueryIDs = map[string][]string{
		sessionID: {"stuck-turn"},
	}
	api.setSessionBusy(sessionID, true)

	err := api.waitForConversationTurnTree(context.Background(), sessionID, "stuck-turn", 10*time.Millisecond)
	if !errors.Is(err, errWorkshopIdleWaitTimeout) {
		t.Fatalf("error = %v, want errWorkshopIdleWaitTimeout", err)
	}

	api.activeSessionsMux.RLock()
	status := api.activeSessions[sessionID].Status
	api.activeSessionsMux.RUnlock()
	if status != "error" {
		t.Fatalf("session status = %q, want the idle-wait timeout to have cleaned up the orphaned session", status)
	}
	if api.isSessionBusy(sessionID) {
		t.Fatal("sessionBusy must be cleared by the idle-wait timeout, not just active-session status")
	}
	api.trackedWorkflowExecutionsMux.RLock()
	execStatus := api.trackedWorkflowExecutions["stuck-turn"].Status
	api.trackedWorkflowExecutionsMux.RUnlock()
	if execStatus != trackedExecutionStatusFailed {
		t.Fatalf("tracked execution status = %q, want failed so the workflow selector and Global Monitor see a terminal state", execStatus)
	}

	api.sessionQueryIDMux.RLock()
	remaining := api.sessionQueryIDs[sessionID]
	api.sessionQueryIDMux.RUnlock()
	if len(remaining) != 0 {
		t.Fatalf("remaining query ids = %v, want the stuck query id removed from the session index", remaining)
	}
}

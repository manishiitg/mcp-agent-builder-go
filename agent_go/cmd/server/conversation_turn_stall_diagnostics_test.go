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

func TestCleanupOrphanedConversationTurnSessionClearsActiveSessionAndQueryIndex(t *testing.T) {
	api := lifecycleTestAPI()
	const sessionID = "session-orphaned"
	api.activeSessions = map[string]*ActiveSessionInfo{
		sessionID: {SessionID: sessionID, Status: "running"},
	}
	api.sessionQueryIDs = map[string][]string{
		sessionID: {"orphaned-query", "other-query"},
	}

	api.cleanupOrphanedConversationTurnSession(sessionID, "orphaned-query")

	api.activeSessionsMux.RLock()
	status := api.activeSessions[sessionID].Status
	api.activeSessionsMux.RUnlock()
	if status != "error" {
		t.Fatalf("session status = %q, want error after orphaned cleanup", status)
	}

	api.sessionQueryIDMux.RLock()
	remaining := api.sessionQueryIDs[sessionID]
	api.sessionQueryIDMux.RUnlock()
	if len(remaining) != 1 || remaining[0] != "other-query" {
		t.Fatalf("remaining query ids = %v, want only the unrelated query left", remaining)
	}
}

func TestCleanupOrphanedConversationTurnSessionIsIdempotent(t *testing.T) {
	api := lifecycleTestAPI()
	const sessionID = "session-double-cleanup"
	api.activeSessions = map[string]*ActiveSessionInfo{
		sessionID: {SessionID: sessionID, Status: "running"},
	}
	api.sessionQueryIDs = map[string][]string{}

	// Simulates the orphaned goroutine eventually unblocking and running its
	// own normal completion cleanup after the safety net already ran this one.
	api.cleanupOrphanedConversationTurnSession(sessionID, "query-id")
	api.cleanupOrphanedConversationTurnSession(sessionID, "query-id")

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

	api.sessionQueryIDMux.RLock()
	remaining := api.sessionQueryIDs[sessionID]
	api.sessionQueryIDMux.RUnlock()
	if len(remaining) != 0 {
		t.Fatalf("remaining query ids = %v, want the stuck query id removed from the session index", remaining)
	}
}

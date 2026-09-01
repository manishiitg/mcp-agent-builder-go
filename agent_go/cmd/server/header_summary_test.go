package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHeaderSummaryIncludesActiveSessionsAndDefaultsScheduleSummaryWithoutScheduler(t *testing.T) {
	const sessionID = "workflow-after-restart"
	startedAt := time.Now().Add(-time.Minute).UTC()
	api := &StreamingAPI{
		runtimeCoordinator: NewRuntimeCoordinator(),
		activeSessions:     map[string]*ActiveSessionInfo{},
		trackedWorkflowExecutions: map[string]*TrackedWorkflowExecution{
			"workflow-1": {
				ExecutionID: "workflow-1", SessionID: sessionID,
				Source: trackedExecutionSourceWorkflowRun, Kind: "workflow", Status: trackedExecutionStatusRunning,
				PresetName: "Daily QA", WorkspacePath: "Workflow/daily-qa", StartedAt: startedAt,
			},
		},
	}

	recorder := httptest.NewRecorder()
	api.handleGetHeaderSummary(recorder, httptest.NewRequest("GET", "/api/header-summary", nil))

	var response HeaderSummaryResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode header summary: %v", err)
	}
	if len(response.ActiveSessions) != 1 || response.Total != 1 {
		t.Fatalf("active sessions = %#v total=%d, want the one tracked workflow", response.ActiveSessions, response.Total)
	}
	if response.ActiveSessions[0].SessionID != sessionID {
		t.Fatalf("active session = %#v, want tracked workflow to be synthesized in", response.ActiveSessions[0])
	}
	if response.ScheduleSummary != (WorkflowScheduleSummary{}) {
		t.Fatalf("schedule summary = %#v, want zero value when api.scheduler is nil", response.ScheduleSummary)
	}
}

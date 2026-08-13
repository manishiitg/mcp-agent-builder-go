package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestComputeWorkflowScheduleMissedStatusClearsMissesBeforeLatestExecution(t *testing.T) {
	sched := WorkflowSchedule{
		ID:             "daily",
		CronExpression: "30 3 * * *",
		Timezone:       "UTC",
		Enabled:        true,
	}
	tracker := WorkflowScheduleExecutionTrack{
		ScheduleID:     sched.ID,
		CronExpression: sched.CronExpression,
		Timezone:       sched.Timezone,
		Enabled:        true,
		WindowStartAt:  mustParseTime(t, "2026-06-10T00:00:00Z"),
		UpdatedAt:      mustParseTime(t, "2026-06-12T03:31:00Z"),
		Executions: []WorkflowScheduleExecutionRecord{
			{StartedAt: mustParseTime(t, "2026-06-12T03:30:20Z")},
		},
	}

	got := ComputeWorkflowScheduleMissedStatus(sched, &tracker, mustParseTime(t, "2026-06-12T06:00:00Z"))
	if got.MissedRunCount != 0 || got.LatestMissedRunAt != nil {
		t.Fatalf("missed status = %+v, want no active misses after latest execution", got)
	}
}

func TestComputeWorkflowScheduleMissedStatusReportsMissesAfterLatestExecution(t *testing.T) {
	sched := WorkflowSchedule{
		ID:             "daily",
		CronExpression: "30 3 * * *",
		Timezone:       "UTC",
		Enabled:        true,
	}
	tracker := WorkflowScheduleExecutionTrack{
		ScheduleID:     sched.ID,
		CronExpression: sched.CronExpression,
		Timezone:       sched.Timezone,
		Enabled:        true,
		WindowStartAt:  mustParseTime(t, "2026-06-10T00:00:00Z"),
		UpdatedAt:      mustParseTime(t, "2026-06-10T03:31:00Z"),
		Executions: []WorkflowScheduleExecutionRecord{
			{StartedAt: mustParseTime(t, "2026-06-10T03:30:20Z")},
		},
	}

	got := ComputeWorkflowScheduleMissedStatus(sched, &tracker, mustParseTime(t, "2026-06-12T06:00:00Z"))
	if got.MissedRunCount != 2 {
		t.Fatalf("missed count = %d, want 2", got.MissedRunCount)
	}
	if got.LatestMissedRunAt == nil || !got.LatestMissedRunAt.Equal(mustParseTime(t, "2026-06-12T03:30:00Z")) {
		t.Fatalf("latest missed = %v, want 2026-06-12T03:30:00Z", got.LatestMissedRunAt)
	}
	if got.MissedRunReason != workflowScheduleMissedReasonNoExecution {
		t.Fatalf("missed reason = %q, want %q", got.MissedRunReason, workflowScheduleMissedReasonNoExecution)
	}
}

func TestEnsureWorkflowScheduleExecutionTrackerResetsWindowOnEnabledChange(t *testing.T) {
	history := &WorkflowScheduleExecutionHistoryFile{
		Version:   workflowScheduleExecutionHistoryVersion,
		Schedules: map[string]WorkflowScheduleExecutionTrack{},
	}
	sched := WorkflowSchedule{
		ID:             "daily",
		CronExpression: "30 3 * * *",
		Timezone:       "UTC",
		Enabled:        true,
	}

	createdAt := mustParseTime(t, "2026-06-10T00:00:00Z")
	tracker, changed := ensureWorkflowScheduleExecutionTracker(history, sched, createdAt)
	if !changed {
		t.Fatal("new tracker was not reported as changed")
	}
	history.Schedules[sched.ID] = tracker

	tracker.Executions = []WorkflowScheduleExecutionRecord{{StartedAt: mustParseTime(t, "2026-06-10T03:30:00Z")}}
	history.Schedules[sched.ID] = tracker

	disabledAt := mustParseTime(t, "2026-06-11T12:00:00Z")
	sched.Enabled = false
	tracker, changed = ensureWorkflowScheduleExecutionTracker(history, sched, disabledAt)
	if !changed {
		t.Fatal("disabling tracker was not reported as changed")
	}
	if tracker.Enabled {
		t.Fatal("tracker remained enabled after disabling schedule")
	}
	if !tracker.WindowStartAt.Equal(disabledAt) {
		t.Fatalf("disabled window start = %s, want %s", tracker.WindowStartAt, disabledAt)
	}
	if len(tracker.Executions) != 0 {
		t.Fatalf("executions length = %d, want 0 after enabled-state reset", len(tracker.Executions))
	}
	history.Schedules[sched.ID] = tracker

	enabledAt := mustParseTime(t, "2026-06-13T09:00:00Z")
	sched.Enabled = true
	tracker, changed = ensureWorkflowScheduleExecutionTracker(history, sched, enabledAt)
	if !changed {
		t.Fatal("re-enabling tracker was not reported as changed")
	}
	if !tracker.Enabled {
		t.Fatal("tracker remained disabled after enabling schedule")
	}
	if !tracker.WindowStartAt.Equal(enabledAt) {
		t.Fatalf("enabled window start = %s, want %s", tracker.WindowStartAt, enabledAt)
	}
}

func TestWorkflowScheduleTrackingWindowStartSurvivesEmptySchedulerState(t *testing.T) {
	workspace := httptest.NewServer(&mockWorkspaceAPI{files: map[string]string{}})
	defer workspace.Close()
	t.Setenv("WORKSPACE_API_URL", workspace.URL)
	workspacePath := "Workflow/demo"
	sched := WorkflowSchedule{
		ID:             "weekly",
		CronExpression: "0 8 * * 0",
		Timezone:       "UTC",
		Enabled:        true,
	}
	want := mustParseTime(t, "2026-08-05T10:00:00Z")
	if err := EnsureWorkflowScheduleExecutionTracker(context.Background(), workspacePath, sched, want); err != nil {
		t.Fatal(err)
	}

	got, ok := WorkflowScheduleTrackingWindowStart(context.Background(), workspacePath, sched.ID)
	if !ok {
		t.Fatal("tracking window was not recovered")
	}
	if !got.Equal(want) {
		t.Fatalf("tracking window = %s, want %s", got, want)
	}

	service := NewSchedulerService(nil)
	sctx := &ScheduleContext{
		WorkspacePath: workspacePath,
		SourceType:    "workflow",
		Schedule:      sched,
	}
	if err := service.LoadSchedule(sctx); err != nil {
		t.Fatal(err)
	}
	job := service.jobs[scheduleRuntimeKey(sctx)]
	if job == nil {
		t.Fatal("workflow cron job was not registered")
	}
	if !job.lastFired.Equal(want) {
		t.Fatalf("registered cron cursor = %s, want persisted tracking window %s", job.lastFired, want)
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("failed to parse %q: %v", value, err)
	}
	return parsed
}

func TestRecordWorkflowSchedulePreflightFailureFailsOpenAtThreshold(t *testing.T) {
	const workspacePath = "Workflow/linkedin"
	sched := WorkflowSchedule{ID: "sched-1", CronExpression: "0 10 * * *", Timezone: "UTC", Enabled: true}

	workspace := &mockWorkspaceAPI{files: map[string]string{}}
	server := httptest.NewServer(workspace)
	defer server.Close()
	t.Setenv("WORKSPACE_API_URL", server.URL)

	ctx := context.Background()
	now := mustParseTime(t, "2026-08-08T10:00:00Z")

	for i := 1; i < workflowSchedulePreflightFailOpenThreshold; i++ {
		failOpen, count, err := RecordWorkflowSchedulePreflightFailure(ctx, workspacePath, sched, "1.0.21", now)
		if err != nil {
			t.Fatalf("failure %d: unexpected error: %v", i, err)
		}
		if failOpen {
			t.Fatalf("failure %d/%d: failOpen=true, want false before threshold", count, workflowSchedulePreflightFailOpenThreshold)
		}
		if count != i {
			t.Fatalf("failure count = %d, want %d", count, i)
		}
	}

	failOpen, count, err := RecordWorkflowSchedulePreflightFailure(ctx, workspacePath, sched, "1.0.21", now)
	if err != nil {
		t.Fatalf("threshold failure: unexpected error: %v", err)
	}
	if !failOpen {
		t.Fatalf("failure count = %d, want failOpen=true at threshold %d", count, workflowSchedulePreflightFailOpenThreshold)
	}
	if count != workflowSchedulePreflightFailOpenThreshold {
		t.Fatalf("failure count = %d, want %d", count, workflowSchedulePreflightFailOpenThreshold)
	}

	// A different target version (a different, or since-partially-resolved,
	// migration) must reset the streak rather than keep compounding it.
	failOpen, count, err = RecordWorkflowSchedulePreflightFailure(ctx, workspacePath, sched, "1.0.16", now)
	if err != nil {
		t.Fatalf("changed-target failure: unexpected error: %v", err)
	}
	if failOpen || count != 1 {
		t.Fatalf("changed-target failure: failOpen=%v count=%d, want failOpen=false count=1", failOpen, count)
	}

	if err := ClearWorkflowSchedulePreflightFailures(ctx, workspacePath, sched); err != nil {
		t.Fatalf("ClearWorkflowSchedulePreflightFailures: unexpected error: %v", err)
	}
	history, err := ReadWorkflowScheduleExecutionHistory(ctx, workspacePath)
	if err != nil {
		t.Fatalf("ReadWorkflowScheduleExecutionHistory: unexpected error: %v", err)
	}
	tracker := history.Schedules[sched.ID]
	if tracker.PreflightFailureCount != 0 || tracker.PreflightFailureTarget != "" {
		t.Fatalf("tracker after clear = %+v, want zeroed preflight failure state", tracker)
	}
}

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	virtualtools "github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/terminals"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/costledger"
	todo_creation_human "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/schedulerstate"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workflowtypes"
	"github.com/robfig/cron/v3"
)

func TestBuildScheduleCronExpressionAlwaysSetsTimezone(t *testing.T) {
	tests := []struct {
		name     string
		cronExpr string
		timezone string
		want     string
	}{
		{
			name:     "utc timezone is explicit",
			cronExpr: "0 9 * * *",
			timezone: "UTC",
			want:     "CRON_TZ=UTC 0 9 * * *",
		},
		{
			name:     "empty timezone defaults to UTC",
			cronExpr: "0 9 * * *",
			timezone: "",
			want:     "CRON_TZ=UTC 0 9 * * *",
		},
		{
			name:     "named timezone is preserved",
			cronExpr: "0 18 * * *",
			timezone: "America/New_York",
			want:     "CRON_TZ=America/New_York 0 18 * * *",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildScheduleCronExpression(tt.cronExpr, tt.timezone); got != tt.want {
				t.Fatalf("buildScheduleCronExpression() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPulseReviewFixCostContextUsesOnlyReviewFixWindow(t *testing.T) {
	ledger, err := costledger.NewSQLiteLedger(filepath.Join(t.TempDir(), "costs.sqlite"))
	if err != nil {
		t.Fatalf("NewSQLiteLedger() error = %v", err)
	}
	defer ledger.Close()
	start := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	for _, entry := range []costledger.Entry{
		{EventID: "prior", IdempotencyKey: "prior", Timestamp: start.Add(-time.Second), WorkflowID: "Workflow/demo", Scope: "pulse", LLMCallCount: 1, TotalCostUSD: 90},
		{EventID: "gate", IdempotencyKey: "gate", Timestamp: start.Add(time.Second), WorkflowID: "Workflow/demo", Scope: "pulse", LLMCallCount: 1, TotalCostUSD: 1.25, BillingBasis: "subscription_shadow"},
		{EventID: "review", IdempotencyKey: "review", Timestamp: start.Add(2 * time.Second), WorkflowID: "Workflow/demo", Scope: "pulse", LLMCallCount: 2, TotalCostUSD: 2.75, BillingBasis: "subscription_shadow"},
		{EventID: "run", IdempotencyKey: "run", Timestamp: start.Add(2 * time.Second), WorkflowID: "Workflow/demo", Scope: "workflow_execution", LLMCallCount: 1, TotalCostUSD: 80},
	} {
		if err := ledger.Append(entry); err != nil {
			t.Fatalf("Append(%q) error = %v", entry.EventID, err)
		}
	}

	got := pulseReviewFixCostContext(ledger, "Workflow/demo", start, start.Add(3*time.Second))
	for _, want := range []string{"$4.00", "3 LLM call(s)", "estimated token-equivalent cost", "excludes Gate, Finalize"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pulseReviewFixCostContext() missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "$90") || strings.Contains(got, "$80") {
		t.Fatalf("pulseReviewFixCostContext() mixed prior/workflow cost: %s", got)
	}
}

func TestPulseReviewFixCostContextReportsSkippedStageAsZero(t *testing.T) {
	got := pulseReviewFixCostContext(nil, "Workflow/demo", time.Time{}, time.Time{})
	if !strings.Contains(got, "not run") || !strings.Contains(got, "$0.00") {
		t.Fatalf("skipped Review+Fix cost context = %q", got)
	}
}

func TestMergeRuntimeStatePreservesRunningPulseForSameSession(t *testing.T) {
	runtimeStarted := time.Now().Add(-time.Minute).UTC()
	historyStarted := runtimeStarted.Add(time.Millisecond)
	duration := int64(1200)

	got := mergeRuntimeStateWithRuns(ScheduleRuntimeState{
		LastStatus:    "running",
		LastRunAt:     &runtimeStarted,
		LastSessionID: "session-1",
	}, "schedule-1", []ScheduleRunEntry{{
		ID:         "run-1",
		ScheduleID: "schedule-1",
		SessionID:  "session-1",
		Status:     "success",
		StartedAt:  historyStarted,
		DurationMs: &duration,
	}})

	if got.LastStatus != "running" {
		t.Fatalf("LastStatus = %q, want running while Pulse owns the session", got.LastStatus)
	}
	if got.LastSessionID != "session-1" {
		t.Fatalf("LastSessionID = %q, want session-1", got.LastSessionID)
	}
}

func TestMergeRuntimeStateAdoptsGenuinelyNewerRun(t *testing.T) {
	runtimeStarted := time.Now().Add(-time.Hour).UTC()
	historyStarted := runtimeStarted.Add(30 * time.Minute)

	got := mergeRuntimeStateWithRuns(ScheduleRuntimeState{
		LastStatus:    "running",
		LastRunAt:     &runtimeStarted,
		LastSessionID: "old-session",
	}, "schedule-1", []ScheduleRunEntry{{
		ID:         "run-2",
		ScheduleID: "schedule-1",
		SessionID:  "new-session",
		Status:     "success",
		StartedAt:  historyStarted,
	}})

	if got.LastStatus != "success" || got.LastSessionID != "new-session" {
		t.Fatalf("merged state = %+v, want newer persisted run", got)
	}
}

func TestUpdateRuntimeStateSerializesMutations(t *testing.T) {
	svc := NewSchedulerService(nil)
	const workers = 32
	const increments = 250

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < increments; j++ {
				svc.updateRuntimeState("schedule-1", func(state *ScheduleRuntimeState) {
					state.RunCount++
				})
			}
		}()
	}
	wg.Wait()

	got := svc.GetRuntimeState("schedule-1")
	if got.RunCount != workers*increments {
		t.Fatalf("RunCount = %d, want %d", got.RunCount, workers*increments)
	}
}

func TestRuntimeStateIsScopedAcrossUsersAndWorkflows(t *testing.T) {
	svc := NewSchedulerService(nil)
	userOneKey := multiAgentScheduleRuntimeKey("user-1", "builtin-org-pulse")
	userTwoKey := multiAgentScheduleRuntimeKey("user-2", "builtin-org-pulse")
	workflowOneKey := workflowScheduleRuntimeKey("Workflow/one", "copied-schedule")
	workflowTwoKey := workflowScheduleRuntimeKey("Workflow/two", "copied-schedule")

	svc.updateRuntimeState(userOneKey, func(state *ScheduleRuntimeState) {
		state.LastStatus = "running"
		state.LastSessionID = "user-1-session"
	})
	svc.updateRuntimeState(userTwoKey, func(state *ScheduleRuntimeState) {
		state.LastStatus = "success"
		state.LastSessionID = "user-2-session"
	})
	svc.updateRuntimeState(workflowOneKey, func(state *ScheduleRuntimeState) {
		state.LastStatus = "running"
	})
	svc.updateRuntimeState(workflowTwoKey, func(state *ScheduleRuntimeState) {
		state.LastStatus = "stopped"
	})

	if got := svc.getRuntimeStateByKey(userOneKey); got.LastSessionID != "user-1-session" || got.LastStatus != "running" {
		t.Fatalf("user 1 state = %+v", got)
	}
	if got := svc.getRuntimeStateByKey(userTwoKey); got.LastSessionID != "user-2-session" || got.LastStatus != "success" {
		t.Fatalf("user 2 state = %+v", got)
	}
	if got := svc.getRuntimeStateByKey(workflowOneKey); got.LastStatus != "running" {
		t.Fatalf("workflow one state = %+v", got)
	}
	if got := svc.getRuntimeStateByKey(workflowTwoKey); got.LastStatus != "stopped" {
		t.Fatalf("workflow two state = %+v", got)
	}
	if got := svc.GetRuntimeState("builtin-org-pulse"); got.LastStatus != "" {
		t.Fatalf("ambiguous unscoped state = %+v, want empty", got)
	}
}

func TestScheduleStateLockKeyFromRuntimeKey(t *testing.T) {
	workflowKey := workflowScheduleRuntimeKey("/tmp/Workflow/demo", "daily")
	wantWorkflow := strings.Join([]string{"workflow", "/tmp/Workflow/demo"}, scheduleScopeSeparator)
	if got := scheduleStateLockKeyFromRuntimeKey(workflowKey); got != wantWorkflow {
		t.Fatalf("workflow lock key = %q, want %q", got, wantWorkflow)
	}
	multiAgentKey := multiAgentScheduleRuntimeKey("user-1", "daily")
	if got := scheduleStateLockKeyFromRuntimeKey(multiAgentKey); got != multiAgentKey {
		t.Fatalf("multi-agent lock key = %q, want %q", got, multiAgentKey)
	}
	pulseKey := workflowScheduleRuntimeKey("/tmp/Workflow/demo", manualWorkflowPulseScheduleID)
	wantPulse := strings.Join([]string{"workflow-pulse", "/tmp/Workflow/demo"}, scheduleScopeSeparator)
	if got := scheduleStateLockKeyFromRuntimeKey(pulseKey); got != wantPulse {
		t.Fatalf("Pulse lock key = %q, want %q", got, wantPulse)
	}
}

func TestPulseAndWorkflowScheduleUseSeparateDurableLanes(t *testing.T) {
	store, err := schedulerstate.Open(filepath.Join(t.TempDir(), "schedule-state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc := NewSchedulerService(nil)
	svc.stateStore = store
	manifest := &WorkflowManifest{ID: "demo"}
	pulse := buildScheduleContext("Workflow/demo", manifest, WorkflowSchedule{ID: manualWorkflowPulseScheduleID})
	daily := buildScheduleContext("Workflow/demo", manifest, WorkflowSchedule{ID: "daily"})
	now := time.Now().UTC()

	if err := svc.claimScheduleRun(context.Background(), pulse, "pulse-run", now); err != nil {
		t.Fatalf("claim Pulse lane: %v", err)
	}
	if err := svc.claimScheduleRun(context.Background(), daily, "daily-run", now); err != nil {
		t.Fatalf("workflow schedule should coexist with Pulse/chat lane: %v", err)
	}
	if err := svc.claimScheduleRun(context.Background(), daily, "second-daily-run", now); err == nil {
		t.Fatal("second workflow schedule unexpectedly acquired the workflow lane")
	}
}

func TestStopRunningJobCancelsBeforeSessionStarts(t *testing.T) {
	store, err := schedulerstate.Open(filepath.Join(t.TempDir(), "schedule-state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc := NewSchedulerService(nil)
	svc.stateStore = store
	sctx := buildScheduleContext("Workflow/demo", &WorkflowManifest{ID: "demo"}, WorkflowSchedule{ID: "daily"})
	runID := "run-before-session"
	if err := svc.claimScheduleRun(context.Background(), sctx, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	runCtx := svc.registerScheduleRunContext(runID)
	runtimeKey := scheduleRuntimeKey(sctx)
	svc.updateRuntimeState(runtimeKey, func(state *ScheduleRuntimeState) {
		state.ActiveRunID = runID
		state.LastStatus = "running"
	})

	svc.stopRunningJob(runtimeKey, sctx.Schedule.ID)
	select {
	case <-runCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("run context was not canceled")
	}
	run, err := store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != schedulerstate.StateStopped {
		t.Fatalf("durable state = %s, want stopped", run.State)
	}
	state := svc.getRuntimeStateByKey(runtimeKey)
	if state.LastStatus != "stopped" || state.ActiveRunID != "" {
		t.Fatalf("runtime state after stop = %+v", state)
	}
}

func TestScheduleRunPublicationRegistersCancelBeforeUnlock(t *testing.T) {
	store, err := schedulerstate.Open(filepath.Join(t.TempDir(), "schedule-state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc := NewSchedulerService(nil)
	svc.stateStore = store
	sctx := buildScheduleContext("Workflow/demo", &WorkflowManifest{ID: "demo"}, WorkflowSchedule{ID: "daily"})
	runID := "atomic-publication"
	startedAt := time.Now().UTC()
	if err := svc.claimScheduleRun(context.Background(), sctx, runID, startedAt); err != nil {
		t.Fatal(err)
	}

	runtimeKey := scheduleRuntimeKey(sctx)
	svc.runtimeStatesMu.Lock()
	state := svc.getRuntimeStateLocked(runtimeKey)
	runCtx := svc.activateScheduleRunLocked(state, runID, startedAt)
	stopStarted := make(chan struct{})
	stopDone := make(chan struct{})
	go func() {
		close(stopStarted)
		svc.stopRunningJob(runtimeKey, sctx.Schedule.ID)
		close(stopDone)
	}()
	<-stopStarted
	select {
	case <-stopDone:
		t.Fatal("stop completed before active run publication released its lock")
	case <-time.After(20 * time.Millisecond):
	}
	svc.runtimeStatesMu.Unlock()

	select {
	case <-runCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel the atomically published run context")
	}
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("stop did not complete")
	}
}

func TestStopBetweenReservationAndDurableClaimPreventsExecution(t *testing.T) {
	store, err := schedulerstate.Open(filepath.Join(t.TempDir(), "schedule-state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc := NewSchedulerService(nil)
	svc.stateStore = store
	sctx := buildScheduleContext("Workflow/demo", &WorkflowManifest{ID: "demo"}, WorkflowSchedule{ID: "daily"})
	runtimeKey := scheduleRuntimeKey(sctx)
	runID := "stop-during-claim"
	startedAt := time.Now().UTC()

	svc.runtimeStatesMu.Lock()
	state := svc.getRuntimeStateLocked(runtimeKey)
	runCtx := svc.activateScheduleRunLocked(state, runID, startedAt)
	svc.runtimeStatesMu.Unlock()

	// This is the historical race window: Stop sees the reservation before the
	// SQLite run row exists.
	svc.stopRunningJob(runtimeKey, sctx.Schedule.ID)
	if err := svc.claimScheduleRun(context.Background(), sctx, runID, startedAt); err != nil {
		t.Fatalf("durable claim after stop: %v", err)
	}
	if !svc.abortCanceledScheduleRunBeforeStart(runCtx, sctx, runtimeKey, runID) {
		t.Fatal("canceled reservation was allowed to start")
	}

	run, err := store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != schedulerstate.StateStopped {
		t.Fatalf("durable state = %s, want stopped", run.State)
	}
}

// Org Pulse itself is gone (DefaultBuiltinSchedules returns empty -- see
// builtin_schedules.go), so triggering its ID without a persisted schedule
// entry no longer resolves to a builtin at all -- it is simply not found.
func TestTriggerMultiAgentNowWithoutScheduleFileReportsNotFound(t *testing.T) {
	workspace := httptest.NewServer(&mockWorkspaceAPI{files: map[string]string{}})
	defer workspace.Close()
	t.Setenv("WORKSPACE_API_URL", workspace.URL)

	svc := NewSchedulerService(nil)
	userID := "user-without-schedule-file"

	_, err := svc.TriggerMultiAgentNow(userID, builtinOrgPulseID)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("TriggerMultiAgentNow() error = %v, want not-found now that org pulse has no builtin to resolve", err)
	}
}

func TestGetRuntimeStateForUserReconcilesStaleRun(t *testing.T) {
	workspace := httptest.NewServer(&mockWorkspaceAPI{files: map[string]string{}})
	defer workspace.Close()
	t.Setenv("WORKSPACE_API_URL", workspace.URL)

	userID := "user-1"
	scheduleID := "daily"
	if err := AppendMultiAgentScheduleRun(context.Background(), userID, &ScheduleRunEntry{
		ID:         "stale-run",
		ScheduleID: scheduleID,
		SessionID:  "missing-session",
		Status:     "running",
		StartedAt:  time.Now().Add(-time.Minute).UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewSchedulerService(&StreamingAPI{})
	state := svc.GetRuntimeStateForUser(userID, scheduleID)
	if state.LastStatus != "error" || !strings.Contains(state.LastError, "session not active") {
		t.Fatalf("reconciled state = %+v, want stale run finalized as error", state)
	}
	runs, err := ReadMultiAgentScheduleRuns(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "error" || runs[0].CompletedAt == nil {
		t.Fatalf("persisted runs = %+v, want finalized stale run", runs)
	}
}

func TestScheduleConfigFingerprintChangesWithCapabilities(t *testing.T) {
	sctx := buildMultiAgentScheduleContext("user-1", WorkflowSchedule{ID: "daily", Enabled: true, CronExpression: "0 9 * * *", Timezone: "UTC"}, WorkflowCapabilities{})
	first := scheduleConfigFingerprint(sctx)
	second := scheduleConfigFingerprint(sctx)
	if first == "" || first != second {
		t.Fatalf("fingerprint should be stable and non-empty: %q %q", first, second)
	}
	sctx.Capabilities.SelectedSkills = []string{"research"}
	if changed := scheduleConfigFingerprint(sctx); changed == first {
		t.Fatal("capability change did not change schedule fingerprint")
	}
}

func TestScheduleWithReloadedCalendarItemUsesLatestOverrides(t *testing.T) {
	requested := &CalendarScheduleItem{ID: "item-1", Date: "2030-01-01", Time: "09:00", Messages: []string{"old"}}
	sched := WorkflowSchedule{
		ID: "calendar", ScheduleType: "calendar", Timezone: "UTC",
		Messages:      []string{"base"},
		CalendarItems: []CalendarScheduleItem{{ID: "item-1", Date: requested.Date, Time: requested.Time, Messages: []string{"new"}}},
	}
	resolved, item, ok := scheduleWithReloadedCalendarItem(sched, requested)
	if !ok || item == nil {
		t.Fatal("expected calendar item to resolve")
	}
	if got := strings.Join(resolved.Messages, ","); got != "new" {
		t.Fatalf("resolved messages = %q, want latest override", got)
	}
}

func TestLoadScheduleReplacesCalendarRegistrations(t *testing.T) {
	svc := NewSchedulerService(nil)
	when := time.Now().UTC().Add(24 * time.Hour)
	item := CalendarScheduleItem{ID: "item-1", Date: when.Format("2006-01-02"), Time: when.Format("15:04"), Messages: []string{"old"}}
	sched := WorkflowSchedule{ID: "calendar", Name: "Calendar", Enabled: true, ScheduleType: "calendar", Timezone: "UTC", CalendarItems: []CalendarScheduleItem{item}}
	if err := svc.LoadSchedule(buildMultiAgentScheduleContext("user-1", sched, WorkflowCapabilities{})); err != nil {
		t.Fatalf("initial load: %v", err)
	}

	sched.CalendarItems[0].Messages = []string{"new"}
	if err := svc.LoadSchedule(buildMultiAgentScheduleContext("user-1", sched, WorkflowCapabilities{})); err != nil {
		t.Fatalf("reload: %v", err)
	}

	keyPrefix := multiAgentScheduleRuntimeKey("user-1", sched.ID) + "__cal__"
	matching := 0
	for key, job := range svc.jobs {
		if !strings.HasPrefix(key, keyPrefix) {
			continue
		}
		matching++
		if job.sctx.CalendarItem == nil || strings.Join(job.sctx.Schedule.Messages, ",") != "new" {
			t.Fatalf("calendar registration did not retain latest item override: %+v", job.sctx)
		}
	}
	if matching != 1 {
		t.Fatalf("calendar registrations = %d, want 1", matching)
	}
}

func TestLoadScheduleDoesNotRememberInvalidCronFingerprint(t *testing.T) {
	svc := NewSchedulerService(nil)
	sctx := buildMultiAgentScheduleContext("user-1", WorkflowSchedule{
		ID: "daily", Enabled: true, CronExpression: "not-a-cron", Timezone: "UTC",
	}, WorkflowCapabilities{})
	runtimeKey := scheduleRuntimeKey(sctx)
	if err := svc.LoadSchedule(sctx); err == nil {
		t.Fatal("invalid cron unexpectedly loaded")
	}
	if _, ok := svc.scheduleFingerprints[runtimeKey]; ok {
		t.Fatal("invalid cron fingerprint should not suppress the next rescan")
	}

	sctx.Schedule.CronExpression = "0 9 * * *"
	if err := svc.LoadSchedule(sctx); err != nil {
		t.Fatalf("corrected cron was not retried: %v", err)
	}
	if _, ok := svc.scheduleFingerprints[runtimeKey]; !ok {
		t.Fatal("valid schedule fingerprint was not recorded")
	}
}

func TestRemoveJobDropsInactiveRuntimeState(t *testing.T) {
	svc := NewSchedulerService(nil)
	runtimeKey := workflowScheduleRuntimeKey("Workflow/demo", "daily")
	svc.updateRuntimeState(runtimeKey, func(state *ScheduleRuntimeState) {
		state.LastStatus = "success"
	})
	if err := svc.removeJobByKey(runtimeKey); err != nil {
		t.Fatal(err)
	}
	svc.runtimeStatesMu.RLock()
	_, exists := svc.runtimeStates[runtimeKey]
	svc.runtimeStatesMu.RUnlock()
	if exists {
		t.Fatal("removed schedule retained inactive runtime state")
	}
}

func TestLoadScheduleKeepsSameIDForDifferentUsers(t *testing.T) {
	svc := NewSchedulerService(nil)
	for _, userID := range []string{"user-1", "user-2"} {
		sctx := buildMultiAgentScheduleContext(userID, WorkflowSchedule{
			ID:             "builtin-org-pulse",
			Name:           "Org Pulse",
			Enabled:        true,
			CronExpression: "0 9 * * *",
			Timezone:       "UTC",
			Query:          "Run Org Pulse",
		}, WorkflowCapabilities{})
		if err := svc.LoadSchedule(sctx); err != nil {
			t.Fatalf("LoadSchedule(%s): %v", userID, err)
		}
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 scoped jobs", len(svc.jobs))
	}
	for _, userID := range []string{"user-1", "user-2"} {
		if _, ok := svc.jobs[multiAgentScheduleRuntimeKey(userID, "builtin-org-pulse")]; !ok {
			t.Fatalf("missing scoped job for %s", userID)
		}
	}
}

func TestValidateScheduleTimezone(t *testing.T) {
	valid := []string{"UTC", "Asia/Kolkata", "America/New_York"}
	for _, timezone := range valid {
		t.Run("valid "+timezone, func(t *testing.T) {
			if err := ValidateScheduleTimezone(timezone); err != nil {
				t.Fatalf("ValidateScheduleTimezone(%q) returned error: %v", timezone, err)
			}
		})
	}

	invalid := []string{"", "IST", "EST", "Not/AZone"}
	for _, timezone := range invalid {
		t.Run("invalid "+timezone, func(t *testing.T) {
			if err := ValidateScheduleTimezone(timezone); err == nil {
				t.Fatalf("ValidateScheduleTimezone(%q) returned nil error", timezone)
			}
		})
	}
}

func TestWorkflowScheduleShouldResumePreviousIsOptIn(t *testing.T) {
	trueValue := true
	falseValue := false

	tests := []struct {
		name           string
		resumePrevious *bool
		want           bool
	}{
		{
			name: "omitted starts fresh",
			want: false,
		},
		{
			name:           "explicit false starts fresh",
			resumePrevious: &falseValue,
			want:           false,
		},
		{
			name:           "explicit true resumes",
			resumePrevious: &trueValue,
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched := WorkflowSchedule{ResumePrevious: tt.resumePrevious}
			if got := sched.ShouldResumePrevious(); got != tt.want {
				t.Fatalf("ShouldResumePrevious() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkflowScheduleListExposesWorkshopMode(t *testing.T) {
	workspacePath := "Workflow/social-media"
	manifest := &WorkflowManifest{
		SchemaVersion: WorkflowManifestSchemaVersion,
		ID:            "social-media",
		Label:         "Social Media",
		Schedules: []WorkflowSchedule{
			{
				ID:             "run-schedule",
				Name:           "Daily publish",
				CronExpression: "0 9 * * *",
				Timezone:       "Asia/Kolkata",
				Enabled:        true,
				GroupNames:     []string{"group-1"},
				Mode:           "workshop",
				WorkshopMode:   "run",
			},
			{
				ID:             "optimizer-schedule",
				Name:           "Goal Advisor",
				CronExpression: "0 23 * * 1,4",
				Timezone:       "Asia/Kolkata",
				Enabled:        true,
				GroupNames:     []string{"group-1"},
				Mode:           "workshop",
				WorkshopMode:   "optimizer",
			},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	workspace := httptest.NewServer(&mockWorkspaceAPI{files: map[string]string{
		workspacePath + "/workflow.json": string(manifestJSON),
	}})
	defer workspace.Close()
	t.Setenv("WORKSPACE_API_URL", workspace.URL)

	callbacks := (&StreamingAPI{}).buildSchedulerCallbacks()
	out, err := callbacks.ListSchedules(context.Background(), workspacePath)
	if err != nil {
		t.Fatalf("ListSchedules() error = %v", err)
	}
	for _, want := range []string{
		"## Schedules (2 found)",
		"### Daily publish",
		"- **Mode**: `workshop`",
		"- **Workshop Mode**: `run`",
		"### Goal Advisor",
		"- **Workshop Mode**: `optimizer`",
		"- **Type**: cron",
		"- **Cron**: `0 23 * * 1,4`",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("schedule list missing %q:\n%s", want, out)
		}
	}
}

// TestUpdateScheduleClearsMessagesOnlyWhenExplicitlySet pins PLAT-097:
// update_schedule(messages=[]) or update_schedule(messages=null) must clear a
// schedule's messages back to the route-based default, and omitting the field
// entirely must leave existing messages untouched. Before the fix, all three
// cases collapsed to the same "messages == nil" value downstream, so an
// explicit clear silently no-oped — the schedule reported success but
// workflow.json kept the full prior array, with no way to migrate a
// message-based schedule to the canonical route-backed model without
// deleting and recreating it (losing get_schedule_runs history).
func TestUpdateScheduleClearsMessagesOnlyWhenExplicitlySet(t *testing.T) {
	workspacePath := "Workflow/social-media"
	newManifest := func() *WorkflowManifest {
		return &WorkflowManifest{
			SchemaVersion: WorkflowManifestSchemaVersion,
			ID:            "social-media",
			Label:         "Social Media",
			Schedules: []WorkflowSchedule{
				{
					ID:             "run-schedule",
					Name:           "Daily publish",
					CronExpression: "0 9 * * *",
					Timezone:       "Asia/Kolkata",
					Enabled:        true,
					GroupNames:     []string{"group-1"},
					Mode:           "workshop",
					WorkshopMode:   "run",
					Messages:       []string{"Run the full workflow using run_full_workflow(group_name=\"group-1\")"},
				},
			},
		}
	}

	setup := func(t *testing.T) (*todo_creation_human.SchedulerCallbacks, func() []string) {
		t.Helper()
		manifest := newManifest()
		manifestJSON, err := json.Marshal(manifest)
		if err != nil {
			t.Fatalf("marshal manifest: %v", err)
		}
		variablesJSON, err := json.Marshal(VariablesManifest{
			Groups: []VariableGroup{{Name: "group-1", Enabled: true, Values: map[string]string{}}},
		})
		if err != nil {
			t.Fatalf("marshal variables manifest: %v", err)
		}
		mock := &mockWorkspaceAPI{files: map[string]string{
			workspacePath + "/workflow.json":            string(manifestJSON),
			workspacePath + "/variables/variables.json": string(variablesJSON),
		}}
		workspace := httptest.NewServer(mock)
		t.Cleanup(workspace.Close)
		t.Setenv("WORKSPACE_API_URL", workspace.URL)

		callbacks := (&StreamingAPI{}).buildSchedulerCallbacks()
		currentMessages := func() []string {
			mock.mu.Lock()
			content := mock.files[workspacePath+"/workflow.json"]
			mock.mu.Unlock()
			var current WorkflowManifest
			if err := json.Unmarshal([]byte(content), &current); err != nil {
				t.Fatalf("unmarshal persisted manifest: %v", err)
			}
			return current.Schedules[0].Messages
		}
		return callbacks, currentMessages
	}

	t.Run("omitting messages leaves existing messages untouched", func(t *testing.T) {
		callbacks, currentMessages := setup(t)
		if _, err := callbacks.UpdateSchedule(context.Background(), "run-schedule", "", "", "", nil, false, nil, false, nil, "", nil, false, nil, "", nil, nil); err != nil {
			t.Fatalf("UpdateSchedule() error = %v", err)
		}
		if got := currentMessages(); len(got) != 1 {
			t.Fatalf("messages = %v, want unchanged 1-item array", got)
		}
	})

	// Both interactive_workshop_manager.go's append-loop parser and
	// workflow_schedule_tools.go's stringSlice() helper turn a JSON []
	// (empty array) into a nil Go slice, same as an explicit JSON null — so
	// this is the shape that actually reaches UpdateSchedule for messages=[]
	// through the primary (workshop) tool path. setMessages=true is what
	// must carry the "clear" intent, since messages itself is
	// indistinguishable from "omitted".
	t.Run("explicit empty array clears messages (nil-slice parsing shape)", func(t *testing.T) {
		callbacks, currentMessages := setup(t)
		if _, err := callbacks.UpdateSchedule(context.Background(), "run-schedule", "", "", "", nil, false, nil, false, nil, "", nil, true, nil, "", nil, nil); err != nil {
			t.Fatalf("UpdateSchedule() error = %v", err)
		}
		if got := currentMessages(); len(got) != 0 {
			t.Fatalf("messages = %v, want cleared to empty", got)
		}
	})

	// workflow_schedule_tools.go's stringSlice() helper produces a non-nil
	// empty slice for messages=[] specifically (unlike the workshop path
	// above) — pin that setMessages, not messages' nil-ness, is what
	// UpdateSchedule must trust either way.
	t.Run("explicit empty array clears messages (non-nil empty-slice parsing shape)", func(t *testing.T) {
		callbacks, currentMessages := setup(t)
		if _, err := callbacks.UpdateSchedule(context.Background(), "run-schedule", "", "", "", nil, false, nil, false, nil, "", []string{}, true, nil, "", nil, nil); err != nil {
			t.Fatalf("UpdateSchedule() error = %v", err)
		}
		if got := currentMessages(); len(got) != 0 {
			t.Fatalf("messages = %v, want cleared to empty", got)
		}
	})

	t.Run("explicit null clears messages", func(t *testing.T) {
		callbacks, currentMessages := setup(t)
		if _, err := callbacks.UpdateSchedule(context.Background(), "run-schedule", "", "", "", nil, false, nil, false, nil, "", nil, true, nil, "", nil, nil); err != nil {
			t.Fatalf("UpdateSchedule() error = %v", err)
		}
		if got := currentMessages(); len(got) != 0 {
			t.Fatalf("messages = %v, want cleared to empty", got)
		}
	})
}

func TestShouldUpdateChiefTaskReport(t *testing.T) {
	tests := []struct {
		name string
		sctx *ScheduleContext
		want bool
	}{
		{
			name: "normal chief schedule updates task report",
			sctx: &ScheduleContext{
				SourceType: "multi-agent",
				Schedule: WorkflowSchedule{
					ID:          "weekly-market-review",
					Name:        "Weekly market review",
					Description: "Review three workflows and recommend changes",
					Query:       "Prepare a cross-workflow recommendation report.",
				},
			},
			want: true,
		},
		{
			name: "workflow schedule does not update chief task report",
			sctx: &ScheduleContext{
				SourceType: "workflow",
				Schedule:   WorkflowSchedule{ID: "daily-run", Name: "Daily run"},
			},
			want: false,
		},
		{
			name: "builtin org pulse is excluded",
			sctx: &ScheduleContext{
				SourceType: "multi-agent",
				Schedule:   WorkflowSchedule{ID: builtinOrgPulseID, Name: "Daily Org Pulse"},
			},
			want: false,
		},
		{
			name: "org pulse duplicate is excluded",
			sctx: &ScheduleContext{
				SourceType: "multi-agent",
				Schedule:   WorkflowSchedule{ID: "custom-pulse", Name: "Daily Org Pulse scan"},
			},
			want: false,
		},
		{
			name: "deprecated builtin memory schedule is excluded",
			sctx: &ScheduleContext{
				SourceType: "multi-agent",
				Schedule:   WorkflowSchedule{ID: deprecatedAutoEnrichMemoryID, Name: "Auto-enrich memory"},
			},
			want: false,
		},
		{
			name: "memory-like schedule is excluded",
			sctx: &ScheduleContext{
				SourceType: "multi-agent",
				Schedule: WorkflowSchedule{
					ID:    "custom-memory",
					Name:  "Memory enrichment",
					Query: "Run enrich_memory for recent conversations.",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUpdateChiefTaskReport(tt.sctx); got != tt.want {
				t.Fatalf("shouldUpdateChiefTaskReport() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCreateAndUpdatePulseReviewOnlyScheduleSkipsGroupNamesRequirement pins
// PLAT-115 at the real CreateSchedule/UpdateSchedule implementations (not
// just the tool-layer guard): a pulse_review_only schedule must not be
// rejected for missing group_names at create time, and a later update that
// only changes its cron must not be rejected either — CreateSchedule's own
// explicit skip and UpdateSchedule's final unconditional
// validateScheduleGroupNamesForWorkspace revalidation (which runs
// independent of whether this call touched group_names at all) both had to
// be guarded, not just the one the caller happens to exercise first.
func TestCreateAndUpdatePulseReviewOnlyScheduleSkipsGroupNamesRequirement(t *testing.T) {
	workspacePath := "Workflow/social-media"
	manifest := &WorkflowManifest{
		SchemaVersion: WorkflowManifestSchemaVersion,
		ID:            "social-media",
		Label:         "Social Media",
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	mock := &mockWorkspaceAPI{files: map[string]string{
		workspacePath + "/workflow.json": string(manifestJSON),
		// Deliberately no variables/variables.json — a PulseReviewOnly schedule
		// must never need it, unlike an ordinary workflow-execution schedule.
	}}
	workspace := httptest.NewServer(mock)
	defer workspace.Close()
	t.Setenv("WORKSPACE_API_URL", workspace.URL)

	callbacks := (&StreamingAPI{}).buildSchedulerCallbacks()
	readManifest := func() WorkflowManifest {
		mock.mu.Lock()
		content := mock.files[workspacePath+"/workflow.json"]
		mock.mu.Unlock()
		var current WorkflowManifest
		if err := json.Unmarshal([]byte(content), &current); err != nil {
			t.Fatalf("unmarshal persisted manifest: %v", err)
		}
		return current
	}

	if _, err := callbacks.CreateSchedule(context.Background(), workspacePath, "Periodic Pulse Review", "0 3 * * *", "Asia/Kolkata",
		nil, nil, "workshop", nil, "", "run", nil, true); err != nil {
		t.Fatalf("CreateSchedule(pulseReviewOnly=true) error = %v, want success without group_names", err)
	}
	current := readManifest()
	if len(current.Schedules) != 1 {
		t.Fatalf("schedules = %d, want 1", len(current.Schedules))
	}
	if !current.Schedules[0].PulseReviewOnly {
		t.Fatal("persisted schedule must have pulse_review_only=true")
	}
	if len(current.Schedules[0].GroupNames) != 0 {
		t.Fatalf("group_names = %v, want empty for a pulse_review_only schedule", current.Schedules[0].GroupNames)
	}
	jobID := current.Schedules[0].ID

	newCron := "0 4 * * *"
	if _, err := callbacks.UpdateSchedule(context.Background(), jobID, "", newCron, "", nil, false, nil, false, nil, "", nil, false, nil, "", nil, nil); err != nil {
		t.Fatalf("UpdateSchedule() on an existing pulse_review_only schedule, changing only cron, error = %v", err)
	}
	current = readManifest()
	if current.Schedules[0].CronExpression != newCron {
		t.Fatalf("cron_expression = %q, want %q", current.Schedules[0].CronExpression, newCron)
	}
	if !current.Schedules[0].PulseReviewOnly {
		t.Fatal("pulse_review_only must survive an unrelated update")
	}
}

func TestBuildChiefTaskReportUpdateMessageUsesSingleSharedTaskHTML(t *testing.T) {
	startedAt := time.Date(2026, 7, 4, 10, 15, 0, 0, time.UTC)
	completedAt := startedAt.Add(2 * time.Minute)
	sctx := &ScheduleContext{
		SourceType: "multi-agent",
		Schedule: WorkflowSchedule{
			ID:             "weekly-market-review",
			Name:           "Weekly market review",
			Description:    "Review three workflows and recommend changes",
			Query:          "Prepare a cross-workflow recommendation report.",
			CronExpression: "0 9 * * 1",
			Timezone:       "Asia/Kolkata",
		},
	}

	msg := buildChiefTaskReportUpdateMessage(sctx, "run-123", "success", "", 120000, startedAt, completedAt, "session-abc")
	for _, want := range []string{
		`read_skill(skills=[{"name":"builder-reference","path":"references/chief-task-report.md"}])`,
		"Update the single shared Tasks page at pulse/task.html",
		"Do not create per-task files",
		"Do not edit pulse/org-pulse.html, pulse/goals.html",
		"schedule_id: weekly-market-review",
		"schedule_name: Weekly market review",
		"run_id: run-123",
		"session_id: session-abc",
		"status: success",
		"Prepare a cross-workflow recommendation report.",
		"Prepend one .task-entry",
		"key findings to reuse",
		"Treat the metadata above as internal input",
		"collapsed Agent details",
		"ordinary language",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("task report update message missing %q:\n%s", want, msg)
		}
	}
}

func TestWithChiefTaskRunContextAddsPriorTaskReportInstruction(t *testing.T) {
	sctx := &ScheduleContext{
		SourceType: "multi-agent",
		Schedule: WorkflowSchedule{
			ID:    "weekly-market-review",
			Name:  "Weekly market review",
			Query: "Prepare a cross-workflow recommendation report.",
		},
	}

	msg := withChiefTaskRunContext(sctx, sctx.Schedule.Query)
	for _, want := range []string{
		"NORMAL CHIEF OF STAFF TASK RUN",
		"read pulse/task.html if it exists",
		`data-schedule-id="weekly-market-review"`,
		"key findings",
		"durable context",
		"Do not use or update Chief of Staff memory tools/files",
		`create_human_input_request(source="chief_of_staff"`,
		`workspace_path="pulse"`,
		"do not wait in real time",
		"mark_human_input_consumed",
		"Prepare a cross-workflow recommendation report.",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("task run context missing %q:\n%s", want, msg)
		}
	}
}

func TestWithChiefTaskRunContextSkipsOrgPulse(t *testing.T) {
	sctx := &ScheduleContext{
		SourceType: "multi-agent",
		Schedule:   WorkflowSchedule{ID: builtinOrgPulseID, Name: "Daily Org Pulse"},
	}

	const query = "Run Org Pulse."
	if got := withChiefTaskRunContext(sctx, query); got != query {
		t.Fatalf("withChiefTaskRunContext() = %q, want original query", got)
	}
}

func TestPostRunMonitorUsesDynamicModulesAndSingleFinalizer(t *testing.T) {
	steps := postRunMonitorSteps()
	if got := len(steps); got != 3 {
		t.Fatalf("postRunMonitorSteps() length = %d, want 3", got)
	}
	for i, want := range []string{"gate", "review-fix", "finalize"} {
		if got := steps[i].label; got != want {
			t.Fatalf("postRunMonitorSteps()[%d].label = %q, want %q", i, got, want)
		}
	}
	reviewFix := steps[1].query
	for _, want := range []string{
		"Read the durable Gate worklist",
		"PULSE REVIEW + FIX DISPATCH",
		"run_in_background",
		"one executor message sequence",
		"Engineering Review → Stores Health",
		"every later message_sequence item a non-empty message",
		"Strategy Auditor and Goal Advisor as separate executor agents",
		"The runtime tracks registered children and waits",
	} {
		if !strings.Contains(reviewFix, want) {
			t.Fatalf("review-fix prompt missing %q:\n%s", want, reviewFix)
		}
	}
	if !strings.Contains(steps[2].query, "PULSE FINALIZER") || strings.Contains(steps[2].query, "PULSE DASHBOARD") {
		t.Fatal("only the finalizer must remain after Review+Fix")
	}
	return

	var gate string
	var bugReview string
	var artifact string
	var reportHealth string
	var evalHealth string
	var storesHealth string
	var llmOps string
	var strategyAuditor string
	var goalAdvisor string
	var dashboard string
	var finalizer string
	for _, step := range steps {
		if step.label == "gate" {
			gate = step.query
		}
		if step.label == "bug-review" {
			bugReview = step.query
		}
		if step.label == "artifact" {
			artifact = step.query
		}
		if step.label == "report-health" {
			reportHealth = step.query
		}
		if step.label == "eval-health" {
			evalHealth = step.query
		}
		if step.label == "stores-health" {
			storesHealth = step.query
		}
		if step.label == "llm-ops-review" {
			llmOps = step.query
		}
		if step.label == "strategy-auditor" {
			strategyAuditor = step.query
		}
		if step.label == "goal-advisor" {
			goalAdvisor = step.query
		}
		if step.label == "dashboard" {
			dashboard = step.query
		}
		if step.label == "finalize" {
			finalizer = step.query
		}
	}
	if gate == "" {
		t.Fatal("gate step not found")
	}
	if bugReview == "" {
		t.Fatal("bug-review step not found")
	}
	if artifact == "" {
		t.Fatal("artifact step not found")
	}
	if reportHealth == "" {
		t.Fatal("report-health step not found")
	}
	if evalHealth == "" {
		t.Fatal("eval-health step not found")
	}
	if storesHealth == "" {
		t.Fatal("stores-health step not found")
	}
	if llmOps == "" {
		t.Fatal("llm-ops-review step not found")
	}
	if strategyAuditor == "" {
		t.Fatal("strategy-auditor step not found")
	}
	if goalAdvisor == "" {
		t.Fatal("goal-advisor step not found")
	}
	if dashboard == "" {
		t.Fatal("dashboard step not found")
	}
	if finalizer == "" {
		t.Fatal("finalizer step not found")
	}
	// Detailed contracts live in focused/reference guidance. Scheduler messages
	// intentionally carry only identifiers and the exact reference to load.
	repoRoot := findRepoRoot(t)
	readContract := func(rel string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(raw)
	}
	gatePrompt := gate
	gate = gate + "\n" + readContract("agent_go/cmd/server/guidance/templates/system/pulse-gate.md") +
		"\n" + readContract("docs/design/pulse-post-run-monitor-spec.md")
	dashboardPrompt := dashboard
	dashboard = dashboard + "\n" + readContract("agent_go/cmd/server/guidance/templates/system/review-improve-log.md") +
		"\n" + readContract("docs/design/pulse-post-run-monitor-spec.md")
	finalizerPrompt := finalizer
	finalizerContract := readContract("agent_go/cmd/server/guidance/templates/system/pulse-finalizer.md")
	finalizer = finalizer + "\n" + finalizerContract +
		"\n" + readContract("docs/design/pulse-post-run-monitor-spec.md")
	for _, pair := range []struct{ prompt, ref string }{
		{gatePrompt, `read_skill(skills=[{"name":"builder-reference","path":"references/pulse-gate.md"}])`},
		{dashboardPrompt, `read_skill(skills=[{"name":"builder-reference","path":"references/review-improve-log.md"}])`},
		{finalizerPrompt, `read_skill(skills=[{"name":"builder-reference","path":"references/pulse-finalizer.md"}])`},
	} {
		if !strings.Contains(pair.prompt, pair.ref) {
			t.Fatalf("compact stage prompt missing focused reference %q: %s", pair.ref, pair.prompt)
		}
	}
	for _, want := range []string{
		"exploratory QA review",
		"behavioral contract",
		"risk-ranked exploratory QA matrix",
		"critical path",
		"negative path",
		"boundary or edge case",
		"stale/current-run isolation",
		"side-effect-free",
		"Never send email/messages, post content, trade, publish",
		"expected versus observed",
		"QA coverage",
		"untested risk",
		"semantic execution defects as Bugs too",
		"Observable execution-trace review",
		"*-conversation.json",
		"wrong tool/source",
		"ignored or misinterpreted tool results",
		"unsupported conclusions",
		"Do not request or infer hidden chain-of-thought",
		"correctness_bug",
		"efficiency_or_coaching",
		"insufficient_evidence",
		"future LLM/Ops pass",
	} {
		if !strings.Contains(bugReview, want) {
			t.Fatalf("bug-review step missing observable trace contract %q:\n%s", want, bugReview)
		}
	}
	for _, want := range []string{
		"progressive evidence scan",
		"Successful execution is never proof that behavior was",
		"Missing baseline means",
		"wrong tool/source/route/decision evidence",
		"off-track material goal",
		"bounded adaptive cadence",
	} {
		if !strings.Contains(gate, want) {
			t.Fatalf("gate step missing semantic trace trigger %q:\n%s", want, gate)
		}
	}
	for _, want := range []string{
		"OFF-TRACK GOAL QA",
		"material goal is below target, declining, or stalled",
		"distinguish a correctness bug from a strategy limitation",
		"Do not equate successful execution with correct or goal-effective behavior",
	} {
		if !strings.Contains(bugReview, want) {
			t.Fatalf("bug-review step missing off-track QA contract %q:\n%s", want, bugReview)
		}
	}
	for _, want := range []string{
		"efficiency_or_coaching findings",
		"nested JSON/MCP/shell-envelope interpretation",
		"hidden errors in successful envelopes",
		"whether tool results were actually interpreted and used correctly",
		"material goal criterion is below target",
		"quality-equivalent evidence",
		"do not rely on a deterministic Go detector",
	} {
		if !strings.Contains(llmOps, want) {
			t.Fatalf("llm-ops step missing trace coaching handoff %q:\n%s", want, llmOps)
		}
	}
	if strings.Contains(gate, "call harden_workflow(") || strings.Contains(gate, "call improve_learnings(") {
		t.Fatalf("gate step should not run selected modules directly:\n%s", gate)
	}
	for _, want := range []string{
		"PULSE GATE / WORKLIST",
		`get_pulse_state(view="module")`,
		"record_pulse_worklist exactly once",
		"Gate owns the durable worklist and the cheap per-run goal observation checkpoint",
		"record_pulse_impact",
		"Do not write `builder/improve.html`",
		"suppress a measured miss",
		"Never make one reviewer due, skipped, or delayed because another reviewer",
		"Workflow Review frequently",
		"Strategy Auditor more frequently than Goal Advisor",
		"Goal Advisor selectively",
		"independent blank-sheet",
		"For the supplied run folder, inspect every executed step/item's compact final",
		"CONCERNS:",
		"execution-final-summary.json",
		"execution-attempt-*.json",
		"session.json",
		"not automatic run failure",
	} {
		if !strings.Contains(gate, want) {
			t.Fatalf("gate step missing %q:\n%s", want, gate)
		}
	}
	for _, want := range []string{
		"bug_review",
		"artifact_review",
		"report_health",
		"eval_health",
		"stores_health",
		"llm_ops_review",
		"strategy_auditor",
		"goal_advisor",
		"do not launch reviewers",
		"Operational correctness stays Bug/Eval work",
	} {
		if !strings.Contains(gate, want) {
			t.Fatalf("gate step missing module/gating text %q:\n%s", want, gate)
		}
	}
	if !strings.Contains(bugReview, "PULSE MODULE — BUG REVIEW") {
		t.Fatalf("bug-review step should be the Bug Review module:\n%s", bugReview)
	}
	for _, want := range []string{
		"read-only reliability and exploratory QA review",
		"Pulse Fixer",
		"applies safe fixes sequentially",
		"Bug fix",
		`module="bug_review"`,
		"record_pulse_result",
	} {
		if !strings.Contains(bugReview, want) {
			t.Fatalf("bug-review step missing %q:\n%s", want, bugReview)
		}
	}
	if strings.Contains(bugReview, "harden_workflow") {
		t.Fatalf("bug-review step should not expose the removed harden tool:\n%s", bugReview)
	}
	for _, want := range []string{
		"PULSE MODULE — ARTIFACT REVIEW",
		`get_workflow_command_guidance(kind="review-artifact-drift"`,
		"read-only review separate from Bug Review",
		"mark_changelog_artifact_reviewed",
		"artifact drift",
		"record_pulse_result",
	} {
		if !strings.Contains(artifact, want) {
			t.Fatalf("artifact step missing %q:\n%s", want, artifact)
		}
	}
	for _, want := range []string{
		"PULSE MODULE — REPORT HEALTH",
		"improve-report checklist",
		"generic READ-ONLY REVIEW agent",
		"must not edit files",
		"parent Pulse Fixer applies and verifies",
		"Report fix",
		"record_pulse_result",
	} {
		if !strings.Contains(reportHealth, want) {
			t.Fatalf("report health step missing %q:\n%s", want, reportHealth)
		}
	}
	for _, want := range []string{
		"PULSE MODULE — EVAL HEALTH",
		"improve-evaluation checklist",
		"generic READ-ONLY REVIEW agent",
		"TARGET_RUN_PATH",
		"must not edit files",
		"correctness-preserving",
		"stale-evidence rejection",
		"existing human-input flow before changing goal meaning",
		"Eval fix",
		"record_pulse_result",
	} {
		if !strings.Contains(evalHealth, want) {
			t.Fatalf("eval health step missing %q:\n%s", want, evalHealth)
		}
	}
	for _, want := range []string{
		"PULSE MODULE — STORES HEALTH",
		"generic READ-ONLY REVIEW agent",
		"improve-learnings",
		"improve-knowledge",
		"improve-database",
		"lock/unlock recommendations",
		"every content-bearing Markdown file",
		"complete purity manifest",
		"learning-objective audit",
		"one semantic item, one authoritative owner",
		"kb_purity_manifest",
		"db_ownership_manifest",
		"ownership_manifest",
		"content-bearing TEXT/JSON column",
		"per-step metadata and run evidence",
		"references are part of the skill",
		"re-reading the complete package",
		"never rewrite knowledgebase/context",
		"db/README.md",
		"parent Pulse Fixer",
		"safely routes content to its authoritative owner",
		"record_pulse_result",
		`module="stores_health"`,
	} {
		if !strings.Contains(storesHealth, want) {
			t.Fatalf("stores health step missing %q:\n%s", want, storesHealth)
		}
	}
	for _, removed := range []string{"improve_learnings", "improve_kb", "improve_db"} {
		if strings.Contains(storesHealth, removed) {
			t.Fatalf("Pulse module prompts must not reference removed dedicated tool %q", removed)
		}
	}
	for _, want := range []string{
		"PULSE MODULE — OPS REVIEW",
		"read_skill(skills=[{\"name\":\"builder-reference\",\"path\":\"references/llm-selection.md\"}])",
		"agentic READ-ONLY REVIEW",
		"raw execution/evaluation/Pulse cost ledgers",
		"representative conversation/tool traces",
		"event correlation",
		"failure-status precedence",
		"HTTP and path failures",
		"retries and duplicate calls",
		"timing measurement and timeout risk",
		"serial versus parallel execution opportunities",
		"cost attribution without double-counting",
		"missing/unpriced evidence",
		"semantically correct",
		"Zero duration is unmeasured, not instant",
		"zero exit code containing explicit error evidence is suspicious",
		"proven failure, review candidate, and evidence gap",
		"do not rely on a deterministic Go detector",
		"Inventory exact model pins",
		"list_provider_models",
		"default_tier_models",
		"Do not edit configuration/files",
		"processes existing answered `llm-ops-` requests",
		"at most two material decision requests",
		"module=\"llm_ops_review\"",
		`get_workflow_command_guidance(kind="design-plan")`,
		"judge engineering fitness, never tactic quality",
	} {
		if !strings.Contains(llmOps, want) {
			t.Fatalf("LLM/Ops review step missing %q:\n%s", want, llmOps)
		}
	}
	for _, want := range []string{
		"PULSE MODULE — STRATEGY AUDITOR",
		`read_skill(skills=[{"name":"builder-reference","path":"references/strategy-auditor.md"}])`,
		"retained cross-run evidence",
		"goal-to-action-to-target/source-to-outcome causal chain",
		"repetition, concentration, saturation, exploration gaps",
		"strategy_flaw, execution_bug, measurement_gap, insufficient_evidence, or no_material_problem",
		"Missing target/source/outcome linkage is measurement_gap",
		"do not edit files or DB",
		"bounded missing pieces or corrections within the current strategy",
		"Do not wait for or consume Bug Review, Artifact Review, or Goal Advisor conclusions",
		`module="strategy_auditor"`,
		"record_pulse_result",
	} {
		if !strings.Contains(strategyAuditor, want) {
			t.Fatalf("strategy auditor step missing %q:\n%s", want, strategyAuditor)
		}
	}
	for _, want := range []string{
		"PULSE MODULE — GOAL ADVISOR",
		"blank-sheet lens",
		"Generate materially different approaches before comparing them with the current plan",
		"Do not wait for, consume, or require Strategy Auditor, Bug Review, or Artifact Review conclusions",
		"read-only strategy advisor",
		"separate read-only critic",
		"healthy 10x/headroom",
		"simplify, restructure, or a bounded experiment",
		"not a structural-hygiene fix",
		"at most two credible alternatives",
		"migration/rollback",
		"Instrumentation-only tracking is not an active strategy experiment",
		"must challenge whether the recommendation is materially better",
		"at most one active strategy experiment",
		"why incremental repair is insufficient",
		"never turns a maintenance handoff into the Goal Advisor outcome",
		"Operational correctness issues such as",
		"are handoffs to Bug Review, Eval Health, Report Health",
		// Consolidated Fixer wording, asserted verbatim: the runtime replacer
		// that used to rewrite this into per-module language is gone, so the
		// prompt the reviewer receives is the one written in the source.
		"The parent Pulse Fixer consolidates advisor and critic results",
		"record_pulse_result",
	} {
		if !strings.Contains(goalAdvisor, want) {
			t.Fatalf("goal advisor step missing %q:\n%s", want, goalAdvisor)
		}
	}
	for _, want := range []string{
		"PULSE DASHBOARD",
		"This stage alone owns Pulse render",
		`read_skill(skills=[{"name":"builder-reference","path":"references/review-improve-log.md"}])`,
		"SQLite-backed Pulse lifecycle state",
		"builder/improve.html",
		"builder/card.health.html",
		"Visible HTML contains only the two verdicts",
		"exactly 3 Latest Pulse cells",
		"Use editorial judgment: retain important active history",
		"Never omit or fail the dashboard just to hit an item count",
		"Do not render reviewer coverage",
		`data-pulse-schema="5"`,
		"no duplicated operational-detail sections",
		"#pulse-agent-handoff[data-pulse-run-id]",
		`command="dashboard"`,
		// The prompt used to say "mark command=dashboard done" without naming a
		// tool. On 2026-08-04 the dashboard stage rendered correctly, then
		// reached for mutate_workflow_db to write pulse_final_command_state
		// directly — a reasonable guess, and the wrong one, since that table is
		// framework-owned and the session has no db_access=read-write grant. The
		// stage never called the sanctioned command API, and
		// reconcilePulseDashboardCommand then marked the whole stage failed even
		// though the render was correct. The finalize step below already names
		// record_pulse_result explicitly; this closes the same gap here.
		`record_pulse_result(command="dashboard"`,
		"never mutate_workflow_db or direct SQL for it",
	} {
		if !strings.Contains(dashboard, want) {
			t.Fatalf("dashboard step missing %q:\n%s", want, dashboard)
		}
	}
	for _, want := range []string{
		"PULSE FINALIZER",
		"confirm every due module",
		"never treat missing as success",
		"in that order in this one turn",
		"record_pulse_result(command=...)",
		"dedicated Dashboard stage",
		"do not rewrite them",
		"Backup",
		"Publish",
		"Notify",
		"Backup risk: local only",
		"off-device destination",
		"live URL",
		"notify_user",
		"Issues found this pass",
		"Fixed by Pulse",
		"Still pending",
		"exact active count",
		"`fixed_verified`",
		"`changed_unverified`",
	} {
		if !strings.Contains(finalizer, want) {
			t.Fatalf("finalizer step missing %q:\n%s", want, finalizer)
		}
	}
	for _, forbidden := range []string{"Dashboard + questions", "Refresh `builder/card.health.html`", "create_human_input_request"} {
		finalizerFocusedContract := finalizerPrompt + "\n" + finalizerContract
		if strings.Contains(finalizerFocusedContract, forbidden) {
			t.Fatalf("finalizer still owns dashboard work %q:\n%s", forbidden, finalizerFocusedContract)
		}
	}
}

func TestPulseEvalGuidanceSeparatesCorrectnessRepairsFromSemanticApproval(t *testing.T) {
	repoRoot := findRepoRoot(t)
	read := func(rel string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(raw)
	}

	evalGuidance := read("agent_go/cmd/server/guidance/templates/improve/improve-evaluation.md")
	for _, want := range []string{
		"CORRECTNESS REPAIR — recommend automatic application by the Pulse Fixer; no user question",
		"binding evidence to the current run/group instead of accepting an older receipt",
		"making missing, null, empty, stale, malformed, or provider-unconfirmed evidence fail closed",
		"SEMANTIC CHANGE — require user/business approval",
		"changing a success criterion, threshold, weight, rubric interpretation",
	} {
		if !strings.Contains(evalGuidance, want) {
			t.Fatalf("improve-evaluation guidance missing %q", want)
		}
	}
	if strings.Contains(evalGuidance, "Do not edit `evaluation/evaluation_plan.json` until the user confirms.") {
		t.Fatal("improve-evaluation guidance still contains blanket approval gate")
	}

	advisorGuidance := read("agent_go/cmd/server/guidance/templates/improve/goal-advisor.md")
	for _, want := range []string{
		"strategy-first review",
		"At most one strategy experiment may be active",
		"record_pulse_impact(interventions=[...])",
		"human_input_id whenever",
	} {
		if !strings.Contains(advisorGuidance, want) {
			t.Fatalf("goal-advisor guidance missing %q", want)
		}
	}
}

func TestDesignPlanGuidanceSupportsReadOnlyPulseChecklist(t *testing.T) {
	repoRoot := findRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(repoRoot, "agent_go/cmd/server/guidance/templates/builder/design-plan.md"))
	if err != nil {
		t.Fatalf("read design-plan guidance: %v", err)
	}
	guidance := string(raw)
	for _, want := range []string{
		"parent Pulse prompt",
		"read-only checklist",
		"overrides persistence",
		"do not edit any workspace file",
		"parent Pulse Fixer remains the only writer",
		"llm_ops_review",
		"is this checklist's automated owner",
	} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("design-plan guidance missing Pulse read-only contract %q:\n%s", want, guidance)
		}
	}
}

func TestGateCanRunOpsWithoutEngineering(t *testing.T) {
	ctx := context.Background()
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/ops-only"
	pulseRunID := "pulse-ops-only"
	if _, err := recordPulseWorklist(ctx, workspacePath, pulseRunID, completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleLLMOpsReview: {Module: pulseModuleLLMOpsReview, Due: true, Reason: "Runtime cost regressed."},
	})); err != nil {
		t.Fatalf("record ops-only worklist: %v", err)
	}
	due, err := pulseWorklistHasDueModule(ctx, workspacePath, pulseRunID)
	if err != nil {
		t.Fatalf("inspect ops-only worklist: %v", err)
	}
	if !due {
		t.Fatal("ops-only worklist should schedule the agent-owned Review+Fix turn")
	}
}

func TestGateCanSkipEveryReviewAndFixer(t *testing.T) {
	ctx := context.Background()
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspacePath := "Workflow/all-reviews-skipped"
	pulseRunID := "pulse-all-reviews-skipped"
	if _, err := recordPulseWorklist(ctx, workspacePath, pulseRunID, completePulseWorklistDecisions(nil)); err != nil {
		t.Fatalf("record all-skipped worklist: %v", err)
	}

	due, err := pulseWorklistHasDueModule(ctx, workspacePath, pulseRunID)
	if err != nil {
		t.Fatalf("inspect all-skipped worklist: %v", err)
	}
	if due {
		t.Fatal("all-skipped worklist must omit the Review+Fix turn")
	}
}

func TestPostRunMonitorPrependsWorkflowVersionUpgradeForOldManifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.0"}, "1.0.0")
}

func TestScheduledWorkshopTurnsCurrentWorkflowStartsWithScheduleMessage(t *testing.T) {
	turns, err := scheduledWorkshopTurns(&WorkflowManifest{Version: WorkflowContractCurrentVersion}, []string{"first", "second"})
	if err != nil {
		t.Fatalf("scheduledWorkshopTurns: %v", err)
	}
	if len(turns) != 2 || turns[0].label != "schedule-message-1" || turns[1].label != "schedule-message-2" {
		t.Fatalf("current workflow turns = %+v", turns)
	}
}

func TestScheduledWorkshopTurnsRejectsUnknownVersionBeforeScheduleMessage(t *testing.T) {
	turns, err := scheduledWorkshopTurns(&WorkflowManifest{Version: "9.9.9"}, []string{"must not run"})
	if err == nil || !strings.Contains(err.Error(), "no complete upgrade path") {
		t.Fatalf("error = %v, want no complete upgrade path", err)
	}
	if turns != nil {
		t.Fatalf("turns = %+v, want nil", turns)
	}
}

func TestPostRunMonitorPrependsWorkflowVersionUpgradeForMissingVersion(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{}, "1.0.0")
}

func TestPostRunMonitorPrependsPublishGateUpgradeForVersion101Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.1"}, "1.0.1")
}

func TestPostRunMonitorPrependsPulseReadabilityUpgradeForVersion103Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.3"}, "1.0.3")
}

func TestPostRunMonitorPrependsPulseFilterUpgradeForVersion104Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.4"}, "1.0.4")
}

func TestPostRunMonitorPrependsRichPulseWidgetUpgradeForVersion105Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.5"}, "1.0.5")
}

func TestPostRunMonitorPrependsLegacyOptimizerCleanupUpgradeForVersion106Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.6"}, "1.0.6")
}

func TestPostRunMonitorPrependsPulseDatePickerCleanupForVersion107Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.7"}, "1.0.7")
}

func TestPostRunMonitorPrependsStableSoulAndPulseHierarchyUpgradeForVersion108Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.8"}, "1.0.8")
}

func TestPostRunMonitorPrependsMessageSequenceCodeMigrationForVersion109Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.9"}, "1.0.9")
}

func TestPostRunMonitorPrependsPulseHistoryContractUpgradeForVersion110Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.10"}, "1.0.10")
}

// assertDirectContractUpgrade checks the upgrade turns a workflow at `from`
// receives. It reads the blocking pre-run preflight, which is the only path
// that delivers contract migrations: Pulse used to deliver a second copy of
// them (b4e4fc14), the preflight replaced that (f58ac5b5), and the older path
// was finally removed once it started bundling four unverified rungs into one
// Review+Fix turn.
func assertDirectContractUpgrade(t *testing.T, manifest *WorkflowManifest, from string) {
	t.Helper()
	turns, err := scheduledWorkshopTurns(manifest, nil)
	if err != nil {
		t.Fatalf("scheduledWorkshopTurns(%s): %v", from, err)
	}
	upgrades := workflowVersionUpgradePlan(&WorkflowManifest{Version: from})
	if got, want := len(turns), len(upgrades); got != want {
		t.Fatalf("upgrade turns for %s = %d, want %d", from, got, want)
	}
	for index, upgrade := range upgrades {
		if got := turns[index].label; got != upgrade.label {
			t.Fatalf("upgrade %d label = %q, want %q", index, got, upgrade.label)
		}
		if got := turns[index].upgradeTarget; got != upgrade.to {
			t.Fatalf("upgrade %q target = %q, want %q", upgrade.label, got, upgrade.to)
		}
		// The version pair reached only the retired Pulse path for a while, so
		// the preflight never told the agent what it was migrating between.
		for _, want := range []string{
			"WORKFLOW CONTRACT UPGRADE",
			`Current workflow.json version seen by scheduler: "` + from + `"`,
			`Target workflow contract version: "` + upgrade.to + `"`,
		} {
			if !strings.Contains(turns[index].query, want) {
				t.Fatalf("upgrade %q missing %q:\n%s", upgrade.label, want, turns[index].query)
			}
		}
	}
}

func TestNoUpgradeTurnsForAWorkflowAlreadyAtTheCurrentContract(t *testing.T) {
	turns, err := scheduledWorkshopTurns(&WorkflowManifest{Version: WorkflowContractCurrentVersion}, []string{"run the workflow"})
	if err != nil {
		t.Fatalf("scheduledWorkshopTurns: %v", err)
	}
	if len(turns) != 1 || turns[0].upgradeTarget != "" {
		t.Fatalf("a current-contract workflow should start straight at its schedule message: %+v", turns)
	}
}

func TestPostRunMonitorPrependsLightweightPulseReportUpgradeForVersion118Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.18"}, "1.0.18")
}

func TestPostRunMonitorPrependsExecutivePulseJournalUpgradeForVersion119Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.19"}, "1.0.19")
}

func TestPostRunMonitorPrependsArtifactPurityUpgradeForVersion120Manifest(t *testing.T) {
	assertDirectContractUpgrade(t, &WorkflowManifest{Version: "1.0.20"}, "1.0.20")
}

func TestWorkflowHasPendingPlanChangelogArtifactReview(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		want      bool
		wantError bool
	}{
		{
			name:  "missing changelog folder",
			files: map[string]string{},
			want:  false,
		},
		{
			name: "unreviewed changelog entry",
			files: map[string]string{
				"Workflow/demo/planning/changelog/changelog-2026-07-02-06-12-46.json": `{"entries":[{"timestamp":"2026-07-02T06:12:46Z","tool":"update_regular_step","reason":"test","step_ids":["step-a"]}]}`,
			},
			want: true,
		},
		{
			name: "reviewed changelog entries",
			files: map[string]string{
				"Workflow/demo/planning/changelog/changelog-2026-07-02-06-12-46.json": `{"entries":[{"timestamp":"2026-07-02T06:12:46Z","tool":"update_regular_step","reason":"test","step_ids":["step-a"],"artifact_review":{"done":true,"reviewed_at":"2026-07-02T06:20:00Z","reviewed_by":"pulse_fixer","result":"clean"}}]}`,
			},
			want: false,
		},
		{
			name: "one unreviewed entry keeps review pending",
			files: map[string]string{
				"Workflow/demo/planning/changelog/changelog-2026-07-02-06-12-46.json": `{"entries":[{"timestamp":"2026-07-02T06:12:46Z","tool":"update_regular_step","reason":"old","artifact_review":{"done":true}},{"timestamp":"2026-07-02T06:13:46Z","tool":"update_step_config","reason":"new"}]}`,
			},
			want: true,
		},
		{
			name: "malformed changelog is pending",
			files: map[string]string{
				"Workflow/demo/planning/changelog/changelog-2026-07-02-06-12-46.json": `{`,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := httptest.NewServer(&mockWorkspaceAPI{files: tt.files})
			defer workspace.Close()
			t.Setenv("WORKSPACE_API_URL", workspace.URL)

			got, err := workflowHasPendingPlanChangelogArtifactReview(context.Background(), "Workflow/demo")
			if tt.wantError && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("workflowHasPendingPlanChangelogArtifactReview() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidatePulseDueModuleResultsRequiresAgentReceipts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/demo"
	pulseRunID := "pulse-run-results"
	if _, err := recordPulseWorklist(ctx, workspacePath, pulseRunID, completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {Module: pulseModuleWorkflowReview, Due: true, Reason: "Operational evidence."},
		pulseModuleGoalAdvisor:    {Module: pulseModuleGoalAdvisor, Due: true, Reason: "Goal evidence."},
	})); err != nil {
		t.Fatalf("record worklist: %v", err)
	}
	if err := validatePulseDueModuleResults(ctx, workspacePath, pulseRunID); err == nil || !strings.Contains(err.Error(), "workflow_review, goal_advisor") {
		t.Fatalf("missing-result validation error = %v", err)
	}
	if _, err := markPulseModuleResultFromAgent(ctx, workspacePath, pulseModuleWorkflowReview, pulseRunID, "done", "Clean review.", []string{"pulse_review_log:run:workflow_review"}); err != nil {
		t.Fatalf("mark bug review: %v", err)
	}
	if _, err := markPulseModuleResultFromAgent(ctx, workspacePath, pulseModuleGoalAdvisor, pulseRunID, "done", "Advisor review complete.", []string{"pulse_review_log:run:goal_advisor"}); err != nil {
		t.Fatalf("mark goal advisor: %v", err)
	}
	if err := validatePulseDueModuleResults(ctx, workspacePath, pulseRunID); err == nil || !strings.Contains(err.Error(), "terminal current-run review receipts") {
		t.Fatalf("missing typed-review validation error = %v", err)
	}
	seedPulseReviewLogRow(ctx, t, workspacePath, pulseModuleWorkflowReview, pulseRunID, "completed", "Clean review.")
	seedPulseReviewLogRow(ctx, t, workspacePath, pulseModuleGoalAdvisor, pulseRunID, "completed", "Advisor review complete.")
	if err := validatePulseDueModuleResults(ctx, workspacePath, pulseRunID); err != nil {
		t.Fatalf("terminal validation: %v", err)
	}
	worklist, _, err := getPulseWorklistForRun(ctx, workspacePath, pulseRunID)
	if err != nil {
		t.Fatalf("read worklist: %v", err)
	}
	if got := worklist[pulseModuleWorkflowReview].LastResult; got != "done" {
		t.Fatalf("existing completed module was overwritten: %q", got)
	}
	if got := worklist[pulseModuleGoalAdvisor].LastResult; got != "done" {
		t.Fatalf("goal advisor result = %q, want done", got)
	}
}

func TestValidatePulseDueModuleResultsRejectsRunningReviewReceipt(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/demo"
	pulseRunID := "pulse-running-review"
	if _, err := recordPulseWorklist(ctx, workspacePath, pulseRunID, completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {Module: pulseModuleWorkflowReview, Due: true, Reason: "Operational evidence."},
	})); err != nil {
		t.Fatalf("record worklist: %v", err)
	}
	if _, err := markPulseModuleResultFromAgent(ctx, workspacePath, pulseModuleWorkflowReview, pulseRunID, "done", "Review turn ended.", []string{"pulse_review_log:run:workflow_review"}); err != nil {
		t.Fatalf("mark review: %v", err)
	}
	seedPulseReviewLogRow(ctx, t, workspacePath, pulseModuleWorkflowReview, pulseRunID, "running", "")
	if err := validatePulseDueModuleResults(ctx, workspacePath, pulseRunID); err == nil || !strings.Contains(err.Error(), "workflow_review (running)") {
		t.Fatalf("running review receipt validation error = %v", err)
	}
}

func TestPulseFinalBackupRunsOnlyInParentTurn(t *testing.T) {
	finalizer := pulseStepQueryByLabel(t, postRunMonitorFinalSteps("pulse-run-1"), "finalize")
	if !strings.Contains(finalizer, `read_skill(skills=[{"name":"builder-reference","path":"references/pulse-finalizer.md"}])`) {
		t.Fatalf("finalizer does not load its focused contract: %s", finalizer)
	}
	raw, err := os.ReadFile(filepath.Join(findRepoRoot(t), "agent_go/cmd/server/guidance/templates/system/pulse-finalizer.md"))
	if err != nil {
		t.Fatalf("read finalizer contract: %v", err)
	}
	for _, required := range []string{"directly in this parent", "never through a reviewer/sub-agent", "zero-config local-git default"} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("finalizer contract missing parent-only backup guard %q", required)
		}
	}
}

func TestPostRunMonitorFinalStepsIncludesOwnerNotificationInstructions(t *testing.T) {
	runInstructions := "Include delivered outputs and the primary metric."
	pulseInstructions := "Put decisions and material fixes first."
	finalizer := pulseStepQueryByLabel(t, postRunMonitorFinalSteps("pulse-run-1", workflowNotificationContentInstructions{
		runSummary: runInstructions, pulseSummary: pulseInstructions,
	}), "finalize")
	for _, required := range []string{"WORKFLOW RUN SUMMARY INSTRUCTIONS", runInstructions, "PULSE REVIEW SUMMARY INSTRUCTIONS", pulseInstructions, "recipients, channels, secrets, permissions"} {
		if !strings.Contains(finalizer, required) {
			t.Fatalf("finalizer missing notification instruction guard %q: %s", required, finalizer)
		}
	}
	withoutInstructions := pulseStepQueryByLabel(t, postRunMonitorFinalSteps("pulse-run-1"), "finalize")
	if strings.Contains(withoutInstructions, "SUMMARY INSTRUCTIONS") {
		t.Fatalf("finalizer included owner instructions when none were configured: %s", withoutInstructions)
	}
}

func TestPostRunMonitorFinalStepsIncludesSplitNotificationRouting(t *testing.T) {
	finalizer := pulseStepQueryByLabel(t, postRunMonitorFinalSteps("pulse-run-1", workflowNotificationContentInstructions{
		runSummaryChannels:   []string{"slack"},
		pulseSummaryChannels: []string{"gmail"},
	}), "finalize")
	for _, required := range []string{"SPLIT NOTIFICATION ROUTING", `notification_kind="run_summary"`, `notification_kind="pulse_summary"`, "configured channels: slack", "configured channels: gmail"} {
		if !strings.Contains(finalizer, required) {
			t.Fatalf("finalizer missing split-route instruction %q: %s", required, finalizer)
		}
	}
}

func TestOptimizerScheduleMessagesKeepsCustomMessages(t *testing.T) {
	stored := []string{`Do not ask for confirmation. Run this custom optimizer audit and stop. Compare with any Auto Improve history already logged.`}

	got := optimizerScheduleMessages(context.Background(), "Workflow/test", stored, []string{"prod"})
	if len(got) != 1 {
		t.Fatalf("optimizerScheduleMessages() length = %d, want 1", len(got))
	}
	if got[0] != stored[0] {
		t.Fatalf("optimizerScheduleMessages() = %#v, want stored custom message", got)
	}
}

func TestLegacyGoalAdvisorMessageQueueIgnoresCustomTopicMentions(t *testing.T) {
	stored := []string{`Run a custom optimizer audit for this workflow. Compare the result with Goal Advisor and Auto Improve history, then stop.`}
	if isLegacyGoalAdvisorMessageQueue(stored) {
		t.Fatalf("custom optimizer prompt was incorrectly classified as legacy: %q", stored[0])
	}
}

func TestOptimizerScheduleMessagesNoopsWhenNoStoredMessage(t *testing.T) {
	got := optimizerScheduleMessages(context.Background(), "Workflow/test", nil, []string{"group-a"})
	if len(got) != 1 {
		t.Fatalf("optimizerScheduleMessages(nil) length = %d, want 1", len(got))
	}
	for _, want := range []string{
		"optimizer schedule is no longer the product Goal Advisor loop",
		"Goal Advisor now runs as a Pulse-selected module",
		"legacy optimizer schedule should be disabled",
	} {
		if !strings.Contains(got[0], want) {
			t.Fatalf("optimizer no-op message missing %q:\n%s", want, got[0])
		}
	}
}

func TestExecuteWorkshopJobDisablesLegacyOptimizerBeforeStartingSession(t *testing.T) {
	ctx := context.Background()
	workspacePath := "Workflow/demo"
	manifest := &WorkflowManifest{
		SchemaVersion: WorkflowManifestSchemaVersion,
		ID:            "demo",
		Label:         "Demo",
		Schedules: []WorkflowSchedule{
			{
				ID:             "legacy-optimizer",
				Name:           "Goal Advisor",
				CronExpression: "0 23 * * *",
				Timezone:       "UTC",
				Enabled:        true,
				GroupNames:     []string{"group-1"},
				Mode:           "workshop",
				WorkshopMode:   "optimizer",
				Messages: []string{
					"STEP 1/5 — PRE-BACKUP",
					"STEP 2/5 — IMPROVE",
				},
			},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	workspace := httptest.NewServer(&mockWorkspaceAPI{files: map[string]string{
		workspacePath + "/workflow.json": string(manifestJSON),
	}})
	defer workspace.Close()
	t.Setenv("WORKSPACE_API_URL", workspace.URL)

	s := NewSchedulerService(nil)
	_, _, err = s.executeWorkshopJob(ctx, &ScheduleContext{
		WorkspacePath: workspacePath,
		Schedule:      manifest.Schedules[0],
		SourceType:    "workflow",
	}, "")
	if err != nil {
		t.Fatalf("executeWorkshopJob() error = %v", err)
	}

	updated, found, err := ReadWorkflowManifest(ctx, workspacePath)
	if err != nil || !found {
		t.Fatalf("read updated manifest: found=%v err=%v", found, err)
	}
	if len(updated.Schedules) != 1 || updated.Schedules[0].Enabled {
		t.Fatalf("legacy optimizer schedule was not disabled: %+v", updated.Schedules)
	}
	if !updated.MonitorEnabled() {
		t.Fatal("legacy optimizer disable should enable Pulse/post_run_monitor")
	}
}

func TestOptimizerScheduleMessagesReplacesLegacyGoalAdvisorQueue(t *testing.T) {
	legacy := []string{
		"STEP 1/5 — PRE-BACKUP",
		"STEP 2/5 — IMPROVE",
		"STEP 3/5 — BACKUP FINAL STATE",
		"STEP 4/5 — PUBLISH",
		"STEP 5/5 — NOTIFY",
	}

	got := optimizerScheduleMessages(context.Background(), "Workflow/test", legacy, nil)
	if len(got) != 1 {
		t.Fatalf("optimizerScheduleMessages() length = %d, want 1", len(got))
	}
	if strings.Contains(strings.Join(got, "\n"), strings.Join(legacy, "\n")) {
		t.Fatalf("optimizerScheduleMessages() should replace legacy stored queues, got:\n%s", strings.Join(got, "\n"))
	}
	for _, want := range []string{
		"optimizer schedule is no longer the product Goal Advisor loop",
		"Pulse-selected module",
	} {
		if !strings.Contains(strings.Join(got, "\n"), want) {
			t.Fatalf("optimizerScheduleMessages() missing %q:\n%s", want, strings.Join(got, "\n"))
		}
	}
}

func TestApplyLLMAndSecretsToReqMapUsesAutoImproveOverrideOnlyForOptimizer(t *testing.T) {
	builder := &workflowtypes.AgentLLMConfig{Provider: "claude-code", ModelID: "claude-opus-4-6"}
	maintenance := &workflowtypes.AgentLLMConfig{Provider: "vertex", ModelID: "gemini-2.5-pro"}
	baseConfig := &workflowtypes.PresetLLMConfig{
		SchemaVersion:  workflowtypes.LLMConfigSchemaVersion,
		Mode:           workflowtypes.LLMConfigModeExplicit,
		BuilderLLM:     builder,
		MaintenanceLLM: maintenance,
		PulseLLM:       builder,
		TieredConfig:   &workflowtypes.TieredLLMConfig{Tier1: maintenance, Tier2: builder, Tier3: builder},
	}

	tests := []struct {
		name         string
		workshopMode string
		wantProvider string
		wantModelID  string
	}{
		{
			name:         "normal schedule uses workflow model",
			workshopMode: "run",
			wantProvider: "claude-code",
			wantModelID:  "claude-opus-4-6",
		},
		{
			name:         "optimizer schedule uses Goal Advisor override",
			workshopMode: "optimizer",
			wantProvider: "vertex",
			wantModelID:  "gemini-2.5-pro",
		},
		{
			name:         "optimizer mode is case insensitive",
			workshopMode: " OPTIMIZER ",
			wantProvider: "vertex",
			wantModelID:  "gemini-2.5-pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqMap := map[string]interface{}{}
			(&SchedulerService{}).applyLLMAndSecretsToReqMap(context.Background(), reqMap, &ScheduleContext{
				Schedule: WorkflowSchedule{WorkshopMode: tt.workshopMode},
				Capabilities: WorkflowCapabilities{
					LLMConfig: baseConfig,
				},
			})

			llmConfig, ok := reqMap["llm_config"].(map[string]interface{})
			if !ok {
				t.Fatalf("llm_config missing or wrong type: %#v", reqMap["llm_config"])
			}
			primary, ok := llmConfig["primary"].(map[string]interface{})
			if !ok {
				t.Fatalf("llm_config.primary missing or wrong type: %#v", llmConfig["primary"])
			}
			if got := primary["provider"]; got != tt.wantProvider {
				t.Fatalf("provider = %#v, want %q", got, tt.wantProvider)
			}
			if got := primary["model_id"]; got != tt.wantModelID {
				t.Fatalf("model_id = %#v, want %q", got, tt.wantModelID)
			}
			if tt.workshopMode == "run" {
				if _, ok := reqMap["llm_config_source"]; ok {
					t.Fatalf("normal run set llm_config_source: %#v", reqMap["llm_config_source"])
				}
			} else if got := reqMap["llm_config_source"]; got != llmConfigSourceScheduledAutoImprove {
				t.Fatalf("llm_config_source = %#v, want %q", got, llmConfigSourceScheduledAutoImprove)
			}
		})
	}
}

func TestApplyLLMAndSecretsToReqMapUsesCodingAgentAutoImproveDefaultForOptimizer(t *testing.T) {
	reqMap := map[string]interface{}{}
	(&SchedulerService{}).applyLLMAndSecretsToReqMap(context.Background(), reqMap, &ScheduleContext{
		Schedule: WorkflowSchedule{WorkshopMode: "optimizer"},
		Capabilities: WorkflowCapabilities{
			LLMConfig: &workflowtypes.PresetLLMConfig{
				SchemaVersion: workflowtypes.LLMConfigSchemaVersion,
				Mode:          workflowtypes.LLMConfigModeProviderProfile,
				Provider:      "claude-code",
			},
		},
	})

	llmConfig, ok := reqMap["llm_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config missing or wrong type: %#v", reqMap["llm_config"])
	}
	primary, ok := llmConfig["primary"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config.primary missing or wrong type: %#v", llmConfig["primary"])
	}
	if got := primary["provider"]; got != "claude-code" {
		t.Fatalf("provider = %#v, want claude-code", got)
	}
	if got := primary["model_id"]; got != "claude-opus-5" {
		t.Fatalf("model_id = %#v, want claude-opus-5", got)
	}
	options, ok := primary["options"].(map[string]interface{})
	if !ok {
		t.Fatalf("options missing or wrong type: %#v", primary["options"])
	}
	if got := options["reasoning_effort"]; got != "medium" {
		t.Fatalf("reasoning_effort = %#v, want medium", got)
	}
	if got := reqMap["llm_config_source"]; got != llmConfigSourceScheduledAutoImprove {
		t.Fatalf("llm_config_source = %#v, want %q", got, llmConfigSourceScheduledAutoImprove)
	}
}

func TestApplyLLMAndSecretsToReqMapPreservesAutoImproveDefaultOptions(t *testing.T) {
	reqMap := map[string]interface{}{}
	(&SchedulerService{}).applyLLMAndSecretsToReqMap(context.Background(), reqMap, &ScheduleContext{
		Schedule: WorkflowSchedule{WorkshopMode: "optimizer"},
		Capabilities: WorkflowCapabilities{
			LLMConfig: &workflowtypes.PresetLLMConfig{
				SchemaVersion: workflowtypes.LLMConfigSchemaVersion,
				Mode:          workflowtypes.LLMConfigModeProviderProfile,
				Provider:      "codex-cli",
			},
		},
	})

	llmConfig, ok := reqMap["llm_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config missing or wrong type: %#v", reqMap["llm_config"])
	}
	primary, ok := llmConfig["primary"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config.primary missing or wrong type: %#v", llmConfig["primary"])
	}
	if got := primary["provider"]; got != "codex-cli" {
		t.Fatalf("provider = %#v, want codex-cli", got)
	}
	if got := primary["model_id"]; got != "gpt-5.6-sol" {
		t.Fatalf("model_id = %#v, want gpt-5.6-sol", got)
	}
	options, ok := primary["options"].(map[string]interface{})
	if !ok {
		t.Fatalf("options missing or wrong type: %#v", primary["options"])
	}
	if got := options["reasoning_effort"]; got != "high" {
		t.Fatalf("reasoning_effort = %#v, want high", got)
	}
	if got := reqMap["llm_config_source"]; got != llmConfigSourceScheduledAutoImprove {
		t.Fatalf("llm_config_source = %#v, want %q", got, llmConfigSourceScheduledAutoImprove)
	}
}

func TestApplyPulseLLMToReqMapUsesPulseOverrideWhenConfigured(t *testing.T) {
	reqMap := map[string]interface{}{}
	builder := &workflowtypes.AgentLLMConfig{Provider: "claude-code", ModelID: "claude-opus-4-6"}
	sctx := &ScheduleContext{
		Schedule: WorkflowSchedule{WorkshopMode: "run"},
		Capabilities: WorkflowCapabilities{
			LLMConfig: &workflowtypes.PresetLLMConfig{
				SchemaVersion:  workflowtypes.LLMConfigSchemaVersion,
				Mode:           workflowtypes.LLMConfigModeExplicit,
				BuilderLLM:     builder,
				MaintenanceLLM: builder,
				TieredConfig:   &workflowtypes.TieredLLMConfig{Tier1: builder, Tier2: builder, Tier3: builder},
				PulseLLM: &workflowtypes.AgentLLMConfig{
					Provider: "codex-cli",
					ModelID:  "gpt-5.5",
					Options:  map[string]interface{}{"reasoning_effort": "high"},
				},
			},
		},
	}

	svc := &SchedulerService{}
	svc.applyLLMAndSecretsToReqMap(context.Background(), reqMap, sctx)
	svc.applyPulseLLMToReqMap(reqMap, sctx, "test-session")

	llmConfig, ok := reqMap["llm_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config missing or wrong type: %#v", reqMap["llm_config"])
	}
	primary, ok := llmConfig["primary"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config.primary missing or wrong type: %#v", llmConfig["primary"])
	}
	if got := primary["provider"]; got != "codex-cli" {
		t.Fatalf("provider = %#v, want codex-cli", got)
	}
	if got := primary["model_id"]; got != "gpt-5.5" {
		t.Fatalf("model_id = %#v, want gpt-5.5", got)
	}
	if got := reqMap["llm_config_source"]; got != llmConfigSourceScheduledPulse {
		t.Fatalf("llm_config_source = %#v, want %q", got, llmConfigSourceScheduledPulse)
	}
	options, ok := primary["options"].(map[string]interface{})
	if !ok {
		t.Fatalf("options missing or wrong type: %#v", primary["options"])
	}
	if got := options["reasoning_effort"]; got != "high" {
		t.Fatalf("reasoning_effort = %#v, want high", got)
	}
	if got := reqMap["llm_config_source"]; got != llmConfigSourceScheduledPulse {
		t.Fatalf("llm_config_source = %#v, want %q", got, llmConfigSourceScheduledPulse)
	}
}

func TestApplyGoalAdvisorLLMToReqMapUsesAdvisorOverrideWhenConfigured(t *testing.T) {
	reqMap := map[string]interface{}{}
	builder := &workflowtypes.AgentLLMConfig{Provider: "claude-code", ModelID: "claude-sonnet-5"}
	sctx := &ScheduleContext{
		Schedule: WorkflowSchedule{WorkshopMode: "run"},
		Capabilities: WorkflowCapabilities{
			LLMConfig: &workflowtypes.PresetLLMConfig{
				SchemaVersion: workflowtypes.LLMConfigSchemaVersion,
				Mode:          workflowtypes.LLMConfigModeExplicit,
				BuilderLLM:    builder,
				MaintenanceLLM: &workflowtypes.AgentLLMConfig{
					Provider: "claude-code",
					ModelID:  "claude-opus-4-8",
					Options:  map[string]interface{}{"reasoning_effort": "high"},
				},
				PulseLLM: &workflowtypes.AgentLLMConfig{
					Provider: "claude-code",
					ModelID:  "claude-sonnet-5",
					Options:  map[string]interface{}{"reasoning_effort": "high"},
				},
				TieredConfig: &workflowtypes.TieredLLMConfig{Tier1: builder, Tier2: builder, Tier3: builder},
			},
		},
	}

	svc := &SchedulerService{}
	svc.applyLLMAndSecretsToReqMap(context.Background(), reqMap, sctx)
	svc.applyGoalAdvisorLLMToReqMap(reqMap, sctx, "test-session")

	llmConfig, ok := reqMap["llm_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config missing or wrong type: %#v", reqMap["llm_config"])
	}
	primary, ok := llmConfig["primary"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config.primary missing or wrong type: %#v", llmConfig["primary"])
	}
	if got := primary["provider"]; got != "claude-code" {
		t.Fatalf("provider = %#v, want claude-code", got)
	}
	if got := primary["model_id"]; got != "claude-opus-4-8" {
		t.Fatalf("model_id = %#v, want claude-opus-4-8", got)
	}
	if got := reqMap["llm_config_source"]; got != llmConfigSourceScheduledAutoImprove {
		t.Fatalf("llm_config_source = %#v, want %q", got, llmConfigSourceScheduledAutoImprove)
	}
}

func TestBuildWorkshopRequestDisablesLiveInputDeliveryForSchedulerTurns(t *testing.T) {
	svc := &SchedulerService{}
	sctx := &ScheduleContext{
		WorkflowID:    "wf_test",
		WorkspacePath: "Workflow/test",
		Schedule:      WorkflowSchedule{ID: "daily", Name: "Daily"},
		Capabilities:  WorkflowCapabilities{},
		SourceType:    "workflow",
	}

	reqMap := svc.buildWorkshopRequest(context.Background(), sctx)
	if got := reqMap["disable_live_input_delivery"]; got != true {
		t.Fatalf("disable_live_input_delivery = %#v, want true", got)
	}
}

func TestRefreshSessionTmuxSnapshotsForIdleCheckCapturesFreshPane(t *testing.T) {
	store := terminals.NewStore()
	sessionID := "session-scheduler-refresh"
	tmuxSession := "tmux-scheduler-refresh"
	store.HandleEvent(sessionID, terminalRouteChunkEvent(sessionID, "workflow-step:review-plan", tmuxSession, "old pane", 1))

	oldRunOutput := runTerminalTmuxOutputCommand
	defer func() { runTerminalTmuxOutputCommand = oldRunOutput }()
	var calls [][]string
	runTerminalTmuxOutputCommand = func(ctx context.Context, args ...string) (string, error) {
		calls = append(calls, append([]string(nil), args...))
		return "fresh pane\n❯", nil
	}

	svc := &SchedulerService{api: &StreamingAPI{terminalStore: store}}
	if err := svc.refreshSessionTmuxSnapshotsForIdleCheck(context.Background(), sessionID); err != nil {
		t.Fatalf("refreshSessionTmuxSnapshotsForIdleCheck returned error: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("tmux capture calls = %d, want 1", len(calls))
	}
	snapshots := store.ListMetadata(sessionID)
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snapshots))
	}
	if got := snapshots[0].Content; !strings.Contains(got, "fresh pane") {
		t.Fatalf("snapshot content = %q, want fresh capture", got)
	}
	if got := snapshots[0].ContentSource; got != "tmux_capture" {
		t.Fatalf("content source = %q, want tmux_capture", got)
	}
}

func TestRefreshSessionTmuxSnapshotsForIdleCheckMarksMissingPaneStale(t *testing.T) {
	store := terminals.NewStore()
	sessionID := "session-scheduler-missing"
	tmuxSession := "tmux-scheduler-missing"
	store.HandleEvent(sessionID, terminalRouteChunkEvent(sessionID, "workflow-step:review-plan", tmuxSession, "old pane", 1))

	oldRunOutput := runTerminalTmuxOutputCommand
	defer func() { runTerminalTmuxOutputCommand = oldRunOutput }()
	runTerminalTmuxOutputCommand = func(ctx context.Context, args ...string) (string, error) {
		return "", errors.New("can't find session: tmux-scheduler-missing")
	}

	svc := &SchedulerService{api: &StreamingAPI{terminalStore: store}}
	if err := svc.refreshSessionTmuxSnapshotsForIdleCheck(context.Background(), sessionID); err != nil {
		t.Fatalf("refreshSessionTmuxSnapshotsForIdleCheck returned error: %v", err)
	}
	snapshots := store.ListMetadata(sessionID)
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snapshots))
	}
	if snapshots[0].Active {
		t.Fatalf("missing tmux snapshot should be inactive")
	}
	if got := snapshots[0].State; got != "stale" {
		t.Fatalf("state = %q, want stale", got)
	}
	if got := snapshots[0].TmuxSession; got != "" {
		t.Fatalf("tmux session = %q, want cleared", got)
	}
}

func TestRunJobDoesNotJoinAnotherActiveRun(t *testing.T) {
	startedAt := time.Now().Add(-time.Minute)
	sctx := &ScheduleContext{
		WorkspacePath: "/tmp/workflow",
		Schedule:      WorkflowSchedule{ID: "schedule-1", Name: "Active schedule"},
	}
	runtimeKey := scheduleRuntimeKey(sctx)
	svc := &SchedulerService{runtimeStates: map[string]*ScheduleRuntimeState{
		runtimeKey: {
			ActiveRunID: "active-run",
			LastStatus:  "running",
			LastRunAt:   &startedAt,
		},
	}, runCancels: map[string]context.CancelFunc{
		"stale-run": func() {},
	}}

	_, err := svc.runJob(context.Background(), sctx, "stale-run")
	if !errors.Is(err, errWorkshopSequenceInterrupted) {
		t.Fatalf("runJob error = %v, want errWorkshopSequenceInterrupted", err)
	}
	if got := svc.runtimeStates[runtimeKey].ActiveRunID; got != "active-run" {
		t.Fatalf("active run ownership changed to %q", got)
	}
}

func TestPostRunMonitorStepsUseOneTurnInactivityBoundary(t *testing.T) {
	oldNormal := schedulerWorkshopMaxInactivity
	schedulerWorkshopMaxInactivity = 10 * time.Minute
	defer func() {
		schedulerWorkshopMaxInactivity = oldNormal
	}()

	for _, label := range []string{"gate", "review-fix", "review-fix-continuation", "dashboard", "finalize"} {
		if got := (postRunMonitorStep{label: label}).idleMaxInactivity(); got != 10*time.Minute {
			t.Fatalf("%s max inactivity = %s, want 10m", label, got)
		}
	}
}

func TestReviewFixContinuationIsParentReceiptReconciliationOnly(t *testing.T) {
	step := postRunMonitorReviewFixContinuationStep("pulse-test", errors.New("workflow_review receipt missing"))
	for _, want := range []string{
		"one parent reconciliation turn, not a second fixer",
		"do not restart completed reviews",
		"do not restart completed reviews, automatically create a duplicate recovery/Fixer agent, re-run a child, or mutate workflow artifacts",
		"A separately scoped repair agent remains available when a later parent turn deliberately chooses one",
		"record_pulse_result exactly once",
	} {
		if !strings.Contains(step.query, want) {
			t.Fatalf("continuation prompt missing %q:\n%s", want, step.query)
		}
	}
}

// TestPostRunMonitorUsesLightweightFinalizeRequiresRealEvidence pins PLAT-115:
// the lightweight backup+notify-only finalizer only ever applies to a run
// that produced real evidence under an explicitly "periodic" workflow. Every
// other combination — no evidence, the periodic review pass itself
// (PulseOnly), an unset/legacy/unrecognized mode — must fall through to the
// existing behavior unchanged.
func TestPostRunMonitorUsesLightweightFinalizeRequiresRealEvidence(t *testing.T) {
	periodic := &WorkflowManifest{PostRunMonitorMode: "periodic"}
	perRun := &WorkflowManifest{}

	tests := []struct {
		name                    string
		reviewEvidenceAvailable bool
		pulseOnly               bool
		manifest                *WorkflowManifest
		want                    bool
	}{
		{"periodic, evidence, not the review pass itself", true, false, periodic, true},
		{"periodic, but no evidence this invocation", false, false, periodic, false},
		{"periodic, but this IS the periodic review pass (PulseOnly)", true, true, periodic, false},
		{"per_run (default) manifest, with evidence", true, false, perRun, false},
		{"nil manifest fails safe to per_run", true, false, nil, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := postRunMonitorUsesLightweightFinalize(test.reviewEvidenceAvailable, test.pulseOnly, test.manifest); got != test.want {
				t.Fatalf("postRunMonitorUsesLightweightFinalize(%v, %v, %+v) = %v, want %v",
					test.reviewEvidenceAvailable, test.pulseOnly, test.manifest, got, test.want)
			}
		})
	}
}

// TestLightweightFinalizeStepNeverRunsGateOrPublishesFindings pins the
// content contract of the periodic-mode per-run finalizer: it must forbid
// Gate/reviewers/Fixer, must not present old findings as new, and must mark
// publish skipped with a stated reason rather than silently omitting it.
func TestLightweightFinalizeStepNeverRunsGateOrPublishesFindings(t *testing.T) {
	steps := postRunMonitorLightweightFinalizeStep("pulse-test")
	if len(steps) != 1 || steps[0].label != "finalize" {
		t.Fatalf("steps = %+v, want exactly one \"finalize\" step", steps)
	}
	query := steps[0].query
	for _, want := range []string{
		"Do not run Gate, reviewers, or Fixer",
		"not a failure or a skip due to missing evidence",
		"mark publish skipped",
		"do not include a Pulse findings/fixes section",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("lightweight finalize prompt missing %q:\n%s", want, query)
		}
	}
}

// TestPulseReviewBacklogSummaryExcludesNonTerminalIterationZero pins PLAT-115:
// iteration-0 is the live/reused slot, never a stable identity across time,
// so Gate must never be handed it while it might still be mid-run. Rotated
// folders (iteration-1, iteration-2, ...) are always included regardless of
// status — a failed run is real evidence too, the same principle
// ProducedRunEvidence already applies elsewhere in this file.
func TestPulseReviewBacklogSummaryExcludesNonTerminalIterationZero(t *testing.T) {
	completedAt := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	startedAt := completedAt.Add(-10 * time.Minute)

	t.Run("iteration-0 still running is excluded", func(t *testing.T) {
		folders := []RunFolderInfo{
			{Name: "iteration-0", Metadata: &RunMetadata{Status: "running", StartedAt: startedAt}},
			{Name: "iteration-5", Metadata: &RunMetadata{Status: "completed", StartedAt: startedAt, CompletedAt: &completedAt}},
		}
		summary := pulseReviewBacklogSummary(folders)
		if strings.Contains(summary, "iteration-0") {
			t.Fatalf("summary must exclude a still-running iteration-0:\n%s", summary)
		}
		if !strings.Contains(summary, "iteration-5") {
			t.Fatalf("summary must include the terminal rotated folder:\n%s", summary)
		}
	})

	t.Run("iteration-0 terminal is included", func(t *testing.T) {
		folders := []RunFolderInfo{
			{Name: "iteration-0", Metadata: &RunMetadata{Status: "completed", StartedAt: startedAt, CompletedAt: &completedAt}},
		}
		summary := pulseReviewBacklogSummary(folders)
		if !strings.Contains(summary, "iteration-0") {
			t.Fatalf("summary must include a terminal iteration-0:\n%s", summary)
		}
	})

	t.Run("a failed rotated run is still real evidence", func(t *testing.T) {
		folders := []RunFolderInfo{
			{Name: "iteration-3", Metadata: &RunMetadata{Status: "failed", StartedAt: startedAt, CompletedAt: &completedAt}},
		}
		summary := pulseReviewBacklogSummary(folders)
		if !strings.Contains(summary, "iteration-3") || !strings.Contains(summary, "status=failed") {
			t.Fatalf("a failed rotated folder must still be listed:\n%s", summary)
		}
	})

	t.Run("no folders at all", func(t *testing.T) {
		if summary := pulseReviewBacklogSummary(nil); !strings.Contains(summary, "no run folders") {
			t.Fatalf("empty backlog summary = %q, want an explicit \"no run folders\" statement", summary)
		}
	})
}

// TestBacklogGateStepDefersWhatsNewReasoningToGate pins the design decision:
// Go hands Gate the raw listing and explicitly tells it to compare against
// get_pulse_state's last_checked_at itself — Go must never pre-filter "what's
// new" or silently assume every listed folder is unreviewed.
func TestBacklogGateStepDefersWhatsNewReasoningToGate(t *testing.T) {
	step := postRunMonitorBacklogGateStep("pulse-test", "- iteration-5 (status=completed, started_at=..., completed_at=...)")
	for _, want := range []string{
		"periodic Pulse review pass",
		"last_checked_at",
		"do not assume every listed folder is new",
		"run_retention_count",
		"iteration-5",
	} {
		if !strings.Contains(step.query, want) {
			t.Fatalf("backlog Gate prompt missing %q:\n%s", want, step.query)
		}
	}
}

func TestPostRunMonitorFinalStepClassification(t *testing.T) {
	tests := []struct {
		label     string
		finalStep bool
	}{
		{label: "gate"},
		{label: "review-fix"},
		{label: "dashboard", finalStep: false},
		{label: "finalize", finalStep: true},
	}
	for _, test := range tests {
		if got := isPostRunMonitorFinalStep(test.label); got != test.finalStep {
			t.Fatalf("final-step classification for %q = %v, want %v", test.label, got, test.finalStep)
		}
	}
}

func TestRunningScheduleInSetLockedFindsOtherRunningSchedule(t *testing.T) {
	states := map[string]*ScheduleRuntimeState{
		"daily":     {LastStatus: "running", LastSessionID: "session-daily"},
		"optimizer": {LastStatus: "success", LastSessionID: "session-optimizer"},
	}

	id, sessionID := runningScheduleInSetLocked(states, []string{"current", "daily", "optimizer"}, "current")
	if id != "daily" {
		t.Fatalf("running schedule id = %q, want daily", id)
	}
	if sessionID != "session-daily" {
		t.Fatalf("running schedule session = %q, want session-daily", sessionID)
	}
}

func TestRunningScheduleInSetLockedIgnoresCurrentSchedule(t *testing.T) {
	states := map[string]*ScheduleRuntimeState{
		"current": {LastStatus: "running", LastSessionID: "session-current"},
	}

	id, sessionID := runningScheduleInSetLocked(states, []string{"current"}, "current")
	if id != "" || sessionID != "" {
		t.Fatalf("running schedule = (%q, %q), want empty", id, sessionID)
	}
}

func TestApplyPulseLLMToReqMapUsesCodingAgentPulseDefaultWhenUnset(t *testing.T) {
	reqMap := map[string]interface{}{}
	sctx := &ScheduleContext{
		Schedule: WorkflowSchedule{WorkshopMode: "run"},
		Capabilities: WorkflowCapabilities{
			LLMConfig: &workflowtypes.PresetLLMConfig{
				SchemaVersion: workflowtypes.LLMConfigSchemaVersion,
				Mode:          workflowtypes.LLMConfigModeProviderProfile,
				Provider:      "claude-code",
			},
		},
	}

	svc := &SchedulerService{}
	svc.applyLLMAndSecretsToReqMap(context.Background(), reqMap, sctx)
	svc.applyPulseLLMToReqMap(reqMap, sctx, "test-session")

	llmConfig, ok := reqMap["llm_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config missing or wrong type: %#v", reqMap["llm_config"])
	}
	primary, ok := llmConfig["primary"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config.primary missing or wrong type: %#v", llmConfig["primary"])
	}
	if got := primary["provider"]; got != "claude-code" {
		t.Fatalf("provider = %#v, want claude-code", got)
	}
	if got := primary["model_id"]; got != "claude-sonnet-5" {
		t.Fatalf("model_id = %#v, want claude-sonnet-5", got)
	}
	options, ok := primary["options"].(map[string]interface{})
	if !ok {
		t.Fatalf("options missing or wrong type: %#v", primary["options"])
	}
	if got := options["reasoning_effort"]; got != "high" {
		t.Fatalf("reasoning_effort = %#v, want high", got)
	}
}

func TestApplyPulseLLMToReqMapKeepsWorkflowModelWhenNoProviderDefault(t *testing.T) {
	reqMap := map[string]interface{}{}
	builder := &workflowtypes.AgentLLMConfig{Provider: "openai", ModelID: "gpt-5.4"}
	sctx := &ScheduleContext{
		Schedule: WorkflowSchedule{WorkshopMode: "run"},
		Capabilities: WorkflowCapabilities{
			LLMConfig: &workflowtypes.PresetLLMConfig{
				SchemaVersion:  workflowtypes.LLMConfigSchemaVersion,
				Mode:           workflowtypes.LLMConfigModeExplicit,
				BuilderLLM:     builder,
				MaintenanceLLM: builder,
				PulseLLM:       builder,
				TieredConfig:   &workflowtypes.TieredLLMConfig{Tier1: builder, Tier2: builder, Tier3: builder},
			},
		},
	}

	svc := &SchedulerService{}
	svc.applyLLMAndSecretsToReqMap(context.Background(), reqMap, sctx)
	svc.applyPulseLLMToReqMap(reqMap, sctx, "test-session")

	llmConfig, ok := reqMap["llm_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config missing or wrong type: %#v", reqMap["llm_config"])
	}
	primary, ok := llmConfig["primary"].(map[string]interface{})
	if !ok {
		t.Fatalf("llm_config.primary missing or wrong type: %#v", llmConfig["primary"])
	}
	if got := primary["provider"]; got != "openai" {
		t.Fatalf("provider = %#v, want openai", got)
	}
	if got := primary["model_id"]; got != "gpt-5.4" {
		t.Fatalf("model_id = %#v, want gpt-5.4", got)
	}
}

func TestResolveChiefOfStaffLLMForScheduleUsesExplicitOverride(t *testing.T) {
	sctx := &ScheduleContext{
		SourceType: "multi-agent",
		Capabilities: WorkflowCapabilities{
			LLMConfig: &workflowtypes.PresetLLMConfig{
				SchemaVersion: workflowtypes.LLMConfigSchemaVersion,
				Mode:          workflowtypes.LLMConfigModeProviderProfile,
				Provider:      "claude-code",
				ChiefOfStaffLLM: &workflowtypes.AgentLLMConfig{
					Provider: "codex-cli",
					ModelID:  "gpt-5.5",
					Options:  map[string]interface{}{"reasoning_effort": "xhigh"},
				},
			},
		},
	}

	got := resolveChiefOfStaffLLMForSchedule(context.Background(), sctx)
	if got == nil {
		t.Fatal("resolveChiefOfStaffLLMForSchedule() = nil")
	}
	if got.Provider != "codex-cli" || got.ModelID != "gpt-5.5" {
		t.Fatalf("resolveChiefOfStaffLLMForSchedule() = %+v, want codex-cli/gpt-5.5", got)
	}
	if got.Options["reasoning_effort"] != "xhigh" {
		t.Fatalf("reasoning_effort = %#v, want xhigh", got.Options["reasoning_effort"])
	}
}

func TestResolveChiefOfStaffLLMForScheduleUsesCodingAgentDefault(t *testing.T) {
	sctx := &ScheduleContext{
		SourceType: "multi-agent",
		Capabilities: WorkflowCapabilities{
			LLMConfig: &workflowtypes.PresetLLMConfig{
				SchemaVersion: workflowtypes.LLMConfigSchemaVersion,
				Mode:          workflowtypes.LLMConfigModeProviderProfile,
				Provider:      "claude-code",
			},
		},
	}

	got := resolveChiefOfStaffLLMForSchedule(context.Background(), sctx)
	if got == nil {
		t.Fatal("resolveChiefOfStaffLLMForSchedule() = nil")
	}
	if got.Provider != "claude-code" || got.ModelID != "claude-opus-5" {
		t.Fatalf("resolveChiefOfStaffLLMForSchedule() = %+v, want claude-code/claude-opus-5", got)
	}
	if got.Options["reasoning_effort"] != "medium" {
		t.Fatalf("reasoning_effort = %#v, want medium", got.Options["reasoning_effort"])
	}
}

func TestResolveChiefOfStaffLLMFromDelegationConfigUsesExplicitScheduledModel(t *testing.T) {
	got := resolveChiefOfStaffLLMFromDelegationConfig(&virtualtools.DelegationTierConfig{
		ChiefOfStaff: &virtualtools.TierModel{
			Provider: "codex-cli",
			ModelID:  "gpt-5.5",
		},
		Main: &virtualtools.TierModel{
			Provider: "claude-code",
			ModelID:  "claude-code",
		},
	})
	if got == nil {
		t.Fatal("resolveChiefOfStaffLLMFromDelegationConfig() = nil")
	}
	if got.Provider != "codex-cli" || got.ModelID != "gpt-5.5" {
		t.Fatalf("resolveChiefOfStaffLLMFromDelegationConfig() = %+v, want codex-cli/gpt-5.5", got)
	}
}

func TestResolveChiefOfStaffLLMFromDelegationConfigUsesProviderDefault(t *testing.T) {
	got := resolveChiefOfStaffLLMFromDelegationConfig(&virtualtools.DelegationTierConfig{
		Main: &virtualtools.TierModel{
			Provider: "claude-code",
			ModelID:  "claude-code",
		},
	})
	if got == nil {
		t.Fatal("resolveChiefOfStaffLLMFromDelegationConfig() = nil")
	}
	if got.Provider != "claude-code" || got.ModelID != "claude-opus-5" {
		t.Fatalf("resolveChiefOfStaffLLMFromDelegationConfig() = %+v, want claude-code/claude-opus-5", got)
	}
}

func TestMaybeResumeLatestWorkflowThreadUsesPreviousScheduledSessionOnly(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)

	workspacePath := "Workflow/rtslatency"
	scheduleID := "schedule-1"
	writeWorkflowChatRuntime(t, root, workspacePath, "normal-user-chat", "claude-code", true)
	writeWorkflowChatRuntime(t, root, workspacePath, "previous-schedule-chat", "claude-code", true)
	writeScheduleRunsForTest(t, root, workspacePath, []ScheduleRunEntry{
		{
			ID:         "current-run",
			ScheduleID: scheduleID,
			SessionID:  "current-schedule-chat",
			Status:     "running",
			StartedAt:  time.Now().UTC(),
		},
		{
			ID:         "previous-run",
			ScheduleID: scheduleID,
			SessionID:  "previous-schedule-chat",
			Status:     "success",
			StartedAt:  time.Now().Add(-time.Hour).UTC(),
		},
	})

	reqMap := map[string]interface{}{}
	resumed := (&SchedulerService{}).maybeResumeLatestWorkflowThread(context.Background(), resumeTestScheduleContext(workspacePath, scheduleID), reqMap, "current-schedule-chat")
	if resumed != "previous-schedule-chat" {
		t.Fatalf("resumed session = %q, want previous scheduled session", resumed)
	}
	if got := reqMap["restored_conversation_session_id"]; got != "previous-schedule-chat" {
		t.Fatalf("restored_conversation_session_id = %#v, want previous scheduled session", got)
	}
}

func TestMaybeResumeLatestWorkflowThreadIgnoresNormalUserChat(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)

	workspacePath := "Workflow/rtslatency"
	scheduleID := "schedule-1"
	writeWorkflowChatRuntime(t, root, workspacePath, "normal-user-chat", "claude-code", true)
	writeScheduleRunsForTest(t, root, workspacePath, []ScheduleRunEntry{
		{
			ID:         "current-run",
			ScheduleID: scheduleID,
			SessionID:  "current-schedule-chat",
			Status:     "running",
			StartedAt:  time.Now().UTC(),
		},
	})

	reqMap := map[string]interface{}{}
	resumed := (&SchedulerService{}).maybeResumeLatestWorkflowThread(context.Background(), resumeTestScheduleContext(workspacePath, scheduleID), reqMap, "current-schedule-chat")
	if resumed != "" {
		t.Fatalf("resumed session = %q, want empty because normal user chats are not schedule runs", resumed)
	}
	if _, ok := reqMap["restored_conversation_session_id"]; ok {
		t.Fatalf("restored_conversation_session_id was set for a normal user chat: %#v", reqMap)
	}
}

func TestMaybeResumeLatestMultiAgentThreadUsesPreviousScheduledSessionOnly(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)

	userID := "default"
	scheduleID := "schedule-1"
	writeUserChatRuntime(t, root, userID, "normal-user-chat", "claude-code", true)
	writeUserChatRuntime(t, root, userID, "previous-schedule-chat", "claude-code", true)
	writeMultiAgentScheduleRunsForTest(t, root, userID, []ScheduleRunEntry{
		{
			ID:         "current-run",
			ScheduleID: scheduleID,
			SessionID:  "current-schedule-chat",
			Status:     "running",
			StartedAt:  time.Now().UTC(),
		},
		{
			ID:         "previous-run",
			ScheduleID: scheduleID,
			SessionID:  "previous-schedule-chat",
			Status:     "success",
			StartedAt:  time.Now().Add(-time.Hour).UTC(),
		},
	})

	reqMap := map[string]interface{}{}
	resumed := (&SchedulerService{}).maybeResumeLatestMultiAgentThread(context.Background(), resumeTestMultiAgentScheduleContext(userID, scheduleID), reqMap, "current-schedule-chat")
	if resumed != "previous-schedule-chat" {
		t.Fatalf("resumed session = %q, want previous scheduled session", resumed)
	}
	if got := reqMap["restored_conversation_session_id"]; got != "previous-schedule-chat" {
		t.Fatalf("restored_conversation_session_id = %#v, want previous scheduled session", got)
	}
}

func TestMaybeResumeLatestMultiAgentThreadIgnoresNormalUserChat(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)

	userID := "default"
	scheduleID := "schedule-1"
	writeUserChatRuntime(t, root, userID, "normal-user-chat", "claude-code", true)
	writeMultiAgentScheduleRunsForTest(t, root, userID, []ScheduleRunEntry{
		{
			ID:         "current-run",
			ScheduleID: scheduleID,
			SessionID:  "current-schedule-chat",
			Status:     "running",
			StartedAt:  time.Now().UTC(),
		},
	})

	reqMap := map[string]interface{}{}
	resumed := (&SchedulerService{}).maybeResumeLatestMultiAgentThread(context.Background(), resumeTestMultiAgentScheduleContext(userID, scheduleID), reqMap, "current-schedule-chat")
	if resumed != "" {
		t.Fatalf("resumed session = %q, want empty because normal user chats are not schedule runs", resumed)
	}
	if _, ok := reqMap["restored_conversation_session_id"]; ok {
		t.Fatalf("restored_conversation_session_id was set for a normal user chat: %#v", reqMap)
	}
}

func resumeTestScheduleContext(workspacePath, scheduleID string) *ScheduleContext {
	resumePrevious := true
	builder := &workflowtypes.AgentLLMConfig{Provider: "claude-code", ModelID: "claude-opus-4-6"}
	return &ScheduleContext{
		WorkspacePath: workspacePath,
		UserID:        "default",
		Schedule: WorkflowSchedule{
			ID:             scheduleID,
			ResumePrevious: &resumePrevious,
		},
		Capabilities: WorkflowCapabilities{
			LLMConfig: &workflowtypes.PresetLLMConfig{
				SchemaVersion:  workflowtypes.LLMConfigSchemaVersion,
				Mode:           workflowtypes.LLMConfigModeExplicit,
				BuilderLLM:     builder,
				MaintenanceLLM: builder,
				PulseLLM:       builder,
				TieredConfig:   &workflowtypes.TieredLLMConfig{Tier1: builder, Tier2: builder, Tier3: builder},
			},
		},
	}
}

func resumeTestMultiAgentScheduleContext(userID, scheduleID string) *ScheduleContext {
	resumePrevious := true
	builder := &workflowtypes.AgentLLMConfig{Provider: "claude-code", ModelID: "claude-opus-4-6"}
	return &ScheduleContext{
		UserID:     userID,
		SourceType: "multi-agent",
		Schedule: WorkflowSchedule{
			ID:             scheduleID,
			ResumePrevious: &resumePrevious,
		},
		Capabilities: WorkflowCapabilities{
			LLMConfig: &workflowtypes.PresetLLMConfig{
				SchemaVersion:  workflowtypes.LLMConfigSchemaVersion,
				Mode:           workflowtypes.LLMConfigModeExplicit,
				BuilderLLM:     builder,
				MaintenanceLLM: builder,
				PulseLLM:       builder,
				TieredConfig:   &workflowtypes.TieredLLMConfig{Tier1: builder, Tier2: builder, Tier3: builder},
			},
		},
	}
}

func writeScheduleRunsForTest(t *testing.T, root, workspacePath string, runs []ScheduleRunEntry) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(workspacePath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(runs, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schedule-runs.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeMultiAgentScheduleRunsForTest(t *testing.T, root, userID string, runs []ScheduleRunEntry) {
	t.Helper()
	dir := filepath.Join(root, "_users", userID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(runs, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "multiagent-schedule-runs.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeWorkflowChatRuntime(t *testing.T, root, workspacePath, sessionID, provider string, resumeSupported bool) {
	t.Helper()
	convDir := filepath.Join(root, filepath.FromSlash(workspacePath), "builder", "conversation", "2026-05-20")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(map[string]interface{}{
		"session_id":    sessionID,
		"agent_mode":    "workflow_phase",
		"workshop_mode": "workshop",
		"runtime": map[string]interface{}{
			"kind":                 "coding_agent",
			"provider":             provider,
			"model_id":             "claude-opus-4-6",
			"external_session_id":  "external-" + sessionID,
			"resume_supported":     resumeSupported,
			"resume_flag":          "--resume",
			"workspace_path":       workspacePath,
			"workshop_mode":        "workshop",
			"agent_session_handle": map[string]interface{}{},
		},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(convDir, "session-"+sessionID+"-conversation.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeUserChatRuntime(t *testing.T, root, userID, sessionID, provider string, resumeSupported bool) {
	t.Helper()
	convDir := filepath.Join(root, "_users", userID, "chat_history", "2026-05-20")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(map[string]interface{}{
		"session_id": sessionID,
		"agent_mode": "simple",
		"runtime": map[string]interface{}{
			"kind":                 "coding_agent",
			"provider":             provider,
			"model_id":             "claude-opus-4-6",
			"external_session_id":  "external-" + sessionID,
			"resume_supported":     resumeSupported,
			"resume_flag":          "--resume",
			"agent_session_handle": map[string]interface{}{},
		},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(convDir, "session-"+sessionID+"-conversation.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestReconcileWorkshopRunOutcomeDetectsNewFailedRun reproduces BUG-20260729-10
// (social-media, 2026-07-29): the scheduler recorded "success" for a scheduled
// run whose triggered workflow run fully failed at its first posting step,
// because the orchestrating workshop chat session itself completed its turns
// without an infrastructure-level error. reconcileWorkshopRunOutcome closes
// that gap by checking the real run_metadata.json of any NEW run folder.
func TestReconcileWorkshopRunOutcomeDetectsNewFailedRun(t *testing.T) {
	before := map[string]bool{"iteration-231": true}
	after := []RunFolderInfo{
		{Name: "iteration-231", Metadata: &RunMetadata{Status: "completed"}},
		{Name: "iteration-232", Metadata: &RunMetadata{Status: "failed"}},
	}
	failedFolder, found := reconcileWorkshopRunOutcome(before, after)
	if !found {
		t.Fatal("expected the new failed run to be found")
	}
	if failedFolder != "iteration-232" {
		t.Fatalf("failedFolder = %q, want iteration-232", failedFolder)
	}
}

// TestReconcileWorkshopRunOutcomeIgnoresPreexistingFailure proves an older
// run's failure — already present before this invocation started — must never
// flip THIS invocation's own result. Only a run created during this call
// counts.
func TestReconcileWorkshopRunOutcomeIgnoresPreexistingFailure(t *testing.T) {
	before := map[string]bool{"iteration-231": true}
	after := []RunFolderInfo{
		{Name: "iteration-231", Metadata: &RunMetadata{Status: "failed"}},
	}
	if _, found := reconcileWorkshopRunOutcome(before, after); found {
		t.Fatal("a pre-existing run's failure must not be attributed to this invocation")
	}
}

// TestReconcileWorkshopRunOutcomeMisattributesWhenBaselineIsLost pins the hazard
// that makes the caller's guard necessary, rather than asserting a behavior we
// want. The function decides "new" purely by absence from the before-set, so an
// EMPTY before-set means every folder looks new and any old failure is reported
// as this invocation's.
//
// That is correct for a primitive that is handed a real baseline, and it is why
// the baseline must never be a guess. loadRunFoldersInternal used to return an
// empty slice with a nil error whenever the workspace listing failed, so the
// scheduler could not tell "no run folders" from "could not look" and passed
// exactly this empty set. On 2026-08-10 a hetznerssh production security audit
// completed cleanly in iteration-0 and was recorded as an error, blamed on
// iteration-25, which had failed the previous day — moments after workspace
// folder calls were returning errors following a restart.
//
// The fix is at the call site: propagate the listing error and skip
// reconciliation when either snapshot is unavailable. If this test ever starts
// failing because the function itself learned to ignore an empty baseline, that
// is fine — delete it and keep the caller guard.
func TestReconcileWorkshopRunOutcomeMisattributesWhenBaselineIsLost(t *testing.T) {
	lostBaseline := map[string]bool{} // what a failed listing produced
	after := []RunFolderInfo{
		{Name: "iteration-0", Metadata: &RunMetadata{Status: "completed"}},
		{Name: "iteration-25", Metadata: &RunMetadata{Status: "failed"}}, // yesterday's
	}
	failedFolder, found := reconcileWorkshopRunOutcome(lostBaseline, after)
	if !found || failedFolder != "iteration-25" {
		t.Fatalf("expected the documented misattribution (iteration-25), got found=%v folder=%q; "+
			"if the primitive now tolerates an empty baseline this test is obsolete", found, failedFolder)
	}
}

// TestReconcileWorkshopRunOutcomeIgnoresAmbiguousStates proves that anything
// other than an explicit "failed" — no metadata, "running", "completed" — is
// left alone rather than treated as a failure. A transient listing hiccup or
// an in-flight run must fail open toward "cannot verify", never toward a
// false failure that would mislabel a genuinely successful run.
func TestReconcileWorkshopRunOutcomeIgnoresAmbiguousStates(t *testing.T) {
	before := map[string]bool{}
	after := []RunFolderInfo{
		{Name: "iteration-1", Metadata: nil},
		{Name: "iteration-2", Metadata: &RunMetadata{Status: "running"}},
		{Name: "iteration-3", Metadata: &RunMetadata{Status: "completed"}},
	}
	if _, found := reconcileWorkshopRunOutcome(before, after); found {
		t.Fatal("no folder here is explicitly \"failed\"; none should be flagged")
	}
}

// TestReconcileWorkshopRunOutcomeHandlesGroupNestedFolders proves a group-run
// folder name like "iteration-232/default" is diffed correctly, matching the
// naming convention extractIterationFoldersFromTypedChildren produces for
// group-nested runs.
func TestReconcileWorkshopRunOutcomeHandlesGroupNestedFolders(t *testing.T) {
	before := map[string]bool{"iteration-232/production": true}
	after := []RunFolderInfo{
		{Name: "iteration-232/production", Metadata: &RunMetadata{Status: "failed"}},
		{Name: "iteration-232/staging", Metadata: &RunMetadata{Status: "failed"}},
	}
	failedFolder, found := reconcileWorkshopRunOutcome(before, after)
	if !found {
		t.Fatal("expected the new group folder's failure to be found")
	}
	if failedFolder != "iteration-232/staging" {
		t.Fatalf("failedFolder = %q, want iteration-232/staging (not the pre-existing production group)", failedFolder)
	}
}

func TestRunFolderNameSetSkipsEmptyNames(t *testing.T) {
	set := runFolderNameSet([]RunFolderInfo{
		{Name: "iteration-1"},
		{Name: ""},
		{Name: "iteration-2"},
	})
	if len(set) != 2 || !set["iteration-1"] || !set["iteration-2"] {
		t.Fatalf("unexpected set: %#v", set)
	}
}

func TestPostRunMonitorModuleStepsReserveHTMLForDashboard(t *testing.T) {
	steps := postRunMonitorSteps()
	checked := 0
	for _, step := range steps {
		if step.label != "review-fix" {
			continue
		}
		checked++
		for _, want := range []string{"Do not render the dashboard", "durable Gate worklist"} {
			if !strings.Contains(step.query, want) {
				t.Fatalf("module step %q missing single-renderer guard %q:\n%s", step.label, want, step.query)
			}
		}
		if strings.Contains(step.query, `read_skill(skills=[{"name":"builder-reference","path":"references/review-improve-log.md"}])`) {
			t.Fatalf("module step %q still loads the presentation contract:\n%s", step.label, step.query)
		}
	}
	if checked != 1 {
		t.Fatalf("checked %d Review+Fix steps, want 1", checked)
	}
}

// pulseStepQueryByLabel selects a stage by label so these assertions survive
// new stages being added to the final sequence.
func pulseStepQueryByLabel(t *testing.T, steps []postRunMonitorStep, label string) string {
	t.Helper()
	for _, st := range steps {
		if st.label == label {
			return st.query
		}
	}
	t.Fatalf("no %q stage in final steps", label)
	return ""
}

// The dashboard stage used to say "mark command=dashboard done" without naming
// a tool, unlike the finalize stage right below it, which spells out
// record_pulse_result(command=...) explicitly. On 2026-08-04 rtslatency's
// dashboard stage rendered builder/improve.html correctly, then reached for
// mutate_workflow_db to write pulse_final_command_state directly — a
// reasonable guess for "mark this row," and the wrong one: that table is
// framework-owned bookkeeping, mutate_workflow_db correctly denied a session
// without db_access=read-write, and the stage ended without ever calling the
// sanctioned command API. reconcilePulseDashboardCommand then marked the
// whole stage failed even though the render was correct.
//
// This test is deliberately independent of
// TestPostRunMonitorUsesDynamicModulesAndSingleFinalizer, whose detailed
// per-stage assertions are currently unreachable dead code after a premature
// return partway through that function (a concurrent, unrelated in-progress
// edit — see the go vet "unreachable code" flag at this file's line 787).
func TestPulseHasNoDashboardStage(t *testing.T) {
	for _, step := range postRunMonitorSteps() {
		if step.label == "dashboard" || strings.Contains(step.query, "PULSE DASHBOARD") {
			t.Fatalf("retired HTML dashboard leaked into Pulse stage: %+v", step)
		}
	}
}

func TestWorkshopRunProducedEvidence(t *testing.T) {
	start := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	before := map[string]bool{"iteration-0": true}

	tests := []struct {
		name  string
		after []RunFolderInfo
		want  bool
	}{
		{
			name: "pre-existing folder untouched",
			after: []RunFolderInfo{{Name: "iteration-0", Metadata: &RunMetadata{
				StartedAt: start.Add(-2 * time.Hour), CreatedAt: start.Add(-3 * time.Hour), Status: "completed",
			}}},
			want: false,
		},
		{
			name: "new folder created",
			after: []RunFolderInfo{
				{Name: "iteration-0", Metadata: &RunMetadata{StartedAt: start.Add(-2 * time.Hour)}},
				{Name: "iteration-1", Metadata: &RunMetadata{StartedAt: start.Add(time.Minute)}},
			},
			want: true,
		},
		{
			name: "existing folder restarted",
			after: []RunFolderInfo{{Name: "iteration-0", Metadata: &RunMetadata{
				StartedAt: start.Add(30 * time.Second), CreatedAt: start.Add(-3 * time.Hour),
			}}},
			want: true,
		},
		{
			name:  "failed new run still counts",
			after: []RunFolderInfo{{Name: "iteration-2", Metadata: &RunMetadata{StartedAt: start.Add(time.Minute), Status: "failed"}}},
			want:  true,
		},
		{
			name:  "known folder without metadata proves nothing",
			after: []RunFolderInfo{{Name: "iteration-0"}},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workshopRunProducedEvidence(before, tt.after, start); got != tt.want {
				t.Fatalf("workshopRunProducedEvidence() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestScheduledWorkflowStepProducedEvidenceUsesLinkedStepExecutions(t *testing.T) {
	since := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	registry := NewBackgroundAgentRegistry()
	registry.Register("schedule-session", &BackgroundAgent{
		ID:        "exec-daily-report-1",
		SessionID: "schedule-session",
		Kind:      "workflow_step",
		Status:    BGAgentCompleted,
		CreatedAt: since.Add(time.Minute),
		Metadata:  map[string]string{"execution_type": "workflow-step"},
	})
	service := &SchedulerService{api: &StreamingAPI{bgAgentRegistry: registry}}
	if !service.scheduledWorkflowStepProducedEvidence("schedule-session", since) {
		t.Fatal("a workflow step launched by this schedule must be Pulse evidence")
	}

	registry.Register("schedule-session", &BackgroundAgent{
		ID:        "bg-unrelated-1",
		SessionID: "schedule-session",
		Kind:      "workshop_background",
		Status:    BGAgentCompleted,
		CreatedAt: since.Add(2 * time.Minute),
	})
	if !service.scheduledWorkflowStepProducedEvidence("schedule-session", since) {
		t.Fatal("unrelated background work must not erase linked workflow-step evidence")
	}

	otherRegistry := NewBackgroundAgentRegistry()
	otherRegistry.Register("schedule-session", &BackgroundAgent{
		ID:        "bg-only-1",
		SessionID: "schedule-session",
		Kind:      "workshop_background",
		Status:    BGAgentCompleted,
		CreatedAt: since.Add(time.Minute),
	})
	otherService := &SchedulerService{api: &StreamingAPI{bgAgentRegistry: otherRegistry}}
	if otherService.scheduledWorkflowStepProducedEvidence("schedule-session", since) {
		t.Fatal("generic background work must not manufacture workflow evidence")
	}
}

func TestNoRunFinalizerSkipsEvidenceStagesAndReportsReason(t *testing.T) {
	reason := `workflow upgrade preflight upgrade-1.0.18 did not stamp required version "1.0.18"; normal schedule message was not started`
	steps := postRunMonitorNoRunSteps("pulse-run-1", reason, workflowNotificationContentInstructions{
		runSummaryChannels:   []string{"electron", "slack"},
		runSummaryRecipients: []string{"owner@example.com"},
	})
	if len(steps) != 1 || steps[0].label != "finalize" {
		t.Fatalf("no-run finalizer = %#v, want one finalize step", steps)
	}
	for _, want := range []string{
		"WORKFLOW DID NOT RUN",
		"Gate, reviewers, Fixer, dashboard, and publish were intentionally skipped",
		`notification_kind="run_summary"`,
		"dashboard has no record_pulse_result command and needs no receipt",
		"mark publish skipped",
		"source-hash-gated backup",
		"electron, slack",
		"owner@example.com",
		// The label survives so the operator's run summary can name which
		// migration stalled.
		"workflow upgrade preflight upgrade-1.0.18 did not complete",
	} {
		if !strings.Contains(steps[0].query, want) {
			t.Fatalf("no-run finalizer missing %q:\n%s", want, steps[0].query)
		}
	}
	// The finalizer shares the scheduler's session. Restating the stamp
	// instruction here is what invited the confida-login 2026-08-12 out-of-turn
	// stamp, so the target version and the verb must not reach this turn.
	for _, forbidden := range []string{"did not stamp", `"1.0.18"`} {
		if strings.Contains(steps[0].query, forbidden) {
			t.Fatalf("no-run finalizer restates the upgrade instruction (%q):\n%s", forbidden, steps[0].query)
		}
	}
	if strings.Contains(steps[0].query, "write builder/improve.html once") {
		t.Fatal("no-run finalizer must not contain the normal dashboard mutation contract")
	}
	// PLAT-083 regression: the numbered record_pulse_result instructions must
	// never ask the agent to record "dashboard" — it was never a valid final
	// command (only backup/publish/notify are), so the tool rejected every
	// attempt and the agent had to recover from an error the prompt itself
	// caused.
	if strings.Contains(steps[0].query, "mark dashboard skipped") {
		t.Fatal("no-run finalizer must not instruct the agent to record_pulse_result an invalid \"dashboard\" command")
	}

	fallback := postRunMonitorNoRunSteps("pulse-run-2", "   ")[0].query
	if !strings.Contains(fallback, "no workflow run was started or resumed") {
		t.Fatal("empty scheduler error must still produce a truthful explanation")
	}
}

func TestDueCronOccurrencesRetainsEveryRecentOccurrenceAndLatestCatchup(t *testing.T) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse("*/5 * * * *")
	if err != nil {
		t.Fatal(err)
	}
	after := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 7, 10, 17, 0, 0, time.UTC)
	got := dueCronOccurrences(schedule, after, now)
	want := []time.Time{
		time.Date(2026, 8, 7, 10, 5, 0, 0, time.UTC),
		time.Date(2026, 8, 7, 10, 10, 0, 0, time.UTC),
		time.Date(2026, 8, 7, 10, 15, 0, 0, time.UTC),
	}
	if len(got) != len(want) {
		t.Fatalf("occurrences=%v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("occurrence[%d]=%s, want %s", i, got[i], want[i])
		}
	}
}

func TestLatestCronOccurrenceReconcilesInterruptedAttempt(t *testing.T) {
	store, err := schedulerstate.Open(filepath.Join(t.TempDir(), "schedule-state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	scheduledFor := time.Date(2026, 8, 7, 10, 30, 0, 0, time.UTC)
	if err := store.RecordFireDecision(context.Background(), schedulerstate.FireDecision{
		DecisionID: "attempt", ScopeType: "workflow", ScopeID: "Workflow/demo", ScheduleID: "daily",
		TriggerSource: "cron", Decision: "attempted", ScheduledFor: scheduledFor, FiredAt: scheduledFor,
	}); err != nil {
		t.Fatal(err)
	}
	service := NewSchedulerService(nil)
	service.stateStore = store
	sctx := &ScheduleContext{WorkspacePath: "Workflow/demo", SourceType: "workflow", Schedule: WorkflowSchedule{ID: "daily"}}
	got, ok := service.latestCronOccurrence(sctx)
	if !ok || !got.Equal(scheduledFor) {
		t.Fatalf("latest occurrence=(%s,%t), want (%s,true)", got, ok, scheduledFor)
	}
	latest, err := store.LatestFireDecision(context.Background(), "workflow", "Workflow/demo", "daily", "cron")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Decision != "launch_outcome_unknown" || !strings.Contains(latest.Reason, "may or may not have started") {
		t.Fatalf("interrupted attempt was not reconciled: %+v", latest)
	}
}

func TestDueCronOccurrencesDoesNotDropMinuteOccurrencesWithinRecoveryWindow(t *testing.T) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse("* * * * *")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	after := now.Add(-12 * time.Hour)
	got := dueCronOccurrences(schedule, after, now)
	if len(got) != 12*60 {
		t.Fatalf("retained occurrences=%d, want %d", len(got), 12*60)
	}
	if !got[0].Equal(after.Add(time.Minute)) || !got[len(got)-1].Equal(now) {
		t.Fatalf("unexpected occurrence bounds: first=%s last=%s", got[0], got[len(got)-1])
	}
}

// TestWorkshopRunStartedDuringInvocationIgnoresBaseline pins the reason this
// helper exists separately from workshopRunProducedEvidence: it must answer
// correctly with no before-set at all.
//
// social-media 2026-08-10: the run completed at 12:15:18Z, the workshop idle
// wait expired 34 minutes later, and because that path returns before the
// evidence check, ProducedRunEvidence kept its initialized false. Pulse was told
// "the workflow did not run" and skipped publish — against a run that had landed
// 19 actions, 18 of them independently verified. The durable record said
// "completed" the whole time; nothing consulted it.
func TestWorkshopRunStartedDuringInvocationIgnoresBaseline(t *testing.T) {
	since := time.Date(2026, 8, 10, 9, 30, 53, 0, time.UTC)
	folders := []RunFolderInfo{
		// Pre-existing folder reused by this invocation — the shape that made the
		// baseline-dependent helper unreliable here. iteration-0 always exists.
		{Name: "iteration-0", Metadata: &RunMetadata{
			Status:    "completed",
			StartedAt: since.Add(4 * time.Minute),
		}},
	}
	if !workshopRunStartedDuringInvocation(folders, since) {
		t.Fatal("a run whose own metadata says it started during this invocation is evidence, baseline or not")
	}
}

// TestWorkshopRunStartedDuringInvocationRejectsOlderAndUnstamped proves the
// helper cannot manufacture evidence: a run from a previous invocation, and a
// record with no usable timestamp, both count as nothing. The caller treats
// false as "no evidence recorded", which is the pre-existing behavior.
func TestWorkshopRunStartedDuringInvocationRejectsOlderAndUnstamped(t *testing.T) {
	since := time.Date(2026, 8, 10, 9, 30, 53, 0, time.UTC)
	folders := []RunFolderInfo{
		{Name: "iteration-254", Metadata: &RunMetadata{Status: "completed", StartedAt: since.Add(-2 * time.Hour)}},
		{Name: "iteration-253", Metadata: &RunMetadata{Status: "failed"}}, // no timestamps
		{Name: "iteration-252"}, // no metadata
	}
	if workshopRunStartedDuringInvocation(folders, since) {
		t.Fatal("older or unstamped runs must not be reported as evidence for this invocation")
	}
}

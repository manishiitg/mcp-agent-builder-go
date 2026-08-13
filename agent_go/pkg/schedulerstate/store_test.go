package schedulerstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "schedule-state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestRunLifecycleAndEvents(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	startedAt := time.Now().UTC()
	run := Run{
		RunID:         "run-1",
		ScopeType:     "workflow",
		ScopeID:       "Workflow/demo",
		LockKey:       "workflow:Workflow/demo",
		ScheduleID:    "schedule-1",
		TriggerSource: "cron",
		StartedAt:     startedAt,
	}
	if err := store.BeginRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	transitions := []Transition{
		{RunID: run.RunID, To: StateWorkflowRunning, SessionID: "session-1", SessionKind: "workflow", Reason: "session started"},
		{RunID: run.RunID, To: StateWorkflowFinished, SessionID: "session-1", RunFolder: "iteration-0", Reason: "workflow finished"},
		{RunID: run.RunID, To: StatePulseGate, SessionID: "session-1", SessionKind: "pulse", Reason: "Pulse enabled"},
		{RunID: run.RunID, To: StatePulseModules, SessionID: "session-1", SessionKind: "pulse", Reason: "Gate recorded worklist"},
		{RunID: run.RunID, To: StatePulseFinalizing, SessionID: "recovery-session", SessionKind: "pulse_recovery", Reason: "finalizing"},
		{RunID: run.RunID, To: StateCompleted, SessionID: "recovery-session", SessionKind: "pulse_recovery", Reason: "done"},
	}
	for _, transition := range transitions {
		if err := store.Transition(ctx, transition); err != nil {
			t.Fatalf("Transition(%s): %v", transition.To, err)
		}
	}

	got, err := store.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateCompleted || got.CompletedAt == nil || got.ActiveSessionID != "recovery-session" {
		t.Fatalf("run = %+v", got)
	}
	events, err := store.ListEvents(ctx, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != len(transitions)+1 {
		t.Fatalf("len(events) = %d, want %d", len(events), len(transitions)+1)
	}
}

func TestActiveLeaseRejectsOverlappingWorkflowSchedules(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	first := Run{RunID: "run-1", ScopeType: "workflow", ScopeID: "Workflow/demo", LockKey: "workflow:Workflow/demo", ScheduleID: "schedule-1", TriggerSource: "cron"}
	second := Run{RunID: "run-2", ScopeType: "workflow", ScopeID: "Workflow/demo", LockKey: "workflow:Workflow/demo", ScheduleID: "schedule-2", TriggerSource: "manual"}
	if err := store.BeginRun(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginRun(ctx, second); !errors.Is(err, ErrRunAlreadyActive) {
		t.Fatalf("BeginRun(overlap) error = %v, want ErrRunAlreadyActive", err)
	}
	if err := store.Transition(ctx, Transition{RunID: first.RunID, To: StateStopped, Reason: "user stopped"}); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginRun(ctx, second); err != nil {
		t.Fatalf("BeginRun(after terminal): %v", err)
	}
}

func TestRejectsInvalidAndTerminalRegression(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	run := Run{RunID: "run-1", ScopeType: "workflow", ScopeID: "Workflow/demo", LockKey: "workflow:Workflow/demo", ScheduleID: "schedule-1", TriggerSource: "cron"}
	if err := store.BeginRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(ctx, Transition{RunID: run.RunID, To: StatePulseModules}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("invalid transition error = %v", err)
	}
	if err := store.Transition(ctx, Transition{RunID: run.RunID, To: StateFailed, Reason: "failed to start"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(ctx, Transition{RunID: run.RunID, To: StateWorkflowRunning}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("terminal regression error = %v", err)
	}
}

func TestPulseGateCanCompleteWithoutSelectedModules(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	run := Run{RunID: "run-no-modules", ScopeType: "workflow", ScopeID: "Workflow/demo", LockKey: "workflow:Workflow/demo", ScheduleID: "schedule-1"}
	if err := store.BeginRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	for _, transition := range []Transition{
		{RunID: run.RunID, To: StateWorkflowRunning},
		{RunID: run.RunID, To: StateWorkflowFinished},
		{RunID: run.RunID, To: StatePulseGate},
		{RunID: run.RunID, To: StateCompleted},
	} {
		if err := store.Transition(ctx, transition); err != nil {
			t.Fatalf("transition to %s: %v", transition.To, err)
		}
	}
}

func TestForceTerminalReleasesStuckLease(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	first := Run{RunID: "run-stuck", ScopeType: "workflow", ScopeID: "Workflow/demo", LockKey: "workflow:Workflow/demo", ScheduleID: "schedule-1"}
	if err := store.BeginRun(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := store.ForceTerminal(ctx, Transition{RunID: first.RunID, To: StateCompleted, Reason: "recover lease"}); err != nil {
		t.Fatalf("force terminal: %v", err)
	}
	second := Run{RunID: "run-next", ScopeType: "workflow", ScopeID: "Workflow/demo", LockKey: first.LockKey, ScheduleID: "schedule-2"}
	if err := store.BeginRun(ctx, second); err != nil {
		t.Fatalf("lease remained stuck: %v", err)
	}
}

func TestInterruptActiveRunsReleasesLeases(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	run := Run{RunID: "run-1", ScopeType: "workflow", ScopeID: "Workflow/demo", LockKey: "workflow:Workflow/demo", ScheduleID: "schedule-1", TriggerSource: "cron"}
	if err := store.BeginRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	count, err := store.InterruptActiveRuns(ctx, "server restarted", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	got, err := store.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateInterrupted {
		t.Fatalf("state = %s, want interrupted", got.State)
	}
}

func TestActiveRunByLockKey(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	run := Run{RunID: "run-active", ScopeType: "workflow", ScopeID: "demo", LockKey: "workflow/demo", ScheduleID: "daily", StartedAt: now}
	if err := store.BeginRun(ctx, run); err != nil {
		t.Fatalf("begin run: %v", err)
	}

	active, err := store.ActiveRunByLockKey(ctx, run.LockKey)
	if err != nil {
		t.Fatalf("active run: %v", err)
	}
	if active.RunID != run.RunID || active.State != StateStarting {
		t.Fatalf("unexpected active run: %+v", active)
	}

	if err := store.Transition(ctx, Transition{RunID: run.RunID, To: StateStopped, Reason: "test stop", At: now.Add(time.Second)}); err != nil {
		t.Fatalf("stop run: %v", err)
	}
	if _, err := store.ActiveRunByLockKey(ctx, run.LockKey); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected no active run after stop, got %v", err)
	}
}

func TestFireDecisionsAreDurableAndScoped(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for i, decision := range []string{"skipped_busy", "started"} {
		if err := store.RecordFireDecision(ctx, FireDecision{
			DecisionID: fmt.Sprintf("decision-%d", i), ScopeType: "workflow", ScopeID: "Workflow/demo",
			ScheduleID: "daily", TriggerSource: "cron", Decision: decision, Reason: "test", FiredAt: now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("record decision: %v", err)
		}
	}
	if err := store.RecordFireDecision(ctx, FireDecision{
		DecisionID: "other", ScopeType: "workflow", ScopeID: "Workflow/other", ScheduleID: "daily",
		TriggerSource: "cron", Decision: "started", Reason: "other", FiredAt: now,
	}); err != nil {
		t.Fatalf("record other decision: %v", err)
	}

	decisions, err := store.ListFireDecisions(ctx, "workflow", "Workflow/demo", "daily", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 2 || decisions[0].Decision != "started" || decisions[1].Decision != "skipped_busy" {
		t.Fatalf("unexpected decisions: %+v", decisions)
	}
}

func TestOpenMigratesLegacyFireDecisionsToOccurrenceIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedule-state.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schedule_fire_decisions (
		decision_id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL,
		schedule_id TEXT NOT NULL, trigger_source TEXT NOT NULL, decision TEXT NOT NULL,
		reason TEXT NOT NULL DEFAULT '', run_id TEXT NOT NULL DEFAULT '', fired_at TEXT NOT NULL
	);
	INSERT INTO schedule_fire_decisions VALUES
		('legacy','workflow','Workflow/demo','daily','cron','started','','run-1','2026-08-07T10:30:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	latest, err := store.LatestFireDecision(context.Background(), "workflow", "Workflow/demo", "daily", "cron")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ScheduledFor.IsZero() || !latest.ScheduledFor.Equal(latest.FiredAt) {
		t.Fatalf("legacy occurrence was not backfilled: %+v", latest)
	}
}

func TestFireDecisionUpdatesOneDurableOccurrence(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	scheduledFor := time.Date(2026, 8, 7, 10, 30, 0, 0, time.UTC)
	for i, decision := range []string{"attempted", "started"} {
		if err := store.RecordFireDecision(ctx, FireDecision{
			DecisionID: fmt.Sprintf("decision-%d", i), ScopeType: "workflow", ScopeID: "Workflow/demo",
			ScheduleID: "daily", TriggerSource: "cron", Decision: decision,
			Reason: "test", RunID: fmt.Sprintf("run-%d", i), ScheduledFor: scheduledFor,
			FiredAt: scheduledFor.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("record %s: %v", decision, err)
		}
	}
	decisions, err := store.ListFireDecisions(ctx, "workflow", "Workflow/demo", "daily", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "started" || decisions[0].RunID != "run-1" {
		t.Fatalf("occurrence was not updated in place: %+v", decisions)
	}
	latest, err := store.LatestFireDecision(ctx, "workflow", "Workflow/demo", "daily", "cron")
	if err != nil {
		t.Fatal(err)
	}
	if !latest.ScheduledFor.Equal(scheduledFor) {
		t.Fatalf("scheduled_for=%s, want %s", latest.ScheduledFor, scheduledFor)
	}
}

func TestLatestFireDecisionDoesNotMixManualAndCron(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	cronAt := time.Date(2026, 8, 7, 10, 30, 0, 0, time.UTC)
	manualAt := cronAt.Add(time.Hour)
	for _, decision := range []FireDecision{
		{DecisionID: "cron", ScopeType: "workflow", ScopeID: "Workflow/demo", ScheduleID: "daily", TriggerSource: "cron", Decision: "started", ScheduledFor: cronAt, FiredAt: cronAt},
		{DecisionID: "manual", ScopeType: "workflow", ScopeID: "Workflow/demo", ScheduleID: "daily", TriggerSource: "manual", Decision: "started", ScheduledFor: manualAt, FiredAt: manualAt},
	} {
		if err := store.RecordFireDecision(ctx, decision); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := store.LatestFireDecision(ctx, "workflow", "Workflow/demo", "daily", "cron")
	if err != nil {
		t.Fatal(err)
	}
	if !latest.ScheduledFor.Equal(cronAt) {
		t.Fatalf("cron cursor=%s, want %s", latest.ScheduledFor, cronAt)
	}
}

func TestFireDecisionRetentionIsBoundedPerTriggerSource(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < fireDecisionRetentionPerSchedule+4; i++ {
		stamp := now.Add(time.Duration(i) * time.Second)
		if _, err := tx.ExecContext(ctx, `INSERT INTO schedule_fire_decisions
			(decision_id, scope_type, scope_id, schedule_id, trigger_source, decision, scheduled_for, fired_at)
			VALUES (?, 'workflow', 'Workflow/demo', 'frequent', 'cron', 'skipped_paused', ?, ?)`,
			fmt.Sprintf("decision-%06d", i), formatTime(stamp), formatTime(stamp)); err != nil {
			_ = tx.Rollback()
			t.Fatalf("seed decision %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	last := fireDecisionRetentionPerSchedule + 4
	lastStamp := now.Add(time.Duration(last) * time.Second)
	if err := store.RecordFireDecision(ctx, FireDecision{
		DecisionID: fmt.Sprintf("decision-%06d", last), ScopeType: "workflow", ScopeID: "Workflow/demo",
		ScheduleID: "frequent", TriggerSource: "cron", Decision: "skipped_paused",
		ScheduledFor: lastStamp, FiredAt: lastStamp,
	}); err != nil {
		t.Fatalf("record cleanup decision: %v", err)
	}
	// A manual occurrence has its own retention lane and must not be deleted by
	// cron cleanup for the same schedule.
	if err := store.RecordFireDecision(ctx, FireDecision{
		DecisionID: "manual", ScopeType: "workflow", ScopeID: "Workflow/demo",
		ScheduleID: "frequent", TriggerSource: "manual", Decision: "started",
		ScheduledFor: lastStamp.Add(time.Second), FiredAt: lastStamp.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	decisions, err := store.ListFireDecisions(ctx, "workflow", "Workflow/demo", "frequent", fireDecisionRetentionPerSchedule)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != fireDecisionRetentionPerSchedule {
		t.Fatalf("retained decisions = %d, want %d", len(decisions), fireDecisionRetentionPerSchedule)
	}
	var cronCount, manualCount int
	if err := store.db.QueryRowContext(ctx, `SELECT
		SUM(CASE WHEN trigger_source='cron' THEN 1 ELSE 0 END),
		SUM(CASE WHEN trigger_source='manual' THEN 1 ELSE 0 END)
		FROM schedule_fire_decisions WHERE scope_type='workflow' AND scope_id='Workflow/demo' AND schedule_id='frequent'`).Scan(&cronCount, &manualCount); err != nil {
		t.Fatal(err)
	}
	if cronCount != fireDecisionRetentionPerSchedule || manualCount != 1 {
		t.Fatalf("retained cron/manual decisions = %d/%d, want %d/1", cronCount, manualCount, fireDecisionRetentionPerSchedule)
	}
}

package server

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestManualPulseScheduleContract(t *testing.T) {
	if manualWorkflowPulseScheduleID == "" {
		t.Fatal("manual workflow Pulse schedule id must not be empty")
	}

	manifest := &WorkflowManifest{ID: "workflow-id", Label: "Workflow"}
	sched := WorkflowSchedule{
		ID:           manualWorkflowPulseScheduleID,
		Name:         "Run Pulse",
		Mode:         "workshop",
		WorkshopMode: "run",
	}
	sctx := buildScheduleContext("/tmp/workflow", manifest, sched)
	sctx.TriggerSource = "manual"
	sctx.ForcePostRunMonitor = true
	sctx.PulseOnly = true
	sctx.PulseEvidenceRunFolder = "iteration-2/default"
	sctx.PulseEvidenceRunStatus = "error"

	if sctx.Schedule.Mode != "workshop" || sctx.Schedule.WorkshopMode != "run" {
		t.Fatalf("manual Pulse must use the workshop preflight path: %+v", sctx.Schedule)
	}
	if !sctx.ForcePostRunMonitor {
		t.Fatal("manual Pulse must force post-run Pulse without changing workflow config")
	}
	if !sctx.PulseOnly {
		t.Fatal("manual toolbar action must be marked Pulse-only")
	}
	if messages := scheduledWorkshopMessages(sctx); len(messages) != 0 {
		t.Fatalf("Pulse-only action must not enqueue a workflow message: %v", messages)
	}
	turns, err := scheduledWorkshopTurns(manifest, scheduledWorkshopMessages(sctx))
	if err != nil {
		t.Fatalf("build Pulse preflight turns: %v", err)
	}
	if len(turns) == 0 {
		t.Fatal("an unversioned workflow must still receive version preflight turns")
	}
	for _, turn := range turns {
		if turn.upgradeTarget == "" || strings.HasPrefix(turn.label, "schedule-message-") {
			t.Fatalf("Pulse-only action queued a non-upgrade turn: %+v", turn)
		}
	}
	if !shouldRunPostRunMonitor(sctx, manifest) {
		t.Fatal("one-off Pulse must execute even when post_run_monitor is disabled")
	}

	normalCtx := buildScheduleContext("/tmp/workflow", manifest, WorkflowSchedule{Mode: "workshop", WorkshopMode: "run"})
	normalMessages := scheduledWorkshopMessages(normalCtx)
	if len(normalMessages) != 1 || !strings.Contains(normalMessages[0], "run_full_workflow") {
		t.Fatalf("ordinary empty schedule must retain the default workflow message: %v", normalMessages)
	}

	sctx.ForcePostRunMonitor = false
	if shouldRunPostRunMonitor(sctx, manifest) {
		t.Fatal("ordinary run must respect the disabled post_run_monitor setting")
	}
}

// TestPulseReviewOnlyScheduleContractMirrorsManualPulseTrigger pins PLAT-115:
// a persisted, recurring PulseReviewOnly schedule must reach the exact same
// PulseOnly/ForcePostRunMonitor state buildScheduleContext + the manual
// toolbar trigger already produce (TestManualPulseScheduleContract, above) —
// automatically, from the fired schedule's own field, not hand-set by the
// caller. Unlike the manual trigger it must NOT set PulseEvidenceRunFolder:
// that means "review exactly this one folder," but a periodic pass reviews
// whatever backlog Gate itself decides is new (Part D), not a Go-selected
// single folder.
func TestPulseReviewOnlyScheduleContractMirrorsManualPulseTrigger(t *testing.T) {
	manifest := &WorkflowManifest{ID: "workflow-id", Label: "Workflow", PostRunMonitor: boolPtr(false)}
	sched := WorkflowSchedule{
		ID:              "periodic-pulse-review",
		Name:            "Periodic Pulse Review",
		Mode:            "workshop",
		WorkshopMode:    "run",
		CronExpression:  "0 3 * * *",
		PulseReviewOnly: true,
	}
	sctx := buildScheduleContext("/tmp/workflow", manifest, sched)

	if !sctx.PulseOnly {
		t.Fatal("a PulseReviewOnly schedule must set sctx.PulseOnly")
	}
	if !sctx.ForcePostRunMonitor {
		t.Fatal("a PulseReviewOnly schedule must force Pulse to run even with post_run_monitor disabled")
	}
	if sctx.PulseEvidenceRunFolder != "" {
		t.Fatalf("PulseEvidenceRunFolder = %q, want empty — a periodic pass must not be pinned to a single folder", sctx.PulseEvidenceRunFolder)
	}
	if !shouldRunPostRunMonitor(sctx, manifest) {
		t.Fatal("a PulseReviewOnly schedule must execute Pulse even when the workflow's own post_run_monitor is disabled")
	}
	if messages := scheduledWorkshopMessages(sctx); len(messages) != 0 {
		t.Fatalf("PulseReviewOnly must not enqueue a workflow message: %v", messages)
	}

	// A schedule with the flag unset (the overwhelming majority of schedules,
	// including every existing one before this field existed) must be
	// completely unaffected.
	ordinary := buildScheduleContext("/tmp/workflow", manifest, WorkflowSchedule{Mode: "workshop", WorkshopMode: "run"})
	if ordinary.PulseOnly || ordinary.ForcePostRunMonitor {
		t.Fatalf("an ordinary schedule must not be marked Pulse-only: %+v", ordinary)
	}
}

func TestLatestRetainedPulseEvidenceSkipsPriorManualPulseRuns(t *testing.T) {
	now := time.Now().UTC()
	runs := []ScheduleRunEntry{
		{ID: "pulse", ScheduleID: manualWorkflowPulseScheduleID, RunFolder: "iteration-0", Status: "success", StartedAt: now},
		{ID: "workflow", ScheduleID: "daily", RunFolder: "iteration-1/group-a", Status: "error", StartedAt: now.Add(-time.Minute)},
	}
	runFolder, status, ok := latestRetainedPulseEvidenceFromRuns(runs)
	if !ok || runFolder != "iteration-1/group-a" || status != "error" {
		t.Fatalf("latest retained evidence = (%q, %q), want (%q, %q)", runFolder, status, "iteration-1/group-a", "error")
	}
}

func TestLatestRetainedPulseEvidenceFallsBackBeforeFirstRun(t *testing.T) {
	runFolder, status := latestRetainedPulseEvidence(context.Background(), t.TempDir())
	if runFolder != "iteration-0" || status != "unknown" {
		t.Fatalf("fallback evidence = (%q, %q), want (iteration-0, unknown)", runFolder, status)
	}
}

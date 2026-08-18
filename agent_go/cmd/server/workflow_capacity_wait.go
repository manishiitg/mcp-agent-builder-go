package server

import (
	"context"
	"errors"
	"time"

	stepworkflow "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
)

// scheduleRunStatusWaitingForCapacity is a run that stopped without failing.
//
// PLAT-101. It is deliberately its own status rather than a flavour of
// "error". A failed run is finished and its remaining steps are abandoned; a
// waiting run is mid-flight, holds completed steps whose side effects must not
// be replayed, and will continue on its own. Collapsing the two is what made a
// capacity wall look identical to a defect in run history, and what let the
// next cron tick start a duplicate run over the top of it.
const scheduleRunStatusWaitingForCapacity = "waiting_for_capacity"

// readWorkflowCapacityWait loads a run's capacity-wait record, if any.
func readWorkflowCapacityWait(ctx context.Context, workspacePath, runFolder string) (*stepworkflow.WorkflowCapacityWait, bool) {
	if workspacePath == "" || runFolder == "" {
		return nil, false
	}
	content, exists, err := readFileFromWorkspace(ctx, stepworkflow.WorkflowCapacityWaitPath(workspacePath, runFolder))
	if err != nil || !exists {
		return nil, false
	}
	return stepworkflow.ParseWorkflowCapacityWait(content)
}

// outstandingCapacityWait returns the newest run of a workflow that is
// currently suspended on capacity, along with its record.
//
// "Currently" is decided by the run's own status rather than by the presence of
// the file, so a record left behind by a run that later completed, was stopped,
// or failed cannot suppress the schedule.
func (s *SchedulerService) outstandingCapacityWait(ctx context.Context, workspacePath string) (*ScheduleRunEntry, *stepworkflow.WorkflowCapacityWait) {
	if workspacePath == "" {
		return nil, nil
	}
	runs, err := ReadScheduleRuns(ctx, workspacePath)
	if err != nil {
		return nil, nil
	}
	var newest *ScheduleRunEntry
	for i := range runs {
		if runs[i].Status != scheduleRunStatusWaitingForCapacity {
			continue
		}
		if newest == nil || runs[i].StartedAt.After(newest.StartedAt) {
			newest = &runs[i]
		}
	}
	if newest == nil {
		return nil, nil
	}
	wait, ok := readWorkflowCapacityWait(ctx, workspacePath, newest.RunFolder)
	if !ok {
		// The run says it is waiting but the record that says where to resume
		// from is gone. Reporting no wait is the safe reading: it lets the
		// schedule fire again rather than blocking forever on a resume that
		// cannot be armed.
		return nil, nil
	}
	return newest, wait
}

// capacityWaitOutlastsNextRun reports whether the wait extends past when this
// schedule would next have fired.
//
// A pause shorter than the gap to the next run is self-healing and worth no
// interruption. A pause that outlasts it means the schedule is genuinely
// degraded — runs are being skipped — which is the point at which somebody
// should be told rather than left to discover a quiet gap in the history.
func capacityWaitOutlastsNextRun(wait *stepworkflow.WorkflowCapacityWait, nextRun time.Time) bool {
	if wait == nil {
		return false
	}
	if wait.RetryAt.IsZero() {
		// An unknown reset has no end, so it always outlasts the next run.
		return true
	}
	if nextRun.IsZero() {
		return false
	}
	return wait.RetryAt.After(nextRun)
}

// classifyCapacityWait decides whether a finished run stopped on a capacity
// wall rather than failing.
//
// Two independent signals must agree, and both are needed. The typed error says
// the step loop chose to suspend, but it crosses several wrapping layers on the
// way here and any one of them formatting instead of wrapping would lose it.
// The on-disk record is what a resume is actually armed from, so a wall with no
// record is not resumable and must be reported as the failure it effectively
// is.
//
// The record is also bounded by the run's own start time. Run folders are
// reused — iteration-0 is the live slot — so a record left by an earlier run in
// the same folder would otherwise suspend a run that never hit a wall at all.
func (s *SchedulerService) classifyCapacityWait(ctx context.Context, sctx *ScheduleContext, execErr error, runFolder string, runStartedAt time.Time) (*stepworkflow.WorkflowCapacityWait, bool) {
	if execErr == nil || sctx == nil || sctx.SourceType == "multi-agent" {
		return nil, false
	}
	if !errors.Is(execErr, stepworkflow.ErrWorkflowWaitingForCapacity) {
		return nil, false
	}
	wait, ok := readWorkflowCapacityWait(ctx, sctx.WorkspacePath, runFolder)
	if !ok {
		s.logf(sctx, "[SCHEDULER] a step reported a capacity wall but no %s was written for run folder %q; treating it as a failure because there is nothing to resume from",
			stepworkflow.WorkflowCapacityWaitFilename, runFolder)
		return nil, false
	}
	if !capacityWaitBelongsToRun(wait, runStartedAt) {
		s.logf(sctx, "[SCHEDULER] ignoring a capacity-wait record from %s: it predates this run, which started %s",
			wait.RecordedAt.Format(time.RFC3339), runStartedAt.Format(time.RFC3339))
		return nil, false
	}
	return wait, true
}

// resumeDueCapacityWaits wakes suspended runs whose window has reopened.
//
// Runs suspended on capacity do not wake on a cron occurrence — their reset
// instant is set by the provider and almost never lands on the schedule — so
// they are evaluated on every tick instead.
//
// A wait with no stated reset is never woken here. Waking on a guessed instant
// resumes straight back into the same wall and burns the run's remaining steps
// a second time; those runs wait for a person, which is the honest outcome when
// the provider declined to say when capacity returns.
func (s *SchedulerService) resumeDueCapacityWaits(ctx context.Context, now time.Time) {
	if s == nil {
		return
	}
	type candidate struct {
		sctx *ScheduleContext
	}
	seen := make(map[string]bool)
	var candidates []candidate
	s.mu.Lock()
	for _, job := range s.jobs {
		if job == nil || job.sctx == nil || job.sctx.SourceType == "multi-agent" {
			continue
		}
		// One workflow can carry several schedules; a suspended run belongs to
		// the workflow, so evaluating it once per workflow avoids several
		// schedules racing to resume the same run.
		if seen[job.sctx.WorkspacePath] {
			continue
		}
		seen[job.sctx.WorkspacePath] = true
		candidates = append(candidates, candidate{sctx: job.sctx})
	}
	s.mu.Unlock()

	for _, c := range candidates {
		waitingRun, wait := s.outstandingCapacityWait(ctx, c.sctx.WorkspacePath)
		if waitingRun == nil || !wait.ResumeDue(now) {
			continue
		}
		resumeCtx := *c.sctx
		resumeCtx.CapacityResumeRunID = waitingRun.ID
		resumeCtx.CapacityResumeRunFolder = waitingRun.RunFolder
		resumeCtx.CapacityResumeFromStep = wait.StepNumber
		s.logf(&resumeCtx, "[SCHEDULER] ▶️ Provider capacity returned; resuming run %s at step %d (%s)",
			waitingRun.ID, wait.StepNumber, wait.Describe())
		go s.triggerSchedule(&resumeCtx, now)
	}
}

// capacityWaitBelongsToRun reports whether a record was written by the run that
// just finished.
//
// Run folders are reused — iteration-0 is the live slot, rotated to a permanent
// name only on the next run — so a record left behind by an earlier run in the
// same folder is readable by a later one. Without this check that stale record
// would suspend a run that never hit a wall, and suspend it at whatever step
// the earlier run happened to stop on.
func capacityWaitBelongsToRun(wait *stepworkflow.WorkflowCapacityWait, runStartedAt time.Time) bool {
	if wait == nil {
		return false
	}
	if runStartedAt.IsZero() {
		// Nothing to compare against. Trusting the record is the lesser risk:
		// the typed error already established that this run hit a wall.
		return true
	}
	return !wait.RecordedAt.Before(runStartedAt)
}

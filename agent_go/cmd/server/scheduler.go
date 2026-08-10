package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	virtualtools "github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/schedulerstate"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workflowtypes"
	"github.com/robfig/cron/v3"
)

const scheduledBackgroundNoPollingInstruction = "After launching background workflow or step work, do not babysit it with sleep/list_executions/query_step polling loops. Use at most one immediate query_step if you need to confirm the execution_id/status, then stop; [AUTO-NOTIFICATION] messages will resume the conversation when background work completes."

// ScheduleContext bundles everything needed to identify and execute a schedule.
type ScheduleContext struct {
	WorkspacePath string
	WorkflowID    string
	WorkflowLabel string
	Schedule      WorkflowSchedule
	Capabilities  WorkflowCapabilities
	UserID        string // Set for multi-agent schedules (derived from _users/{userID}/ path)
	SourceType    string // "workflow" or "multi-agent"
	TriggerSource string // "cron" (default) or "manual"; encoded into the session ID
	// OriginSessionID is the chat session that triggered this run, when one did.
	// A scheduled run mints its own session, so without this link its terminals
	// are invisible to the tab that asked for the run. Empty for cron.
	OriginSessionID string
	// ForcePostRunMonitor is used by the toolbar's one-off Pulse action. It
	// reuses the scheduled-run pipeline without enabling recurring Pulse.
	ForcePostRunMonitor bool
	// PulseOnly suppresses the normal workflow message for the toolbar's one-off
	// Pulse action. Version preflight still runs before Pulse, which reviews the
	// latest retained workflow evidence and then executes the normal finalizer.
	PulseOnly              bool
	PulseEvidenceRunFolder string
	PulseEvidenceRunStatus string
	CalendarItem           *CalendarScheduleItem
	// ScheduledFor is the durable identity of a cron/calendar occurrence. It is
	// empty for a manual trigger, whose actual trigger time is its identity.
	ScheduledFor time.Time

	// ProducedRunEvidence reports whether this invocation actually started the
	// workflow and created or restarted a run folder. It is deliberately
	// independent of success: a failed run still produced evidence that Pulse
	// should review. A preflight abort against a pre-existing, untouched run
	// folder (e.g. a workflow reusing iteration-0) did not.
	ProducedRunEvidence bool
}

const manualWorkflowPulseScheduleID = "manual-pulse"

const scheduleScopeSeparator = "\x1f"

func workflowScheduleRuntimeKey(workspacePath, scheduleID string) string {
	return strings.Join([]string{"workflow", filepath.Clean(strings.TrimSpace(workspacePath)), strings.TrimSpace(scheduleID)}, scheduleScopeSeparator)
}

func multiAgentScheduleRuntimeKey(userID, scheduleID string) string {
	return strings.Join([]string{"multi-agent", strings.TrimSpace(userID), strings.TrimSpace(scheduleID)}, scheduleScopeSeparator)
}

func scheduleRuntimeKey(sctx *ScheduleContext) string {
	if sctx == nil {
		return ""
	}
	if sctx.SourceType == "multi-agent" {
		return multiAgentScheduleRuntimeKey(sctx.UserID, sctx.Schedule.ID)
	}
	return workflowScheduleRuntimeKey(sctx.WorkspacePath, sctx.Schedule.ID)
}

func scheduleRuntimeKeyHasID(key, scheduleID string) bool {
	return strings.HasSuffix(key, scheduleScopeSeparator+strings.TrimSpace(scheduleID))
}

func scheduleStateLockKeyFromRuntimeKey(runtimeKey string) string {
	parts := strings.Split(runtimeKey, scheduleScopeSeparator)
	if len(parts) < 3 || parts[0] != "workflow" {
		return runtimeKey
	}
	if parts[2] == manualWorkflowPulseScheduleID {
		return strings.Join([]string{"workflow-pulse", parts[1]}, scheduleScopeSeparator)
	}
	return strings.Join(parts[:2], scheduleScopeSeparator)
}

func scheduleStateScope(sctx *ScheduleContext) (scopeType, scopeID, lockKey string) {
	if sctx != nil && sctx.SourceType == "multi-agent" {
		scopeID = strings.TrimSpace(sctx.UserID)
		return "multi-agent", scopeID, strings.Join([]string{"multi-agent", scopeID, strings.TrimSpace(sctx.Schedule.ID)}, scheduleScopeSeparator)
	}
	if sctx != nil {
		scopeID = filepath.Clean(strings.TrimSpace(sctx.WorkspacePath))
		if sctx.Schedule.ID == manualWorkflowPulseScheduleID {
			return "workflow", scopeID, strings.Join([]string{"workflow-pulse", scopeID}, scheduleScopeSeparator)
		}
	}
	return "workflow", scopeID, strings.Join([]string{"workflow", scopeID}, scheduleScopeSeparator)
}

// newScheduleSessionID mints the session ID for a scheduled run. Encoding the
// trigger source (cron vs. manual) and the schedule ID prefix makes it easy to
// tell, just from the builder/ filename, where a conversation originated.
func (s *SchedulerService) newScheduleSessionID(sctx *ScheduleContext) string {
	trigger := sctx.TriggerSource
	if trigger == "" {
		trigger = "cron"
	}
	idPrefix := sctx.Schedule.ID
	if len(idPrefix) > 8 {
		idPrefix = idPrefix[:8]
	}
	sessionID := fmt.Sprintf("schedule-%s--%s_%d", trigger, idPrefix, time.Now().UnixNano())
	// Link it back to the chat that asked, so that chat's terminal rail can show
	// the run it started. No-op for cron, which has no originating chat.
	rememberScheduleOrigin(sessionID, sctx.OriginSessionID)
	return sessionID
}

// ScheduleRuntimeState holds in-memory runtime state for a schedule (not persisted in manifest).
type ScheduleRuntimeState struct {
	ActiveRunID         string     `json:"active_run_id,omitempty"`
	LastStatus          string     `json:"last_status,omitempty"`
	LastRunAt           *time.Time `json:"last_run_at,omitempty"`
	NextRunAt           *time.Time `json:"next_run_at,omitempty"`
	LastSessionID       string     `json:"last_session_id,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	LastDurationMs      *int64     `json:"last_duration_ms,omitempty"`
	RunCount            int        `json:"run_count"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
}

// registeredJob is a schedule registered for wall-clock evaluation.
type registeredJob struct {
	sctx      *ScheduleContext
	cronSched cron.Schedule // nil for calendar (one-time) jobs
	runAt     *time.Time    // non-nil for calendar (one-time) jobs
	lastFired time.Time     // durable scheduled occurrence cursor, not the wall-clock dispatch time
}

type dueRegisteredJob struct {
	job          *registeredJob
	scheduledFor time.Time
}

// SchedulerService manages cron job execution using wall-clock polling.
// Every 60 seconds it evaluates each registered schedule's cron expression against
// the current wall-clock time. This approach is immune to macOS App Nap and sleep/wake
// issues that wedge monotonic-clock-based timers (like gocron).
type SchedulerService struct {
	api  *StreamingAPI
	mu   sync.Mutex
	jobs map[string]*registeredJob // scoped schedule key → job
	// scheduleFingerprints tracks the persisted config loaded for each scoped
	// schedule, including disabled and calendar schedules with no future items.
	scheduleFingerprints map[string]string

	stateStoreMu sync.RWMutex
	stateStore   *schedulerstate.Store
	runCancelsMu sync.Mutex
	runCancels   map[string]context.CancelFunc

	// In-memory runtime state per schedule (survives within server lifetime, reset on restart)
	runtimeStates   map[string]*ScheduleRuntimeState
	runtimeStatesMu sync.RWMutex

	// Schedule-to-workspace index for quick lookups
	workspaceIndex   map[string]string // scheduleID → workspacePath
	workspaceIndexMu sync.RWMutex

	// Schedule-to-user index for multi-agent schedules
	userIndex   map[string]string // scheduleID → userID
	userIndexMu sync.RWMutex

	workflowManifestCacheMu        sync.Mutex
	workflowManifestCacheExpiresAt time.Time
	workflowManifestCache          []DiscoveredWorkflow
}

func (s *SchedulerService) logf(sctx *ScheduleContext, format string, args ...interface{}) {
	scheduleLogfWithContext(scheduleLogContext(sctx), format, args...)
}

func (s *SchedulerService) sessionLogf(sctx *ScheduleContext, sessionID string, format string, args ...interface{}) {
	scheduleLogfWithContext(scheduleLogContext(sctx).WithSession(sessionID), format, args...)
}

// NewSchedulerService creates a new manifest-based SchedulerService.
func NewSchedulerService(api *StreamingAPI) *SchedulerService {
	return &SchedulerService{
		api:                  api,
		jobs:                 make(map[string]*registeredJob),
		scheduleFingerprints: make(map[string]string),
		runCancels:           make(map[string]context.CancelFunc),
		runtimeStates:        make(map[string]*ScheduleRuntimeState),
		workspaceIndex:       make(map[string]string),
		userIndex:            make(map[string]string),
	}
}

func (s *SchedulerService) DiscoverWorkflowManifestsCached(ctx context.Context, ttl time.Duration) ([]DiscoveredWorkflow, error) {
	now := time.Now()

	s.workflowManifestCacheMu.Lock()
	if ttl > 0 && now.Before(s.workflowManifestCacheExpiresAt) && s.workflowManifestCache != nil {
		cached := append([]DiscoveredWorkflow(nil), s.workflowManifestCache...)
		s.workflowManifestCacheMu.Unlock()
		return cached, nil
	}
	s.workflowManifestCacheMu.Unlock()

	discovered, err := DiscoverWorkflowManifests(ctx)
	if err != nil {
		return nil, err
	}

	s.workflowManifestCacheMu.Lock()
	s.workflowManifestCache = append([]DiscoveredWorkflow(nil), discovered...)
	if ttl > 0 {
		s.workflowManifestCacheExpiresAt = now.Add(ttl)
	} else {
		s.workflowManifestCacheExpiresAt = time.Time{}
	}
	s.workflowManifestCacheMu.Unlock()

	return discovered, nil
}

func (s *SchedulerService) InvalidateWorkflowManifestCache() {
	s.workflowManifestCacheMu.Lock()
	s.workflowManifestCache = nil
	s.workflowManifestCacheExpiresAt = time.Time{}
	s.workflowManifestCacheMu.Unlock()
}

// Start scans all workspace folders for workflow.json manifests, loads enabled schedules,
// and starts the wall-clock tick loop.
func (s *SchedulerService) Start(ctx context.Context) error {
	scheduleLogf("[SCHEDULER] Starting manifest-based scheduler service...")
	s.stateStoreMu.Lock()
	if s.stateStore == nil {
		storePath := filepath.Join(fsutil.WorkspaceDocsRoot(), "_system", "schedule-state.sqlite")
		store, err := schedulerstate.Open(storePath)
		if err != nil {
			s.stateStoreMu.Unlock()
			return fmt.Errorf("initialize schedule state store: %w", err)
		}
		s.stateStore = store
	}
	if interrupted, err := s.stateStore.InterruptActiveRuns(ctx, "interrupted: server restarted", time.Now().UTC()); err != nil {
		s.stateStoreMu.Unlock()
		return fmt.Errorf("reconcile interrupted schedule runs: %w", err)
	} else if interrupted > 0 {
		scheduleLogf("[SCHEDULER] Marked %d durable schedule run(s) interrupted after restart", interrupted)
	}
	s.stateStoreMu.Unlock()

	// Discover all workflows by scanning workspace-docs/Workflow/*/workflow.json
	workflows := s.discoverWorkflows(ctx)
	scheduleLogf("[SCHEDULER] Discovered %d workflows with manifests", len(workflows))
	for _, wf := range workflows {
		finalized, err := finalizeAllUnresolvedPulseFinalCommands(ctx, wf.WorkspacePath, "failed", "Pulse interrupted because the server restarted")
		if err != nil {
			scheduleLogf("[SCHEDULER] Failed to reconcile stale Pulse final commands in %s: %v", wf.WorkspacePath, err)
		} else if finalized > 0 {
			scheduleLogf("[SCHEDULER] Marked %d stale Pulse final command(s) failed in %s", finalized, wf.WorkspacePath)
		}

		// Reviewer rows are stranded by the same interruption, and were not
		// covered by the sweep above (PLAT-017 reproduction).
		reviews, err := finalizeAllRunningPulseReviewLogs(ctx, wf.WorkspacePath, "Pulse interrupted because the server restarted")
		if err != nil {
			scheduleLogf("[SCHEDULER] Failed to reconcile stale Pulse review rows in %s: %v", wf.WorkspacePath, err)
		} else if reviews > 0 {
			scheduleLogf("[SCHEDULER] Marked %d stale Pulse review row(s) failed in %s", reviews, wf.WorkspacePath)
		}
	}

	// Mark any stale "running" runs as "error" — they were interrupted by a server restart
	for _, wf := range workflows {
		runs, err := ReadScheduleRuns(ctx, wf.WorkspacePath)
		if err != nil {
			continue
		}
		fixed := 0
		for i := range runs {
			if runs[i].Status == "running" {
				runs[i].Status = "error"
				runs[i].Error = "interrupted: server restarted"
				fixed++
			}
		}
		if fixed > 0 {
			_ = WriteScheduleRuns(ctx, wf.WorkspacePath, runs)
			scheduleLogf("[SCHEDULER] Marked %d stale running run(s) as error in %s", fixed, wf.WorkspacePath)
		}
	}

	loaded := 0
	for _, wf := range workflows {
		for _, sched := range wf.Manifest.Schedules {
			sctx := buildScheduleContext(wf.WorkspacePath, wf.Manifest, sched)
			if err := s.LoadSchedule(sctx); err != nil {
				scheduleLogf("[SCHEDULER] Failed to load schedule %s (%s): %v", sched.ID, sched.Name, err)
			} else if sched.Enabled {
				loaded++
			}
		}
	}

	// Discover multi-agent schedules from _users/*/multiagent-schedules.json
	maScheds, err := DiscoverMultiAgentSchedules(ctx)
	if err != nil {
		scheduleLogf("[SCHEDULER] Warning: failed to discover multi-agent schedules: %v", err)
	} else {
		scheduleLogf("[SCHEDULER] Discovered %d users with multi-agent schedules", len(maScheds))

		// Mark stale runs
		for _, ma := range maScheds {
			runs, err := ReadMultiAgentScheduleRuns(ctx, ma.UserID)
			if err != nil {
				continue
			}
			fixed := 0
			for i := range runs {
				if runs[i].Status == "running" {
					runs[i].Status = "error"
					runs[i].Error = "interrupted: server restarted"
					fixed++
				}
			}
			if fixed > 0 {
				_ = WriteMultiAgentScheduleRuns(ctx, ma.UserID, runs)
				scheduleLogf("[SCHEDULER] Marked %d stale multi-agent run(s) as error for user %s", fixed, ma.UserID)
			}
		}

		for _, ma := range maScheds {
			for _, sched := range MergeBuiltinSchedules(ma.ScheduleFile.Schedules) {
				sctx := buildMultiAgentScheduleContext(ma.UserID, sched, ma.ScheduleFile.Capabilities)
				if err := s.LoadSchedule(sctx); err != nil {
					scheduleLogf("[SCHEDULER] Failed to load multi-agent schedule %s (%s) for user %s: %v", sched.ID, sched.Name, ma.UserID, err)
				} else if sched.Enabled {
					loaded++
				}
			}
		}
	}

	scheduleLogf("[SCHEDULER] ✅ Started with %d schedules. Server time: %s, timezone: %s",
		loaded, time.Now().Format(time.RFC3339), time.Now().Location().String())

	// Periodically rescan multi-agent schedule files for changes (written by agents via shell)
	go s.multiAgentRescanLoop(ctx)

	// Wall-clock tick loop: every 60s, evaluate all registered schedules against current time.
	go s.tickLoop(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	scheduleLogf("[SCHEDULER] Shutting down (context canceled)")
	return nil
}

// multiAgentRescanLoop periodically checks for new/changed multi-agent schedule files.
// Agents write these files directly via shell commands, so we need to rescan to pick up changes.
func (s *SchedulerService) multiAgentRescanLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.rescanMultiAgentSchedules(ctx)
		}
	}
}

// tickLoop is the wall-clock scheduler. Every 60 seconds it evaluates each
// registered schedule against the current wall-clock time and fires any that
// are due. Unlike timer-based schedulers (gocron), this approach is immune to
// macOS App Nap and sleep/wake monotonic clock drift — if a job was missed
// during sleep, it fires on the first tick after wake.
func (s *SchedulerService) tickLoop(ctx context.Context) {
	const interval = 60 * time.Second
	const wakeThreshold = 90 * time.Second

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastTick := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			gap := t.Sub(lastTick)
			if gap > wakeThreshold {
				scheduleLogf("[SCHEDULER] 💤 WAKE_DETECTED gap=%s now=%s prev_tick=%s",
					gap.Round(time.Second), t.Format(time.RFC3339), lastTick.Format(time.RFC3339))
			}

			s.mu.Lock()
			parts := make([]string, 0, len(s.jobs))
			var toFire []dueRegisteredJob
			var missed []dueRegisteredJob
			for sid, job := range s.jobs {
				if job.cronSched != nil {
					occurrences := dueCronOccurrences(job.cronSched, job.lastFired, t)
					if len(occurrences) > 0 {
						for _, occurrence := range occurrences[:len(occurrences)-1] {
							missed = append(missed, dueRegisteredJob{job: job, scheduledFor: occurrence})
						}
						latest := occurrences[len(occurrences)-1]
						toFire = append(toFire, dueRegisteredJob{job: job, scheduledFor: latest})
					}
					parts = append(parts, fmt.Sprintf("%s next=%s", sid, job.cronSched.Next(t).UTC().Format(time.RFC3339)))
				} else if job.runAt != nil {
					if !job.runAt.After(t) && job.lastFired.Before(*job.runAt) {
						toFire = append(toFire, dueRegisteredJob{job: job, scheduledFor: job.runAt.UTC()})
					}
					parts = append(parts, fmt.Sprintf("%s at=%s", sid, job.runAt.UTC().Format(time.RFC3339)))
				}
			}
			s.mu.Unlock()

			scheduleLogf("[SCHEDULER] ❤️ heartbeat now=%s gap=%s jobs=%d due=%d missed=%d | %s",
				t.Format(time.RFC3339), gap.Round(time.Second), len(parts), len(toFire), len(missed), strings.Join(parts, ", "))

			failedOccurrenceLedger := make(map[*registeredJob]bool)
			for _, due := range missed {
				occurrenceCtx := *due.job.sctx
				occurrenceCtx.ScheduledFor = due.scheduledFor
				if err := s.recordScheduleFireDecision(context.Background(), &occurrenceCtx, "missed_scheduler_gap", "scheduler was not evaluating at the scheduled occurrence; the latest due occurrence will run as catch-up", "", t.UTC()); err != nil {
					failedOccurrenceLedger[due.job] = true
				}
			}
			for _, due := range toFire {
				occurrenceCtx := *due.job.sctx
				occurrenceCtx.ScheduledFor = due.scheduledFor
				if failedOccurrenceLedger[due.job] {
					s.logf(&occurrenceCtx, "[SCHEDULER] refusing to advance occurrence scheduled_for=%s because an earlier gap decision could not be recorded", due.scheduledFor.Format(time.RFC3339))
					continue
				}
				if err := s.recordScheduleFireDecision(context.Background(), &occurrenceCtx, "attempted", "scheduler selected this occurrence for execution", "", t.UTC()); err != nil {
					s.logf(&occurrenceCtx, "[SCHEDULER] refusing to launch occurrence scheduled_for=%s because its durable attempt could not be recorded: %v", due.scheduledFor.Format(time.RFC3339), err)
					continue
				}
				s.mu.Lock()
				if due.job.lastFired.Before(due.scheduledFor) {
					due.job.lastFired = due.scheduledFor
				}
				s.mu.Unlock()
				go s.triggerSchedule(due.job.sctx, due.scheduledFor)
			}

			lastTick = t
		}
	}
}

const maxCronOccurrenceScans = int(workflowScheduleHistoryRetention/time.Minute) + 1

// dueCronOccurrences returns the retained occurrence window after a durable
// cursor. Cron expressions have one-minute resolution, so a seven-day scan is
// bounded to at most 10,080 real occurrences. Every occurrence in that window
// is returned and durably classified; only the newest one executes as catch-up.
func dueCronOccurrences(schedule cron.Schedule, after, now time.Time) []time.Time {
	if schedule == nil || !now.After(after) {
		return nil
	}
	if floor := now.Add(-workflowScheduleHistoryRetention); after.Before(floor) {
		after = floor
	}
	occurrences := make([]time.Time, 0, 64)
	cursor := after
	for scans := 0; scans < maxCronOccurrenceScans; scans++ {
		next := schedule.Next(cursor)
		if next.After(now) || !next.After(cursor) {
			break
		}
		occurrences = append(occurrences, next.UTC())
		cursor = next
	}
	return occurrences
}

// rescanMultiAgentSchedules discovers multi-agent schedules and loads/unloads as needed.
func (s *SchedulerService) rescanMultiAgentSchedules(ctx context.Context) {
	maScheds, err := DiscoverMultiAgentSchedules(ctx)
	if err != nil {
		return
	}

	// Build set of all discovered scoped schedule keys.
	discovered := make(map[string]bool)

	for _, ma := range maScheds {
		for _, sched := range MergeBuiltinSchedules(ma.ScheduleFile.Schedules) {
			sctx := buildMultiAgentScheduleContext(ma.UserID, sched, ma.ScheduleFile.Capabilities)
			key := scheduleRuntimeKey(sctx)
			discovered[key] = true

			fingerprint := scheduleConfigFingerprint(sctx)
			s.mu.Lock()
			loadedFingerprint, isKnown := s.scheduleFingerprints[key]
			s.mu.Unlock()

			if sched.Enabled && (!isKnown || loadedFingerprint != fingerprint) {
				// New, re-enabled, or changed schedule.
				if err := s.LoadSchedule(sctx); err != nil {
					scheduleLogf("[SCHEDULER] Rescan: failed to load multi-agent schedule %s: %v", sched.ID, err)
				} else {
					scheduleLogf("[SCHEDULER] Rescan: loaded new or changed multi-agent schedule %s (%s) for user %s", sched.ID, sched.Name, ma.UserID)
				}
			} else if !sched.Enabled && (!isKnown || loadedFingerprint != fingerprint) {
				// Newly disabled or changed while disabled. LoadSchedule removes any
				// live registration and remembers this exact disabled config.
				if err := s.LoadSchedule(sctx); err != nil {
					scheduleLogf("[SCHEDULER] Rescan: failed to disable multi-agent schedule %s: %v", sched.ID, err)
				} else {
					scheduleLogf("[SCHEDULER] Rescan: removed disabled multi-agent schedule %s", sched.ID)
				}
			}
		}
	}

	// Remove schedules that were deleted from files
	s.userIndexMu.RLock()
	toRemove := []string{}
	for key := range s.userIndex {
		if !discovered[key] {
			toRemove = append(toRemove, key)
		}
	}
	s.userIndexMu.RUnlock()

	for _, key := range toRemove {
		_ = s.removeJobByKey(key)
		scheduleLogf("[SCHEDULER] Rescan: removed deleted multi-agent schedule %s", key)
	}
}

// discoveredWorkflow holds a manifest + its workspace path.
type discoveredWorkflow struct {
	WorkspacePath string
	Manifest      *WorkflowManifest
}

// discoverWorkflows scans workspace-docs/Workflow/ for workflow.json files.
func (s *SchedulerService) discoverWorkflows(ctx context.Context) []discoveredWorkflow {
	var results []discoveredWorkflow

	discovered, err := DiscoverWorkflowManifests(ctx)
	if err != nil {
		scheduleLogf("[SCHEDULER] Cannot scan workflow directory: %v", err)
		return nil
	}

	for _, item := range discovered {
		if len(item.Manifest.Schedules) > 0 {
			results = append(results, discoveredWorkflow{
				WorkspacePath: item.WorkspacePath,
				Manifest:      item.Manifest,
			})
		}
	}

	return results
}

// buildScheduleContext creates a ScheduleContext from a manifest and schedule.
func buildScheduleContext(workspacePath string, manifest *WorkflowManifest, sched WorkflowSchedule) *ScheduleContext {
	return &ScheduleContext{
		WorkspacePath: workspacePath,
		WorkflowID:    manifest.ID,
		WorkflowLabel: manifest.Label,
		Schedule:      sched,
		Capabilities:  manifest.Capabilities,
		SourceType:    "workflow",
	}
}

func shouldRunPostRunMonitor(sctx *ScheduleContext, manifest *WorkflowManifest) bool {
	if manifest == nil {
		return false
	}
	return manifest.MonitorEnabled() || (sctx != nil && sctx.ForcePostRunMonitor)
}

// buildMultiAgentScheduleContext creates a ScheduleContext for a multi-agent schedule.
func buildMultiAgentScheduleContext(userID string, sched WorkflowSchedule, caps WorkflowCapabilities) *ScheduleContext {
	return &ScheduleContext{
		WorkspacePath: "_users/" + userID,
		UserID:        userID,
		Schedule:      sched,
		Capabilities:  caps,
		SourceType:    "multi-agent",
	}
}

// Stop shuts down the scheduler.
func (s *SchedulerService) Stop() {
	s.runCancelsMu.Lock()
	runCancels := s.runCancels
	s.runCancels = make(map[string]context.CancelFunc)
	s.runCancelsMu.Unlock()
	for _, cancel := range runCancels {
		cancel()
	}
	s.mu.Lock()
	s.jobs = make(map[string]*registeredJob)
	s.scheduleFingerprints = make(map[string]string)
	s.mu.Unlock()
	s.stateStoreMu.Lock()
	if s.stateStore != nil {
		if err := s.stateStore.Close(); err != nil {
			scheduleLogf("[SCHEDULER] Failed to close schedule state store: %v", err)
		}
		s.stateStore = nil
	}
	s.stateStoreMu.Unlock()
	scheduleLogf("[SCHEDULER] Stopped")
}

// LoadSchedule registers a schedule for wall-clock evaluation from a ScheduleContext.
func (s *SchedulerService) LoadSchedule(sctx *ScheduleContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sched := sctx.Schedule
	runtimeKey := scheduleRuntimeKey(sctx)
	if s.scheduleFingerprints == nil {
		s.scheduleFingerprints = make(map[string]string)
	}

	// Remove existing registration if any.
	delete(s.jobs, runtimeKey)
	calendarPrefix := runtimeKey + "__cal__"
	for key := range s.jobs {
		if strings.HasPrefix(key, calendarPrefix) {
			delete(s.jobs, key)
		}
	}

	// Update workspace index
	s.workspaceIndexMu.Lock()
	s.workspaceIndex[runtimeKey] = sctx.WorkspacePath
	s.workspaceIndexMu.Unlock()

	if sctx.SourceType == "workflow" {
		if err := EnsureWorkflowScheduleExecutionTracker(context.Background(), sctx.WorkspacePath, sched, time.Now().UTC()); err != nil {
			s.logf(sctx, "[SCHEDULER] Warning: failed to initialize execution history for %s: %v", sched.ID, err)
		}
	}

	// Update user index for multi-agent schedules
	if sctx.UserID != "" {
		s.userIndexMu.Lock()
		s.userIndex[runtimeKey] = sctx.UserID
		s.userIndexMu.Unlock()
	}

	if !sched.Enabled {
		s.scheduleFingerprints[runtimeKey] = scheduleConfigFingerprint(sctx)
		return nil
	}

	scheduleType := scheduleTypeOrDefault(sched.ScheduleType)
	var nextRun *time.Time
	sctxCopy := *sctx

	if scheduleType == "calendar" {
		// Calendar schedules: register one job per future calendar item.
		for _, item := range sched.CalendarItems {
			runAt, err := calendarItemRunTime(sched, item)
			if err != nil || !runAt.After(time.Now().UTC()) {
				continue
			}
			if nextRun == nil || runAt.Before(*nextRun) {
				runAtCopy := runAt
				nextRun = &runAtCopy
			}
			itemCopy := item
			itemSctx := sctxCopy
			itemSctx.Schedule = scheduleWithCalendarItem(sched, itemCopy)
			itemSctx.CalendarItem = &itemCopy
			calID := fmt.Sprintf("%s__cal__%s_%s", runtimeKey, item.Date, item.Time)
			s.jobs[calID] = &registeredJob{
				sctx:  &itemSctx,
				runAt: &runAt,
			}
		}
		if nextRun == nil {
			s.logf(sctx, "[SCHEDULER] Calendar schedule %s (%s) has no future items; not registering", sched.ID, sched.Name)
		}
	} else {
		// Cron schedules: parse the expression and register for wall-clock eval.
		loc, err := time.LoadLocation(scheduleTimezoneOrDefault(sched.Timezone))
		if err != nil {
			loc = time.UTC
		}
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		cronSched, err := parser.Parse(sched.CronExpression)
		if err != nil {
			return fmt.Errorf("failed to parse cron expression %q: %w", sched.CronExpression, err)
		}
		// Wrap with timezone-aware location
		cronSched = &tzSchedule{inner: cronSched, loc: loc}
		lastFired := time.Now().Add(-30 * time.Second) // a newly created schedule starts with its next future occurrence
		if latest, ok := s.latestCronOccurrence(sctx); ok {
			lastFired = latest
		}

		s.jobs[runtimeKey] = &registeredJob{
			sctx:      &sctxCopy,
			cronSched: cronSched,
			lastFired: lastFired,
		}
		nextRun = getNextRunTime(sched.CronExpression, sched.Timezone)
	}

	// Initialize runtime state with next run.
	s.updateRuntimeState(runtimeKey, func(state *ScheduleRuntimeState) {
		state.NextRunAt = nextRun
	})
	s.scheduleFingerprints[runtimeKey] = scheduleConfigFingerprint(sctx)

	nextRunStr := "unknown"
	if nextRun != nil {
		nextRunStr = nextRun.Format(time.RFC3339)
	}
	s.logf(sctx, "[SCHEDULER] Registered schedule %s (%s) type=%s cron=%q timezone=%s next_run=%s",
		sched.ID, sched.Name, scheduleType, sched.CronExpression, sched.Timezone, nextRunStr)
	return nil
}

func (s *SchedulerService) latestCronOccurrence(sctx *ScheduleContext) (time.Time, bool) {
	if sctx == nil {
		return time.Time{}, false
	}
	s.stateStoreMu.RLock()
	defer s.stateStoreMu.RUnlock()
	if s.stateStore == nil {
		return time.Time{}, false
	}
	scopeType, scopeID, _ := scheduleStateScope(sctx)
	decision, err := s.stateStore.LatestFireDecision(context.Background(), scopeType, scopeID, sctx.Schedule.ID, "cron")
	if err != nil || decision.ScheduledFor.IsZero() {
		return time.Time{}, false
	}
	if decision.Decision == "attempted" {
		decision.DecisionID = uuid.NewString()
		decision.Decision = "launch_outcome_unknown"
		decision.Reason = "the occurrence was durably attempted, but no later launch outcome was recorded before restart; execution may or may not have started"
		decision.FiredAt = time.Now().UTC()
		if err := s.stateStore.RecordFireDecision(context.Background(), decision); err != nil {
			s.logf(sctx, "[SCHEDULER_STATE] failed to reconcile interrupted attempted occurrence scheduled_for=%s: %v", decision.ScheduledFor.Format(time.RFC3339), err)
		}
	}
	return decision.ScheduledFor.UTC(), true
}

// tzSchedule wraps a cron.Schedule to evaluate in a specific timezone.
type tzSchedule struct {
	inner cron.Schedule
	loc   *time.Location
}

func (tz *tzSchedule) Next(t time.Time) time.Time {
	return tz.inner.Next(t.In(tz.loc))
}

// ReloadSchedule reloads a schedule from its manifest after it's been updated.
func (s *SchedulerService) ReloadSchedule(ctx context.Context, workspacePath string, scheduleID string) error {
	manifest, found, err := ReadWorkflowManifest(ctx, workspacePath)
	if err != nil || !found {
		return fmt.Errorf("failed to read manifest from %s: %w", workspacePath, err)
	}

	// Find the schedule
	for _, sched := range manifest.Schedules {
		if sched.ID == scheduleID {
			return s.LoadSchedule(buildScheduleContext(workspacePath, manifest, sched))
		}
	}

	// Schedule not found — remove it
	return s.removeJobByKey(workflowScheduleRuntimeKey(workspacePath, scheduleID))
}

// ReloadMultiAgentSchedule reloads a multi-agent schedule after it's been updated.
func (s *SchedulerService) ReloadMultiAgentSchedule(ctx context.Context, userID string, scheduleID string) error {
	f, exists, err := ReadMultiAgentSchedules(ctx, userID)
	if err != nil || !exists {
		return s.removeJobByKey(multiAgentScheduleRuntimeKey(userID, scheduleID))
	}

	for _, sched := range MergeBuiltinSchedules(f.Schedules) {
		if sched.ID == scheduleID {
			return s.LoadSchedule(buildMultiAgentScheduleContext(userID, sched, f.Capabilities))
		}
	}

	return s.removeJobByKey(multiAgentScheduleRuntimeKey(userID, scheduleID))
}

func (s *SchedulerService) removeJobByKey(key string) error {
	s.mu.Lock()
	delete(s.jobs, key)
	delete(s.scheduleFingerprints, key)
	// Also remove calendar sub-jobs.
	prefix := key + "__cal__"
	for k := range s.jobs {
		if strings.HasPrefix(k, prefix) {
			delete(s.jobs, k)
		}
	}
	s.mu.Unlock()

	s.workspaceIndexMu.Lock()
	delete(s.workspaceIndex, key)
	s.workspaceIndexMu.Unlock()

	s.userIndexMu.Lock()
	delete(s.userIndex, key)
	s.userIndexMu.Unlock()

	s.runtimeStatesMu.Lock()
	if state := s.runtimeStates[key]; state == nil || state.ActiveRunID == "" {
		delete(s.runtimeStates, key)
	}
	s.runtimeStatesMu.Unlock()

	return nil
}

// RemoveJob removes a schedule only when its ID resolves to one loaded scope.
// Scoped callers should use ReloadSchedule/ReloadMultiAgentSchedule or the
// dedicated helpers below so a copied schedule cannot remove another scope.
func (s *SchedulerService) RemoveJob(scheduleID string) error {
	keys := s.loadedScheduleKeys(scheduleID)
	if len(keys) == 0 {
		return nil
	}
	if len(keys) > 1 {
		return fmt.Errorf("schedule ID %q is ambiguous across %d scopes", scheduleID, len(keys))
	}
	return s.removeJobByKey(keys[0])
}

func (s *SchedulerService) RemoveWorkflowJob(workspacePath, scheduleID string) error {
	return s.removeJobByKey(workflowScheduleRuntimeKey(workspacePath, scheduleID))
}

func (s *SchedulerService) RemoveMultiAgentJob(userID, scheduleID string) error {
	return s.removeJobByKey(multiAgentScheduleRuntimeKey(userID, scheduleID))
}

func (s *SchedulerService) loadedScheduleKeys(scheduleID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, 1)
	for key, job := range s.jobs {
		if strings.Contains(key, "__cal__") || job == nil || job.sctx == nil || job.sctx.Schedule.ID != scheduleID {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// GetRuntimeState is the legacy unscoped lookup. It returns state only when the
// schedule ID resolves to one loaded scope; scoped callers must use the methods
// below so copied workflows and per-user built-ins cannot collide.
func (s *SchedulerService) GetRuntimeState(scheduleID string) ScheduleRuntimeState {
	keys := s.runtimeKeysForScheduleID(scheduleID)
	if len(keys) == 1 {
		return s.getRuntimeStateByKey(keys[0])
	}
	// Preserve tests and pre-migration in-memory state that used a bare key.
	return s.getRuntimeStateByKey(scheduleID)
}

func (s *SchedulerService) GetRuntimeStateForWorkflow(workspacePath, scheduleID string) ScheduleRuntimeState {
	key := workflowScheduleRuntimeKey(workspacePath, scheduleID)
	merged := s.getRuntimeStateByKey(key)
	_ = s.reconcileWorkflowScheduleRuns(context.Background(), workspacePath, scheduleID)
	runs, err := ReadScheduleRuns(context.Background(), workspacePath)
	if err == nil {
		return mergeRuntimeStateWithRuns(merged, scheduleID, runs)
	}
	return merged
}

func (s *SchedulerService) GetRuntimeStateForUser(userID, scheduleID string) ScheduleRuntimeState {
	merged := s.getRuntimeStateByKey(multiAgentScheduleRuntimeKey(userID, scheduleID))
	_ = s.reconcileMultiAgentScheduleRuns(context.Background(), userID, scheduleID)
	runs, err := ReadMultiAgentScheduleRuns(context.Background(), userID)
	if err == nil {
		return mergeRuntimeStateWithRuns(merged, scheduleID, runs)
	}
	return merged
}

func (s *SchedulerService) getRuntimeStateByKey(key string) ScheduleRuntimeState {
	s.runtimeStatesMu.RLock()
	var merged ScheduleRuntimeState
	if state, ok := s.runtimeStates[key]; ok {
		merged = cloneScheduleRuntimeState(state)
	}
	s.runtimeStatesMu.RUnlock()
	return merged
}

func cloneScheduleRuntimeState(state *ScheduleRuntimeState) ScheduleRuntimeState {
	if state == nil {
		return ScheduleRuntimeState{}
	}
	copy := *state
	if state.LastRunAt != nil {
		value := *state.LastRunAt
		copy.LastRunAt = &value
	}
	if state.NextRunAt != nil {
		value := *state.NextRunAt
		copy.NextRunAt = &value
	}
	if state.LastDurationMs != nil {
		value := *state.LastDurationMs
		copy.LastDurationMs = &value
	}
	return copy
}

func (s *SchedulerService) runtimeKeysForScheduleID(scheduleID string) []string {
	s.runtimeStatesMu.RLock()
	defer s.runtimeStatesMu.RUnlock()
	keys := make([]string, 0, 1)
	for key := range s.runtimeStates {
		if scheduleRuntimeKeyHasID(key, scheduleID) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func (s *SchedulerService) reconcileWorkflowScheduleRuns(ctx context.Context, workspacePath, scheduleID string) error {
	if s == nil || s.api == nil || strings.TrimSpace(workspacePath) == "" {
		return nil
	}

	runs, err := ReadScheduleRuns(ctx, workspacePath)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	changed := false
	for i := range runs {
		if runs[i].Status != "running" {
			continue
		}
		if scheduleID != "" && runs[i].ScheduleID != scheduleID {
			continue
		}

		status, errMsg, shouldFinalize := s.reconciledScheduleRunStatus(&runs[i], now)
		if !shouldFinalize {
			continue
		}

		runs[i].Status = status
		runs[i].Error = errMsg
		durationMs := now.Sub(runs[i].StartedAt).Milliseconds()
		if durationMs < 0 {
			durationMs = 0
		}
		runs[i].DurationMs = &durationMs
		runs[i].CompletedAt = &now
		changed = true
	}

	if !changed {
		return nil
	}
	return WriteScheduleRuns(ctx, workspacePath, runs)
}

func (s *SchedulerService) reconcileMultiAgentScheduleRuns(ctx context.Context, userID, scheduleID string) error {
	if s == nil || s.api == nil || strings.TrimSpace(userID) == "" {
		return nil
	}

	runs, err := ReadMultiAgentScheduleRuns(ctx, userID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	changed := false
	for i := range runs {
		if runs[i].Status != "running" {
			continue
		}
		if scheduleID != "" && runs[i].ScheduleID != scheduleID {
			continue
		}

		status, errMsg, shouldFinalize := s.reconciledScheduleRunStatus(&runs[i], now)
		if !shouldFinalize {
			continue
		}

		runs[i].Status = status
		runs[i].Error = errMsg
		durationMs := now.Sub(runs[i].StartedAt).Milliseconds()
		if durationMs < 0 {
			durationMs = 0
		}
		runs[i].DurationMs = &durationMs
		runs[i].CompletedAt = &now
		changed = true
	}

	if !changed {
		return nil
	}
	return WriteMultiAgentScheduleRuns(ctx, userID, runs)
}

func (s *SchedulerService) reconciledScheduleRunStatus(run *ScheduleRunEntry, now time.Time) (string, string, bool) {
	if run == nil {
		return "", "", false
	}

	if strings.TrimSpace(run.SessionID) == "" {
		if now.Sub(run.StartedAt) > 10*time.Minute {
			return "error", "interrupted: no session id recorded", true
		}
		return "", "", false
	}

	session, exists := s.api.getActiveSession(run.SessionID)
	if !exists {
		return "error", "interrupted: session not active", true
	}

	switch session.Status {
	case "running":
		return "", "", false
	case "completed":
		// A scheduled run is many sequential turns sharing one session, and
		// between them the session reads "completed" — the previous turn is
		// done, the next has not started. Finalizing here called the whole run
		// a success while it was on turn 1 of 10.
		//
		// rtslatency's 2026-07-31 02:30 run was recorded success/84,599 ms with
		// completed_at 84 seconds after start, while the log shows turns 2-5
		// running for another hour and turn 6 still going when the process shut
		// down. Once stamped, the record never corrected itself, so every run
		// killed by a restart also read as successful and the workflow looked
		// healthy while its digest had not shipped since 2026-07-29.
		//
		// This reconciler exists to clean up runs the scheduler abandoned, and
		// it cannot tell "between turns" from "finished". Success is the
		// scheduler's verdict to record when its own turn loop ends. Past the
		// grace window an abandoned run is reported interrupted, never success.
		if now.Sub(run.StartedAt) <= scheduleRunAbandonedAfter {
			return "", "", false
		}
		return "error", "interrupted: scheduler never recorded a terminal result", true
	case "stopped", "dismissed":
		return "stopped", fmt.Sprintf("session ended with status %s", session.Status), true
	case "error", "failed", "inactive":
		return "error", fmt.Sprintf("session ended with status %s", session.Status), true
	default:
		return "", "", false
	}
}

// scheduleRunAbandonedAfter bounds how long a run may sit between turns before
// the reconciler treats it as abandoned. It must exceed a real run: rtslatency's
// healthy runs take 45 minutes to 2.5 hours, so a shorter window would finalize
// live work as interrupted.
var scheduleRunAbandonedAfter = 6 * time.Hour

func mergeRuntimeStateWithRuns(state ScheduleRuntimeState, scheduleID string, runs []ScheduleRunEntry) ScheduleRuntimeState {
	var filtered []ScheduleRunEntry
	for _, run := range runs {
		if run.ScheduleID == scheduleID {
			filtered = append(filtered, run)
		}
	}
	if len(filtered) == 0 {
		return state
	}

	latest := filtered[0]
	if state.RunCount < len(filtered) {
		state.RunCount = len(filtered)
	}
	// File history records the workflow result before Pulse finishes. Preserve
	// the live in-memory state for that same run so the UI cannot report success
	// and admit another trigger while Pulse still owns the session.
	if state.LastStatus == "running" &&
		(state.LastSessionID == "" || latest.SessionID == "" || latest.SessionID == state.LastSessionID) {
		return state
	}

	shouldAdoptLatest := state.LastRunAt == nil || latest.StartedAt.After(*state.LastRunAt)
	sameRun := state.LastRunAt != nil && latest.StartedAt.Equal(*state.LastRunAt)

	if shouldAdoptLatest {
		startedAt := latest.StartedAt
		state.LastRunAt = &startedAt
		state.LastStatus = latest.Status
		state.LastSessionID = latest.SessionID
		state.LastError = latest.Error
		state.LastDurationMs = latest.DurationMs
		return state
	}

	if sameRun {
		if state.LastStatus == "" {
			state.LastStatus = latest.Status
		}
		if state.LastSessionID == "" {
			state.LastSessionID = latest.SessionID
		}
		if state.LastError == "" {
			state.LastError = latest.Error
		}
		if state.LastDurationMs == nil {
			state.LastDurationMs = latest.DurationMs
		}
	}

	return state
}

// GetWorkspaceForSchedule returns the workspace path for a schedule ID.
func (s *SchedulerService) GetWorkspaceForSchedule(scheduleID string) string {
	s.workspaceIndexMu.RLock()
	defer s.workspaceIndexMu.RUnlock()
	match := ""
	for key, workspacePath := range s.workspaceIndex {
		if !scheduleRuntimeKeyHasID(key, scheduleID) {
			continue
		}
		if match != "" && match != workspacePath {
			return ""
		}
		match = workspacePath
	}
	return match
}

// GetUserForSchedule returns the user ID for a multi-agent schedule ID.
func (s *SchedulerService) GetUserForSchedule(scheduleID string) string {
	s.userIndexMu.RLock()
	defer s.userIndexMu.RUnlock()
	match := ""
	for key, userID := range s.userIndex {
		if !scheduleRuntimeKeyHasID(key, scheduleID) {
			continue
		}
		if match != "" && match != userID {
			return ""
		}
		match = userID
	}
	return match
}

// TriggerNow triggers a schedule immediately (for manual trigger API).
func (s *SchedulerService) TriggerNow(workspacePath string, scheduleID string) (string, error) {
	return s.TriggerNowFromSession(workspacePath, scheduleID, "")
}

// TriggerNowFromSession is TriggerNow with the chat session that asked for the
// run, so the run's terminals can be surfaced in that chat.
func (s *SchedulerService) TriggerNowFromSession(workspacePath, scheduleID, originSessionID string) (string, error) {
	ctx := context.Background()

	manifest, found, err := ReadWorkflowManifest(ctx, workspacePath)
	if err != nil || !found {
		return "", fmt.Errorf("failed to read manifest from %s: %w", workspacePath, err)
	}

	var sched *WorkflowSchedule
	for i := range manifest.Schedules {
		if manifest.Schedules[i].ID == scheduleID {
			sched = &manifest.Schedules[i]
			break
		}
	}
	if sched == nil {
		return "", fmt.Errorf("schedule %s not found in manifest at %s", scheduleID, workspacePath)
	}
	sctx := buildScheduleContext(workspacePath, manifest, *sched)
	sctx.TriggerSource = "manual"
	sctx.OriginSessionID = originSessionID
	startTime := time.Now().UTC()

	// A workflow may have one interactive builder chat and one schedule at the
	// same time. Other workflow executions still block a schedule start.
	if activeExec := s.findActiveNonBuilderExecutionForWorkspace(workspacePath); activeExec != nil {
		triggeredBy := activeExec.TriggeredBy
		if strings.TrimSpace(triggeredBy) == "" {
			triggeredBy = "unknown"
		}
		err := fmt.Errorf(
			"workflow already has an active %s run (session: %s)",
			triggeredBy,
			activeExec.SessionID,
		)
		s.recordScheduleFireDecision(ctx, sctx, "skipped_busy", err.Error(), "", startTime)
		return "", err
	}

	// Reserve the in-memory run and cancellation handle atomically, then claim
	// the durable lease without holding the global runtime-state mutex.
	runtimeKey := workflowScheduleRuntimeKey(workspacePath, scheduleID)
	workflowRuntimeKeys := make([]string, 0, len(manifest.Schedules))
	for i := range manifest.Schedules {
		workflowRuntimeKeys = append(workflowRuntimeKeys, workflowScheduleRuntimeKey(workspacePath, manifest.Schedules[i].ID))
	}
	runID := uuid.NewString()
	s.runtimeStatesMu.Lock()
	state := s.getRuntimeStateLocked(runtimeKey)
	if state.LastStatus == "running" {
		s.runtimeStatesMu.Unlock()
		s.recordScheduleFireDecision(ctx, sctx, "skipped_busy", "schedule is already running", "", startTime)
		return "", fmt.Errorf("job is already running (session: %s)", state.LastSessionID)
	}
	if otherKey, otherSession := runningScheduleInSetLocked(s.runtimeStates, workflowRuntimeKeys, runtimeKey); otherKey != "" {
		s.runtimeStatesMu.Unlock()
		s.recordScheduleFireDecision(ctx, sctx, "skipped_busy", "another schedule owns the workflow", "", startTime)
		return "", fmt.Errorf("another schedule is already running (session: %s)", otherSession)
	}
	previousState := *state
	runCtx := s.activateScheduleRunLocked(state, runID, startTime)
	s.runtimeStatesMu.Unlock()
	if err := s.claimScheduleRun(ctx, sctx, runID, startTime); err != nil {
		s.rollbackScheduleRunActivation(runtimeKey, runID, previousState)
		s.recordScheduleFireDecision(ctx, sctx, "skipped_busy", err.Error(), "", startTime)
		return "", err
	}
	if s.abortCanceledScheduleRunBeforeStart(runCtx, sctx, runtimeKey, runID) {
		return "", context.Canceled
	}
	s.recordScheduleFireDecision(ctx, sctx, "started", "manual trigger accepted", runID, startTime)

	if err := RecordWorkflowScheduleExecution(context.Background(), workspacePath, *sched, startTime); err != nil {
		s.logf(sctx, "[SCHEDULER] Warning: failed to record manual schedule execution for %s: %v", scheduleID, err)
	}

	go func() {
		if _, err := s.runJob(runCtx, sctx, runID); err != nil {
			scheduleLogf("[SCHEDULER] Triggered job %s failed: %v", scheduleID, err)
		}
	}()

	return "triggered", nil
}

// TriggerPulseNow runs version-preflight -> Pulse against the latest
// retained workflow evidence. It does not execute the workflow or change a saved
// schedule.
func (s *SchedulerService) TriggerPulseNow(workspacePath string) (string, error) {
	ctx := context.Background()
	workspacePath = filepath.Clean(strings.TrimSpace(workspacePath))
	if workspacePath == "." || workspacePath == "" {
		return "", errors.New("workspace_path is required")
	}

	manifest, found, err := ReadWorkflowManifest(ctx, workspacePath)
	if err != nil {
		return "", fmt.Errorf("failed to read manifest from %s: %w", workspacePath, err)
	}
	if !found {
		return "", fmt.Errorf("workflow manifest not found at %s", workspacePath)
	}

	sched := WorkflowSchedule{
		ID:             manualWorkflowPulseScheduleID,
		Name:           "Run Pulse",
		Description:    "One-off Pulse review of the latest retained run; not persisted as a schedule",
		ScheduleType:   "cron",
		CronExpression: "",
		Timezone:       "UTC",
		Mode:           "workshop",
		WorkshopMode:   "run",
	}
	sctx := buildScheduleContext(workspacePath, manifest, sched)
	sctx.TriggerSource = "manual"
	sctx.ForcePostRunMonitor = true
	sctx.PulseOnly = true
	sctx.PulseEvidenceRunFolder, sctx.PulseEvidenceRunStatus = latestRetainedPulseEvidence(ctx, workspacePath)
	startTime := time.Now().UTC()

	// One interactive builder chat may coexist with this run, matching normal
	// schedule behavior. Another workflow/schedule execution may not.
	if activeExec := s.findActiveNonBuilderExecutionForWorkspace(workspacePath); activeExec != nil {
		triggeredBy := strings.TrimSpace(activeExec.TriggeredBy)
		if triggeredBy == "" {
			triggeredBy = "unknown"
		}
		return "", fmt.Errorf("workflow already has an active %s run (session: %s)", triggeredBy, activeExec.SessionID)
	}

	runtimeKey := workflowScheduleRuntimeKey(workspacePath, sched.ID)
	workflowRuntimeKeys := make([]string, 0, len(manifest.Schedules)+1)
	for i := range manifest.Schedules {
		workflowRuntimeKeys = append(workflowRuntimeKeys, workflowScheduleRuntimeKey(workspacePath, manifest.Schedules[i].ID))
	}
	workflowRuntimeKeys = append(workflowRuntimeKeys, runtimeKey)
	runID := uuid.NewString()
	s.runtimeStatesMu.Lock()
	state := s.getRuntimeStateLocked(runtimeKey)
	if state.LastStatus == "running" {
		s.runtimeStatesMu.Unlock()
		return "", fmt.Errorf("Pulse run is already active (session: %s)", state.LastSessionID)
	}
	if _, otherSession := runningScheduleInSetLocked(s.runtimeStates, workflowRuntimeKeys, runtimeKey); otherSession != "" {
		s.runtimeStatesMu.Unlock()
		return "", fmt.Errorf("another schedule is already running (session: %s)", otherSession)
	}
	previousState := *state
	runCtx := s.activateScheduleRunLocked(state, runID, startTime)
	s.runtimeStatesMu.Unlock()

	if err := s.claimScheduleRun(ctx, sctx, runID, startTime); err != nil {
		s.rollbackScheduleRunActivation(runtimeKey, runID, previousState)
		return "", err
	}
	if s.abortCanceledScheduleRunBeforeStart(runCtx, sctx, runtimeKey, runID) {
		return "", context.Canceled
	}

	go func() {
		defer s.cleanupRemovedScheduleRuntimeState(runtimeKey)
		if _, runErr := s.runJob(runCtx, sctx, runID); runErr != nil {
			scheduleLogf("[SCHEDULER] One-off Pulse run failed for %s: %v", workspacePath, runErr)
		}
	}()

	return runID, nil
}

func latestRetainedPulseEvidence(ctx context.Context, workspacePath string) (string, string) {
	runs, err := ReadScheduleRuns(ctx, workspacePath)
	if err == nil {
		if runFolder, status, ok := latestRetainedPulseEvidenceFromRuns(runs); ok {
			return runFolder, status
		}
	}

	// iteration-0 is the active retained slot even before schedule history has
	// been written. Pulse can still review plan/config/report evidence when the
	// folder has no completed run artifacts yet.
	return "iteration-0", "unknown"
}

func latestRetainedPulseEvidenceFromRuns(runs []ScheduleRunEntry) (string, string, bool) {
	fallbackFolder := ""
	fallbackStatus := ""
	for _, run := range runs {
		if run.ScheduleID == manualWorkflowPulseScheduleID || strings.TrimSpace(run.RunFolder) == "" {
			continue
		}
		status := strings.TrimSpace(run.Status)
		if status == "" {
			status = "unknown"
		}
		if status == "running" {
			if fallbackFolder == "" {
				fallbackFolder = strings.TrimSpace(run.RunFolder)
				fallbackStatus = status
			}
			continue
		}
		return strings.TrimSpace(run.RunFolder), status, true
	}
	if fallbackFolder != "" {
		return fallbackFolder, fallbackStatus, true
	}
	return "", "", false
}

// TriggerMultiAgentNow triggers a multi-agent schedule immediately.
func (s *SchedulerService) TriggerMultiAgentNow(userID string, scheduleID string) (string, error) {
	ctx := context.Background()

	f, _, err := ReadMultiAgentSchedules(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to read multi-agent schedules for user %s: %w", userID, err)
	}

	var sched *WorkflowSchedule
	schedules := MergeBuiltinSchedules(f.Schedules)
	for i := range schedules {
		if schedules[i].ID == scheduleID {
			sched = &schedules[i]
			break
		}
	}
	if sched == nil {
		return "", fmt.Errorf("multi-agent schedule %s not found for user %s", scheduleID, userID)
	}

	startTime := time.Now().UTC()
	sctx := buildMultiAgentScheduleContext(userID, *sched, f.Capabilities)
	sctx.TriggerSource = "manual"
	runtimeKey := multiAgentScheduleRuntimeKey(userID, scheduleID)
	multiAgentRuntimeKeys := make([]string, 0, len(schedules))
	for i := range schedules {
		multiAgentRuntimeKeys = append(multiAgentRuntimeKeys, multiAgentScheduleRuntimeKey(userID, schedules[i].ID))
	}
	runID := uuid.NewString()
	s.runtimeStatesMu.Lock()
	state := s.getRuntimeStateLocked(runtimeKey)
	if state.LastStatus == "running" {
		s.runtimeStatesMu.Unlock()
		s.recordScheduleFireDecision(ctx, sctx, "skipped_busy", "schedule is already running", "", startTime)
		return "", fmt.Errorf("job is already running (session: %s)", state.LastSessionID)
	}
	if otherKey, otherSession := runningScheduleInSetLocked(s.runtimeStates, multiAgentRuntimeKeys, runtimeKey); otherKey != "" {
		s.runtimeStatesMu.Unlock()
		s.recordScheduleFireDecision(ctx, sctx, "skipped_busy", "another Chief of Staff schedule is running", "", startTime)
		return "", fmt.Errorf("another Chief of Staff schedule is already running (session: %s)", otherSession)
	}
	previousState := *state
	runCtx := s.activateScheduleRunLocked(state, runID, startTime)
	s.runtimeStatesMu.Unlock()
	if err := s.claimScheduleRun(ctx, sctx, runID, startTime); err != nil {
		s.rollbackScheduleRunActivation(runtimeKey, runID, previousState)
		s.recordScheduleFireDecision(ctx, sctx, "skipped_busy", err.Error(), "", startTime)
		return "", err
	}
	if s.abortCanceledScheduleRunBeforeStart(runCtx, sctx, runtimeKey, runID) {
		return "", context.Canceled
	}

	s.recordScheduleFireDecision(ctx, sctx, "started", "manual trigger accepted", runID, startTime)

	go func() {
		if _, err := s.runJob(runCtx, sctx, runID); err != nil {
			scheduleLogf("[SCHEDULER] Triggered multi-agent job %s failed: %v", scheduleID, err)
		}
	}()

	return "triggered", nil
}

// StopRunningJob stops a running scheduled job by canceling its session.
func (s *SchedulerService) StopRunningJobForWorkflow(workspacePath, scheduleID string) {
	s.stopRunningJob(workflowScheduleRuntimeKey(workspacePath, scheduleID), scheduleID)
}

func (s *SchedulerService) StopRunningJobForUser(userID, scheduleID string) {
	s.stopRunningJob(multiAgentScheduleRuntimeKey(userID, scheduleID), scheduleID)
}

func (s *SchedulerService) StopRunningJob(scheduleID string) {
	keys := s.runtimeKeysForScheduleID(scheduleID)
	if len(keys) != 1 {
		return
	}
	s.stopRunningJob(keys[0], scheduleID)
}

func (s *SchedulerService) stopRunningJob(runtimeKey, scheduleID string) {
	s.runtimeStatesMu.Lock()
	state := s.getRuntimeStateLocked(runtimeKey)
	sessionID := state.LastSessionID
	runID := state.ActiveRunID
	state.LastStatus = "stopped"
	state.LastError = "stopped by user"
	state.ActiveRunID = ""
	s.runtimeStatesMu.Unlock()
	if runID == "" {
		lockKey := scheduleStateLockKeyFromRuntimeKey(runtimeKey)
		s.stateStoreMu.RLock()
		store := s.stateStore
		if store != nil {
			if active, err := store.ActiveRunByLockKey(context.Background(), lockKey); err == nil {
				runID = active.RunID
			}
		}
		s.stateStoreMu.RUnlock()
	}
	if runID != "" {
		s.cancelScheduleRunContext(runID)
		s.transitionScheduleRun(context.Background(), nil, schedulerstate.Transition{
			RunID: runID, To: schedulerstate.StateStopped, Reason: "stopped by user", SessionID: sessionID,
			ErrorMessage: "stopped by user", At: time.Now().UTC(),
		})
	}
	if sessionID == "" {
		return
	}

	scheduleLogf("[SCHEDULER] Stopping running job %s (session: %s)", scheduleID, sessionID)
	if isScheduledSession(sessionID) {
		s.api.markSessionTurnInterrupted(sessionID)
	}
	s.cancelScheduledSessionWork(sessionID, "scheduled job stopped by user", runtimePhaseCanceled)

	scheduleLogf("[SCHEDULER] Stopped job %s (session: %s)", scheduleID, sessionID)
}

// cancelScheduledSessionWork stops agent, workflow, background, and tmux work
// owned by a scheduled session without changing the schedule's recorded run
// result. Pulse timeout recovery uses this before continuing finalization in a
// fresh session.
func (s *SchedulerService) cancelScheduledSessionWork(sessionID, closeReason string, terminalPhase RuntimePhase) {
	if s == nil || s.api == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	s.api.cancelSessionRuntimeWork(sessionID, closeReason, terminalPhase)
}

// triggerSchedule is called by the tick loop when a schedule is due.
func (s *SchedulerService) triggerSchedule(sctx *ScheduleContext, scheduledFor time.Time) {
	triggerCtx := *sctx
	triggerCtx.ScheduledFor = scheduledFor.UTC()
	sctx = &triggerCtx
	schedID := sctx.Schedule.ID
	runtimeKey := scheduleRuntimeKey(sctx)
	now := time.Now()
	ctx := context.Background()
	s.logf(sctx, "[SCHEDULER] ⏰ Cron fired for %s (%s) at %s", schedID, sctx.Schedule.Name, now.Format(time.RFC3339))

	// Late-fire detection: compare to the next_run we recorded last time. Drift > 60s
	// usually means a missed-fire catch-up after macOS sleep/wake, or scheduler stall.
	s.runtimeStatesMu.RLock()
	if st, ok := s.runtimeStates[runtimeKey]; ok && st.NextRunAt != nil {
		expected := *st.NextRunAt
		drift := now.Sub(expected)
		if drift > 60*time.Second {
			s.logf(sctx, "[SCHEDULER] ⚠️ LATE_FIRE schedule=%s expected=%s actual=%s drift=%s",
				schedID, expected.Format(time.RFC3339), now.Format(time.RFC3339), drift.Round(time.Second))
		}
	}
	s.runtimeStatesMu.RUnlock()

	paused, cfg, err := s.IsGloballyPaused(context.Background())
	if err != nil {
		s.logf(sctx, "[SCHEDULER] ⚠️ Failed to read scheduler config before trigger %s: %v", schedID, err)
	} else if paused {
		pausedAt := ""
		if cfg != nil && cfg.PausedAt != nil {
			pausedAt = cfg.PausedAt.Format(time.RFC3339)
		}
		if pausedAt != "" {
			s.logf(sctx, "[SCHEDULER] ⏸️ Global scheduler pause active since %s, skipping %s", pausedAt, schedID)
		} else {
			s.logf(sctx, "[SCHEDULER] ⏸️ Global scheduler pause active, skipping %s", schedID)
		}
		s.recordScheduleFireDecision(ctx, sctx, "skipped_paused", "global scheduler pause is active", "", now.UTC())
		return
	}

	// Reload schedule for latest config — different paths for workflow vs multi-agent
	var freshCtx *ScheduleContext
	var workflowScheduleIDs []string
	var multiAgentScheduleIDs []string
	if sctx.SourceType == "multi-agent" {
		f, exists, err := ReadMultiAgentSchedules(context.Background(), sctx.UserID)
		if err != nil || !exists {
			s.logf(sctx, "[SCHEDULER] ❌ Failed to reload multi-agent schedules for user %s: %v", sctx.UserID, err)
			s.recordScheduleFireDecision(ctx, sctx, "failed_to_start", "failed to reload multi-agent schedule", "", now.UTC())
			return
		}
		var currentSched *WorkflowSchedule
		schedules := MergeBuiltinSchedules(f.Schedules)
		multiAgentScheduleIDs = make([]string, 0, len(schedules))
		for i := range schedules {
			multiAgentScheduleIDs = append(multiAgentScheduleIDs, schedules[i].ID)
		}
		for i := range schedules {
			if schedules[i].ID == schedID {
				currentSched = &schedules[i]
				break
			}
		}
		if currentSched == nil {
			s.logf(sctx, "[SCHEDULER] ❌ Multi-agent schedule %s not found for user %s, skipping", schedID, sctx.UserID)
			s.recordScheduleFireDecision(ctx, sctx, "failed_to_start", "schedule no longer exists", "", now.UTC())
			return
		}
		if !currentSched.Enabled {
			s.logf(sctx, "[SCHEDULER] ⏭️ Multi-agent schedule %s is disabled, skipping", schedID)
			s.recordScheduleFireDecision(ctx, sctx, "skipped_disabled", "schedule is disabled", "", now.UTC())
			return
		}
		resolvedSchedule, calendarItem, ok := scheduleWithReloadedCalendarItem(*currentSched, sctx.CalendarItem)
		if !ok {
			s.logf(sctx, "[SCHEDULER] Calendar item for %s no longer exists, skipping", schedID)
			s.recordScheduleFireDecision(ctx, sctx, "failed_to_start", "calendar item no longer exists", "", now.UTC())
			return
		}
		freshCtx = buildMultiAgentScheduleContext(sctx.UserID, resolvedSchedule, f.Capabilities)
		freshCtx.CalendarItem = calendarItem
	} else {
		// Reload manifest for latest config
		manifest, found, err := ReadWorkflowManifest(context.Background(), sctx.WorkspacePath)
		if err != nil || !found {
			s.logf(sctx, "[SCHEDULER] ❌ Failed to reload manifest for %s: %v", schedID, err)
			s.recordScheduleFireDecision(ctx, sctx, "failed_to_start", "failed to reload workflow manifest", "", now.UTC())
			return
		}

		workflowScheduleIDs = make([]string, 0, len(manifest.Schedules))
		for i := range manifest.Schedules {
			workflowScheduleIDs = append(workflowScheduleIDs, manifest.Schedules[i].ID)
		}

		// Find current schedule in manifest (may have been updated)
		var currentSched *WorkflowSchedule
		for i := range manifest.Schedules {
			if manifest.Schedules[i].ID == schedID {
				currentSched = &manifest.Schedules[i]
				break
			}
		}
		if currentSched == nil {
			s.logf(sctx, "[SCHEDULER] ❌ Schedule %s not found in manifest, skipping", schedID)
			s.recordScheduleFireDecision(ctx, sctx, "failed_to_start", "schedule no longer exists", "", now.UTC())
			return
		}
		if !currentSched.Enabled {
			if err := EnsureWorkflowScheduleExecutionTracker(context.Background(), sctx.WorkspacePath, *currentSched, time.Now().UTC()); err != nil {
				s.logf(sctx, "[SCHEDULER] Warning: failed to sync disabled execution history for %s: %v", schedID, err)
			}
			s.logf(sctx, "[SCHEDULER] ⏭️ Schedule %s is disabled, skipping", schedID)
			s.recordScheduleFireDecision(ctx, sctx, "skipped_disabled", "schedule is disabled", "", now.UTC())
			return
		}

		if activeExec := s.findActiveNonBuilderExecutionForWorkspace(sctx.WorkspacePath); activeExec != nil {
			triggeredBy := activeExec.TriggeredBy
			if strings.TrimSpace(triggeredBy) == "" {
				triggeredBy = "unknown"
			}
			s.logf(sctx, "[SCHEDULER] ⏭️ Workflow %s already has an active %s run (session: %s), skipping schedule %s",
				sctx.WorkspacePath, triggeredBy, activeExec.SessionID, schedID)
			s.recordScheduleFireDecision(ctx, sctx, "skipped_busy", "workflow already has an active execution", "", now.UTC())
			return
		}

		resolvedSchedule, calendarItem, ok := scheduleWithReloadedCalendarItem(*currentSched, sctx.CalendarItem)
		if !ok {
			s.logf(sctx, "[SCHEDULER] Calendar item for %s no longer exists, skipping", schedID)
			s.recordScheduleFireDecision(ctx, sctx, "failed_to_start", "calendar item no longer exists", "", now.UTC())
			return
		}
		freshCtx = buildScheduleContext(sctx.WorkspacePath, manifest, resolvedSchedule)
		freshCtx.CalendarItem = calendarItem
	}
	freshCtx.TriggerSource = "cron"
	freshCtx.ScheduledFor = scheduledFor.UTC()

	// Built-in pre-fire check: if the built-in registered a gating function and
	// it returns false, skip this tick entirely. No LLM session is spawned.
	if check, ok := PreFireChecks[freshCtx.Schedule.ID]; ok {
		if !check(freshCtx.UserID) {
			s.logf(freshCtx, "[SCHEDULER] ⏭️ Pre-fire check returned false for %s (user %s) — skipping", freshCtx.Schedule.ID, freshCtx.UserID)
			s.recordScheduleFireDecision(ctx, freshCtx, "skipped_prefire", "pre-fire check returned false", "", now.UTC())
			return
		}
	}

	// Reserve in memory before the durable claim so Stop can cancel even while
	// SQLite is claiming the lease. The database call itself runs without the
	// global runtime-state mutex.
	startTime := time.Now().UTC()
	runID := uuid.NewString()
	runtimeKey = scheduleRuntimeKey(freshCtx)
	s.runtimeStatesMu.Lock()
	state := s.getRuntimeStateLocked(runtimeKey)
	if state.LastStatus == "running" {
		s.runtimeStatesMu.Unlock()
		s.sessionLogf(freshCtx, state.LastSessionID, "[SCHEDULER] ⏭️ Schedule %s is already running (session: %s), skipping", schedID, state.LastSessionID)
		s.recordScheduleFireDecision(ctx, freshCtx, "skipped_busy", "schedule is already running", "", startTime)
		return
	}
	if freshCtx.SourceType == "workflow" {
		workflowRuntimeKeys := make([]string, 0, len(workflowScheduleIDs))
		for _, workflowScheduleID := range workflowScheduleIDs {
			workflowRuntimeKeys = append(workflowRuntimeKeys, workflowScheduleRuntimeKey(freshCtx.WorkspacePath, workflowScheduleID))
		}
		if otherKey, otherSession := runningScheduleInSetLocked(s.runtimeStates, workflowRuntimeKeys, scheduleRuntimeKey(freshCtx)); otherKey != "" {
			s.runtimeStatesMu.Unlock()
			s.logf(freshCtx, "[SCHEDULER] ⏭️ Workflow %s already has running schedule %s (session: %s), skipping schedule %s",
				freshCtx.WorkspacePath, otherKey, otherSession, schedID)
			s.recordScheduleFireDecision(ctx, freshCtx, "skipped_busy", "another schedule owns the workflow", "", startTime)
			return
		}
	} else if freshCtx.SourceType == "multi-agent" {
		multiAgentRuntimeKeys := make([]string, 0, len(multiAgentScheduleIDs))
		for _, multiAgentScheduleID := range multiAgentScheduleIDs {
			multiAgentRuntimeKeys = append(multiAgentRuntimeKeys, multiAgentScheduleRuntimeKey(freshCtx.UserID, multiAgentScheduleID))
		}
		if otherKey, otherSession := runningScheduleInSetLocked(s.runtimeStates, multiAgentRuntimeKeys, scheduleRuntimeKey(freshCtx)); otherKey != "" {
			s.runtimeStatesMu.Unlock()
			s.logf(freshCtx, "[SCHEDULER] ⏭️ Chief of Staff already has running schedule %s (session: %s), skipping schedule %s",
				otherKey, otherSession, schedID)
			s.recordScheduleFireDecision(ctx, freshCtx, "skipped_busy", "another Chief of Staff schedule is running", "", startTime)
			return
		}
	}
	previousState := *state
	runCtx := s.activateScheduleRunLocked(state, runID, startTime)
	s.runtimeStatesMu.Unlock()
	if err := s.claimScheduleRun(ctx, freshCtx, runID, startTime); err != nil {
		s.rollbackScheduleRunActivation(runtimeKey, runID, previousState)
		s.recordScheduleFireDecision(ctx, freshCtx, "skipped_busy", err.Error(), "", startTime)
		s.logf(freshCtx, "[SCHEDULER] ⏭️ Durable run lease rejected schedule %s: %v", schedID, err)
		return
	}
	if s.abortCanceledScheduleRunBeforeStart(runCtx, freshCtx, runtimeKey, runID) {
		return
	}
	s.recordScheduleFireDecision(ctx, freshCtx, "started", "cron fire accepted", runID, startTime)

	if freshCtx.SourceType == "workflow" {
		if err := RecordWorkflowScheduleExecution(context.Background(), freshCtx.WorkspacePath, freshCtx.Schedule, startTime); err != nil {
			s.logf(freshCtx, "[SCHEDULER] Warning: failed to record scheduled execution for %s: %v", schedID, err)
		}
	}

	s.logf(freshCtx, "[SCHEDULER] 🚀 Starting %s (%s)", schedID, freshCtx.Schedule.Name)
	if _, err := s.runJob(runCtx, freshCtx, runID); err != nil {
		s.logf(freshCtx, "[SCHEDULER] ❌ %s failed: %v", schedID, err)
	} else {
		s.logf(freshCtx, "[SCHEDULER] ✅ %s completed", schedID)
	}
}

// runJob executes a scheduled job: updates runtime state, creates run history, executes, updates results.
func (s *SchedulerService) runJob(ctx context.Context, sctx *ScheduleContext, runID string) (string, error) {
	defer s.releaseScheduleRunContext(runID)
	schedID := sctx.Schedule.ID
	runtimeKey := scheduleRuntimeKey(sctx)
	startTime := time.Now().UTC()
	s.logf(sctx, "[SCHEDULER] runJob starting for %s (%s) at %s, groups=%v",
		schedID, sctx.Schedule.Name, startTime.Format(time.RFC3339), sctx.Schedule.GroupNames)
	if err := ctx.Err(); err != nil {
		s.updateRuntimeState(runtimeKey, func(state *ScheduleRuntimeState) {
			state.ActiveRunID = ""
			state.LastStatus = "stopped"
			state.LastError = "stopped by user before execution started"
		})
		s.transitionScheduleRun(context.Background(), sctx, schedulerstate.Transition{
			RunID: runID, To: schedulerstate.StateStopped, Reason: "stopped before workflow execution started",
			ErrorMessage: "stopped by user", At: time.Now().UTC(),
		})
		return "", errors.Join(errWorkshopSequenceInterrupted, err)
	}
	if strings.TrimSpace(runID) == "" {
		return "", errors.Join(errWorkshopSequenceInterrupted, errors.New("scheduled run is missing its run id"))
	}
	s.runtimeStatesMu.RLock()
	activeState := s.runtimeStates[runtimeKey]
	ownsActiveRun := activeState != nil && activeState.LastStatus == "running" && activeState.ActiveRunID == runID
	s.runtimeStatesMu.RUnlock()
	if !ownsActiveRun {
		return "", errors.Join(errWorkshopSequenceInterrupted, fmt.Errorf("scheduled run %s no longer owns %s", runID, runtimeKey))
	}

	// Clear session/error fields — status is already "running" (set atomically by caller)
	s.updateRuntimeState(runtimeKey, func(state *ScheduleRuntimeState) {
		state.LastSessionID = ""
		state.LastError = ""
	})

	// Create run history entry (file-based)
	if strings.TrimSpace(runID) == "" {
		runID = uuid.New().String()
	}
	run := &ScheduleRunEntry{
		ID:         runID,
		ScheduleID: schedID,
		Status:     "running",
		GroupNames: sctx.Schedule.GroupNames,
		StartedAt:  startTime,
	}
	if sctx.SourceType == "multi-agent" {
		if err := AppendMultiAgentScheduleRun(ctx, sctx.UserID, run); err != nil {
			s.logf(sctx, "[SCHEDULER] Failed to create multi-agent run entry for %s: %v", schedID, err)
		}
	} else {
		if err := AppendScheduleRun(ctx, sctx.WorkspacePath, run); err != nil {
			s.logf(sctx, "[SCHEDULER] Failed to create run entry for %s: %v", schedID, err)
		}
	}
	startReason := "workflow execution starting"
	if sctx.PulseOnly {
		startReason = "Pulse version preflight starting; workflow execution is skipped"
	}
	s.transitionScheduleRun(ctx, sctx, schedulerstate.Transition{
		RunID:  runID,
		To:     schedulerstate.StateWorkflowRunning,
		Reason: startReason,
		At:     time.Now().UTC(),
	})

	// Execute
	sessionID, runFolder, execErr := s.executeJob(ctx, sctx, runID)

	// Calculate results
	durationMs := time.Since(startTime).Milliseconds()
	nextRun := getNextRunTime(sctx.Schedule.CronExpression, sctx.Schedule.Timezone)

	status := "success"
	errMsg := ""
	userInterrupted := errors.Is(execErr, errWorkshopSequenceInterrupted) || errors.Is(execErr, context.Canceled)
	if userInterrupted {
		status = "stopped"
		if execErr != nil {
			errMsg = execErr.Error()
		} else {
			errMsg = "stopped by user"
			execErr = errWorkshopSequenceInterrupted
		}
		s.sessionLogf(sctx, sessionID, "[SCHEDULER] %s stopped by user after %dms", schedID, durationMs)
	} else if execErr != nil {
		status = "error"
		errMsg = execErr.Error()
		s.sessionLogf(sctx, sessionID, "[SCHEDULER] %s failed in %dms: %v", schedID, durationMs, execErr)
	} else {
		s.sessionLogf(sctx, sessionID, "[SCHEDULER] %s completed in %dms, session: %s, folder: %s", schedID, durationMs, sessionID, runFolder)
	}
	finishedReason := "workflow execution finished with status " + status
	finishedSessionKind := "workflow"
	if sctx.PulseOnly {
		finishedReason = "Pulse version preflight finished with status " + status + "; workflow execution was skipped"
		finishedSessionKind = "pulse"
	}
	s.transitionScheduleRun(ctx, sctx, schedulerstate.Transition{
		RunID:        runID,
		To:           schedulerstate.StateWorkflowFinished,
		Reason:       finishedReason,
		SessionID:    sessionID,
		SessionKind:  finishedSessionKind,
		RunFolder:    runFolder,
		ErrorMessage: errMsg,
		At:           time.Now().UTC(),
	})

	// Keep the runtime state as "running" until all post-run side effects finish.
	// Pulse runs as several resumed builder-chat turns after the workflow result
	// is recorded; if we mark the schedule successful before Pulse finishes, a
	// frequent cron can start the next workflow run while Pulse is between steps.
	// That makes the next Pulse turn fail with workflow_busy (commonly after the
	// LLM/cost/time report), so cadence/backup/publish/notify never run.
	s.updateRuntimeState(runtimeKey, func(state *ScheduleRuntimeState) {
		state.LastSessionID = sessionID
	})

	// Update run history entry for the actual workflow/task run. Post-run Pulse
	// may continue after this, but it does not change the recorded run result.
	pulseResult := postRunMonitorNotRun
	if sctx.SourceType == "multi-agent" {
		if err := UpdateMultiAgentScheduleRun(ctx, sctx.UserID, runID, status, errMsg, &durationMs, sessionID); err != nil {
			s.sessionLogf(sctx, sessionID, "[SCHEDULER] Failed to update multi-agent run entry for %s: %v", schedID, err)
		}
		if !userInterrupted && shouldUpdateChiefTaskReport(sctx) {
			if err := s.runChiefTaskReportUpdate(ctx, sctx, runID, status, errMsg, durationMs, startTime, time.Now().UTC(), sessionID); err != nil {
				s.sessionLogf(sctx, sessionID, "[TASK_REPORT] Failed to update pulse/task.html for %s: %v", schedID, err)
			}
		}
	} else {
		if err := UpdateScheduleRun(ctx, sctx.WorkspacePath, runID, status, errMsg, &durationMs, runFolder, sessionID); err != nil {
			s.sessionLogf(sctx, sessionID, "[SCHEDULER] Failed to update run entry for %s: %v", schedID, err)
		}

		// Pulse: the post-run steward. When enabled it runs a Gate turn that reads
		// run evidence and records a module worklist in db/db.sqlite, then executes
		// only the selected agents (consolidated Workflow Review, independent
		// Strategy Auditor, and independent Goal Advisor), then one Fixer, backs
		// up the final state, publishes, and sends
		// a run summary notification — see runPostRunMonitor.
		// Opt-in per workflow (post_run_monitor in workflow.json) — runs only when
		// the user / builder enabled Pulse. Only after an actual workflow RUN, not an
		// optimizer/improvement pass (there's no fresh run output to scan there).
		// Never affects the run's recorded result.
		if !userInterrupted && runFolder != "" && sctx.Schedule.WorkshopMode != "optimizer" {
			if manifest, found, mErr := ReadWorkflowManifest(ctx, sctx.WorkspacePath); mErr == nil && found && shouldRunPostRunMonitor(sctx, manifest) {
				pulseEvidenceStatus := status
				if sctx.PulseOnly && strings.TrimSpace(sctx.PulseEvidenceRunStatus) != "" {
					pulseEvidenceStatus = sctx.PulseEvidenceRunStatus
				}
				// Pass the run's sessionID so Pulse resumes the SAME chat (not a fresh one).
				s.transitionScheduleRun(ctx, sctx, schedulerstate.Transition{
					RunID: runID, To: schedulerstate.StatePulseGate, Reason: "Pulse enabled for workflow", SessionID: sessionID, SessionKind: "pulse", At: time.Now().UTC(),
				})
				pulseResult = s.runPostRunMonitor(ctx, sctx, manifest, pulseEvidenceStatus, runFolder, sessionID, runID, errMsg)
			}
		}
	}

	// Now the whole scheduled job, including post-run side effects, is done.
	terminalState := schedulerstate.StateCompleted
	if userInterrupted {
		terminalState = schedulerstate.StateStopped
	} else if status == "error" {
		terminalState = schedulerstate.StateFailed
	} else if pulseResult == postRunMonitorPartial {
		terminalState = schedulerstate.StatePartial
	} else if pulseResult == postRunMonitorStopped {
		terminalState = schedulerstate.StateStopped
	}
	overallStatus := status
	overallError := errMsg
	if terminalState == schedulerstate.StateStopped {
		overallStatus = "stopped"
		if overallError == "" {
			overallError = "stopped by user"
		}
	} else if terminalState == schedulerstate.StatePartial {
		overallStatus = "partial"
		if overallError == "" {
			overallError = "Pulse completed partially"
		}
	}
	if sctx.PulseOnly {
		durationMs = time.Since(startTime).Milliseconds()
		if err := UpdateScheduleRun(ctx, sctx.WorkspacePath, runID, overallStatus, overallError, &durationMs, runFolder, sessionID); err != nil {
			s.sessionLogf(sctx, sessionID, "[PULSE] failed to finalize one-off Pulse run history: %v", err)
		}
	}
	s.updateRuntimeState(runtimeKey, func(state *ScheduleRuntimeState) {
		state.ActiveRunID = ""
		state.LastStatus = overallStatus
		state.LastError = overallError
		state.LastDurationMs = &durationMs
		state.NextRunAt = nextRun
		state.RunCount++
		if overallStatus == "error" {
			state.ConsecutiveFailures++
		} else {
			state.ConsecutiveFailures = 0
		}
	})
	terminalReason := "scheduled run finished with status " + status
	if sctx.PulseOnly {
		terminalReason = "one-off Pulse finished with status " + overallStatus + "; workflow execution was skipped"
	}
	s.transitionScheduleRun(ctx, sctx, schedulerstate.Transition{
		RunID: runID, To: terminalState, Reason: terminalReason,
		SessionID: sessionID, ErrorMessage: overallError, At: time.Now().UTC(),
	})
	s.cleanupRemovedScheduleRuntimeState(runtimeKey)

	return sessionID, execErr
}

func shouldUpdateChiefTaskReport(sctx *ScheduleContext) bool {
	if sctx == nil || sctx.SourceType != "multi-agent" {
		return false
	}
	if IsDefaultBuiltinSchedule(sctx.Schedule.ID) || IsOrgPulseSchedule(sctx.Schedule) {
		return false
	}
	hay := strings.ToLower(strings.Join([]string{
		sctx.Schedule.ID,
		sctx.Schedule.Name,
		sctx.Schedule.Description,
		sctx.Schedule.Query,
		strings.Join(sctx.Schedule.Messages, "\n"),
	}, "\n"))
	return !strings.Contains(hay, "enrich_memory") &&
		!strings.Contains(hay, "memory enrichment") &&
		!strings.Contains(hay, "auto-enrich memory") &&
		sctx.Schedule.ID != deprecatedAutoEnrichMemoryID
}

func (s *SchedulerService) runChiefTaskReportUpdate(ctx context.Context, sctx *ScheduleContext, runID, status, errMsg string, durationMs int64, startedAt, completedAt time.Time, sessionID string) error {
	if s == nil || s.api == nil {
		return fmt.Errorf("scheduler API is not configured")
	}
	if sessionID == "" {
		return fmt.Errorf("missing session id")
	}

	reqMap := map[string]interface{}{
		"agent_mode":                  "simple",
		"triggered_by":                "cron",
		"query":                       buildChiefTaskReportUpdateMessage(sctx, runID, status, errMsg, durationMs, startedAt, completedAt, sessionID),
		"task_report_update_turn":     true,
		"disable_live_input_delivery": true,
	}
	if len(sctx.Capabilities.SelectedServers) > 0 {
		reqMap["servers"] = sctx.Capabilities.SelectedServers
	}
	if len(sctx.Capabilities.SelectedSkills) > 0 {
		reqMap["selected_skills"] = sctx.Capabilities.SelectedSkills
	}
	if sctx.Capabilities.BrowserMode != "" && sctx.Capabilities.BrowserMode != "none" {
		reqMap["browser_mode"] = sctx.Capabilities.BrowserMode
	}
	if len(sctx.Capabilities.CDPPorts) > 0 {
		reqMap["cdp_ports"] = append([]int(nil), sctx.Capabilities.CDPPorts...)
	}
	if sctx.Capabilities.UseCodeExecutionMode {
		reqMap["use_code_execution_mode"] = true
	}
	s.applyChiefOfStaffLLMToReqMap(ctx, reqMap, sctx, sessionID)

	s.sessionLogf(sctx, sessionID, "[TASK_REPORT] updating pulse/task.html for schedule %s run %s", sctx.Schedule.ID, runID)
	if err := s.api.startSessionInternal(ctx, reqMap, sessionID, sctx.UserID, nil); err != nil {
		return fmt.Errorf("task report update turn failed: %w", err)
	}
	if err := s.waitForWorkshopIdle(ctx, sessionID); err != nil {
		return fmt.Errorf("task report update idle wait failed: %w", err)
	}
	return nil
}

func buildChiefTaskReportUpdateMessage(sctx *ScheduleContext, runID, status, errMsg string, durationMs int64, startedAt, completedAt time.Time, sessionID string) string {
	if sctx == nil {
		sctx = &ScheduleContext{}
	}
	taskText := strings.TrimSpace(sctx.Schedule.Query)
	if taskText == "" && len(sctx.Schedule.Messages) > 0 {
		taskText = strings.TrimSpace(strings.Join(sctx.Schedule.Messages, "\n\n"))
	}
	if taskText == "" {
		taskText = "(no query recorded)"
	}
	errLine := ""
	if strings.TrimSpace(errMsg) != "" {
		errLine = "\n- error: " + strings.TrimSpace(errMsg)
	}
	return fmt.Sprintf(`TASK REPORT UPDATE - normal Chief of Staff schedule completed.

Call read_skill(skills=[{"name":"builder-reference","path":"references/chief-task-report.md"}]) and follow it exactly.

Update the single shared Tasks page at pulse/task.html. This is separate from Org Pulse.
Do not create per-task files. Do not edit pulse/org-pulse.html, pulse/goals.html, workflow files, schedules, memory tools/files, or secrets.
Do not redo the task; summarize the just-completed scheduled task run from this current conversation.
Do not call notify_user from this report-update turn unless the original task explicitly required a notification.

Run metadata:
- schedule_id: %s
- schedule_name: %s
- schedule_description: %s
- run_id: %s
- session_id: %s
- status: %s%s
- started_at: %s
- completed_at: %s
- duration_ms: %d
- cron_expression: %s
- timezone: %s

Original scheduled task:
%s

		What to write:
		- Create pulse/task.html if missing using the chief-task-report skeleton.
		- Prepend one .task-entry after <!-- CHIEF TASK ENTRIES: newest first -->.
		- Update the top summary tiles/counts and latest update timestamp.
		- Capture the plain-language outcome, why it matters, key findings to reuse on the next run, affected workflows/entities, and next action.
		- Treat the metadata above as internal input. Keep schedule/run ids in data attributes and put session id, exact evidence paths, and raw errors only inside collapsed Agent details.
		- If the task failed, explain the failure and suggested next action in ordinary language; do not expose the raw error outside Agent details.
		- Keep the page concise; this is a durable task ledger, not a transcript dump.
	`, sctx.Schedule.ID, sctx.Schedule.Name, sctx.Schedule.Description, runID, sessionID, status, errLine, startedAt.Format(time.RFC3339), completedAt.Format(time.RFC3339), durationMs, sctx.Schedule.CronExpression, sctx.Schedule.Timezone, taskText)
}

func withChiefTaskRunContext(sctx *ScheduleContext, query string) string {
	if !shouldUpdateChiefTaskReport(sctx) {
		return query
	}
	return fmt.Sprintf(`NORMAL CHIEF OF STAFF TASK RUN.

Before doing the task, read pulse/task.html if it exists. Use only prior .task-entry items with data-schedule-id=%q as reusable context for this same scheduled task: key findings, open next actions, prior decisions, recurring entities/workflows, and evidence paths. Treat that page as the task's durable context. Do not use or update Chief of Staff memory tools/files.

After the task finishes, stop normally. The scheduler will send a separate report-update turn to write this run's summary and key findings back into pulse/task.html.

If progress requires a non-blocking user decision, clarification, or approval, do not guess and do not wait in real time. Call create_human_input_request(source="chief_of_staff", workspace_path="pulse" for an org-wide question or the affected Workflow/<name> path for a workflow-specific question). Continue any independent work that remains safe. A future Chief of Staff or workflow Pulse run will receive the saved answer and must record what it did with mark_human_input_consumed.

Scheduled task:
%s`, sctx.Schedule.ID, query)
}

// runPostRunMonitor continues the scheduled run's main-agent conversation with
// three conceptual turns: Gate, optional Review+Fix, and Finalize.
// The agent owns reviewer selection, specialist delegation, diagnosis, repair,
// and verification. Go supplies tools and permissions, preserves turn ordering,
// and validates the typed durable receipts between turns. Pulse never changes
// the workflow run's recorded status; post-run failures are logged separately.
type postRunMonitorResult string

const (
	postRunMonitorNotRun    postRunMonitorResult = "not_run"
	postRunMonitorCompleted postRunMonitorResult = "completed"
	postRunMonitorPartial   postRunMonitorResult = "partial"
	postRunMonitorStopped   postRunMonitorResult = "stopped"
)

func (s *SchedulerService) runPostRunMonitor(ctx context.Context, sctx *ScheduleContext, manifest *WorkflowManifest, runStatus, runFolder, runSessionID, scheduleRunID, runFailureReason string) (pulseResult postRunMonitorResult) {
	pulseResult = postRunMonitorPartial

	// Resume the SAME session the workflow run just used, so Pulse continues in the
	// run's chat thread — the user sees the run and its post-run steward as one
	// conversation, not a fresh session spun up out of nowhere. Fall back to a new id
	// only if the run somehow didn't record one.
	sessionID := strings.TrimSpace(runSessionID)
	if sessionID == "" {
		sessionID = s.newScheduleSessionID(sctx)
	}
	pulseRunID := sessionID
	defer func() {
		if r := recover(); r != nil {
			s.logf(sctx, "[PULSE] post-run pulse panic (recovered): %v", r)
			if err := finalizeUnresolvedPulseFinalCommands(ctx, sctx.WorkspacePath, pulseRunID, "failed", "Pulse stopped because the server recovered a panic"); err != nil {
				s.logf(sctx, "[PULSE] failed to reconcile final commands after panic: %v", err)
			}
		}
	}()
	baseReqMap := s.buildWorkshopRequest(ctx, sctx)
	if err := initializePulseFinalCommandStates(ctx, sctx.WorkspacePath, pulseRunID); err != nil {
		s.sessionLogf(sctx, sessionID, "[PULSE] failed to initialize final command state: %v", err)
		return
	}

	// Pulse is one continuing agent conversation. Go sends three ordered turns:
	// Gate, Review+Fix, and Finalize. The agent owns review selection,
	// specialist delegation, repair bundling, and verification; Go only preserves
	// ordering and validates the durable receipts between turns.
	pulseContext := "A scheduled run of this workflow just finished"
	if sctx.PulseOnly {
		pulseContext = "This is a manual Pulse-only review of the latest retained workflow evidence. The workflow was not executed by this action"
	}
	intro := postRunMonitorIntro(pulseContext, sctx.WorkspacePath, pulseRunID, runStatus, runFolder)

	upgradeSteps := postRunMonitorUpgradeStepsForManifest(manifest)
	s.sessionLogf(sctx, sessionID, "[PULSE] starting pulse for %s (run_folder=%s status=%s, upgrades=%d)", sctx.Schedule.ID, runFolder, runStatus, len(upgradeSteps))
	reviewFixPreflight := make([]string, 0, len(upgradeSteps)+1)
	for _, upgrade := range upgradeSteps {
		reviewFixPreflight = append(reviewFixPreflight, upgrade.query)
	}
	introSent := false
	recoveryNotes := []string{}
	runStep := func(st postRunMonitorStep) postRunMonitorStepRunResult {
		reqMap := cloneStringInterfaceMap(baseReqMap)
		s.applyPulseLLMToReqMap(reqMap, sctx, sessionID)
		query := st.query
		includesIntro := false
		if !introSent {
			priorFailureContext := ""
			if len(recoveryNotes) > 0 {
				priorFailureContext = "\n\nPRIOR PULSE FAILURE CONTEXT. Earlier work in this same conversation did not finish cleanly. Preserve partial/failed status honestly and do not claim skipped work succeeded:\n- " + strings.Join(recoveryNotes, "\n- ")
			}
			query = intro + priorFailureContext + "\n\n" + query
			includesIntro = true
		}
		reqMap["query"] = query
		if err := s.api.startSessionInternal(ctx, reqMap, sessionID, "", nil); err != nil {
			s.sessionLogf(sctx, sessionID, "[PULSE] step %q failed to start: %v", st.label, err)
			if st.label == "finalize" {
				_ = finalizeUnresolvedPulseFinalCommands(ctx, sctx.WorkspacePath, pulseRunID, "failed", "Pulse finalizer failed to start")
			}
			return postRunMonitorStepRunResult{outcome: postRunMonitorStepStartFailed, err: err}
		}
		if includesIntro {
			introSent = true
		}
		if err := s.waitForPulseTurnCompletion(ctx, sessionID, st.idleMaxInactivity()); err != nil {
			s.sessionLogf(sctx, sessionID, "[PULSE] step %q idle wait failed: %v", st.label, err)
			outcome := postRunMonitorStepWaitFailed
			if errors.Is(err, errWorkshopSequenceInterrupted) || errors.Is(err, context.Canceled) {
				outcome = postRunMonitorStepInterrupted
			} else if errors.Is(err, errWorkshopIdleWaitTimeout) {
				outcome = postRunMonitorStepTimedOut
			}
			status := "failed"
			if outcome == postRunMonitorStepTimedOut {
				status = "timed_out"
			}
			if st.label == "finalize" {
				_ = finalizeUnresolvedPulseFinalCommands(ctx, sctx.WorkspacePath, pulseRunID, status, "Pulse finalizer did not finish cleanly")
			}
			return postRunMonitorStepRunResult{outcome: outcome, err: err}
		}
		if st.label == "finalize" {
			if err := finalizeUnresolvedPulseFinalCommands(ctx, sctx.WorkspacePath, pulseRunID, "failed", "Finalizer ended without recording this command's outcome"); err != nil {
				s.sessionLogf(sctx, sessionID, "[PULSE] failed to reconcile final command state: %v", err)
			}
		}
		s.sessionLogf(sctx, sessionID, "[PULSE] step %q done for %s", st.label, sctx.Schedule.ID)
		return postRunMonitorStepRunResult{outcome: postRunMonitorStepCompleted}
	}
	handleStepFailure := func(st postRunMonitorStep, result postRunMonitorStepRunResult, needsFollowup bool) string {
		failureLabel := "failed"
		if result.outcome == postRunMonitorStepTimedOut {
			failureLabel = fmt.Sprintf("made no observable progress for %s", st.idleMaxInactivity())
		}
		reason := fmt.Sprintf("Pulse step %s %s", st.label, failureLabel)
		if result.err != nil && result.outcome != postRunMonitorStepTimedOut {
			reason += ": " + result.err.Error()
		}
		recoveryNotes = append(recoveryNotes, reason)
		if needsFollowup {
			s.sessionLogf(sctx, sessionID, "[PULSE] continuing in the same Pulse conversation after %s", reason)
		}
		return reason
	}
	abortIfInterrupted := func(st postRunMonitorStep, result postRunMonitorStepRunResult) bool {
		if result.outcome != postRunMonitorStepInterrupted {
			return false
		}
		reason := fmt.Sprintf("Pulse stopped by user during %s", st.label)
		pulseResult = postRunMonitorStopped
		s.sessionLogf(sctx, sessionID, "[PULSE] %s; no later Review+Fix, Finalize, publish, or notification turn will run", reason)
		_ = finalizeUnresolvedPulseFinalCommands(ctx, sctx.WorkspacePath, pulseRunID, "skipped", reason)
		return true
	}
	abortIfTurnStillBusy := func(st postRunMonitorStep, result postRunMonitorStepRunResult) bool {
		busy := s.api.sessionIsBusy(sessionID)
		if !pulseStepFailureMustStopBeforeNextTurn(result, busy) {
			return false
		}
		// PLAT-065: this abort can fire even when the step's own durable state
		// (e.g. Gate's worklist) already committed successfully — the recovery
		// logic that would notice that only runs for the Gate step, after this
		// check, so it never gets the chance. The original incident's log
		// window rotated out before this could be captured; logging outcome/err
		// and the completion-recovery result here (best-effort, may not apply
		// to every step) is what the next occurrence needs to confirm or rule
		// out the reorder fix described in PLAT-065.
		completionRecovered := false
		if completionErr := validatePulseGateCompletion(ctx, sctx.WorkspacePath, pulseRunID); completionErr == nil {
			completionRecovered = true
		}
		s.sessionLogf(sctx, sessionID, "[PULSE] abortIfTurnStillBusy diagnostic: step=%s outcome=%v err=%v sessionBusy=%v durableWorklistComplete=%v",
			st.label, result.outcome, result.err, busy, completionRecovered)
		reason := fmt.Sprintf("Pulse stopped after %s failed while its agent turn was still live; refusing to overlap another message in the same conversation", st.label)
		s.sessionLogf(sctx, sessionID, "[PULSE] %s", reason)
		_ = finalizeUnresolvedPulseFinalCommands(ctx, sctx.WorkspacePath, pulseRunID, "failed", reason)
		pulseResult = postRunMonitorPartial
		return true
	}

	// A scheduled Pulse reviews evidence produced by this invocation. A run that
	// failed still counts; a preflight abort against a pre-existing, untouched
	// run folder does not. Manual PulseOnly is the intentional exception because
	// it explicitly reviews retained evidence without executing the workflow.
	reviewEvidenceAvailable := sctx.ProducedRunEvidence || sctx.PulseOnly
	if !reviewEvidenceAvailable {
		s.sessionLogf(sctx, sessionID, "[PULSE] workflow did not start in this invocation; skipping Gate, reviewers, Fixer, dashboard and publish")
	}

	var steps []postRunMonitorStep
	if !reviewEvidenceAvailable {
		steps = postRunMonitorNoRunSteps(pulseRunID, runFailureReason, notificationInstructionsFromCapabilities(sctx.Capabilities))
	} else {
		gateStep := postRunMonitorGateStep(pulseRunID, runFolder, runStatus)
		gateCompleted := false
		for attempt := 1; attempt <= 2; attempt++ {
			result := runStep(gateStep)
			if abortIfInterrupted(gateStep, result) {
				return
			}
			if abortIfTurnStillBusy(gateStep, result) {
				return
			}
			if result.outcome == postRunMonitorStepCompleted {
				if err := validatePulseGateCompletion(ctx, sctx.WorkspacePath, pulseRunID); err == nil {
					gateCompleted = true
					break
				} else {
					s.sessionLogf(sctx, sessionID, "[PULSE] Gate completion contract failed (attempt %d/2): %v", attempt, err)
					result = postRunMonitorStepRunResult{outcome: postRunMonitorStepWaitFailed, err: err}
				}
			} else if err := validatePulseGateCompletion(ctx, sctx.WorkspacePath, pulseRunID); err == nil {
				// The agent may time out after it has already committed the
				// complete durable worklist. Preserve the failure truth, but do
				// not discard its due-module routing.
				handleStepFailure(gateStep, result, true)
				gateCompleted = true
				s.sessionLogf(sctx, sessionID, "[PULSE] Gate stage failed after recording a complete durable worklist; continuing with selected modules")
				break
			}
			if attempt == 1 {
				handleStepFailure(gateStep, result, true)
				continue
			}
			handleStepFailure(gateStep, result, true)
		}
		if !gateCompleted {
			if err := validatePulseGateCompletion(ctx, sctx.WorkspacePath, pulseRunID); err == nil {
				gateCompleted = true
				s.sessionLogf(sctx, sessionID, "[PULSE] recovered complete durable Gate worklist after the stage failure")
			}
		}
		if gateCompleted {
			if due, err := pulseWorklistHasDueModule(ctx, sctx.WorkspacePath, pulseRunID); err != nil {
				s.sessionLogf(sctx, sessionID, "[PULSE] could not inspect due-module receipt after Gate; preserving Review+Fix turn: %v", err)
				steps = append(steps, postRunMonitorAgenticReviewFixStep(pulseRunID, reviewFixPreflight))
			} else if due {
				steps = append(steps, postRunMonitorAgenticReviewFixStep(pulseRunID, reviewFixPreflight))
			} else {
				s.sessionLogf(sctx, sessionID, "[PULSE] Gate skipped every review perspective; omitting Review+Fix turn")
			}
			steps = append(steps, postRunMonitorFinalSteps(pulseRunID, notificationInstructionsFromCapabilities(sctx.Capabilities))...)
			if len(steps) > 0 && !isPostRunMonitorFinalStep(steps[0].label) {
				s.transitionScheduleRun(ctx, sctx, schedulerstate.Transition{
					RunID: scheduleRunID, To: schedulerstate.StatePulseModules, Reason: "Pulse Gate recorded due modules",
					SessionID: sessionID, SessionKind: "pulse", At: time.Now().UTC(),
				})
			}
			s.sessionLogf(sctx, sessionID, "[PULSE] selected %d post-gate steps for %s", len(steps), sctx.Schedule.ID)
		} else {
			steps = postRunMonitorFinalSteps(pulseRunID, notificationInstructionsFromCapabilities(sctx.Capabilities))
		}
	}

	for i, st := range steps {
		result := runStep(st)
		if abortIfInterrupted(st, result) {
			return
		}
		if abortIfTurnStillBusy(st, result) {
			return
		}
		if result.outcome == postRunMonitorStepCompleted && st.label == "review-fix" {
			if err := validatePulseDueModuleResults(ctx, sctx.WorkspacePath, pulseRunID); err != nil {
				s.sessionLogf(sctx, sessionID, "[PULSE] Review+Fix receipt incomplete; asking the same conversation to finish it: %v", err)
				continuation := postRunMonitorReviewFixContinuationStep(pulseRunID, err)
				result = runStep(continuation)
				if abortIfInterrupted(continuation, result) {
					return
				}
				if abortIfTurnStillBusy(continuation, result) {
					return
				}
				if result.outcome == postRunMonitorStepCompleted {
					if receiptErr := validatePulseDueModuleResults(ctx, sctx.WorkspacePath, pulseRunID); receiptErr != nil {
						result = postRunMonitorStepRunResult{outcome: postRunMonitorStepWaitFailed, err: receiptErr}
					}
				}
			}
		}
		if result.outcome != postRunMonitorStepCompleted {
			handleStepFailure(st, result, i < len(steps)-1)
		}
	}
	if len(recoveryNotes) > 0 {
		pulseResult = postRunMonitorPartial
		s.sessionLogf(sctx, sessionID, "[PULSE] pulse finalized partially for %s after %d failed/timed-out step(s)", sctx.Schedule.ID, len(recoveryNotes))
	} else {
		pulseResult = postRunMonitorCompleted
		s.sessionLogf(sctx, sessionID, "[PULSE] pulse completed for %s", sctx.Schedule.ID)
	}
	// Pulse owns its own notification from the durable SQLite state. The popup is
	// the only Pulse presentation; there is no parallel HTML journal to update.
	return pulseResult
}

type postRunMonitorStep struct{ label, query string }

type postRunMonitorStepOutcome string

const (
	postRunMonitorStepCompleted   postRunMonitorStepOutcome = "completed"
	postRunMonitorStepStartFailed postRunMonitorStepOutcome = "start_failed"
	postRunMonitorStepWaitFailed  postRunMonitorStepOutcome = "wait_failed"
	postRunMonitorStepTimedOut    postRunMonitorStepOutcome = "timed_out"
	postRunMonitorStepInterrupted postRunMonitorStepOutcome = "interrupted"
)

type postRunMonitorStepRunResult struct {
	outcome postRunMonitorStepOutcome
	err     error
}

func pulseStepFailureMustStopBeforeNextTurn(result postRunMonitorStepRunResult, sessionBusy bool) bool {
	return result.outcome != postRunMonitorStepCompleted && sessionBusy
}

func postRunMonitorIntro(contextSummary, workspacePath, pulseRunID, runStatus, runFolder string) string {
	return fmt.Sprintf("PULSE RUN CONTEXT. %s. workspace_path=%q, pulse_run_id=%q, evidence_status=%q, run_folder=%q. This is one continuing Pulse conversation. The scheduler sends Gate, Review+Fix, and Finalize turns in order. Own the reasoning and any useful specialist delegation inside the current turn, use durable workflow state for human answers, keep user-facing output concise, persist the required receipt, then stop so the next turn can continue.",
		contextSummary, workspacePath, pulseRunID, runStatus, runFolder)
}

// isPostRunMonitorFinalStep marks the finalizer, which must still run when
// earlier maintenance failed so notification/final command state stay truthful.
func isPostRunMonitorFinalStep(label string) bool {
	return label == "finalize"
}

func (st postRunMonitorStep) idleMaxInactivity() time.Duration {
	return schedulerWorkshopMaxInactivity
}

func workflowHasPendingPlanChangelogArtifactReview(ctx context.Context, workspacePath string) (bool, error) {
	workspacePath = strings.Trim(strings.TrimSpace(workspacePath), "/")
	if workspacePath == "" {
		return false, nil
	}

	folder := workspacePath + "/planning/changelog"
	listing, exists, err := listWorkspaceFolder(ctx, folder, 1)
	if err != nil {
		return true, err
	}
	if !exists {
		return false, nil
	}

	var filePaths []string
	collectWorkspaceFilePaths(listing, &filePaths)
	for _, filePath := range filePaths {
		base := filepath.Base(filePath)
		if !strings.HasPrefix(base, "changelog-") || !strings.HasSuffix(strings.ToLower(base), ".json") {
			continue
		}

		content, exists, err := readFileFromWorkspace(ctx, filePath)
		if err != nil {
			return true, err
		}
		if !exists || strings.TrimSpace(content) == "" {
			continue
		}

		var changelog planChangelogFile
		if err := json.Unmarshal([]byte(content), &changelog); err != nil {
			// A malformed changelog still needs human/agent attention; keep the
			// Pulse Artifact Review turn rather than treating it as clean.
			return true, nil
		}
		for _, entry := range changelog.Entries {
			if entry.ArtifactReview == nil || !entry.ArtifactReview.Done {
				return true, nil
			}
		}
	}

	return false, nil
}

func postRunMonitorSteps() []postRunMonitorStep {
	steps := []postRunMonitorStep{
		postRunMonitorGateStep("<pulse_run_id>", "<run_folder>", "<run_status>"),
		postRunMonitorAgenticReviewFixStep("<pulse_run_id>", nil),
	}
	steps = append(steps, postRunMonitorFinalSteps("<pulse_run_id>")...)
	return steps
}

func postRunMonitorGateStep(pulseRunID, runFolder, runStatus string) postRunMonitorStep {
	return postRunMonitorStep{
		label: "gate",
		query: fmt.Sprintf("PULSE GATE / WORKLIST. pulse_run_id=%q, run_folder=%q, run_status=%q. Load read_skill(skills=[{\"name\":\"builder-reference\",\"path\":\"references/pulse-gate.md\"}]) and follow it exactly. Perform only the progressive Gate scan. Choose the pass mode yourself from backlog and new-run evidence, then call record_pulse_worklist exactly once with mode, mode_reason, and all %d module decisions. Then record trustworthy current-run success-criterion observations with record_pulse_impact when available, and stop. Do not fabricate measurements, create interventions/assessments, write workflow artifacts, launch reviewers, fix, back up, publish, or notify.",
			pulseRunID, runFolder, runStatus, len(pulseModuleOrder)),
	}
}

func postRunMonitorAgenticReviewFixStep(pulseRunID string, preflight []string) postRunMonitorStep {
	preflightContext := ""
	if len(preflight) > 0 {
		preflightContext = "\n\nMAINTENANCE ALREADY IDENTIFIED FOR THIS TURN. Handle these items inside this same Review+Fix turn; they are not separate scheduler stages:\n- " + strings.Join(preflight, "\n- ")
	}
	return postRunMonitorStep{
		label: "review-fix",
		query: fmt.Sprintf(`PULSE REVIEW + FIX DISPATCH. pulse_run_id=%q. Load read_skill(skills=[{"name":"builder-reference","path":"references/pulse-review-fixer.md"}]) and follow it as the operating contract. Read the durable Gate worklist via get_pulse_state(view="module", pulse_run_id=<this id>) to obtain its persisted mode; handle only modules marked due and only in that mode.

		This is a dispatch turn. In backlog_drain, launch only one Engineering/Ops executor sequence when its module is due: it starts with verification of due prior fixes, then repairs retained active issues; do not launch broad discovery or Strategy/Goal children. In discovery, use ordinary run_in_background agents only: launch one executor message sequence for selected Engineering and/or Operations work. Its fixed order is Engineering Review → Stores Health when the workflow_review Gate evidence selects learnings/knowledgebase/DB integrity → Operations Review when llm_ops_review is due → consolidate, repair, and verify safe technical changes. Stores Health is an internal Engineering turn, never a separate Pulse module or receipt. In strategy, launch Strategy Auditor and Goal Advisor as separate executor agents only when their own modules are due. In observe, launch no child. For run_in_background, put the first review in instruction and give every later message_sequence item a non-empty message; IDs are optional diagnostic labels generated by the backend. Give every child the pulse run ID, selected mode, selected module/lens, relevant Gate evidence, full normal builder authority for its task, and a compact evidence/result contract. Do not use a deleted generic-review tool, a residual Fixer, polling, or a Go-selected reviewer. Children return their evidence and completed work through normal automatic notifications.

		After dispatching every needed child, end this turn immediately. The runtime tracks registered children and waits for them before it advances Pulse. A later continuation turn will reconcile their evidence, persist the typed Pulse lifecycle rows, and record exactly one terminal record_pulse_result receipt per due module. If no modules are due, record the required skipped/terminal receipts yourself and stop. Do not render the dashboard, back up, publish, or notify in this turn.%s`, pulseRunID, preflightContext),
	}
}

func postRunMonitorReviewFixContinuationStep(pulseRunID string, receiptErr error) postRunMonitorStep {
	return postRunMonitorStep{
		label: "review-fix-continuation",
		query: fmt.Sprintf(`PULSE REVIEW + FIX CONSOLIDATION. pulse_run_id=%q. Continue in this same conversation after the normal background children have completed. This is the one parent reconciliation turn, not a second fixer: do not restart completed reviews, automatically create a duplicate recovery/Fixer agent, re-run a child, or mutate workflow artifacts. A separately scoped repair agent remains available when a later parent turn deliberately chooses one; this reconciliation turn merely records the truth about the children that already ran. The prior stage is missing receipts: %s. Load the Gate worklist, current Pulse state, and child completion evidence. Reconcile semantic duplicates; persist truthful findings, fixes, and verification through the typed Pulse tools; and call record_pulse_result exactly once for every due module still missing a terminal current-run result. If a child failed, record that truthfully for its module while preserving the other modules' results. Keep the response compact, then stop.`, pulseRunID, receiptErr),
	}
}

type workflowNotificationContentInstructions struct {
	runSummary             string
	pulseSummary           string
	runSummaryChannels     []string
	pulseSummaryChannels   []string
	runSummaryRecipients   []string
	pulseSummaryRecipients []string
}

func notificationInstructionsFromCapabilities(capabilities WorkflowCapabilities) workflowNotificationContentInstructions {
	if capabilities.Notifications == nil {
		return workflowNotificationContentInstructions{}
	}
	notifications := capabilities.Notifications
	return workflowNotificationContentInstructions{
		runSummary:             notifications.EffectiveRunSummaryInstructions(),
		pulseSummary:           notifications.EffectivePulseSummaryInstructions(),
		runSummaryChannels:     append([]string(nil), notifications.RunSummaryChannels...),
		pulseSummaryChannels:   append([]string(nil), notifications.PulseSummaryChannels...),
		runSummaryRecipients:   append([]string(nil), notifications.RunSummaryRecipients...),
		pulseSummaryRecipients: append([]string(nil), notifications.PulseSummaryRecipients...),
	}
}

// postRunMonitorNoRunSteps is the truthful finalizer for an invocation that
// never produced new run evidence — the workshop session ran but the workflow
// itself never started or restarted a run folder (e.g. a preflight abort
// against a pre-existing, untouched iteration-0). There is no run to review or
// render, so it skips Gate, Review+Fix, and dashboard, preserves any preflight
// edits through the normal backup contract, and tells the user why no result
// exists.
func postRunMonitorNoRunSteps(pulseRunID, reason string, instructions ...workflowNotificationContentInstructions) []postRunMonitorStep {
	ownerInstructions := workflowNotificationContentInstructions{}
	if len(instructions) > 0 {
		ownerInstructions = instructions[0]
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "the scheduler recorded no error, but no workflow run was started or resumed during this invocation"
	}
	routing := ""
	if len(ownerInstructions.runSummaryChannels) > 0 {
		routing += fmt.Sprintf(" Configured run-summary channels: %s; the backend enforces them from notification_kind.", notificationChannelSummary(ownerInstructions.runSummaryChannels))
	}
	if len(ownerInstructions.runSummaryRecipients) > 0 {
		routing += fmt.Sprintf(" The backend addresses email from the workflow's saved run-summary recipients (%s); do not pass email_to.", notificationRecipientSummary(ownerInstructions.runSummaryRecipients))
	}
	content := ""
	if runInstructions := strings.TrimSpace(ownerInstructions.runSummary); runInstructions != "" {
		content = "\n\nApply these saved run-summary content instructions without changing the facts:\n" + runInstructions
	}
	return []postRunMonitorStep{{"finalize", fmt.Sprintf(
		"PULSE FINALIZER — WORKFLOW DID NOT RUN. pulse_run_id=%q. The scheduled workflow never started in this invocation, so there is no new run evidence. Gate, reviewers, Fixer, dashboard, and publish were intentionally skipped. Do not run them, do not read old evidence as this run, do not write builder/improve.html, and do not invent an outcome.\n\n"+
			"Do these actions in order and record every command with record_pulse_result(command=..., result=..., reason=...): (1) mark dashboard skipped because no run exists; (2) run the configured source-hash-gated backup and record its truthful terminal result; (3) mark publish skipped because nothing was produced; (4) call notify_user exactly once with notification_kind=\"run_summary\" and plainly say the workflow did not start, no results were produced, and the next schedule will retry unless the cause is fixed; then record notify truthfully.%s\n\nThe scheduler's reason is:\n%s%s",
		pulseRunID, routing, reason, content,
	)}}
}

func postRunMonitorFinalSteps(pulseRunID string, instructions ...workflowNotificationContentInstructions) []postRunMonitorStep {
	ownerInstructions := workflowNotificationContentInstructions{}
	if len(instructions) > 0 {
		ownerInstructions = instructions[0]
	}
	notificationContext := ""
	if runInstructions := strings.TrimSpace(ownerInstructions.runSummary); runInstructions != "" {
		notificationContext += "\n\nWORKFLOW RUN SUMMARY INSTRUCTIONS. Apply these only to the section describing what happened in the workflow execution:\n" + runInstructions
	}
	if pulseInstructions := strings.TrimSpace(ownerInstructions.pulseSummary); pulseInstructions != "" {
		notificationContext += "\n\nPULSE REVIEW SUMMARY INSTRUCTIONS. Apply these only to the section describing what Pulse reviewed, fixed, recommended, or needs from the user:\n" + pulseInstructions
	}
	if len(ownerInstructions.runSummaryChannels) > 0 || len(ownerInstructions.pulseSummaryChannels) > 0 {
		notificationContext += fmt.Sprintf("\n\nSPLIT NOTIFICATION ROUTING. Send two notify_user calls, not one combined message. Send the workflow outcome with notification_kind=\"run_summary\"; configured channels: %s. Send Pulse activity with notification_kind=\"pulse_summary\"; configured channels: %s. The backend enforces these routes.", notificationChannelSummary(ownerInstructions.runSummaryChannels), notificationChannelSummary(ownerInstructions.pulseSummaryChannels))
	}
	if len(ownerInstructions.runSummaryRecipients) > 0 || len(ownerInstructions.pulseSummaryRecipients) > 0 {
		// Stated so the finalizer does not "helpfully" pass email_to and override
		// the owner's saved lists. The backend applies these by notification_kind
		// on its own; an explicit email_to would replace them for that send.
		notificationContext += fmt.Sprintf("\n\nCONFIGURED EMAIL RECIPIENTS. The backend addresses each email automatically from the workflow's saved lists — run summary: %s; Pulse summary: %s. Do NOT set email_to; sending with the correct notification_kind is what routes it to the right people.", notificationRecipientSummary(ownerInstructions.runSummaryRecipients), notificationRecipientSummary(ownerInstructions.pulseSummaryRecipients))
	}
	if notificationContext != "" {
		notificationContext += "\n\nThese instructions control content detail and emphasis only; they never change recipients, channels, secrets, permissions, or safety rules."
	}
	return []postRunMonitorStep{{"finalize", fmt.Sprintf("PULSE FINALIZER. pulse_run_id=%q. Load read_skill(skills=[{\"name\":\"builder-reference\",\"path\":\"references/pulse-finalizer.md\"}]) and follow it exactly. First confirm every due module has a terminal current-run result; never treat missing as success. The Pulse popup is the only presentation: do not write a Pulse HTML document or dashboard card. Complete backup, publish, and notify in that order in this one turn, recording running and terminal status for each with record_pulse_result(command=...). Continue after individual failures, keep every status truthful, then stop.%s", pulseRunID, notificationContext)}}
}

func notificationChannelSummary(channels []string) string {
	if len(channels) == 0 {
		return "all enabled channels (legacy default)"
	}
	return strings.Join(channels, ", ")
}

// notificationRecipientSummary describes a saved recipient list for the Pulse
// finalizer prompt. An unset list is spelled out as the account default rather
// than left blank, so the finalizer does not read it as "nobody".
func notificationRecipientSummary(recipients []string) string {
	if len(recipients) == 0 {
		return "the account default recipient"
	}
	return strings.Join(recipients, ", ")
}

func validatePulseDueModuleResults(ctx context.Context, workspacePath, pulseRunID string) error {
	worklist, ok, err := getPulseWorklistForRun(ctx, workspacePath, pulseRunID)
	if err != nil {
		return fmt.Errorf("read Pulse worklist: %w", err)
	}
	if !ok {
		return fmt.Errorf("Pulse worklist %q is missing", pulseRunID)
	}
	var unresolved []string
	for _, module := range pulseModuleOrder {
		state, exists := worklist[module]
		if !exists || strings.TrimSpace(strings.ToLower(state.LastDecision)) != "due" {
			continue
		}
		if strings.TrimSpace(state.LastResult) == "" {
			unresolved = append(unresolved, module)
		}
	}
	if len(unresolved) > 0 {
		return fmt.Errorf("due Pulse modules lack terminal current-run results: %s", strings.Join(unresolved, ", "))
	}
	return nil
}

func pulseWorklistHasDueModule(ctx context.Context, workspacePath, pulseRunID string) (bool, error) {
	worklist, ok, err := getPulseWorklistForRun(ctx, workspacePath, pulseRunID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("Pulse worklist %q is missing", pulseRunID)
	}
	for _, module := range pulseModuleOrder {
		state, exists := worklist[module]
		if exists && strings.TrimSpace(strings.ToLower(state.LastDecision)) == "due" {
			return true, nil
		}
	}
	return false, nil
}

func optimizerScheduleMessages(_ context.Context, _ string, stored []string, _ []string) []string {
	messages := compactScheduleMessages(stored)
	if len(messages) > 0 && !isLegacyGoalAdvisorMessageQueue(messages) {
		return messages
	}
	return []string{
		"Do not ask for confirmation. This optimizer schedule is no longer the product Goal Advisor loop. Goal Advisor now runs as a Pulse-selected module after normal scheduled workflow runs. Do not modify workflow files. Report that this legacy optimizer schedule should be disabled or converted to an explicit custom optimizer job.",
	}
}

func isLegacyOrEmptyOptimizerSchedule(messages []string) bool {
	messages = compactScheduleMessages(messages)
	return len(messages) == 0 || isLegacyGoalAdvisorMessageQueue(messages)
}

func isLegacyGoalAdvisorMessageQueue(messages []string) bool {
	joined := normalizeLegacyScheduleText(strings.Join(messages, "\n"))
	if !strings.Contains(joined, "step 1/5") || !strings.Contains(joined, "pre backup") {
		return false
	}
	if !strings.Contains(joined, "step 2/5") {
		return false
	}
	return strings.Contains(joined, "goal advisor") || strings.Contains(joined, "improve")
}

func normalizeLegacyScheduleText(text string) string {
	text = strings.ToLower(text)
	replacer := strings.NewReplacer(
		"—", " ",
		"–", " ",
		"-", " ",
		"_", " ",
		"\n", " ",
		"\t", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(text)), " ")
}

func compactScheduleMessages(messages []string) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		if strings.TrimSpace(msg) == "" {
			continue
		}
		out = append(out, msg)
	}
	return out
}

type scheduledWorkshopTurn struct {
	label         string
	query         string
	upgradeTarget string
}

func scheduledWorkshopTurns(manifest *WorkflowManifest, messages []string) ([]scheduledWorkshopTurn, error) {
	upgradePlan := workflowVersionUpgradePlan(manifest)
	manifestVersion := workflowContractVersionForUpgrade(manifest)
	if manifestVersion != WorkflowContractCurrentVersion && (len(upgradePlan) == 0 || upgradePlan[len(upgradePlan)-1].to != WorkflowContractCurrentVersion) {
		return nil, fmt.Errorf(
			"workflow upgrade preflight has no complete upgrade path from version %q to %q; normal schedule message was not started",
			manifestVersion,
			WorkflowContractCurrentVersion,
		)
	}

	turns := make([]scheduledWorkshopTurn, 0, len(upgradePlan)+len(messages))
	for _, upgrade := range upgradePlan {
		turns = append(turns, scheduledWorkshopTurn{
			label:         upgrade.label,
			query:         upgrade.query,
			upgradeTarget: upgrade.to,
		})
	}
	for i, message := range messages {
		turns = append(turns, scheduledWorkshopTurn{
			label: fmt.Sprintf("schedule-message-%d", i+1),
			query: message,
		})
	}
	return turns, nil
}

func scheduledWorkshopMessages(sctx *ScheduleContext) []string {
	if sctx == nil {
		return nil
	}
	messages := compactScheduleMessages(sctx.Schedule.Messages)
	isOptimizer := strings.EqualFold(strings.TrimSpace(sctx.Schedule.WorkshopMode), "optimizer")
	if len(messages) == 0 && !isOptimizer && !sctx.PulseOnly {
		return []string{"Run the full workflow using run_full_workflow tool. " + scheduledBackgroundNoPollingInstruction}
	}
	return messages
}

// executeJob builds a session request from the manifest and runs it.
// Returns (sessionID, runFolder, error).
func (s *SchedulerService) executeJob(ctx context.Context, sctx *ScheduleContext, runID string) (string, string, error) {
	// Multi-agent schedules live in a separate user-level schedule store. All
	// workflow-manifest schedules execute through the workshop builder path.
	if sctx.SourceType == "multi-agent" {
		return s.executeMultiAgentJob(ctx, sctx, runID)
	}

	if mode := strings.TrimSpace(sctx.Schedule.Mode); mode != "" && mode != "workshop" {
		s.logf(sctx, "[SCHEDULER] Schedule %s uses legacy mode=%s; executing through workshop mode", sctx.Schedule.ID, mode)
	}
	return s.executeWorkshopJob(ctx, sctx, runID)
}

// executeWorkshopJob runs a workflow via the workshop builder path (workflow_phase mode).
func (s *SchedulerService) executeWorkshopJob(ctx context.Context, sctx *ScheduleContext, runID string) (string, string, error) {
	messages := scheduledWorkshopMessages(sctx)
	isOptimizer := strings.EqualFold(strings.TrimSpace(sctx.Schedule.WorkshopMode), "optimizer")
	if isOptimizer {
		if isLegacyOrEmptyOptimizerSchedule(messages) {
			runFolder := "iteration-0"
			sessionID := s.newScheduleSessionID(sctx)
			if runID != "" {
				_ = UpdateScheduleRun(ctx, sctx.WorkspacePath, runID, "running", "", nil, runFolder, sessionID)
			}
			if err := s.disableLegacyOptimizerSchedule(ctx, sctx, sessionID); err != nil {
				return sessionID, runFolder, err
			}
			return sessionID, runFolder, nil
		}
	}
	runFolder := "iteration-0"
	if sctx.PulseOnly && strings.TrimSpace(sctx.PulseEvidenceRunFolder) != "" {
		runFolder = strings.TrimSpace(sctx.PulseEvidenceRunFolder)
	}

	// Snapshot existing run folders before this invocation. executeWorkshopJob's
	// own runFolder above is workshop-session bookkeeping (always "iteration-0"
	// outside PulseOnly, never the folder the triggered workflow run actually
	// used — the LLM's run_full_workflow tool call picks that via the normal
	// iteration rotation). This snapshot lets the post-run check below tell a
	// NEW run folder created by this invocation from a pre-existing one.
	preRunFolders, preRunFoldersErr := s.api.loadRunFoldersInternal(ctx, sctx.WorkspacePath)
	preRunFolderNames := runFolderNameSet(preRunFolders)
	// A failed snapshot is not an empty workspace. Without this distinction the
	// reconciler below compares against an empty baseline, concludes every
	// pre-existing folder is new, and attributes an old failed run to this
	// invocation. The listing is most likely to fail right after a server
	// restart, while the workspace API is still warming up — exactly when
	// scheduled runs resume.
	runFolderBaselineKnown := preRunFoldersErr == nil
	invocationStartedAt := time.Now().UTC()
	sctx.ProducedRunEvidence = false

	sessionID := s.newScheduleSessionID(sctx)

	s.updateRuntimeState(scheduleRuntimeKey(sctx), func(state *ScheduleRuntimeState) {
		state.LastSessionID = sessionID
	})

	if runID != "" {
		_ = UpdateScheduleRun(ctx, sctx.WorkspacePath, runID, "running", "", nil, runFolder, sessionID)
	}

	s.sessionLogf(sctx, sessionID, "[SCHEDULER] Workshop mode: executing %d messages for %s (session=%s workspace=%s run_folder=%s pulse_only=%t)",
		len(messages), sctx.Schedule.ID, sessionID, sctx.WorkspacePath, runFolder, sctx.PulseOnly)

	baseReqMap := s.buildWorkshopRequest(ctx, sctx)

	// Workflow contract upgrades are a blocking preflight, not post-run cleanup.
	// A breaking runtime migration (for example message_sequence code items to
	// standalone scripted steps) must finish before the schedule's first normal
	// message can execute. The same builder session is reused so the upgrade is
	// visible in the schedule terminal and the normal run starts only after the
	// on-disk manifest confirms each target version.
	manifest, found, err := ReadWorkflowManifest(ctx, sctx.WorkspacePath)
	if err != nil {
		return sessionID, runFolder, fmt.Errorf("workflow upgrade preflight could not read manifest: %w", err)
	}
	if !found {
		return sessionID, runFolder, fmt.Errorf("workflow upgrade preflight: workflow manifest not found at %s", sctx.WorkspacePath)
	}
	turns, err := scheduledWorkshopTurns(manifest, messages)
	if err != nil {
		return sessionID, runFolder, err
	}
	upgradeCount := len(turns) - len(messages)
	if upgradeCount > 0 {
		s.sessionLogf(sctx, sessionID, "[SCHEDULER] Running %d blocking workflow upgrade preflight turn(s) before %d schedule message(s)", upgradeCount, len(messages))
	}

	// Once a preflight upgrade turn has failed open (see below), any FURTHER
	// upgrade turn in this same run is skipped without attempting it — later
	// migrations may assume the skipped one already ran. Message turns still
	// run normally; the next scheduled run recomputes the upgrade plan from
	// scratch and tries the preflight again.
	preflightFailedOpen := false
	for i, turn := range turns {
		if preflightFailedOpen && turn.upgradeTarget != "" {
			s.sessionLogf(sctx, sessionID, "[SCHEDULER] Skipping workshop turn %d/%d (%s): a prior upgrade preflight failed open this run", i+1, len(turns), turn.label)
			continue
		}
		s.sessionLogf(sctx, sessionID, "[SCHEDULER] Workshop turn %d/%d (%s): %q", i+1, len(turns), turn.label, turn.query)

		reqMap := make(map[string]interface{})
		for k, v := range baseReqMap {
			reqMap[k] = v
		}
		reqMap["query"] = turn.query
		if turn.upgradeTarget != "" {
			s.applyPulseLLMToReqMap(reqMap, sctx, sessionID)
		}

		// Resume the workflow's latest thread (same CLI) on the first message
		// only — later messages already share this run's live session.
		if i == 0 {
			if resumed := s.maybeResumeLatestWorkflowThread(ctx, sctx, reqMap, sessionID); resumed != "" {
				s.sessionLogf(sctx, sessionID, "[SCHEDULER] Resuming latest workflow thread %s for schedule %s", resumed, sctx.Schedule.ID)
			}
		}

		if err := s.api.startSessionInternal(ctx, reqMap, sessionID, "", nil); err != nil {
			return sessionID, runFolder, fmt.Errorf("workshop turn %d/%d (%s) failed: %w", i+1, len(turns), turn.label, err)
		}

		// First message of the workshop sequence — stamp schedule name on
		// the session for frontend tab labeling. Subsequent calls are
		// no-ops (helper guards against overwriting an existing Title).
		s.stampScheduleNameOnSession(sessionID, sctx)

		if err := s.waitForWorkshopIdle(ctx, sessionID); err != nil {
			// A stalled session says the turn stopped progressing. It says nothing
			// about whether the workflow ran. This path returns before the
			// evidence check further below, so ProducedRunEvidence would keep its
			// initialized false and Pulse would be told "the workflow did not run"
			// purely because the wait expired — while run_metadata.json recorded a
			// completed run. Observed on social-media 2026-08-10: the run completed
			// at 12:15:18Z, the idle wait expired 34 minutes later at 12:49:35Z, and
			// the finalizer skipped publish with "no Pulse Gate/reviewer/Fixer"
			// against a run that had landed 19 verified actions.
			//
			// Consult the durable record before returning. Deliberately the
			// baseline-free check: the pre-run snapshot can itself be lost
			// (PLAT-070), and an empty baseline would make every folder look new
			// and answer "evidence" unconditionally — the opposite error.
			if folders, listErr := s.api.loadRunFoldersInternal(ctx, sctx.WorkspacePath); listErr == nil {
				if workshopRunStartedDuringInvocation(folders, invocationStartedAt) {
					sctx.ProducedRunEvidence = true
					s.sessionLogf(sctx, sessionID, "[SCHEDULER] idle wait expired for %s, but a run started during this invocation and its own metadata is the authority; preserving run evidence for Pulse", sctx.Schedule.ID)
				}
			}
			return sessionID, runFolder, fmt.Errorf("workshop idle wait failed after turn %d (%s): %w", i+1, turn.label, err)
		}

		if turn.upgradeTarget != "" {
			updatedManifest, updatedFound, readErr := ReadWorkflowManifest(ctx, sctx.WorkspacePath)
			if readErr != nil {
				return sessionID, runFolder, fmt.Errorf("workflow upgrade preflight %s completed but manifest could not be re-read: %w", turn.label, readErr)
			}
			if !updatedFound || workflowContractVersionForUpgrade(updatedManifest) != turn.upgradeTarget {
				actual := "missing"
				if updatedFound {
					actual = workflowContractVersionForUpgrade(updatedManifest)
				}
				failOpen, failureCount, recordErr := RecordWorkflowSchedulePreflightFailure(ctx, sctx.WorkspacePath, sctx.Schedule, turn.upgradeTarget, time.Now())
				if recordErr != nil {
					s.sessionLogf(sctx, sessionID, "[SCHEDULER] WARNING: failed to persist preflight failure count for %s: %v", turn.label, recordErr)
				}
				if !failOpen {
					return sessionID, runFolder, fmt.Errorf(
						"workflow upgrade preflight %s did not stamp required version %q (found %q, failure %d/%d consecutive); normal schedule message was not started",
						turn.label,
						turn.upgradeTarget,
						actual,
						failureCount,
						workflowSchedulePreflightFailOpenThreshold,
					)
				}
				s.sessionLogf(sctx, sessionID,
					"[SCHEDULER] WARNING: workflow upgrade preflight %s failed to stamp %q %d consecutive times (found %q). Failing OPEN: running the normal schedule message on the unstamped contract. This workflow needs owner attention — a supported configuration control could not complete this migration. Retrying the preflight fresh next scheduled run.",
					turn.label, turn.upgradeTarget, failureCount, actual)
				preflightFailedOpen = true
				continue
			}
			if clearErr := ClearWorkflowSchedulePreflightFailures(ctx, sctx.WorkspacePath, sctx.Schedule); clearErr != nil {
				s.sessionLogf(sctx, sessionID, "[SCHEDULER] WARNING: failed to clear preflight failure count for %s: %v", turn.label, clearErr)
			}
		}

		s.sessionLogf(sctx, sessionID, "[SCHEDULER] Workshop turn %d/%d (%s) completed", i+1, len(turns), turn.label)
	}

	// Note: backup-on-completion is not appended here as a message turn. Backup is
	// owned by two arms that share one source-hash-gated contract: the Pulse pass
	// (runPostRunMonitor, step 4) for scheduled runs when Pulse is enabled, and the
	// interactive-run completion directive for interactive runs (and as the fallback
	// when Pulse is off). The shared source-hash
	// gate means whichever arm runs second sees the state already backed up and skips
	// the push — so the overlap can't double-back up.

	// Previously auto-generated a static markdown report here via the report agent.
	// The dynamic report (design doc §2) is a live frontend view over db/ + graph.json;
	// there is no post-run artifact to produce, so scheduled runs now finish without a
	// report side-effect. Users open the report in the UI whenever they want.

	// The workshop chat session finishing its scheduled turns without an
	// infrastructure-level error is not proof the triggered workflow run
	// succeeded — the LLM's run_full_workflow tool call can fail deep inside
	// (e.g. an orchestrator agent-creation error) while the chat session
	// itself still goes idle normally. Reconcile against the run(s) actually
	// created during this invocation, per their own run_metadata.json — the
	// record the execution machinery writes independently of the scheduler
	// (BUG-20260729-10, social-media 2026-07-29: the scheduler recorded
	// "success" for a run that fully failed at its first posting step).
	postRunFolders, postRunFoldersErr := s.api.loadRunFoldersInternal(ctx, sctx.WorkspacePath)
	sctx.ProducedRunEvidence = workshopRunProducedEvidence(preRunFolderNames, postRunFolders, invocationStartedAt)
	if !sctx.ProducedRunEvidence {
		s.sessionLogf(sctx, sessionID, "[SCHEDULER] no run folder was created or restarted during this invocation for %s; Pulse evidence-dependent stages will be skipped", sctx.Schedule.ID)
	}
	// Reconciliation is a set difference, so it is only meaningful when both
	// sides are real. When either listing failed, skip it and say so rather than
	// reconcile against a baseline we do not have: this check exists to catch a
	// run that failed while its session looked healthy, and inventing a failure
	// from a missing snapshot is a worse error than missing one. Failing open is
	// safe here — the run's own metadata remains the durable record, and the
	// next invocation reconciles normally once the listing recovers.
	if !runFolderBaselineKnown || postRunFoldersErr != nil {
		s.sessionLogf(sctx, sessionID,
			"[SCHEDULER] skipping run-outcome reconciliation for %s: run-folder listing unavailable (pre-run err=%v, post-run err=%v); this invocation's outcome stands on its session result alone",
			sctx.Schedule.ID, preRunFoldersErr, postRunFoldersErr)
	} else if failedFolder, found := reconcileWorkshopRunOutcome(preRunFolderNames, postRunFolders); found {
		s.sessionLogf(sctx, sessionID, "[SCHEDULER] ⚠️ Workshop session for %s completed normally, but run %s recorded status \"failed\" in its own run_metadata.json", sctx.Schedule.ID, failedFolder)
		return sessionID, runFolder, fmt.Errorf("workflow run %s failed (its run_metadata.json records status \"failed\"), even though the orchestrating workshop session completed its turns without an infrastructure error", failedFolder)
	}

	s.sessionLogf(sctx, sessionID, "[SCHEDULER] ✅ Workshop execution completed for %s, session=%s, folder=%s", sctx.Schedule.ID, sessionID, runFolder)
	return sessionID, runFolder, nil
}

func runFolderNameSet(folders []RunFolderInfo) map[string]bool {
	names := make(map[string]bool, len(folders))
	for _, folder := range folders {
		if folder.Name != "" {
			names[folder.Name] = true
		}
	}
	return names
}

// workshopRunProducedEvidence reports whether this invocation actually ran the
// workflow. A new run folder is evidence. A pre-existing folder also counts
// when its independently written metadata says it started during this
// invocation, which covers workflows configured to reuse iteration-0. Status is
// intentionally irrelevant: failed runs are valuable Pulse evidence too.
// workshopRunStartedDuringInvocation reports whether any run's own metadata says
// it started during this invocation, using only the durable record.
//
// Deliberately takes no before-set. workshopRunProducedEvidence answers "is this
// folder new?" first, which is correct when it holds a real baseline but inverts
// when it does not: an empty baseline (a failed listing — PLAT-070) makes the
// very first folder look new and returns true unconditionally. On the idle-wait
// failure path the question is narrower and has a trustworthy answer that needs
// no baseline at all — did a run actually start? — so ask only that.
//
// A run whose metadata carries no usable timestamp is not counted. The caller
// treats a false here as "no evidence recorded", which is the pre-existing
// behaviour, so an unreadable record never manufactures evidence.
func workshopRunStartedDuringInvocation(after []RunFolderInfo, since time.Time) bool {
	for _, folder := range after {
		if folder.Name == "" || folder.Metadata == nil {
			continue
		}
		for _, stamp := range []time.Time{folder.Metadata.StartedAt, folder.Metadata.CreatedAt} {
			if !stamp.IsZero() && !stamp.Before(since) {
				return true
			}
		}
	}
	return false
}

func workshopRunProducedEvidence(before map[string]bool, after []RunFolderInfo, since time.Time) bool {
	for _, folder := range after {
		if folder.Name == "" {
			continue
		}
		if !before[folder.Name] {
			return true
		}
		if folder.Metadata == nil {
			continue
		}
		for _, stamp := range []time.Time{folder.Metadata.StartedAt, folder.Metadata.CreatedAt} {
			if !stamp.IsZero() && !stamp.Before(since) {
				return true
			}
		}
	}
	return false
}

// reconcileWorkshopRunOutcome finds the first run folder created during this
// workshop invocation (present in after but not in before — a pre-existing
// run's status must never affect this invocation's own result) whose own
// run_metadata.json recorded status "failed". Ambiguous states — no metadata,
// "running", "completed", anything else — are never treated as failure; only
// an explicit "failed" is, so a transient listing hiccup fails open toward
// "cannot verify" rather than toward a false failure.
func reconcileWorkshopRunOutcome(before map[string]bool, after []RunFolderInfo) (failedFolder string, found bool) {
	for _, folder := range after {
		if folder.Name == "" || before[folder.Name] {
			continue
		}
		if folder.Metadata == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(folder.Metadata.Status), "failed") {
			return folder.Name, true
		}
	}
	return "", false
}

func (s *SchedulerService) disableLegacyOptimizerSchedule(ctx context.Context, sctx *ScheduleContext, sessionID string) error {
	if sctx == nil {
		return nil
	}
	if sctx.SourceType != "" && sctx.SourceType != "workflow" {
		s.sessionLogf(sctx, sessionID, "[SCHEDULER] legacy optimizer schedule %s is non-workflow source %q; skipping manifest disable", sctx.Schedule.ID, sctx.SourceType)
		return nil
	}
	manifest, found, err := ReadWorkflowManifest(ctx, sctx.WorkspacePath)
	if err != nil {
		return fmt.Errorf("disable legacy optimizer schedule: %w", err)
	}
	if !found {
		return fmt.Errorf("disable legacy optimizer schedule: workflow manifest not found at %s", sctx.WorkspacePath)
	}
	for i := range manifest.Schedules {
		if manifest.Schedules[i].ID != sctx.Schedule.ID {
			continue
		}
		if !manifest.Schedules[i].Enabled {
			s.sessionLogf(sctx, sessionID, "[SCHEDULER] legacy optimizer schedule %s already disabled; no LLM session started", sctx.Schedule.ID)
			return nil
		}
		manifest.Schedules[i].Enabled = false
		enabled := true
		// Deliberately migrate the old standalone optimizer loop into Pulse.
		// Disabling the legacy schedule while leaving Pulse off would silently
		// remove Goal Advisor from the workflow.
		manifest.PostRunMonitor = &enabled
		if err := WriteWorkflowManifest(ctx, sctx.WorkspacePath, manifest); err != nil {
			return fmt.Errorf("disable legacy optimizer schedule: write workflow.json: %w", err)
		}
		if err := s.ReloadSchedule(ctx, sctx.WorkspacePath, sctx.Schedule.ID); err != nil {
			s.sessionLogf(sctx, sessionID, "[SCHEDULER] legacy optimizer schedule %s disabled but reload failed: %v", sctx.Schedule.ID, err)
		}
		s.sessionLogf(sctx, sessionID, "[SCHEDULER] legacy optimizer schedule %s disabled without starting an LLM session; post_run_monitor enabled so Goal Advisor can run inside Pulse", sctx.Schedule.ID)
		return nil
	}
	return fmt.Errorf("disable legacy optimizer schedule: schedule %s not found in %s", sctx.Schedule.ID, sctx.WorkspacePath)
}

// executeMultiAgentJob runs a multi-agent chat session with the configured query.
// maxWorkflowResumeScan bounds how many of this schedule's most recent runs the
// scheduler inspects when resume_previous=true: if a same-CLI resumable scheduled
// chat is among the latest few, resume it; otherwise start a fresh session.
const maxWorkflowResumeScan = 5

// maybeResumeLatestWorkflowThread wires restored_conversation_session_id into reqMap
// so an opt-in scheduled workflow run continues the schedule's most recent
// scheduled chat instead of starting fresh.
//
// We look at schedule-runs.json for this exact schedule_id first, then validate
// the referenced chat runtime. This deliberately excludes normal user/builder
// chats in the same workflow workspace. A different CLI's external session ID is
// meaningless to the new one. Prior run status (success/error) is intentionally
// ignored: resume happens regardless so the agent can recover from a failed run.
// Returns the resumed thread's session ID, or "" when the run should start fresh.
func (s *SchedulerService) maybeResumeLatestWorkflowThread(ctx context.Context, sctx *ScheduleContext, reqMap map[string]interface{}, currentSessionID string) string {
	if !sctx.Schedule.ShouldResumePrevious() {
		return ""
	}

	runs, err := listScheduleRunsForResume(ctx, sctx.WorkspacePath, sctx.Schedule.ID)
	if err != nil || len(runs) == 0 {
		return ""
	}
	return s.maybeResumeLatestScheduledThread(sctx, reqMap, currentSessionID, runs, sctx.WorkspacePath)
}

func (s *SchedulerService) maybeResumeLatestMultiAgentThread(ctx context.Context, sctx *ScheduleContext, reqMap map[string]interface{}, currentSessionID string) string {
	if !sctx.Schedule.ShouldResumePrevious() {
		return ""
	}

	runs, err := listMultiAgentScheduleRunsForResume(ctx, sctx.UserID, sctx.Schedule.ID)
	if err != nil || len(runs) == 0 {
		return ""
	}
	return s.maybeResumeLatestScheduledThread(sctx, reqMap, currentSessionID, runs, "")
}

func (s *SchedulerService) maybeResumeLatestScheduledThread(sctx *ScheduleContext, reqMap map[string]interface{}, currentSessionID string, runs []ScheduleRunEntry, workspacePath string) string {
	currentProvider := ""
	if llmConfig := sctx.Capabilities.LLMConfig; llmConfig != nil {
		if llmConfig.BuilderLLM != nil {
			currentProvider = strings.TrimSpace(llmConfig.BuilderLLM.Provider)
		}
		if currentProvider == "" {
			currentProvider = strings.TrimSpace(llmConfig.Provider)
		}
		if currentProvider == "" {
			if builder, _, ok := workflowtypes.ResolveProviderProfileConfig(llmConfig); ok && builder != nil {
				currentProvider = strings.TrimSpace(builder.Provider)
			}
		}
	}
	if currentProvider == "" {
		return ""
	}

	// Runs are newest-first. Within the latest maxWorkflowResumeScan scheduled
	// chats, resume the most recent one that is a resumable coding-agent thread
	// on the same CLI; skip any that don't qualify (e.g. an API-model thread).
	// If none qualify, start fresh.
	//
	// Validate via ReadChatHistoryRuntimeForSession(sessionID, workspace) — the
	// SAME resolver handleQuery uses when it later honors
	// restored_conversation_session_id — so what we match here is provably what
	// gets resumed.
	checked := 0
	for _, run := range runs {
		sessionID := strings.TrimSpace(run.SessionID)
		if sessionID == "" || sessionID == currentSessionID {
			continue
		}
		checked++
		if checked > maxWorkflowResumeScan {
			break
		}

		rt, ok, rErr := ReadChatHistoryRuntimeForSession(sctx.UserID, sessionID, workspacePath)
		if rErr != nil || !ok || rt == nil {
			continue
		}
		// A coding-agent thread the CLI can resume: kind, matching CLI provider,
		// and a captured external session ID to hand to the CLI's --resume.
		if rt.Kind != "coding_agent" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(rt.Provider), currentProvider) {
			continue
		}
		if !rt.ResumeSupported || strings.TrimSpace(rt.ExternalSessionID) == "" {
			continue
		}
		reqMap["restored_conversation_session_id"] = sessionID
		return sessionID
	}
	return ""
}

func listScheduleRunsForResume(ctx context.Context, workspacePath, scheduleID string) ([]ScheduleRunEntry, error) {
	if localRuns, ok, err := readLocalScheduleRuns(workspacePath); ok || err != nil {
		if err != nil {
			return nil, err
		}
		return filterScheduleRunsNewestFirst(localRuns, scheduleID), nil
	}
	runs, _, err := ListScheduleRuns(ctx, workspacePath, scheduleID, maxScheduleRuns, 0)
	return runs, err
}

func readLocalScheduleRuns(workspacePath string) ([]ScheduleRunEntry, bool, error) {
	localPath := filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(scheduleRunsPath(workspacePath)))
	data, err := os.ReadFile(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, true, err
	}
	var runs []ScheduleRunEntry
	if err := json.Unmarshal(data, &runs); err != nil {
		return nil, true, err
	}
	return runs, true, nil
}

func readLocalMultiAgentScheduleRuns(userID string) ([]ScheduleRunEntry, bool, error) {
	userID = sanitizeUserIDForPath(userID)
	localPath := filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(multiAgentScheduleRunsPath(userID)))
	data, err := os.ReadFile(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, true, err
	}
	var runs []ScheduleRunEntry
	if err := json.Unmarshal(data, &runs); err != nil {
		return nil, true, err
	}
	return runs, true, nil
}

func listMultiAgentScheduleRunsForResume(ctx context.Context, userID, scheduleID string) ([]ScheduleRunEntry, error) {
	if localRuns, ok, err := readLocalMultiAgentScheduleRuns(userID); ok || err != nil {
		if err != nil {
			return nil, err
		}
		return filterScheduleRunsNewestFirst(localRuns, scheduleID), nil
	}
	runs, _, err := ListMultiAgentScheduleRuns(ctx, userID, scheduleID, maxScheduleRuns, 0)
	return runs, err
}

func filterScheduleRunsNewestFirst(runs []ScheduleRunEntry, scheduleID string) []ScheduleRunEntry {
	filtered := make([]ScheduleRunEntry, 0, len(runs))
	for _, run := range runs {
		if run.ScheduleID == scheduleID {
			filtered = append(filtered, run)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].StartedAt.After(filtered[j].StartedAt)
	})
	if len(filtered) > maxScheduleRuns {
		filtered = filtered[:maxScheduleRuns]
	}
	return filtered
}

func (s *SchedulerService) executeMultiAgentJob(ctx context.Context, sctx *ScheduleContext, runID string) (string, string, error) {
	// A multi-agent schedule runs either a Messages SEQUENCE (one focused turn per
	// message in one resumed session, the way workflow Pulse / runPostRunMonitor
	// does — this is how Org Pulse runs) or a single Query (legacy/fallback).
	// Messages wins when present; Query stays the single-turn fallback so anything
	// that still sets only Query keeps working unchanged.
	messages := sctx.Schedule.Messages
	query := strings.TrimSpace(sctx.Schedule.Query)
	if len(messages) == 0 && query == "" {
		return "", "", fmt.Errorf("multi-agent schedule %s has no messages or query", sctx.Schedule.ID)
	}
	sessionID := s.newScheduleSessionID(sctx)
	chiefInputWorkspaces := []string{"pulse"}
	if workflows, err := s.DiscoverWorkflowManifestsCached(ctx, 5*time.Second); err != nil {
		s.logf(sctx, "[CHIEF_INPUT] Could not discover workflow scopes for answered questions: %v", err)
	} else {
		for _, workflow := range workflows {
			chiefInputWorkspaces = append(chiefInputWorkspaces, workflow.WorkspacePath)
		}
	}
	answeredChiefInputs := claimAnsweredChiefOfStaffInputsForAgent(ctx, chiefInputWorkspaces, sessionID)
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		releaseChiefOfStaffInputClaims(releaseCtx, chiefInputWorkspaces, sessionID)
	}()
	chiefInputNote := ""
	if answeredChiefInputs != "" {
		chiefInputNote = "\n\n" + answeredChiefInputs
	}
	if query != "" {
		query = withChiefTaskRunContext(sctx, query) + chiefInputNote
	}

	s.updateRuntimeState(scheduleRuntimeKey(sctx), func(state *ScheduleRuntimeState) {
		state.LastSessionID = sessionID
	})

	if runID != "" {
		_ = UpdateMultiAgentScheduleRun(ctx, sctx.UserID, runID, "running", "", nil, sessionID)
	}

	// Build the base request once. The per-turn query is set per message below
	// (sequence) or kept as-is (single Query). Everything else — capabilities,
	// servers, skills, browser/code-exec, LLM config, and secrets — is shared by
	// every turn of the sequence.
	reqMap := map[string]interface{}{
		"agent_mode":                  "simple",
		"triggered_by":                "cron",
		"disable_live_input_delivery": true,
		// The global activity monitor is visible while this request is running.
		// Send the concise schedule name on the first request instead of making
		// the UI fall back to the complete scheduler instruction in `query`.
		"session_title": strings.TrimSpace(sctx.Schedule.Name),
	}
	if query != "" {
		reqMap["query"] = query
	}

	// Apply capabilities if set
	if len(sctx.Capabilities.SelectedServers) > 0 {
		reqMap["servers"] = sctx.Capabilities.SelectedServers
	}
	if len(sctx.Capabilities.SelectedSkills) > 0 {
		reqMap["selected_skills"] = sctx.Capabilities.SelectedSkills
	}
	if sctx.Capabilities.BrowserMode != "" && sctx.Capabilities.BrowserMode != "none" {
		reqMap["browser_mode"] = sctx.Capabilities.BrowserMode
	}
	if len(sctx.Capabilities.CDPPorts) > 0 {
		reqMap["cdp_ports"] = append([]int(nil), sctx.Capabilities.CDPPorts...)
	}
	if sctx.Capabilities.UseCodeExecutionMode {
		reqMap["use_code_execution_mode"] = true
	}
	if sctx.Capabilities.Notifications != nil {
		if secretName := strings.TrimSpace(sctx.Capabilities.Notifications.SlackWebhookSecretName); secretName != "" {
			reqMap["notification_slack_webhook_secret_name"] = secretName
		}
	}

	// Apply LLM config and secrets
	s.applyLLMAndSecretsToReqMap(ctx, reqMap, sctx)
	s.applyChiefOfStaffLLMToReqMap(ctx, reqMap, sctx, sessionID)

	// Load user-level secrets if configured
	if len(sctx.Capabilities.SelectedSecrets) > 0 && sctx.UserID != "" {
		userSecrets := s.api.loadSelectedSecrets(context.Background(), sctx.UserID, sctx.WorkspacePath, sctx.Capabilities.SelectedSecrets)
		if len(userSecrets) > 0 {
			reqMap["decrypted_secrets"] = userSecrets
			s.sessionLogf(sctx, sessionID, "[SCHEDULER] Loaded %d user secrets for multi-agent schedule %s", len(userSecrets), sctx.Schedule.ID)
		}
	}

	// Sequence path: run each message as its own focused turn in the same resumed
	// session, mirroring executeWorkshopJob / runPostRunMonitor. The agent builds on
	// the prior turns' context, and the user watches it progress step by step.
	if len(messages) > 0 {
		s.sessionLogf(sctx, sessionID, "[ORG_PULSE] executeMultiAgentJob for %s (%s): session=%s user=%s running %d-step sequence",
			sctx.Schedule.ID, sctx.Schedule.Name, sessionID, sctx.UserID, len(messages))
		for i, msg := range messages {
			s.sessionLogf(sctx, sessionID, "[ORG_PULSE] step %d/%d: %q", i+1, len(messages), msg)

			stepReq := make(map[string]interface{}, len(reqMap))
			for k, v := range reqMap {
				stepReq[k] = v
			}
			if i == 0 {
				msg = withChiefTaskRunContext(sctx, msg) + chiefInputNote
			}
			stepReq["query"] = msg

			// Resume the latest prior scheduled thread on the first turn only — later
			// turns already share this run's live session.
			if i == 0 {
				if resumed := s.maybeResumeLatestMultiAgentThread(ctx, sctx, stepReq, sessionID); resumed != "" {
					s.sessionLogf(sctx, sessionID, "[ORG_PULSE] Resuming previous multi-agent schedule thread %s for %s", resumed, sctx.Schedule.ID)
				}
			}

			if err := s.api.startSessionInternal(ctx, stepReq, sessionID, sctx.UserID, nil); err != nil {
				return sessionID, "", fmt.Errorf("multi-agent step %d/%d failed: %w", i+1, len(messages), err)
			}

			// Stamp the schedule name on the first turn for frontend tab labeling;
			// later calls are no-ops (the helper guards an existing Title).
			if i == 0 {
				s.stampScheduleNameOnSession(sessionID, sctx)
			}

			if err := s.waitForWorkshopIdle(ctx, sessionID); err != nil {
				s.sessionLogf(sctx, sessionID, "[ORG_PULSE] step %d/%d idle wait failed: %v", i+1, len(messages), err)
				return sessionID, "", fmt.Errorf("multi-agent step %d/%d idle wait failed: %w", i+1, len(messages), err)
			}
			s.sessionLogf(sctx, sessionID, "[ORG_PULSE] step %d/%d done for %s", i+1, len(messages), sctx.Schedule.ID)
		}
		s.sessionLogf(sctx, sessionID, "[ORG_PULSE] sequence completed for %s", sctx.Schedule.ID)
		return sessionID, "", nil
	}

	// Single-query path. Use the same tmux/background-aware bounded wait as
	// sequence turns so an abandoned coding CLI cannot hold a schedule forever.
	if resumed := s.maybeResumeLatestMultiAgentThread(ctx, sctx, reqMap, sessionID); resumed != "" {
		s.sessionLogf(sctx, sessionID, "[SCHEDULER] Resuming previous multi-agent schedule thread %s for %s", resumed, sctx.Schedule.ID)
	}

	s.sessionLogf(sctx, sessionID, "[SCHEDULER] executeMultiAgentJob for %s (%s): session=%s user=%s query=%q",
		sctx.Schedule.ID, sctx.Schedule.Name, sessionID, sctx.UserID, query)

	// Start the session with the user's identity
	runErr := s.api.startSessionInternal(ctx, reqMap, sessionID, sctx.UserID, nil)
	if runErr != nil {
		return sessionID, "", fmt.Errorf("multi-agent session execution failed: %w", runErr)
	}

	s.stampScheduleNameOnSession(sessionID, sctx)

	if err := s.waitForWorkshopIdle(ctx, sessionID); err != nil {
		s.sessionLogf(sctx, sessionID, "[SCHEDULER] Multi-agent session wait failed for %s: %v", sctx.Schedule.ID, err)
		return sessionID, "", fmt.Errorf("multi-agent session idle wait failed: %w", err)
	}

	return sessionID, "", nil
}

// stampScheduleNameOnSession updates the tracked session with the
// schedule's display name + triggered_by=cron so the frontend reconnect
// path can identify this as a scheduled run and label the tab using the
// schedule name instead of falling back to the literal "Workflow".
// Safe to call after startSessionInternal returns — the session is
// already tracked by then.
func (s *SchedulerService) stampScheduleNameOnSession(sessionID string, sctx *ScheduleContext) {
	if sctx == nil || strings.TrimSpace(sctx.Schedule.Name) == "" {
		return
	}
	s.api.activeSessionsMux.Lock()
	defer s.api.activeSessionsMux.Unlock()
	if sess, ok := s.api.activeSessions[sessionID]; ok && sess != nil {
		if sess.Title == "" {
			sess.Title = sctx.Schedule.Name
		}
		if sess.TriggeredBy == "" {
			sess.TriggeredBy = "cron"
		}
	}
}

// applyLLMAndSecretsToReqMap adds LLM config, API keys, secrets, and trigger payload to a request map.
func (s *SchedulerService) applyLLMAndSecretsToReqMap(ctx context.Context, reqMap map[string]interface{}, sctx *ScheduleContext) {
	if sctx.Capabilities.SelectedGlobalSecretNames != nil {
		reqMap["selected_global_secrets"] = sctx.Capabilities.SelectedGlobalSecretNames
	}

	if sctx.Capabilities.LLMConfig != nil {
		llmCfg := sctx.Capabilities.LLMConfig
		builderLLM := llmCfg.BuilderLLM
		if builderLLM == nil {
			if resolved, _, ok := workflowtypes.ResolveProviderProfileConfig(llmCfg); ok {
				builderLLM = resolved
			}
		}
		provider := ""
		modelID := ""
		var options map[string]interface{}
		if builderLLM != nil {
			provider = strings.TrimSpace(builderLLM.Provider)
			modelID = strings.TrimSpace(builderLLM.ModelID)
			options = builderLLM.Options
		}
		llmConfigSource := ""
		if strings.EqualFold(strings.TrimSpace(sctx.Schedule.WorkshopMode), "optimizer") {
			maintenanceLLM := llmCfg.MaintenanceLLM
			if maintenanceLLM == nil {
				if resolved, ok := workflowtypes.ResolveProviderProfileMaintenanceConfig(llmCfg); ok {
					maintenanceLLM = resolved
				}
			}
			if maintenanceLLM != nil {
				maintenanceProvider := strings.TrimSpace(maintenanceLLM.Provider)
				maintenanceModelID := strings.TrimSpace(maintenanceLLM.ModelID)
				if maintenanceProvider != "" && maintenanceModelID != "" {
					provider = maintenanceProvider
					modelID = maintenanceModelID
					options = maintenanceLLM.Options
					llmConfigSource = llmConfigSourceScheduledAutoImprove
				}
			}
		}
		if provider != "" && modelID != "" {
			primary := map[string]interface{}{
				"provider": provider,
				"model_id": modelID,
			}
			if len(options) > 0 {
				primary["options"] = options
			}
			llmConfig := map[string]interface{}{
				"primary": primary,
			}
			reqMap["llm_config"] = llmConfig
			if llmConfigSource != "" {
				reqMap["llm_config_source"] = llmConfigSource
			}
		}
	}
	// API keys are now handled by MergedProviderAPIKeys in buildWorkshopConfig

	if len(sctx.Schedule.TriggerPayload) > 0 {
		var overrides map[string]interface{}
		if err := json.Unmarshal(sctx.Schedule.TriggerPayload, &overrides); err == nil {
			for k, v := range overrides {
				reqMap[k] = v
			}
		}
	}
}

func applyPrimaryLLMConfigToReqMap(reqMap map[string]interface{}, cfg *workflowtypes.AgentLLMConfig) bool {
	if reqMap == nil || cfg == nil {
		return false
	}
	provider := strings.TrimSpace(cfg.Provider)
	modelID := strings.TrimSpace(cfg.ModelID)
	if provider == "" || modelID == "" {
		return false
	}

	primary := map[string]interface{}{
		"provider": provider,
		"model_id": modelID,
	}
	if len(cfg.Options) > 0 {
		primary["options"] = cfg.Options
	}
	reqMap["llm_config"] = map[string]interface{}{
		"primary": primary,
	}
	return true
}

func cloneStringInterfaceMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *SchedulerService) applyPulseLLMToReqMap(reqMap map[string]interface{}, sctx *ScheduleContext, sessionID string) {
	if sctx == nil || sctx.Capabilities.LLMConfig == nil {
		return
	}
	pulseLLM := sctx.Capabilities.LLMConfig.PulseLLM
	if pulseLLM == nil {
		if resolved, ok := workflowtypes.ResolveProviderProfilePulseConfig(sctx.Capabilities.LLMConfig); ok {
			pulseLLM = resolved
		}
	}
	if !applyPrimaryLLMConfigToReqMap(reqMap, pulseLLM) {
		return
	}
	reqMap["llm_config_source"] = llmConfigSourceScheduledPulse
	s.sessionLogf(sctx, sessionID, "[PULSE] using configured pulse LLM %s/%s", strings.TrimSpace(pulseLLM.Provider), strings.TrimSpace(pulseLLM.ModelID))
}

func (s *SchedulerService) applyGoalAdvisorLLMToReqMap(reqMap map[string]interface{}, sctx *ScheduleContext, sessionID string) {
	if sctx == nil || sctx.Capabilities.LLMConfig == nil {
		return
	}
	goalAdvisorLLM := sctx.Capabilities.LLMConfig.MaintenanceLLM
	if goalAdvisorLLM == nil {
		if resolved, ok := workflowtypes.ResolveProviderProfileMaintenanceConfig(sctx.Capabilities.LLMConfig); ok {
			goalAdvisorLLM = resolved
		}
	}
	if !applyPrimaryLLMConfigToReqMap(reqMap, goalAdvisorLLM) {
		return
	}
	reqMap["llm_config_source"] = llmConfigSourceScheduledAutoImprove
	s.sessionLogf(sctx, sessionID, "[PULSE] using configured Goal Advisor LLM %s/%s", strings.TrimSpace(goalAdvisorLLM.Provider), strings.TrimSpace(goalAdvisorLLM.ModelID))
}

func (s *SchedulerService) applyChiefOfStaffLLMToReqMap(ctx context.Context, reqMap map[string]interface{}, sctx *ScheduleContext, sessionID string) {
	if sctx == nil || sctx.SourceType != "multi-agent" {
		return
	}
	chiefOfStaffLLM := resolveChiefOfStaffLLMForSchedule(ctx, sctx)
	if !applyPrimaryLLMConfigToReqMap(reqMap, chiefOfStaffLLM) {
		return
	}
	reqMap["llm_config_source"] = llmConfigSourceScheduledChiefOfStaff
	s.sessionLogf(sctx, sessionID, "[SCHEDULER] using configured Chief of Staff LLM %s/%s", strings.TrimSpace(chiefOfStaffLLM.Provider), strings.TrimSpace(chiefOfStaffLLM.ModelID))
}

func resolveChiefOfStaffLLMForSchedule(ctx context.Context, sctx *ScheduleContext) *workflowtypes.AgentLLMConfig {
	if sctx == nil {
		return nil
	}
	if llmCfg := sctx.Capabilities.LLMConfig; llmCfg != nil {
		if llmCfg.ChiefOfStaffLLM != nil {
			return llmCfg.ChiefOfStaffLLM
		}
		if resolved, ok := workflowtypes.ResolveProviderProfileChiefOfStaffConfig(llmCfg); ok {
			return resolved
		}
		return nil
	}

	tierConfig := LoadAndResolveTierConfig(ctx, nil)
	return resolveChiefOfStaffLLMFromDelegationConfig(tierConfig)
}

func resolveChiefOfStaffLLMFromDelegationConfig(tierConfig *virtualtools.DelegationTierConfig) *workflowtypes.AgentLLMConfig {
	if tierConfig == nil {
		return nil
	}
	if tierConfig.ChiefOfStaff != nil {
		return agentLLMConfigFromTierModel(tierConfig.ChiefOfStaff)
	}
	for _, candidate := range []*virtualtools.TierModel{tierConfig.Main, tierConfig.High} {
		if cfg := agentLLMConfigFromTierModel(candidate); cfg != nil {
			if resolved, ok := workflowtypes.ResolveProviderProfileChiefOfStaffConfig(&workflowtypes.PresetLLMConfig{
				SchemaVersion: workflowtypes.LLMConfigSchemaVersion,
				Mode:          workflowtypes.LLMConfigModeProviderProfile,
				Provider:      cfg.Provider,
			}); ok {
				return resolved
			}
			return cfg
		}
	}
	return nil
}

func agentLLMConfigFromTierModel(tier *virtualtools.TierModel) *workflowtypes.AgentLLMConfig {
	if tier == nil || strings.TrimSpace(tier.Provider) == "" || strings.TrimSpace(tier.ModelID) == "" {
		return nil
	}
	cfg := &workflowtypes.AgentLLMConfig{
		Provider: strings.TrimSpace(tier.Provider),
		ModelID:  strings.TrimSpace(tier.ModelID),
		Options:  tier.Options,
	}
	if len(tier.Fallbacks) > 0 {
		cfg.Fallbacks = make([]workflowtypes.AgentLLMFallback, 0, len(tier.Fallbacks))
		for _, fallback := range tier.Fallbacks {
			provider := strings.TrimSpace(fallback.Provider)
			modelID := strings.TrimSpace(fallback.ModelID)
			if provider == "" || modelID == "" {
				continue
			}
			cfg.Fallbacks = append(cfg.Fallbacks, workflowtypes.AgentLLMFallback{
				Provider: provider,
				ModelID:  modelID,
				Options:  fallback.Options,
			})
		}
	}
	return cfg
}

// buildWorkshopRequest creates the base request map for workshop mode execution.
func (s *SchedulerService) buildWorkshopRequest(ctx context.Context, sctx *ScheduleContext) map[string]interface{} {
	reqMap := map[string]interface{}{
		"agent_mode":                  "workflow_phase",
		"phase_id":                    workflowtypes.WorkflowStatusWorkflowBuilder,
		"preset_query_id":             sctx.WorkflowID,
		"selected_folder":             sctx.WorkspacePath,
		"triggered_by":                "cron",
		"servers":                     sctx.Capabilities.SelectedServers,
		"selected_tools":              sctx.Capabilities.SelectedTools,
		"selected_skills":             sctx.Capabilities.SelectedSkills,
		"browser_mode":                sctx.Capabilities.BrowserMode,
		"use_code_execution_mode":     sctx.Capabilities.UseCodeExecutionMode,
		"disable_live_input_delivery": true,
		// Upgrade, normal execution, and Pulse are consecutive messages in one
		// known scheduler conversation. Retain its coding CLI between turns so
		// the adapter does not visibly inject /exit just to resume moments later.
		"keep_native_session_alive": true,
	}
	if len(sctx.Capabilities.CDPPorts) > 0 {
		reqMap["cdp_ports"] = append([]int(nil), sctx.Capabilities.CDPPorts...)
	}
	if sctx.Capabilities.Notifications != nil {
		if secretName := strings.TrimSpace(sctx.Capabilities.Notifications.SlackWebhookSecretName); secretName != "" {
			reqMap["notification_slack_webhook_secret_name"] = secretName
		}
	}

	s.applyLLMAndSecretsToReqMap(ctx, reqMap, sctx)

	execOpts := map[string]interface{}{
		"run_mode":            "use_same_run",
		"selected_run_folder": "iteration-0",
		"execution_strategy":  "start_from_beginning_no_human",
		// Scheduled runs execute the workflow builder exactly like a normal
		// interactive chat — workshop mode. This keeps the scheduled run on the
		// same mode as the user's interactive sessions, so it natively resumes
		// the workflow's latest thread (same-mode) with no special handling.
		"workshop_mode": "workshop",
	}
	if len(sctx.Schedule.GroupNames) > 0 {
		execOpts["enabled_group_names"] = sctx.Schedule.GroupNames
	}
	reqMap["execution_options"] = execOpts

	return reqMap
}

var schedulerWorkshopIdlePollInterval = 3 * time.Second
var schedulerWorkshopMaxInactivity = 10 * time.Minute

// schedulerWorkshopLiveChildCeiling bounds how long a turn may be held open by
// child work that is registered as running but emits nothing observable. The
// inactivity window above answers "is anything happening?"; it is the wrong
// question once a child is demonstrably alive, because a legitimate step can be
// silent far longer than that — browser workflows deliberately pace themselves
// in tens of seconds per action, and a full run_full_workflow child runs for
// hours. This ceiling exists only so a child that hangs forever cannot block its
// schedule forever.
var schedulerWorkshopLiveChildCeiling = 3 * time.Hour

// sessionHasLiveChildWork reports whether independently tracked child work is
// still running for this session.
//
// Do not use sessionIsBusy here. Its presentation state includes the retained
// main tmux pane, which intentionally remains live between scheduler messages.
// Completion of the current main turn is already a structured fact
// (startSessionInternal blocks until it) — only tracked child work may delay us.
func (s *SchedulerService) sessionHasLiveChildWork(sessionID string) bool {
	if s == nil || s.api == nil {
		return false
	}
	for _, execution := range s.api.trackedExecutionsForSession(sessionID) {
		if execution != nil && execution.Status == trackedExecutionStatusRunning {
			return true
		}
	}
	return s.api.bgAgentRegistry != nil && s.api.bgAgentRegistry.HasRunningAgents(sessionID)
}
var errWorkshopIdleWaitTimeout = errors.New("workshop idle wait timed out")
var errWorkshopSequenceInterrupted = errors.New("workshop sequence interrupted by user")
var errWorkshopSessionFailed = errors.New("workshop session failed")

var pulseTurnSettleDelay = 500 * time.Millisecond

// waitForPulseTurnCompletion advances Pulse from structured runtime events,
// never from tmux-pane text. startSessionInternal has already received the
// main agent's completion before this is called; the only work that can hold
// the next Pulse message is a child execution or background agent the main
// turn deliberately launched. A short settle window admits registrations that
// race immediately after main-turn completion. The inactivity timer remains a
// safety boundary for genuine stalled child work, not a poll.
func (s *SchedulerService) waitForPulseTurnCompletion(ctx context.Context, sessionID string, maxInactivity time.Duration) error {
	if s == nil || s.api == nil || s.api.eventStore == nil {
		return s.waitForWorkshopIdleWithInactivityTimeout(ctx, sessionID, maxInactivity)
	}
	checkInterruption := func() error {
		if activeSession, exists := s.api.getActiveSession(sessionID); exists &&
			normalizeSessionLifecycleStatus(activeSession.Status) == sessionLifecycleFailed {
			return fmt.Errorf("%w: session %s status is %s", errWorkshopSessionFailed, sessionID, activeSession.Status)
		}
		if s.api.isSessionMarkedStopped(sessionID) {
			return fmt.Errorf("%w: session %s was stopped", errWorkshopSequenceInterrupted, sessionID)
		}
		if s.api.consumeSessionTurnInterrupted(sessionID) {
			return fmt.Errorf("%w: current response in session %s was canceled", errWorkshopSequenceInterrupted, sessionID)
		}
		return nil
	}
	if err := checkInterruption(); err != nil {
		return err
	}

	sub := s.api.eventStore.Subscribe(sessionID)
	defer s.api.eventStore.Unsubscribe(sessionID, sub)

	var inactivity <-chan time.Time
	var inactivityTimer *time.Timer
	if maxInactivity > 0 {
		inactivityTimer = time.NewTimer(maxInactivity)
		defer inactivityTimer.Stop()
		inactivity = inactivityTimer.C
	}
	resetInactivity := func() {
		if inactivityTimer == nil {
			return
		}
		if !inactivityTimer.Stop() {
			select {
			case <-inactivityTimer.C:
			default:
			}
		}
		inactivityTimer.Reset(maxInactivity)
	}

	hasLiveChildWork := func() bool { return s.sessionHasLiveChildWork(sessionID) }
	waitStartedAt := time.Now()

	settleTimer := time.NewTimer(pulseTurnSettleDelay)
	defer settleTimer.Stop()
	settle := settleTimer.C
	resetSettle := func() {
		if !settleTimer.Stop() {
			select {
			case <-settleTimer.C:
			default:
			}
		}
		settleTimer.Reset(pulseTurnSettleDelay)
		settle = settleTimer.C
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-sub.Ch:
			if !ok {
				return fmt.Errorf("Pulse runtime event stream closed for session %s", sessionID)
			}
			if err := checkInterruption(); err != nil {
				return err
			}
			resetInactivity()
			resetSettle()
		case <-settle:
			if err := checkInterruption(); err != nil {
				return err
			}
			if !hasLiveChildWork() {
				return nil
			}
			settle = nil
		case <-inactivity:
			// Silence is not a stall while a child is demonstrably running. The
			// main turn already completed before this wait began, so the only
			// thing we are waiting on is child work — and a legitimate child
			// (a browser step pacing itself, a long model call) can emit
			// nothing for far longer than the inactivity window.
			if hasLiveChildWork() && time.Since(waitStartedAt) < schedulerWorkshopLiveChildCeiling {
				scheduleLogf("[SCHEDULER] Pulse turn quiet for %s in session %s but child work is still running; continuing to wait", maxInactivity, sessionID)
				resetInactivity()
				continue
			}
			return fmt.Errorf("%w: no runtime event progress for %s in session %s (live_child_work=%t)",
				errWorkshopIdleWaitTimeout, maxInactivity, sessionID, hasLiveChildWork())
		}
	}
}

const schedulerWorkshopIdleConsecutiveChecks = 2

// waitForWorkshopIdle polls until all background agents, tracked executions, and
// tmux-backed turns have completed.
func (s *SchedulerService) waitForWorkshopIdle(ctx context.Context, sessionID string) error {
	return s.waitForWorkshopIdleWithInactivityTimeout(ctx, sessionID, schedulerWorkshopMaxInactivity)
}

func (s *SchedulerService) waitForWorkshopIdleWithInactivityTimeout(ctx context.Context, sessionID string, maxInactivity time.Duration) error {
	ticker := time.NewTicker(schedulerWorkshopIdlePollInterval)
	defer ticker.Stop()

	consecutiveIdleChecks := 0
	lastObservedProgress := s.workshopLastProgressAt(sessionID)
	lastProgressAt := time.Now()
	waitStartedAt := time.Now()
	// workshopLastProgressAt can only report the START of still-running work —
	// a running execution has no CompletedAt and an in-flight tool call has
	// Duration 0 — so timestamp staleness alone cannot distinguish a stalled
	// turn from a busy one. Ask liveness directly before expiring anything.
	stillWaitingOnLiveChild := func(now time.Time) bool {
		if !s.sessionHasLiveChildWork(sessionID) {
			return false
		}
		if now.Sub(waitStartedAt) >= schedulerWorkshopLiveChildCeiling {
			return false
		}
		scheduleLogf("[SCHEDULER] Workshop turn quiet for %s in session %s but child work is still running; continuing to wait", maxInactivity, sessionID)
		return true
	}
	checkUserInterruption := func() error {
		if activeSession, exists := s.api.getActiveSession(sessionID); exists &&
			normalizeSessionLifecycleStatus(activeSession.Status) == sessionLifecycleFailed {
			return fmt.Errorf("%w: session %s status is %s", errWorkshopSessionFailed, sessionID, activeSession.Status)
		}
		if s.api.isSessionMarkedStopped(sessionID) {
			return fmt.Errorf("%w: session %s was stopped", errWorkshopSequenceInterrupted, sessionID)
		}
		if s.api.consumeSessionTurnInterrupted(sessionID) {
			return fmt.Errorf("%w: current response in session %s was canceled", errWorkshopSequenceInterrupted, sessionID)
		}
		return nil
	}
	if err := checkUserInterruption(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := checkUserInterruption(); err != nil {
				return err
			}
			refreshErr := s.refreshSessionTmuxSnapshotsForIdleCheck(ctx, sessionID)
			now := time.Now()
			if observedProgress := s.workshopLastProgressAt(sessionID); observedProgress.After(lastObservedProgress) {
				lastObservedProgress = observedProgress
				lastProgressAt = now
			}
			// A transient tmux capture failure is not proof that the agent failed.
			// Keep observing other progress signals and only cancel after the same
			// inactivity window has elapsed. Do not count a failed refresh as an
			// idle-completion check because the pane state is not fresh.
			if refreshErr != nil {
				consecutiveIdleChecks = 0
				if maxInactivity > 0 && now.Sub(lastProgressAt) >= maxInactivity {
					if stillWaitingOnLiveChild(now) {
						lastProgressAt = now
						continue
					}
					return fmt.Errorf(
						"%w: no tmux, tool, execution, or session progress for %s in session %s; last tmux refresh error: %w",
						errWorkshopIdleWaitTimeout,
						maxInactivity,
						sessionID,
						refreshErr,
					)
				}
				continue
			}
			// Consolidated status — same busy/idle/stopped the UI sees, so the
			// scheduler doesn't fire the next message while the (possibly tmux-
			// backed) agent is still working.
			if !s.api.sessionIsBusy(sessionID) {
				consecutiveIdleChecks++
				if consecutiveIdleChecks >= schedulerWorkshopIdleConsecutiveChecks {
					return nil
				}
				continue
			}
			consecutiveIdleChecks = 0
			if maxInactivity > 0 && now.Sub(lastProgressAt) >= maxInactivity {
				if stillWaitingOnLiveChild(now) {
					lastProgressAt = now
					continue
				}
				return fmt.Errorf(
					"%w: no tmux, tool, execution, or session progress for %s in session %s (live_child_work=false)",
					errWorkshopIdleWaitTimeout,
					maxInactivity,
					sessionID,
				)
			}
		}
	}
}

// workshopLastProgressAt returns the latest observable activity timestamp for a
// scheduled workshop turn. The inactivity timeout is deliberately sliding: a
// long-running maintenance agent remains healthy while its tmux pane, tool calls,
// tracked execution, or parent session continues to advance.
func (s *SchedulerService) workshopLastProgressAt(sessionID string) time.Time {
	if s == nil || s.api == nil || strings.TrimSpace(sessionID) == "" {
		return time.Time{}
	}
	api := s.api
	latest := time.Time{}
	record := func(candidate time.Time) {
		if candidate.After(latest) {
			latest = candidate
		}
	}

	api.activeSessionsMux.RLock()
	if session := api.activeSessions[sessionID]; session != nil {
		record(session.CreatedAt)
		record(session.LastActivity)
	}
	api.activeSessionsMux.RUnlock()

	if api.terminalStore != nil {
		for _, snapshot := range api.terminalStore.ListMetadata(sessionID) {
			record(snapshot.CreatedAt)
			record(snapshot.UpdatedAt)
		}
	}

	if api.bgAgentRegistry != nil {
		for _, agent := range api.bgAgentRegistry.GetAll(sessionID) {
			if agent == nil {
				continue
			}
			snapshot := agent.GetSnapshot()
			record(snapshot.CreatedAt)
			if snapshot.CompletedAt != nil {
				record(*snapshot.CompletedAt)
			}
			for _, call := range agent.GetRecentToolCalls(1) {
				record(call.StartedAt)
				if call.Duration > 0 {
					record(call.StartedAt.Add(call.Duration))
				}
			}
		}
	}

	for _, execution := range api.trackedExecutionsForSession(sessionID) {
		if execution == nil {
			continue
		}
		record(execution.StartedAt)
		if execution.CompletedAt != nil {
			record(*execution.CompletedAt)
		}
	}

	return latest
}

func (s *SchedulerService) refreshSessionTmuxSnapshotsForIdleCheck(ctx context.Context, sessionID string) error {
	if s == nil || s.api == nil {
		return nil
	}
	return s.api.refreshSessionTmuxSnapshotsForIdleCheck(ctx, sessionID)
}

func (api *StreamingAPI) refreshSessionTmuxSnapshotsForIdleCheck(ctx context.Context, sessionID string) error {
	if api == nil || api.terminalStore == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	seenTmuxSessions := map[string]struct{}{}
	for _, snapshot := range api.terminalStore.ListMetadata(sessionID) {
		tmuxSession := strings.TrimSpace(snapshot.TmuxSession)
		if tmuxSession == "" {
			continue
		}
		if _, seen := seenTmuxSessions[tmuxSession]; seen {
			continue
		}
		seenTmuxSessions[tmuxSession] = struct{}{}

		captureCtx, cancel := context.WithTimeout(ctx, terminalTmuxActionTimeout)
		content, err := captureTerminalPaneLines(captureCtx, tmuxSession, terminalDefaultRefreshLines)
		cancel()
		if err != nil {
			if isMissingTmuxTargetError(err) {
				api.terminalStore.MarkStale(snapshot.TerminalID)
				continue
			}
			return fmt.Errorf("refresh tmux snapshot %q: %w", tmuxSession, err)
		}
		api.terminalStore.ReplaceContentWithSource(snapshot.TerminalID, content, "tmux_capture")
	}
	return nil
}

// updateRuntimeState owns the complete read-modify-write operation. Callers must
// never retain a runtime-state pointer after the mutex is released.
func (s *SchedulerService) updateRuntimeState(scheduleID string, update func(*ScheduleRuntimeState)) ScheduleRuntimeState {
	s.runtimeStatesMu.Lock()
	defer s.runtimeStatesMu.Unlock()
	state, ok := s.runtimeStates[scheduleID]
	if !ok {
		state = &ScheduleRuntimeState{}
		s.runtimeStates[scheduleID] = state
	}
	if update != nil {
		update(state)
	}
	return *state
}

// activateScheduleRunLocked publishes the active run and its cancellation
// handle as one runtime-state operation. Caller must hold runtimeStatesMu.
func (s *SchedulerService) activateScheduleRunLocked(state *ScheduleRuntimeState, runID string, startedAt time.Time) context.Context {
	state.LastStatus = "running"
	state.ActiveRunID = runID
	state.LastRunAt = &startedAt
	state.LastError = ""
	return s.registerScheduleRunContext(runID)
}

func (s *SchedulerService) rollbackScheduleRunActivation(runtimeKey, runID string, previous ScheduleRuntimeState) {
	s.runtimeStatesMu.Lock()
	if current := s.runtimeStates[runtimeKey]; current != nil && current.ActiveRunID == runID {
		*current = previous
	}
	s.runtimeStatesMu.Unlock()
	s.releaseScheduleRunContext(runID)
}

func (s *SchedulerService) abortCanceledScheduleRunBeforeStart(ctx context.Context, sctx *ScheduleContext, runtimeKey, runID string) bool {
	if ctx.Err() == nil {
		return false
	}
	s.updateRuntimeState(runtimeKey, func(state *ScheduleRuntimeState) {
		if state.ActiveRunID != runID {
			return
		}
		state.ActiveRunID = ""
		state.LastStatus = "stopped"
		state.LastError = "stopped before execution started"
	})
	s.transitionScheduleRun(context.Background(), sctx, schedulerstate.Transition{
		RunID: runID, To: schedulerstate.StateStopped, Reason: "stopped before execution started",
		ErrorMessage: "stopped before execution started", At: time.Now().UTC(),
	})
	s.releaseScheduleRunContext(runID)
	s.cleanupRemovedScheduleRuntimeState(runtimeKey)
	return true
}

func (s *SchedulerService) cleanupRemovedScheduleRuntimeState(runtimeKey string) {
	s.mu.Lock()
	_, known := s.scheduleFingerprints[runtimeKey]
	s.mu.Unlock()
	if known {
		return
	}
	s.runtimeStatesMu.Lock()
	if state := s.runtimeStates[runtimeKey]; state == nil || state.ActiveRunID == "" {
		delete(s.runtimeStates, runtimeKey)
	}
	s.runtimeStatesMu.Unlock()
}

func (s *SchedulerService) registerScheduleRunContext(runID string) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	s.runCancelsMu.Lock()
	if s.runCancels == nil {
		s.runCancels = make(map[string]context.CancelFunc)
	}
	if previous := s.runCancels[runID]; previous != nil {
		previous()
	}
	s.runCancels[runID] = cancel
	s.runCancelsMu.Unlock()
	return ctx
}

func (s *SchedulerService) cancelScheduleRunContext(runID string) {
	s.runCancelsMu.Lock()
	cancel := s.runCancels[runID]
	s.runCancelsMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *SchedulerService) releaseScheduleRunContext(runID string) {
	s.runCancelsMu.Lock()
	cancel := s.runCancels[runID]
	delete(s.runCancels, runID)
	s.runCancelsMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *SchedulerService) claimScheduleRun(ctx context.Context, sctx *ScheduleContext, runID string, startedAt time.Time) error {
	s.stateStoreMu.RLock()
	defer s.stateStoreMu.RUnlock()
	if s.stateStore == nil {
		return nil
	}
	scopeType, scopeID, lockKey := scheduleStateScope(sctx)
	triggerSource := strings.TrimSpace(sctx.TriggerSource)
	if triggerSource == "" {
		triggerSource = "cron"
	}
	return s.stateStore.BeginRun(ctx, schedulerstate.Run{
		RunID:         runID,
		ScopeType:     scopeType,
		ScopeID:       scopeID,
		LockKey:       lockKey,
		ScheduleID:    sctx.Schedule.ID,
		TriggerSource: triggerSource,
		StartedAt:     startedAt,
	})
}

func (s *SchedulerService) transitionScheduleRun(ctx context.Context, sctx *ScheduleContext, transition schedulerstate.Transition) {
	if strings.TrimSpace(transition.RunID) == "" {
		return
	}
	s.stateStoreMu.RLock()
	defer s.stateStoreMu.RUnlock()
	if s.stateStore == nil {
		return
	}
	transitionCtx := ctx
	if transitionCtx == nil {
		transitionCtx = context.Background()
	}
	attempts := 1
	if schedulerstate.IsTerminal(transition.To) {
		transitionCtx = context.WithoutCancel(transitionCtx)
		attempts = 3
	}
	var transitionErr error
	for attempt := 0; attempt < attempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(transitionCtx, 5*time.Second)
		transitionErr = s.stateStore.Transition(attemptCtx, transition)
		cancel()
		if transitionErr == nil {
			return
		}
		if attempt+1 < attempts {
			time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
		}
	}
	if schedulerstate.IsTerminal(transition.To) {
		recoveryCtx, cancel := context.WithTimeout(transitionCtx, 5*time.Second)
		recoveryErr := s.stateStore.ForceTerminal(recoveryCtx, transition)
		cancel()
		if recoveryErr == nil {
			if sctx != nil {
				s.logf(sctx, "[SCHEDULER_STATE] recovered terminal transition run=%s to=%s after error: %v", transition.RunID, transition.To, transitionErr)
			} else {
				scheduleLogf("[SCHEDULER_STATE] recovered terminal transition run=%s to=%s after error: %v", transition.RunID, transition.To, transitionErr)
			}
			return
		}
		transitionErr = errors.Join(transitionErr, recoveryErr)
	}
	if transitionErr != nil {
		if sctx != nil {
			s.logf(sctx, "[SCHEDULER_STATE] transition run=%s to=%s failed: %v", transition.RunID, transition.To, transitionErr)
		} else {
			scheduleLogf("[SCHEDULER_STATE] transition run=%s to=%s failed: %v", transition.RunID, transition.To, transitionErr)
		}
	}
}

func (s *SchedulerService) recordScheduleFireDecision(ctx context.Context, sctx *ScheduleContext, decision, reason, runID string, firedAt time.Time) error {
	if sctx == nil {
		return errors.New("schedule context is required")
	}
	s.stateStoreMu.RLock()
	defer s.stateStoreMu.RUnlock()
	if s.stateStore == nil {
		return errors.New("schedule state store is unavailable")
	}
	scopeType, scopeID, _ := scheduleStateScope(sctx)
	triggerSource := strings.TrimSpace(sctx.TriggerSource)
	if triggerSource == "" {
		triggerSource = "cron"
	}
	scheduledFor := sctx.ScheduledFor
	if scheduledFor.IsZero() {
		scheduledFor = firedAt
	}
	if err := s.stateStore.RecordFireDecision(ctx, schedulerstate.FireDecision{
		DecisionID:    uuid.NewString(),
		ScopeType:     scopeType,
		ScopeID:       scopeID,
		ScheduleID:    sctx.Schedule.ID,
		TriggerSource: triggerSource,
		Decision:      decision,
		Reason:        reason,
		RunID:         runID,
		ScheduledFor:  scheduledFor.UTC(),
		FiredAt:       firedAt,
	}); err != nil {
		s.logf(sctx, "[SCHEDULER_STATE] record fire decision=%s failed: %v", decision, err)
		return err
	}
	return nil
}

// getRuntimeStateLocked returns or creates runtime state. Caller MUST hold runtimeStatesMu write lock.
func (s *SchedulerService) getRuntimeStateLocked(scheduleID string) *ScheduleRuntimeState {
	if state, ok := s.runtimeStates[scheduleID]; ok {
		return state
	}
	state := &ScheduleRuntimeState{}
	s.runtimeStates[scheduleID] = state
	return state
}

func runningScheduleInSetLocked(runtimeStates map[string]*ScheduleRuntimeState, scheduleIDs []string, ignoreScheduleID string) (string, string) {
	for _, scheduleID := range scheduleIDs {
		if scheduleID == "" || scheduleID == ignoreScheduleID {
			continue
		}
		state := runtimeStates[scheduleID]
		if state == nil || state.LastStatus != "running" {
			continue
		}
		return scheduleID, state.LastSessionID
	}
	return "", ""
}

func (s *SchedulerService) findActiveNonBuilderExecutionForWorkspace(workspacePath string) *ActiveWorkflowExecution {
	if s == nil || s.api == nil || strings.TrimSpace(workspacePath) == "" {
		return nil
	}

	tracked := s.api.findRunningTrackedExecutionForWorkspaceWhere(workspacePath, func(exec *TrackedWorkflowExecution) bool {
		return trackedExecutionBlocksScheduledWorkflow(exec)
	})
	if tracked == nil {
		return nil
	}
	active := trackedExecutionToActive(tracked)
	return &active
}

// ScheduleSearchResult holds the result of finding a schedule by ID.
type ScheduleSearchResult struct {
	SourceType    string // "workflow" or "multi-agent"
	WorkspacePath string // For workflow schedules
	Manifest      *WorkflowManifest
	UserID        string // For multi-agent schedules
	ScheduleFile  *MultiAgentScheduleFile
	Index         int
}

// findScheduleByIDAny scans both workflow manifests and multi-agent schedule files.
func findScheduleByIDAny(ctx context.Context, scheduleID string) (*ScheduleSearchResult, error) {
	// Try workflow manifests first
	wsPath, manifest, idx, err := findScheduleByID(ctx, scheduleID)
	if err == nil {
		return &ScheduleSearchResult{
			SourceType:    "workflow",
			WorkspacePath: wsPath,
			Manifest:      manifest,
			Index:         idx,
		}, nil
	}

	// Try multi-agent schedules
	userID, f, idx, err := findMultiAgentScheduleByID(ctx, scheduleID)
	if err == nil {
		return &ScheduleSearchResult{
			SourceType:   "multi-agent",
			UserID:       userID,
			ScheduleFile: f,
			Index:        idx,
		}, nil
	}

	return nil, fmt.Errorf("schedule %s not found", scheduleID)
}

func findBuiltinMultiAgentScheduleForUser(ctx context.Context, userID, scheduleID string) (*ScheduleSearchResult, error) {
	if strings.TrimSpace(userID) == "" {
		userID = GetDefaultUserID()
	}

	sched, ok := FindDefaultBuiltinSchedule(scheduleID)
	if !ok {
		return nil, fmt.Errorf("built-in schedule %s not found", scheduleID)
	}

	f, _, err := ReadMultiAgentSchedules(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to read multi-agent schedules for user %s: %w", userID, err)
	}

	f.Schedules = append(f.Schedules, sched)
	return &ScheduleSearchResult{
		SourceType:   "multi-agent",
		UserID:       userID,
		ScheduleFile: f,
		Index:        len(f.Schedules) - 1,
	}, nil
}

// findScheduleByID scans all workspace manifests to find a schedule by ID.
// Returns (workspacePath, manifest, scheduleIndex, error).
func findScheduleByID(ctx context.Context, scheduleID string) (string, *WorkflowManifest, int, error) {
	discovered, err := DiscoverWorkflowManifests(ctx)
	if err != nil {
		return "", nil, 0, fmt.Errorf("cannot scan workflow directory: %w", err)
	}

	for _, item := range discovered {
		for i, sched := range item.Manifest.Schedules {
			if sched.ID == scheduleID {
				return item.WorkspacePath, item.Manifest, i, nil
			}
		}
	}

	return "", nil, 0, fmt.Errorf("schedule %s not found in any manifest", scheduleID)
}

// getNextRunTime calculates the next scheduled run time.
func getNextRunTime(cronExpr string, timezone string) *time.Time {
	loc, err := time.LoadLocation(scheduleTimezoneOrDefault(timezone))
	if err != nil {
		loc = time.UTC
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return nil
	}

	next := schedule.Next(time.Now().In(loc)).UTC()
	return &next
}

func buildScheduleCronExpression(cronExpr string, timezone string) string {
	return fmt.Sprintf("CRON_TZ=%s %s", scheduleTimezoneOrDefault(timezone), cronExpr)
}

func scheduleTypeOrDefault(scheduleType string) string {
	if scheduleType == "" {
		return "cron"
	}
	return scheduleType
}

func calendarItemRunTime(sched WorkflowSchedule, item CalendarScheduleItem) (time.Time, error) {
	loc, err := time.LoadLocation(scheduleTimezoneOrDefault(sched.Timezone))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone %q: %w", sched.Timezone, err)
	}
	if item.Date == "" || item.Time == "" {
		return time.Time{}, fmt.Errorf("calendar item date and time are required")
	}
	local, err := time.ParseInLocation("2006-01-02 15:04", item.Date+" "+item.Time, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid calendar item %q %q: expected date YYYY-MM-DD and time HH:MM", item.Date, item.Time)
	}
	return local.UTC(), nil
}

func getNextRunTimeForCalendar(sched WorkflowSchedule) *time.Time {
	now := time.Now().UTC()
	var next *time.Time
	for _, item := range sched.CalendarItems {
		runAt, err := calendarItemRunTime(sched, item)
		if err != nil || !runAt.After(now) {
			continue
		}
		if next == nil || runAt.Before(*next) {
			runAtCopy := runAt
			next = &runAtCopy
		}
	}
	return next
}

func scheduleWithCalendarItem(sched WorkflowSchedule, item CalendarScheduleItem) WorkflowSchedule {
	sched.TriggerPayload = item.TriggerPayload
	if len(item.Messages) > 0 {
		sched.Messages = item.Messages
	}
	return sched
}

func scheduleWithReloadedCalendarItem(sched WorkflowSchedule, requested *CalendarScheduleItem) (WorkflowSchedule, *CalendarScheduleItem, bool) {
	if requested == nil {
		return sched, nil, true
	}
	for i := range sched.CalendarItems {
		item := sched.CalendarItems[i]
		matches := requested.ID != "" && item.ID == requested.ID
		if requested.ID == "" {
			matches = item.Date == requested.Date && item.Time == requested.Time
		}
		if !matches {
			continue
		}
		itemCopy := item
		return scheduleWithCalendarItem(sched, itemCopy), &itemCopy, true
	}
	return sched, nil, false
}

func scheduleConfigFingerprint(sctx *ScheduleContext) string {
	if sctx == nil {
		return ""
	}
	payload, err := json.Marshal(struct {
		Schedule     WorkflowSchedule     `json:"schedule"`
		Capabilities WorkflowCapabilities `json:"capabilities"`
	}{Schedule: sctx.Schedule, Capabilities: sctx.Capabilities})
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(payload))
}

func ValidateScheduleTimezone(timezone string) error {
	if timezone == "" {
		return fmt.Errorf("timezone is required; use an IANA timezone like UTC, Asia/Kolkata, or America/New_York")
	}
	if timezone != "UTC" && !strings.Contains(timezone, "/") {
		return fmt.Errorf("invalid timezone %q: use an IANA timezone like UTC, Asia/Kolkata, or America/New_York; abbreviations like EST, PST, or IST are not accepted", timezone)
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return fmt.Errorf("invalid timezone %q: use an IANA timezone like UTC, Asia/Kolkata, or America/New_York", timezone)
	}
	return nil
}

// ValidateCronExpression validates a 5-field cron expression.
func ValidateCronExpression(expr string) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(expr)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}
	return nil
}

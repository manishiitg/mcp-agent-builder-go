import React from 'react'
import {
  Play, Trash2, Clock, Loader, Pause,
  ChevronDown, ChevronRight, Square, MoreHorizontal
} from 'lucide-react'
import type { ScheduledJob } from '../../../services/api-types'
import { ScheduleExecutionHistoryList } from '../../ScheduleExecutionHistoryList'
import { describeCron } from './cron'
import {
  formatExactDateTime,
  formatLastRunLabel,
  formatLocalScheduleTime,
  formatMissedScheduleReason,
  formatOverdueDuration,
  getLocalizedJobName,
  getMissedScheduleDelayMs,
  getScheduleExecutionScope,
  isMissedSchedule,
  isScheduleIssueStatus,
  isSchedulePartialStatus,
  isScheduleWaitingStatus,
} from './helpers'
import type { ScheduleRunsPanelState } from './useScheduleRunsData'

type WorkflowScheduleRowProps = {
  job: ScheduledJob
  panel: ScheduleRunsPanelState
}

export const WorkflowScheduleRow: React.FC<WorkflowScheduleRowProps> = ({ job, panel }) => {
  const {
    presetMap,
    isReadOnlyUser,
    openActionMenuJobId,
    setOpenActionMenuJobId,
    triggering,
    isWorkflowScoped,
    runsByJob,
    expandedRunHistoryJobIds,
    runsLoadingJobIds,
    deletingRunSessionIds,
    handleStopRun,
    handleTrigger,
    handleToggle,
    handleDelete,
    toggleRunHistory,
    openScheduledRun,
    deleteScheduledRunSession,
  } = panel

  const preset = presetMap.get(job.preset_query_id ?? '')
  const cronDesc = describeCron(job.cron_expression)
  const localizedJobName = getLocalizedJobName(job)
  const isRunningJob = job.last_status === 'running'
  const isWaitingJob = isScheduleWaitingStatus(job.last_status)
  const isMissedJob = isMissedSchedule(job)
  const missedDelayMs = getMissedScheduleDelayMs(job)
  const missedReason = isMissedJob ? formatMissedScheduleReason(job) : ''
  const hasWorkspace = !!job.workspace_path || !!preset?.workspacePath
  const executionScope = getScheduleExecutionScope(job)

  return (
    <div className={`px-4 py-3 ${!job.enabled ? 'opacity-65' : ''}`}>
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex items-start gap-3">
            <div className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${
              isRunningJob ? 'bg-amber-500 animate-pulse' :
              isWaitingJob ? 'bg-sky-500 animate-pulse' :
              isMissedJob ? 'bg-amber-500' :
              job.last_status === 'error' ? 'bg-red-500' :
              isSchedulePartialStatus(job.last_status) ? 'bg-amber-500' :
              job.enabled ? 'bg-green-500' : 'bg-gray-300 dark:bg-gray-600'
            }`} />
            <div className="min-w-0 flex-1">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <span className="truncate text-sm font-medium text-foreground" title={job.name}>
                  {localizedJobName}
                </span>
                {isRunningJob && (
                  <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-1.5 py-0.5 text-[11px] font-medium text-amber-700 dark:text-amber-300">
                    Running
                  </span>
                )}
                {isWaitingJob && (
                  <span className="rounded-full border border-sky-500/30 bg-sky-500/10 px-1.5 py-0.5 text-[11px] font-medium text-sky-700 dark:text-sky-300">
                    {job.last_status === 'waiting_for_capacity'
                      ? 'Waiting for capacity'
                      : `Queued${job.queued_occurrences && job.queued_occurrences > 1 ? ` · ${job.queued_occurrences} combined` : ''}`}
                  </span>
                )}
                {isMissedJob && (
                  <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-1.5 py-0.5 text-[11px] font-medium text-amber-700 dark:text-amber-300">
                    Missed
                  </span>
                )}
                {!job.enabled && (
                  <span className="rounded-full border border-border bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
                    Paused
                  </span>
                )}
                {job.last_status === 'error' && (
                  <span className="rounded-full border border-red-500/30 bg-red-500/10 px-1.5 py-0.5 text-[11px] font-medium text-red-600 dark:text-red-300">
                    Issue
                  </span>
                )}
                {isSchedulePartialStatus(job.last_status) && (
                  <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-1.5 py-0.5 text-[11px] font-medium text-amber-700 dark:text-amber-300">
                    {job.last_status === 'partial' ? 'Partial' : 'Interrupted'}
                  </span>
                )}
              </div>

              <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
                <span className="flex items-center gap-1">
                  <Clock className="h-3 w-3" />
                  {cronDesc}
                </span>
                <span>
                  {job.enabled
                    ? isMissedJob && missedDelayMs != null
                      ? job.missed_run_count && job.missed_run_count > 1
                        ? `${job.missed_run_count} missed · latest ${formatOverdueDuration(missedDelayMs)} ago`
                        : `Missed by ${formatOverdueDuration(missedDelayMs)}`
                      : `Next ${formatLocalScheduleTime(job.next_run_at)}`
                    : 'Paused'}
                </span>
                {isMissedJob && (
                  <span className="text-amber-700 dark:text-amber-300" title={missedReason}>
                    {missedReason}
                  </span>
                )}
                <span title={formatExactDateTime(job.last_run_at)}>
                  {isWaitingJob
                    ? job.waiting_until
                      ? `Waiting until ${formatLocalScheduleTime(job.waiting_until)}`
                      : 'Waiting to start'
                    : isRunningJob
                    ? job.last_run_at
                      ? `Running since ${formatLocalScheduleTime(job.last_run_at)}`
                      : 'Running now'
                    : `Last ran ${formatLastRunLabel(job.last_run_at)}`}
                </span>
                <span>{job.run_count} run{job.run_count !== 1 ? 's' : ''}</span>
                {job.group_names && job.group_names.length > 0 && (
                  <span className="truncate" title={`Groups: ${job.group_names.join(', ')}`}>
                    Groups: {job.group_names.join(', ')}
                  </span>
                )}
                {executionScope && (
                  <span className="rounded-full bg-purple-100 px-1.5 py-0.5 text-[11px] font-medium text-purple-600 dark:bg-purple-900/30 dark:text-purple-300" title={executionScope.title}>
                    {executionScope.label}
                  </span>
                )}
                {!hasWorkspace && (
                  <span className="text-amber-600 dark:text-amber-300">Workspace missing</span>
                )}
              </div>

              {isScheduleIssueStatus(job.last_status) && job.last_error && (
                <div
                  className={`mt-1 truncate text-xs ${isSchedulePartialStatus(job.last_status) ? 'text-amber-600 dark:text-amber-400' : 'text-red-500'}`}
                  title={job.last_error}
                >
                  {job.last_error}
                </div>
              )}
              {isWaitingJob && job.waiting_reason && (
                <div className="mt-1 truncate text-xs text-sky-700 dark:text-sky-300" title={job.waiting_reason}>
                  {job.waiting_reason}
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-1 lg:justify-end">
          {!isReadOnlyUser && (
            job.enabled ? (
              isRunningJob ? (
                <button
                  type="button"
                  onClick={() => handleStopRun(job)}
                  className="inline-flex items-center gap-1 rounded-md border border-red-200 bg-red-50 px-2 py-1 text-xs font-medium text-red-600 transition-colors hover:bg-red-100 dark:border-red-800 dark:bg-red-900/30 dark:text-red-400 dark:hover:bg-red-900/50"
                >
                  <Square className="h-3 w-3" />
                  Stop
                </button>
              ) : (
                <button
                  type="button"
                  onClick={() => handleTrigger(job)}
                  disabled={triggering === job.id}
                  className={`inline-flex items-center gap-1 rounded-md border px-2 py-1 text-xs font-medium transition-colors disabled:opacity-40 ${
                    isMissedJob
                      ? 'border-amber-200 bg-amber-50 text-amber-700 hover:bg-amber-100 dark:border-amber-800 dark:bg-amber-900/30 dark:text-amber-300 dark:hover:bg-amber-900/50'
                      : 'border-border bg-background text-muted-foreground hover:bg-muted hover:text-green-600'
                  }`}
                >
                  <Play className="h-3 w-3" />
                  Run now
                </button>
              )
            ) : (
              <button
                type="button"
                onClick={() => handleToggle(job)}
                className="inline-flex items-center gap-1 rounded-md border border-green-200 bg-green-50 px-2 py-1 text-xs font-medium text-green-600 transition-colors hover:bg-green-100 dark:border-green-800 dark:bg-green-900/30 dark:text-green-400 dark:hover:bg-green-900/50"
              >
                <Play className="h-3 w-3" />
                Resume
              </button>
            )
          )}
          <div className="relative">
            <button
              type="button"
              aria-label="More schedule actions"
              aria-expanded={openActionMenuJobId === job.id}
              onPointerDown={(event) => event.stopPropagation()}
              onClick={(event) => {
                event.stopPropagation()
                setOpenActionMenuJobId((openId) => openId === job.id ? null : job.id)
              }}
              className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <MoreHorizontal className="h-3.5 w-3.5" />
            </button>
            {openActionMenuJobId === job.id && (
              <div role="menu" onPointerDown={(event) => event.stopPropagation()} className="absolute right-0 top-8 z-30 w-36 rounded-md border border-border bg-popover p-1 shadow-lg">
                {!isReadOnlyUser && job.enabled && !isRunningJob && (
                  <button type="button" role="menuitem" onClick={() => { setOpenActionMenuJobId(null); handleToggle(job) }} className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-popover-foreground hover:bg-muted">
                    <Pause className="h-3.5 w-3.5" /> Pause schedule
                  </button>
                )}
                {!isReadOnlyUser && (
                  <button type="button" role="menuitem" onClick={() => { setOpenActionMenuJobId(null); handleDelete(job) }} className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-red-600 hover:bg-red-500/10 dark:text-red-400">
                    <Trash2 className="h-3.5 w-3.5" /> Delete schedule
                  </button>
                )}
              </div>
            )}
          </div>
        </div>
      </div>

      {isWorkflowScoped && (
        <ScheduleExecutionHistoryList
          job={job}
          runs={runsByJob[job.id] ?? []}
          historyOpen={expandedRunHistoryJobIds.has(job.id)}
          historyLoading={runsLoadingJobIds.has(job.id)}
          recordedRunCount={job.run_count ?? (runsByJob[job.id]?.length ?? 0)}
          onToggle={() => void toggleRunHistory(job)}
          onOpen={run => void openScheduledRun(run, job)}
          onDelete={isReadOnlyUser ? undefined : run => void deleteScheduledRunSession(run)}
          deletingRunIds={deletingRunSessionIds}
        />
      )}
    </div>
  )
}

type ScheduleGroupsViewProps = {
  panel: ScheduleRunsPanelState
}

export const ScheduleGroupsView: React.FC<ScheduleGroupsViewProps> = ({ panel }) => {
  const {
    workflowGroups,
    normalizedSearch,
    activeFilter,
    selectedWorkflowFilter,
    expandedWorkflowKeys,
    toggleWorkflowGroup,
    isReadOnlyUser,
    handleToggleWorkflowGroupPause,
    bulkUpdatingGroupKey,
  } = panel

  return (
    <div className="space-y-3 px-5 py-4">
      {workflowGroups.map((group) => {
        const forcedOpen = !!normalizedSearch || activeFilter !== 'all' || selectedWorkflowFilter !== 'all' || group.running > 0
        const isExpanded = forcedOpen || expandedWorkflowKeys.includes(group.key)
        const groupStatusClass = group.running > 0
          ? 'bg-amber-500 animate-pulse'
          : group.missed > 0
            ? 'bg-amber-500'
            : group.issues > 0
              ? 'bg-red-500'
              : group.enabled > 0
                ? 'bg-green-500'
                : 'bg-gray-300 dark:bg-gray-600'

        return (
          <div key={group.key} className="overflow-hidden rounded-lg border border-border bg-background">
            <div
              role="button"
              tabIndex={0}
              onClick={() => toggleWorkflowGroup(group.key)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  toggleWorkflowGroup(group.key)
                }
              }}
              className="w-full cursor-pointer px-4 py-3 text-left transition-colors hover:bg-muted/40"
            >
              <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                <div className="flex min-w-0 items-start gap-3">
                  <div className={`mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full ${groupStatusClass}`} />
                  <div className="min-w-0">
                    <div className="flex min-w-0 items-center gap-2">
                      <span className="truncate text-sm font-semibold text-foreground" title={group.label}>
                        {group.label}
                      </span>
                      {isExpanded ? (
                        <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      ) : (
                        <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      )}
                    </div>
                    <div className="mt-1 truncate text-xs text-muted-foreground" title={group.workspacePath}>
                      {group.workspacePath || 'Workspace path not recorded'}
                    </div>
                  </div>
                </div>

                <div className="flex flex-wrap items-center gap-1.5 lg:justify-end">
                  {!isReadOnlyUser && (
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        handleToggleWorkflowGroupPause(group)
                      }}
                      disabled={bulkUpdatingGroupKey === group.key}
                      title={group.enabled > 0
                        ? `Pause all ${group.enabled} active schedule${group.enabled === 1 ? '' : 's'} in this automation`
                        : `Resume all ${group.paused} paused schedule${group.paused === 1 ? '' : 's'} in this automation`}
                      className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium transition-colors disabled:opacity-60 ${
                        group.enabled > 0
                          ? 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300 hover:bg-amber-500/20'
                          : 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/20'
                      }`}
                    >
                      {bulkUpdatingGroupKey === group.key ? (
                        <Loader className="h-3 w-3 animate-spin" />
                      ) : group.enabled > 0 ? (
                        <Pause className="h-3 w-3" />
                      ) : (
                        <Play className="h-3 w-3" />
                      )}
                      {group.enabled > 0 ? 'Pause all' : 'Resume all'}
                    </button>
                  )}
                  <span className="rounded-full border border-border bg-card px-2 py-0.5 text-xs text-muted-foreground">
                    {group.jobs.length} schedule{group.jobs.length === 1 ? '' : 's'}
                  </span>
                  {group.running > 0 && (
                    <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-300">
                      {group.running} running
                    </span>
                  )}
                  {group.missed > 0 && (
                    <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-300">
                      {group.missed} missed
                    </span>
                  )}
                  {group.issues > 0 && (
                    <span className="rounded-full border border-red-500/30 bg-red-500/10 px-2 py-0.5 text-xs font-medium text-red-600 dark:text-red-300">
                      {group.issues} issue{group.issues === 1 ? '' : 's'}
                    </span>
                  )}
                  {group.paused > 0 && (
                    <span className="rounded-full border border-border bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                      {group.paused} paused
                    </span>
                  )}
                </div>
              </div>

              <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 pl-5 text-xs text-muted-foreground">
                <span>{group.enabled} active</span>
                <span>{group.runCount} total run{group.runCount === 1 ? '' : 's'}</span>
                <span>Next {formatLocalScheduleTime(group.nextRunAt)}</span>
                <span title={formatExactDateTime(group.lastRunAt)}>
                  Last ran {formatLastRunLabel(group.lastRunAt)}
                </span>
              </div>
            </div>

            {isExpanded && (
              <div className="divide-y divide-border border-t border-border bg-card/40">
                {group.jobs.map((job) => (
                  <WorkflowScheduleRow key={job.id} job={job} panel={panel} />
                ))}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

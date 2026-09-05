import React from 'react'
import { AlertTriangle, ChevronRight, Clock, Pause, Play, Radio } from 'lucide-react'
import { formatExactDateTime, formatLastRunLabel, formatLocalScheduleTime } from './helpers'
import type { ScheduleRunsPanelState } from './useScheduleRunsData'

type ScheduleGroupsViewProps = {
  panel: Pick<ScheduleRunsPanelState,
    | 'workflowGroups' | 'isReadOnlyUser' | 'setActiveFilter' | 'setActiveView'
    | 'setSelectedWorkflowFilter' | 'handleToggleWorkflowGroupPause' | 'bulkUpdatingGroupKey'
  >
}

/** Global scheduling is a workflow-level view. Per-schedule detail belongs in All Schedules. */
export const ScheduleGroupsView: React.FC<ScheduleGroupsViewProps> = ({ panel }) => {
  const {
    workflowGroups,
    isReadOnlyUser,
    setActiveFilter,
    setActiveView,
    setSelectedWorkflowFilter,
    handleToggleWorkflowGroupPause,
    bulkUpdatingGroupKey,
  } = panel

  const openWorkflowSchedules = (workflowKey: string) => {
    setSelectedWorkflowFilter(workflowKey)
    setActiveFilter('all')
    setActiveView('schedules')
  }

  return (
    <div className="grid gap-3 px-5 py-4 md:grid-cols-2 xl:grid-cols-3">
      {workflowGroups.map((group) => {
        const fullyPaused = group.enabled === 0
        const partlyPaused = !fullyPaused && group.paused > 0
        const hasAttention = group.issues > 0 || group.missed > 0
        const isRunning = group.running > 0
        const stateLabel = fullyPaused ? 'Fully paused' : partlyPaused ? 'Partly paused' : 'Active'
        const statusDotClass = isRunning
          ? 'bg-amber-500 animate-pulse'
          : hasAttention
            ? 'bg-red-500'
            : fullyPaused
              ? 'bg-muted-foreground/50'
              : partlyPaused
                ? 'bg-amber-500'
                : 'bg-emerald-500'

        return (
          <article key={group.key} className="flex min-h-48 flex-col rounded-xl border border-border bg-background p-4 shadow-sm">
            <div className="flex items-start gap-3">
              <span className={`mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full ${statusDotClass}`} />
              <div className="min-w-0 flex-1">
                <h3 className="truncate text-sm font-semibold text-foreground" title={group.label}>{group.label}</h3>
                <p className="mt-1 truncate text-xs text-muted-foreground" title={group.workspacePath}>
                  {group.workspacePath || 'Workspace path not recorded'}
                </p>
              </div>
            </div>

            <div className="mt-4 flex flex-wrap items-center gap-1.5">
              <span className={`rounded-full border px-2 py-0.5 text-xs font-medium ${
                fullyPaused
                  ? 'border-border bg-muted text-muted-foreground'
                  : partlyPaused
                    ? 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
                    : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
              }`}>{stateLabel}</span>
              {isRunning && (
                <span className="inline-flex items-center gap-1 rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-300">
                  <Radio className="h-3 w-3" /> Running now
                </span>
              )}
              {hasAttention && (
                <span className="inline-flex items-center gap-1 rounded-full border border-red-500/30 bg-red-500/10 px-2 py-0.5 text-xs font-medium text-red-600 dark:text-red-300">
                  <AlertTriangle className="h-3 w-3" /> Needs attention
                </span>
              )}
            </div>

            <div className="mt-4 space-y-1.5 text-xs text-muted-foreground">
              <div className="flex items-center gap-1.5">
                <Clock className="h-3.5 w-3.5 shrink-0" />
                {fullyPaused ? 'No scheduled runs while paused' : `Next ${formatLocalScheduleTime(group.nextRunAt)}`}
              </div>
              <div title={formatExactDateTime(group.lastRunAt)}>Last activity {formatLastRunLabel(group.lastRunAt)}</div>
            </div>

            <div className="mt-auto flex items-center justify-between gap-2 pt-4">
              {!isReadOnlyUser && (
                <button
                  type="button"
                  onClick={() => handleToggleWorkflowGroupPause(group)}
                  disabled={bulkUpdatingGroupKey === group.key}
                  className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs font-medium transition-colors disabled:opacity-60 ${
                    fullyPaused
                      ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/20 dark:text-emerald-300'
                      : 'border-amber-500/40 bg-amber-500/10 text-amber-700 hover:bg-amber-500/20 dark:text-amber-300'
                  }`}
                >
                  {fullyPaused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
                  {fullyPaused ? 'Resume workflow' : 'Pause workflow'}
                </button>
              )}
              <button type="button" onClick={() => openWorkflowSchedules(group.key)} className="ml-auto inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline">
                View schedules <ChevronRight className="h-3.5 w-3.5" />
              </button>
            </div>
          </article>
        )
      })}
    </div>
  )
}

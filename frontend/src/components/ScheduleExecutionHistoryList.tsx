import { ChevronDown, ChevronRight, Loader2 } from 'lucide-react'
import type { ChatHistorySession, ScheduledJob, ScheduledJobRun } from '../services/api-types'
import { ScheduleRunCard } from './ScheduleRunCard'

interface ScheduleExecutionHistoryListProps {
  job: ScheduledJob
  runs: ScheduledJobRun[]
  historyOpen: boolean
  historyLoading: boolean
  recordedRunCount: number
  onToggle: () => void
  // Only a caller that already has resolved ChatHistorySession objects (today,
  // just PreviousChatHistoryPanel) can supply this to enrich "latest agent
  // update" and the started-with fallback with real conversation content.
  // Without it, the card still renders correctly: scheduleRunStartMessage
  // falls back to the job's configured message, and the outcome line falls
  // back to the status presentation's detail text.
  resolveSession?: (run: ScheduledJobRun) => ChatHistorySession | undefined
  onOpen: (run: ScheduledJobRun) => void
  onDelete?: (run: ScheduledJobRun) => void
  deletingRunIds: Set<string>
  compact?: boolean
}

export function ScheduleExecutionHistoryList({
  job,
  runs,
  historyOpen,
  historyLoading,
  recordedRunCount,
  onToggle,
  resolveSession,
  onOpen,
  onDelete,
  deletingRunIds,
  compact = false,
}: ScheduleExecutionHistoryListProps) {
  return (
    <>
      <div className="mt-2">
        <button
          type="button"
          onClick={onToggle}
          className="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          {historyLoading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : historyOpen ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          <span>{historyOpen ? 'Hide execution history' : `Show all ${recordedRunCount} recorded execution${recordedRunCount === 1 ? '' : 's'}`}</span>
        </button>
      </div>
      {historyOpen && !historyLoading && (
        <div className="mt-2 divide-y divide-border/70 rounded border border-border bg-muted/10">
          {runs.length === 0 ? (
            <div className="px-3 py-2 text-xs text-muted-foreground">No execution record exists for this schedule yet.</div>
          ) : runs.map(run => (
            <div key={run.id} className="px-3 py-3">
              <ScheduleRunCard
                job={job}
                run={run}
                resolveSession={resolveSession}
                onOpen={onOpen}
                onDelete={onDelete}
                deletingRunIds={deletingRunIds}
                compact={compact}
              />
            </div>
          ))}
        </div>
      )}
    </>
  )
}

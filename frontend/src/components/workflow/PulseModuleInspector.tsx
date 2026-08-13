import { useMemo, useState } from 'react'
import { FileText, ShieldCheck } from 'lucide-react'
import type { PulseReviewRecord } from '../../services/api-types'
import { PulseReviewHistory } from './PulseReviewHistory'

type InspectorView = 'summary' | 'history'

function formatDate(value?: string): string {
  if (!value) return 'Not recorded'
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
}

function readableStatus(value?: string): string {
  const status = (value || '').trim()
  return status ? status.replaceAll('_', ' ') : 'Recorded'
}

function reviewTone(status?: string): string {
  switch ((status || '').toLowerCase()) {
    case 'completed':
    case 'clean':
    case 'done':
      return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    case 'failed':
    case 'blocked':
    case 'contract_failed':
      return 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300'
    default:
      return 'border-border bg-muted text-muted-foreground'
  }
}

/**
 * Reviewer details contain only compact run receipts. Findings, fixes,
 * verification, and activity live in the canonical Issues section.
 */
export function PulseModuleInspector({
  workspacePath,
  module,
  label,
  reviews,
  className = '',
}: {
  workspacePath: string
  module: string
  label: string
  reviews: PulseReviewRecord[]
  className?: string
}) {
  const [view, setView] = useState<InspectorView>('summary')
  const moduleReviews = useMemo(
    () => [...reviews].sort((a, b) => b.recorded_at.localeCompare(a.recorded_at)),
    [reviews],
  )
  const latestReview = moduleReviews[0] || null

  return (
    <section className={`min-w-0 ${className}`} aria-label={`${label} review details`}>
      <div className="flex min-w-0 items-center gap-1 border-b bg-muted/20 p-1" role="tablist" aria-label={`${label} review views`}>
        <button
          type="button"
          role="tab"
          aria-selected={view === 'summary'}
          onClick={() => setView('summary')}
          className={`inline-flex h-8 items-center gap-1.5 rounded-md px-3 text-[11px] font-medium transition-colors ${
            view === 'summary'
              ? 'bg-background text-foreground shadow-sm'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          <ShieldCheck className="h-3.5 w-3.5" />
          Judgment
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={view === 'history'}
          onClick={() => setView('history')}
          className={`inline-flex h-8 items-center gap-1.5 rounded-md px-3 text-[11px] font-medium transition-colors ${
            view === 'history'
              ? 'bg-background text-foreground shadow-sm'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          <FileText className="h-3.5 w-3.5" />
          Review history
          {moduleReviews.length > 0 && (
            <span className="rounded-full bg-muted px-1.5 py-0.5 text-[9px] tabular-nums text-muted-foreground">
              {moduleReviews.length}
            </span>
          )}
        </button>
      </div>

      {view === 'summary' && (
        <div className="p-3 sm:p-4">
          {latestReview ? (
            <div className="rounded-xl border bg-gradient-to-br from-primary/5 via-background to-background p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <div className="text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                    Latest judgment
                  </div>
                  <div className="mt-2 text-sm font-semibold leading-6 text-foreground">
                    {latestReview.verdict || 'Review recorded without a verdict line.'}
                  </div>
                  <div className="mt-1 text-[10px] text-muted-foreground">
                    {formatDate(latestReview.recorded_at)}
                  </div>
                </div>
                <span className={`rounded-full border px-2.5 py-1 text-[10px] font-semibold capitalize ${reviewTone(latestReview.status)}`}>
                  {readableStatus(latestReview.status)}
                </span>
              </div>
              <button
                type="button"
                onClick={() => setView('history')}
                className="mt-4 inline-flex h-8 items-center gap-1.5 rounded-md border bg-background px-3 text-[11px] font-medium text-foreground hover:bg-muted"
              >
                <FileText className="h-3.5 w-3.5" />
                View review history
              </button>
            </div>
          ) : (
            <div className="flex min-h-32 flex-col items-center justify-center rounded-lg border border-dashed px-6 py-8 text-center">
              <ShieldCheck className="h-5 w-5 text-muted-foreground" />
              <div className="mt-2 text-sm font-medium text-foreground">No saved judgment yet</div>
              <div className="mt-1 text-xs text-muted-foreground">This reviewer has not completed a stored review for this workflow.</div>
            </div>
          )}
        </div>
      )}

      {view === 'history' && (
        <PulseReviewHistory
          workspacePath={workspacePath}
          module={module}
          label={label}
          className="border-0"
        />
      )}
    </section>
  )
}

export default PulseModuleInspector

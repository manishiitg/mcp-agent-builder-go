import { useCallback, useEffect, useState } from 'react'
import { CheckCircle2, Loader2, RefreshCw } from 'lucide-react'
import { agentApi } from '../../services/api'
import type { PulseReviewRecord } from '../../services/api-types'
import { WORKFLOW_LOG_REFRESH_EVENT } from './workflowEvents'

const compactNumber = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 })

function formatDuration(milliseconds: number): string {
  if (!Number.isFinite(milliseconds) || milliseconds < 1) return '0s'
  const seconds = Math.round(milliseconds / 1000)
  const minutes = Math.floor(seconds / 60)
  return minutes > 0 ? `${minutes}m ${seconds % 60}s` : `${seconds}s`
}

function recordedLabel(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

export function PulseReviewHistory({
  workspacePath,
  module,
  label,
  className = '',
}: {
  workspacePath: string
  module: string
  label: string
  className?: string
}) {
  const [reviews, setReviews] = useState<PulseReviewRecord[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loadReviews = useCallback(async () => {
    if (!workspacePath || !module) return
    setLoading(true)
    setError(null)
    try {
      const response = await agentApi.getPulseReviews(workspacePath, module)
      if (!response.success) throw new Error(response.error || 'Failed to load Pulse review history.')
      setReviews(response.reviews || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load Pulse review history.')
    } finally {
      setLoading(false)
    }
  }, [module, workspacePath])

  useEffect(() => { void loadReviews() }, [loadReviews])
  useEffect(() => {
    const refresh = () => { void loadReviews() }
    window.addEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
    return () => window.removeEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
  }, [loadReviews])

  return (
    <section className={`min-w-0 ${className}`} aria-label={`${label} review history`}>
      <div className="flex items-center justify-between border-b bg-muted/20 px-4 py-2.5">
        <div>
          <div className="text-xs font-medium text-foreground">Review runs</div>
          <div className="text-[10px] text-muted-foreground">Findings and evidence are stored in the issue lifecycle below.</div>
        </div>
        <button
          type="button"
          onClick={() => { void loadReviews() }}
          disabled={loading}
          className="rounded p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-50"
          aria-label="Refresh review history"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      {error ? (
        <div className="m-4 rounded border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">{error}</div>
      ) : loading && reviews.length === 0 ? (
        <div className="flex min-h-32 items-center justify-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading review history…
        </div>
      ) : reviews.length === 0 ? (
        <div className="flex min-h-32 items-center justify-center text-xs text-muted-foreground">No completed review run yet.</div>
      ) : (
        <div className="divide-y">
          {reviews.map((review) => (
            <div key={review.id} className="px-4 py-3">
              <div className="flex items-start gap-2">
                <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
                    <span className="font-medium text-foreground">{recordedLabel(review.recorded_at)}</span>
                    <span className="text-muted-foreground">{review.status || 'completed'}</span>
                    <span className="text-muted-foreground">{review.finding_count} findings</span>
                    <span className="text-muted-foreground">{review.verification_count} verifications</span>
                  </div>
                  {review.verdict && <p className="mt-1 text-xs leading-5 text-muted-foreground">{review.verdict}</p>}
                  {review.metrics?.usage_status === 'captured' && (
                    <div className="mt-1 flex flex-wrap gap-x-3 text-[10px] text-muted-foreground">
                      <span>{formatDuration(review.metrics.duration_ms)}</span>
                      <span>{compactNumber.format(review.metrics.prompt_tokens)} input</span>
                      <span>{compactNumber.format(review.metrics.completion_tokens)} output</span>
                      <span>${review.metrics.total_cost_usd.toFixed(2)}</span>
                    </div>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

export default PulseReviewHistory

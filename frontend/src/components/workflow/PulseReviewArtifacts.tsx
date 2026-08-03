import { useCallback, useEffect, useMemo, useState } from 'react'
import { Database, Loader2, RefreshCw } from 'lucide-react'
import { agentApi } from '../../services/api'
import type { PulseReviewRecord } from '../../services/api-types'
import { MarkdownRenderer } from '../ui/MarkdownRenderer'
import { WORKFLOW_LOG_REFRESH_EVENT } from './workflowEvents'
import {
  collectPulseReviewArtifacts,
  pulseReviewRunDate,
  type PulseReviewArtifact,
} from './pulseReviewArtifactUtils'

function reviewLabel(review: PulseReviewRecord): string {
  const date = new Date(review.recorded_at)
  const dateLabel = Number.isNaN(date.getTime())
    ? review.review_run_id
    : date.toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
  return review.verdict ? `${dateLabel} · ${review.verdict}` : dateLabel
}

function legacyReviewLabel(review: PulseReviewArtifact): string {
  const date = pulseReviewRunDate(review.reviewRunId)
  return date
    ? date.toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
    : review.reviewRunId
}

const compactNumber = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 })

function formatMetricDuration(milliseconds: number): string {
  if (!Number.isFinite(milliseconds) || milliseconds < 1) return '0s'
  const seconds = Math.round(milliseconds / 1000)
  const minutes = Math.floor(seconds / 60)
  return minutes > 0 ? `${minutes}m ${seconds % 60}s` : `${seconds}s`
}

export function PulseReviewArtifacts({
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
  const [legacyReviews, setLegacyReviews] = useState<PulseReviewArtifact[]>([])
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const [selectedLegacyPath, setSelectedLegacyPath] = useState<string | null>(null)
  const [contentByID, setContentByID] = useState<Record<number, string>>({})
  const [legacyContentByPath, setLegacyContentByPath] = useState<Record<string, string>>({})
  const [loadingList, setLoadingList] = useState(false)
  const [loadingContent, setLoadingContent] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loadReviews = useCallback(async () => {
    if (!workspacePath || !module) return
    setLoadingList(true)
    setError(null)
    let databaseError: unknown = null
    let databaseReviews: PulseReviewRecord[] = []
    try {
      const response = await agentApi.getPulseReviews(workspacePath, module)
      if (!response.success) throw new Error(response.error || 'Failed to load Pulse reviews.')
      databaseReviews = response.reviews || []
    } catch (err) {
      databaseError = err
    }

    if (databaseReviews.length > 0) {
      setReviews(databaseReviews)
      setLegacyReviews([])
      setSelectedLegacyPath(null)
      setSelectedID((current) => (
        databaseReviews.some((review) => review.id === current)
          ? current
          : databaseReviews[0]?.id ?? null
      ))
      setLoadingList(false)
      return
    }

    setReviews([])
    setSelectedID(null)
    try {
      const files = await agentApi.getPlannerFiles(`${workspacePath}/pulse/reviews`, 500, 3)
      const legacy = collectPulseReviewArtifacts(files, workspacePath, module)
      setLegacyReviews(legacy)
      setSelectedLegacyPath((current) => (
        legacy.some((review) => review.path === current)
          ? current
          : legacy[0]?.path ?? null
      ))
      if (legacy.length === 0 && databaseError) {
        setError(databaseError instanceof Error ? databaseError.message : 'Failed to load Pulse reviews.')
      }
    } catch {
      setLegacyReviews([])
      setSelectedLegacyPath(null)
      if (databaseError) {
        setError(databaseError instanceof Error ? databaseError.message : 'Failed to load Pulse reviews.')
      }
    }
    setLoadingList(false)
  }, [module, workspacePath])

  useEffect(() => {
    setReviews([])
    setLegacyReviews([])
    setSelectedID(null)
    setSelectedLegacyPath(null)
    setContentByID({})
    setLegacyContentByPath({})
    void loadReviews()
  }, [loadReviews])

  useEffect(() => {
    const refresh = () => { void loadReviews() }
    window.addEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
    return () => window.removeEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
  }, [loadReviews])

  useEffect(() => {
    if (selectedID === null || Object.hasOwn(contentByID, selectedID)) return
    let cancelled = false
    setLoadingContent(true)
    setError(null)
    void agentApi.getPulseReview(workspacePath, selectedID)
      .then((response) => {
        if (cancelled) return
        if (!response.success || typeof response.review?.markdown !== 'string') {
          throw new Error(response.error || 'The review could not be read from SQLite.')
        }
        setContentByID((current) => ({ ...current, [selectedID]: response.review.markdown || '' }))
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to load the Pulse review.')
        }
      })
      .finally(() => {
        if (!cancelled) setLoadingContent(false)
      })
    return () => { cancelled = true }
  }, [contentByID, selectedID, workspacePath])

  useEffect(() => {
    if (!selectedLegacyPath || Object.hasOwn(legacyContentByPath, selectedLegacyPath)) return
    let cancelled = false
    setLoadingContent(true)
    setError(null)
    void agentApi.getPlannerFileContent(selectedLegacyPath)
      .then((response) => {
        if (cancelled) return
        if (!response?.success || typeof response.data?.content !== 'string') {
          throw new Error('The legacy review file could not be read.')
        }
        setLegacyContentByPath((current) => ({
          ...current,
          [selectedLegacyPath]: response.data.content,
        }))
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to load the legacy Pulse review.')
        }
      })
      .finally(() => {
        if (!cancelled) setLoadingContent(false)
      })
    return () => { cancelled = true }
  }, [legacyContentByPath, selectedLegacyPath])

  const selectedReview = useMemo(
    () => reviews.find((review) => review.id === selectedID) || null,
    [reviews, selectedID],
  )
  const selectedLegacyReview = useMemo(
    () => legacyReviews.find((review) => review.path === selectedLegacyPath) || null,
    [legacyReviews, selectedLegacyPath],
  )
  const usingLegacy = reviews.length === 0 && legacyReviews.length > 0
  const content = usingLegacy
    ? (selectedLegacyPath ? legacyContentByPath[selectedLegacyPath] || '' : '')
    : (selectedID === null ? '' : contentByID[selectedID] || '')

  if (loadingList && reviews.length === 0 && legacyReviews.length === 0) {
    return (
      <div className={`flex min-h-40 items-center justify-center gap-2 text-xs text-muted-foreground ${className}`}>
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
        Loading reviews…
      </div>
    )
  }

  if (error && reviews.length === 0 && legacyReviews.length === 0) {
    return (
      <div className={`m-4 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive ${className}`}>
        {error}
      </div>
    )
  }

  if (reviews.length === 0 && legacyReviews.length === 0) {
    return (
      <div className={`flex min-h-40 flex-col items-center justify-center px-6 py-8 text-center ${className}`}>
        <Database className="h-5 w-5 text-muted-foreground" />
        <div className="mt-3 text-sm font-medium text-foreground">No saved review yet</div>
        <div className="mt-1 max-w-md text-xs leading-5 text-muted-foreground">
          {label} has not produced a persisted review for this workflow.
        </div>
      </div>
    )
  }

  return (
    <section className={`min-w-0 ${className}`} aria-label={`${label} reviews`}>
      <div className="flex flex-wrap items-center gap-2 border-b bg-muted/20 px-3 py-2.5 sm:px-4">
        <Database className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        {usingLegacy ? (
          <select
            value={selectedLegacyPath ?? ''}
            onChange={(event) => setSelectedLegacyPath(event.target.value)}
            className="h-8 min-w-0 flex-1 rounded-md border border-border bg-background px-2 text-xs text-foreground sm:max-w-xl"
            aria-label={`${label} legacy saved review`}
          >
            {legacyReviews.map((review) => (
              <option key={review.path} value={review.path}>
                {legacyReviewLabel(review)}
              </option>
            ))}
          </select>
        ) : (
          <select
            value={selectedID ?? ''}
            onChange={(event) => setSelectedID(Number(event.target.value))}
            className="h-8 min-w-0 flex-1 rounded-md border border-border bg-background px-2 text-xs text-foreground sm:max-w-xl"
            aria-label={`${label} saved review`}
          >
            {reviews.map((review) => (
              <option key={review.id} value={review.id}>
                {reviewLabel(review)}
              </option>
            ))}
          </select>
        )}
        <span className="text-[10px] text-muted-foreground">
          {usingLegacy ? `${legacyReviews.length} legacy file${legacyReviews.length === 1 ? '' : 's'}` : `${reviews.length} saved in SQLite`}
        </span>
        <button
          type="button"
          onClick={() => { void loadReviews() }}
          disabled={loadingList}
          className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
          aria-label="Refresh saved reviews"
          title="Refresh saved reviews"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loadingList ? 'animate-spin' : ''}`} />
        </button>
      </div>

      {selectedReview && (
		<div className="border-b px-4 py-2 text-[10px] text-muted-foreground">
		  <div className="truncate font-mono">
		    {selectedReview.review_run_id} · {selectedReview.artifact_bytes.toLocaleString()} bytes
		  </div>
		  {selectedReview.metrics?.usage_status === 'captured' ? (
		    <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 font-sans">
		      <span>{formatMetricDuration(selectedReview.metrics.duration_ms)}</span>
		      {selectedReview.metrics.queue_duration_ms > 1000 && (
		        <span>{formatMetricDuration(selectedReview.metrics.queue_duration_ms)} queued</span>
		      )}
		      <span>{selectedReview.metrics.llm_call_count} LLM call{selectedReview.metrics.llm_call_count === 1 ? '' : 's'}</span>
		      <span>{compactNumber.format(selectedReview.metrics.prompt_tokens)} input</span>
		      <span>{compactNumber.format(selectedReview.metrics.cache_read_tokens)} cached</span>
		      <span>{compactNumber.format(selectedReview.metrics.completion_tokens)} output</span>
		      <span>${selectedReview.metrics.total_cost_usd.toFixed(2)}</span>
		    </div>
		  ) : selectedReview.metrics ? (
		    <div className="mt-1 font-sans text-amber-700 dark:text-amber-300" title={selectedReview.metrics.usage_error}>
		      Runtime measured; token and cost usage unavailable
		    </div>
		  ) : null}
		</div>
      )}
      {selectedLegacyReview && (
        <div className="truncate border-b px-4 py-2 font-mono text-[10px] text-muted-foreground">
          Legacy compatibility view · {selectedLegacyReview.path}
        </div>
      )}

      {error && (
        <div className="m-4 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {error}
        </div>
      )}

      {loadingContent && !content ? (
        <div className="flex min-h-40 items-center justify-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          Loading full review…
        </div>
      ) : content ? (
        <div className="px-4 py-4 sm:px-6 sm:py-5">
          <MarkdownRenderer
            content={content}
            basePath={workspacePath}
            className="max-w-none !text-sm [&_h1]:!text-xl [&_h2]:!mt-6 [&_h2]:!text-base [&_h3]:!text-sm [&_li]:!text-sm [&_p]:!text-sm"
          />
        </div>
      ) : null}
    </section>
  )
}

export default PulseReviewArtifacts

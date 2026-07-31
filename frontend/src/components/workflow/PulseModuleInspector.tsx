import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  Bug,
  CheckCircle2,
  Clock3,
  FileText,
  History,
  Loader2,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Wrench,
} from 'lucide-react'
import { agentApi } from '../../services/api'
import type {
  PulseFindingLifecycle as PulseFindingLifecycleRecord,
  PulseReviewRecord,
} from '../../services/api-types'
import { PulseFindingLifecycle } from './PulseFindingLifecycle'
import { PulseReviewArtifacts } from './PulseReviewArtifacts'
import { WORKFLOW_LOG_REFRESH_EVENT } from './workflowEvents'
import {
  buildPulseModuleActivity,
  isPulseFindingClosed,
  summarizePulseModule,
} from './pulseModuleInspectorUtils'

type InspectorView = 'overview' | 'findings' | 'raw'

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
      return 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300'
    default:
      return 'border-border bg-muted text-muted-foreground'
  }
}

function eventTone(eventType: string) {
  switch (eventType) {
    case 'closed':
    case 'verified':
      return { Icon: CheckCircle2, className: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-300' }
    case 'reopened':
    case 'failed':
      return { Icon: RotateCcw, className: 'bg-red-500/10 text-red-600 dark:text-red-300' }
    case 'fixing':
    case 'fix_started':
      return { Icon: Wrench, className: 'bg-sky-500/10 text-sky-600 dark:text-sky-300' }
    default:
      return { Icon: History, className: 'bg-muted text-muted-foreground' }
  }
}

function MetricCard({
  label,
  value,
  detail,
  tone,
}: {
  label: string
  value: number
  detail: string
  tone: string
}) {
  return (
    <div className={`rounded-lg border px-3 py-3 ${tone}`}>
      <div className="text-[10px] font-semibold uppercase tracking-wide opacity-75">{label}</div>
      <div className="mt-1 text-2xl font-semibold tabular-nums">{value}</div>
      <div className="mt-0.5 text-[10px] opacity-75">{detail}</div>
    </div>
  )
}

export function PulseModuleInspector({
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
  const [view, setView] = useState<InspectorView>('overview')
  const [reviews, setReviews] = useState<PulseReviewRecord[]>([])
  const [findings, setFindings] = useState<PulseFindingLifecycleRecord[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!workspacePath || !module) return
    setLoading(true)
    setError(null)
    const [reviewResult, findingResult] = await Promise.allSettled([
      agentApi.getPulseReviews(workspacePath, module),
      agentApi.getPulseFindings(workspacePath, module),
    ])
    const errors: string[] = []
    if (reviewResult.status === 'fulfilled' && reviewResult.value.success) {
      setReviews(reviewResult.value.reviews || [])
    } else {
      setReviews([])
      errors.push(
        reviewResult.status === 'rejected'
          ? (reviewResult.reason instanceof Error ? reviewResult.reason.message : 'Could not load review history.')
          : reviewResult.value.error || 'Could not load review history.',
      )
    }
    if (findingResult.status === 'fulfilled' && findingResult.value.success) {
      setFindings(findingResult.value.findings || [])
    } else {
      setFindings([])
      errors.push(
        findingResult.status === 'rejected'
          ? (findingResult.reason instanceof Error ? findingResult.reason.message : 'Could not load findings.')
          : findingResult.value.error || 'Could not load findings.',
      )
    }
    setError(errors.length > 0 ? errors.join(' ') : null)
    setLoading(false)
  }, [module, workspacePath])

  useEffect(() => {
    setView('overview')
    setReviews([])
    setFindings([])
    void load()
  }, [load])

  useEffect(() => {
    const refresh = () => { void load() }
    window.addEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
    return () => window.removeEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
  }, [load])

  const latestReview = reviews[0] || null
  const summary = useMemo(() => summarizePulseModule(findings), [findings])
  const activity = useMemo(() => buildPulseModuleActivity(findings), [findings])
  const attentionFindings = useMemo(
    () => findings
      .filter((finding) => !isPulseFindingClosed(finding.status))
      .sort((a, b) => (b.last_seen_at || '').localeCompare(a.last_seen_at || ''))
      .slice(0, 4),
    [findings],
  )

  const tabs: Array<{ id: InspectorView; label: string; count?: number }> = [
    { id: 'overview', label: 'Overview' },
    { id: 'findings', label: 'Findings', count: summary.total },
    { id: 'raw', label: 'Raw report', count: reviews.length > 0 ? reviews.length : undefined },
  ]

  if (loading && reviews.length === 0 && findings.length === 0) {
    return (
      <div className={`flex min-h-48 items-center justify-center gap-2 text-xs text-muted-foreground ${className}`}>
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
        Loading structured Pulse data…
      </div>
    )
  }

  return (
    <section className={`min-w-0 ${className}`} aria-label={`${label} structured Pulse review`}>
      <div className="flex flex-wrap items-center justify-between gap-2 border-b bg-muted/20 px-3 py-2 sm:px-4">
        <div className="inline-flex min-w-0 items-center rounded-lg border bg-background p-0.5" role="tablist" aria-label={`${label} review views`}>
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={view === tab.id}
              onClick={() => setView(tab.id)}
              className={`inline-flex h-7 items-center gap-1.5 rounded-md px-2.5 text-[11px] font-medium transition-colors ${
                view === tab.id
                  ? 'bg-primary/10 text-primary'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              }`}
            >
              {tab.label}
              {tab.count !== undefined && (
                <span className="rounded-full bg-muted px-1.5 py-0.5 text-[9px] tabular-nums text-muted-foreground">
                  {tab.count}
                </span>
              )}
            </button>
          ))}
        </div>
        <button
          type="button"
          onClick={() => { void load() }}
          disabled={loading}
          className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
          aria-label="Refresh structured Pulse data"
          title="Refresh structured Pulse data"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      {error && (
        <div className="mx-3 mt-3 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300 sm:mx-4">
          Some Pulse data could not be loaded: {error}
        </div>
      )}

      {view === 'overview' && (
        <div className="space-y-4 p-3 sm:p-4">
          <div className="rounded-xl border bg-gradient-to-br from-primary/5 via-background to-background p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                  <ShieldCheck className="h-3.5 w-3.5" />
                  Latest reviewer judgment
                </div>
                <div className="mt-2 text-sm font-semibold leading-6 text-foreground">
                  {latestReview?.verdict || (latestReview ? 'Review recorded without a verdict line.' : 'No SQLite review recorded yet.')}
                </div>
                <div className="mt-1 text-[10px] text-muted-foreground">
                  {latestReview
                    ? `${formatDate(latestReview.recorded_at)} · ${latestReview.review_run_id}`
                    : 'A retained legacy report may still be available under Raw report.'}
                </div>
              </div>
              {latestReview && (
                <span className={`rounded-full border px-2.5 py-1 text-[10px] font-semibold capitalize ${reviewTone(latestReview.status)}`}>
                  {readableStatus(latestReview.status)}
                </span>
              )}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
            <MetricCard
              label="Needs action"
              value={summary.open}
              detail={
                summary.harnessIssues > 0
                  ? `${summary.harnessIssues} platform · ${summary.recurring} recurring`
                  : summary.recurring > 0 ? `${summary.recurring} recurring` : 'new or acknowledged'
              }
              tone="border-red-500/25 bg-red-500/5 text-red-700 dark:text-red-300"
            />
            <MetricCard
              label="Being fixed"
              value={summary.fixing}
              detail={`${summary.attempts} total attempt${summary.attempts === 1 ? '' : 's'}`}
              tone="border-sky-500/25 bg-sky-500/5 text-sky-700 dark:text-sky-300"
            />
            <MetricCard
              label="Needs proof"
              value={summary.awaitingVerification}
              detail={`${summary.inconclusiveChecks} inconclusive check${summary.inconclusiveChecks === 1 ? '' : 's'}`}
              tone="border-amber-500/25 bg-amber-500/5 text-amber-700 dark:text-amber-300"
            />
            <MetricCard
              label="Closed"
              value={summary.closed}
              detail={`${summary.passedChecks} passed · ${summary.failedChecks} failed`}
              tone="border-emerald-500/25 bg-emerald-500/5 text-emerald-700 dark:text-emerald-300"
            />
          </div>

          {attentionFindings.length > 0 ? (
            <div className="overflow-hidden rounded-lg border bg-background">
              <div className="flex items-center justify-between border-b px-3 py-2.5 sm:px-4">
                <div>
                  <div className="text-xs font-semibold text-foreground">Needs attention</div>
                  <div className="text-[10px] text-muted-foreground">Open findings ordered by latest evidence</div>
                </div>
                <button
                  type="button"
                  onClick={() => setView('findings')}
                  className="text-[11px] font-medium text-primary hover:underline"
                >
                  View all
                </button>
              </div>
              <div className="divide-y">
                {attentionFindings.map((finding) => (
                  <button
                    key={finding.fingerprint}
                    type="button"
                    onClick={() => setView('findings')}
                    className="flex w-full items-start gap-3 px-3 py-3 text-left transition-colors hover:bg-muted/40 sm:px-4"
                  >
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
                    <div className="min-w-0 flex-1">
                      <div className="line-clamp-2 text-xs font-medium leading-5 text-foreground">{finding.text}</div>
                      <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[10px] text-muted-foreground">
                        <span className="font-mono">{finding.finding_id || finding.fingerprint}</span>
                        <span>·</span>
                        <span className="capitalize">{readableStatus(finding.status)}</span>
                        {finding.seen_count > 1 && <span>· seen {finding.seen_count} times</span>}
                        {finding.details?.issue_kind === 'harness_issue' && (
                          <span className="inline-flex items-center gap-1 rounded-full border border-orange-500/25 bg-orange-500/10 px-1.5 py-0.5 font-medium text-orange-700 dark:text-orange-300">
                            <Bug className="h-2.5 w-2.5" />
                            Harness · platform
                          </span>
                        )}
                      </div>
                    </div>
                  </button>
                ))}
              </div>
            </div>
          ) : (
            <div className="flex items-start gap-3 rounded-lg border border-emerald-500/25 bg-emerald-500/5 px-4 py-3">
              <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-300" />
              <div>
                <div className="text-xs font-semibold text-emerald-700 dark:text-emerald-300">No open tracked findings</div>
                <div className="mt-0.5 text-[10px] leading-5 text-muted-foreground">
                  This means the structured tracker is clear; consult the latest verdict for unstructured or insufficient-evidence notes.
                </div>
              </div>
            </div>
          )}

          <div className="overflow-hidden rounded-lg border bg-background">
            <div className="flex items-center justify-between border-b px-3 py-2.5 sm:px-4">
              <div>
                <div className="text-xs font-semibold text-foreground">Recent lifecycle activity</div>
                <div className="text-[10px] text-muted-foreground">Database events, newest first</div>
              </div>
              <Activity className="h-4 w-4 text-muted-foreground" />
            </div>
            {activity.length > 0 ? (
              <div className="divide-y">
                {activity.map((event, index) => {
                  const tone = eventTone(event.event_type)
                  const Icon = tone.Icon
                  return (
                    <div key={`${event.fingerprint}-${event.recorded_at}-${index}`} className="flex items-start gap-3 px-3 py-3 sm:px-4">
                      <span className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full ${tone.className}`}>
                        <Icon className="h-3.5 w-3.5" />
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="text-xs leading-5 text-foreground">
                          <span className="font-semibold capitalize">{readableStatus(event.event_type)}</span>
                          <span className="text-muted-foreground"> · {event.summary || event.findingText}</span>
                        </div>
                        <div className="mt-0.5 text-[10px] text-muted-foreground">
                          {formatDate(event.recorded_at)}
                          {event.findingID ? ` · ${event.findingID}` : ''}
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            ) : (
              <div className="flex items-center gap-2 px-4 py-4 text-xs text-muted-foreground">
                <Clock3 className="h-4 w-4" />
                No structured lifecycle events have been recorded yet.
              </div>
            )}
          </div>

          <button
            type="button"
            onClick={() => setView('raw')}
            className="flex w-full items-center justify-between rounded-lg border border-dashed px-4 py-3 text-left transition-colors hover:bg-muted/30"
          >
            <div className="flex min-w-0 items-center gap-3">
              <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0">
                <div className="text-xs font-medium text-foreground">Raw reviewer report</div>
                <div className="truncate text-[10px] text-muted-foreground">Forensic Markdown evidence; not the primary status view</div>
              </div>
            </div>
            <span className="text-[10px] text-muted-foreground">
              {reviews.length > 0 ? `${reviews.length} saved` : 'compatibility view'}
            </span>
          </button>
        </div>
      )}

      {view === 'findings' && (
        <PulseFindingLifecycle
          workspacePath={workspacePath}
          module={module}
          label={label}
          className="border-0"
        />
      )}

      {view === 'raw' && (
        <PulseReviewArtifacts
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

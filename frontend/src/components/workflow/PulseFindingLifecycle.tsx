import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Bug,
  CheckCircle2,
  CircleAlert,
  Clock3,
  Loader2,
  RefreshCw,
  ShieldCheck,
  Terminal,
  Wrench,
  XCircle,
} from 'lucide-react'
import { agentApi } from '../../services/api'
import type {
  PulseFindingLifecycle as PulseFindingLifecycleRecord,
  PulseFindingVerification,
} from '../../services/api-types'
import { WORKFLOW_LOG_REFRESH_EVENT } from './workflowEvents'

function formatDate(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function findingStatus(status: string): { label: string; className: string } {
  switch (status) {
    case 'resolved':
      return { label: 'Closed', className: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' }
    case 'rejected':
      return { label: 'Rejected', className: 'border-border bg-muted text-muted-foreground' }
    case 'fixing':
      return { label: 'Fixing', className: 'border-sky-500/30 bg-sky-500/10 text-sky-700 dark:text-sky-300' }
    case 'awaiting_verification':
      return { label: 'Awaiting verification', className: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300' }
    case 'acknowledged':
      return { label: 'Acknowledged', className: 'border-violet-500/30 bg-violet-500/10 text-violet-700 dark:text-violet-300' }
    default:
      return { label: 'Open', className: 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300' }
  }
}

function verificationTone(verification: PulseFindingVerification) {
  switch (verification.verdict) {
    case 'passed':
      return {
        Icon: CheckCircle2,
        className: 'border-emerald-500/25 bg-emerald-500/5 text-emerald-700 dark:text-emerald-300',
      }
    case 'failed':
      return {
        Icon: XCircle,
        className: 'border-red-500/25 bg-red-500/5 text-red-700 dark:text-red-300',
      }
    default:
      return {
        Icon: Clock3,
        className: 'border-amber-500/25 bg-amber-500/5 text-amber-700 dark:text-amber-300',
      }
  }
}

export function PulseFindingLifecycle({
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
  const [findings, setFindings] = useState<PulseFindingLifecycleRecord[]>([])
  const [filter, setFilter] = useState<'active' | 'closed' | 'all'>('active')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!workspacePath || !module) return
    setLoading(true)
    setError(null)
    try {
      const response = await agentApi.getPulseFindings(workspacePath, module)
      if (!response.success) throw new Error(response.error || 'Failed to load finding lifecycle.')
      setFindings(response.findings || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load finding lifecycle.')
    } finally {
      setLoading(false)
    }
  }, [module, workspacePath])

  useEffect(() => {
    setFilter('active')
    void load()
  }, [load])
  useEffect(() => {
    const refresh = () => { void load() }
    window.addEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
    return () => window.removeEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
  }, [load])

  const activeCount = findings.filter((finding) => finding.status !== 'resolved' && finding.status !== 'rejected').length
  const closedCount = findings.length - activeCount
  const visibleFindings = useMemo(() => findings.filter((finding) => {
    const closed = finding.status === 'resolved' || finding.status === 'rejected'
    if (filter === 'closed') return closed
    if (filter === 'active') return !closed
    return true
  }), [filter, findings])

  if (loading && findings.length === 0) {
    return (
      <div className={`flex min-h-40 items-center justify-center gap-2 text-xs text-muted-foreground ${className}`}>
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
        Loading fix outcomes…
      </div>
    )
  }
  if (error && findings.length === 0) {
    return (
      <div className={`m-4 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive ${className}`}>
        {error}
      </div>
    )
  }
  if (findings.length === 0) {
    return (
      <div className={`flex min-h-40 flex-col items-center justify-center px-6 py-8 text-center ${className}`}>
        <Wrench className="h-5 w-5 text-muted-foreground" />
        <div className="mt-3 text-sm font-medium text-foreground">No tracked fix outcomes yet</div>
        <div className="mt-1 max-w-lg text-xs leading-5 text-muted-foreground">
          New {label} findings will show their filed → fixed → tested → closed or reopened lifecycle here.
          Historical narrative remains available in the full Pulse dashboard.
        </div>
      </div>
    )
  }

  return (
    <section className={`min-w-0 ${className}`} aria-label={`${label} fix outcomes`}>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-muted/20 px-4 py-2.5">
        <div className="inline-flex items-center rounded-md border bg-background p-0.5" aria-label="Filter findings">
          {([
            ['active', 'Active', activeCount],
            ['closed', 'Closed', closedCount],
            ['all', 'All', findings.length],
          ] as const).map(([id, text, count]) => (
            <button
              key={id}
              type="button"
              aria-pressed={filter === id}
              onClick={() => setFilter(id)}
              className={`h-7 rounded px-2.5 text-[10px] font-medium transition-colors ${
                filter === id ? 'bg-muted text-foreground' : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {text} {count}
            </button>
          ))}
        </div>
        <button
          type="button"
          onClick={() => { void load() }}
          disabled={loading}
          className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
          aria-label="Refresh fix outcomes"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      <div className="space-y-3 p-3 sm:p-4">
        {visibleFindings.map((finding) => {
          const status = findingStatus(finding.status)
          const harnessDetails = finding.details?.issue_kind === 'harness_issue' ? finding.details : null
          return (
            <article key={finding.fingerprint} className="overflow-hidden rounded-lg border bg-background">
              <div className="flex flex-wrap items-start justify-between gap-3 border-b px-3 py-3 sm:px-4">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-[11px] font-semibold text-foreground">
                      {finding.finding_id || finding.fingerprint}
                    </span>
                    <span className={`rounded-full border px-2 py-0.5 text-[10px] font-semibold ${status.className}`}>
                      {status.label}
                    </span>
                    {finding.seen_count > 1 && (
                      <span className="rounded-full border border-amber-500/25 bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium text-amber-700 dark:text-amber-300">
                        Seen {finding.seen_count} times
                      </span>
                    )}
                  </div>
                  <p className="mt-2 text-sm leading-6 text-foreground">{finding.text}</p>
                  <div className="mt-1 text-[10px] text-muted-foreground">
                    {finding.phase} · {finding.step_id}
                    {finding.last_seen_at ? ` · last seen ${formatDate(finding.last_seen_at)}` : ''}
                  </div>
                  {finding.resolution_note && (
                    <div className="mt-2 rounded-md bg-muted/50 px-2.5 py-2 text-xs leading-5 text-muted-foreground">
                      {finding.resolution_note}
                    </div>
                  )}
                </div>
              </div>

              {harnessDetails && (
                <div className="border-b border-orange-500/20 bg-orange-500/[0.04] px-3 py-3 sm:px-4">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="inline-flex items-center gap-1.5 rounded-full border border-orange-500/30 bg-orange-500/10 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wide text-orange-700 dark:text-orange-300">
                      <Bug className="h-3 w-3" />
                      Harness issue
                    </span>
                    <span className="inline-flex items-center gap-1.5 rounded-full border border-violet-500/25 bg-violet-500/10 px-2.5 py-1 text-[10px] font-medium text-violet-700 dark:text-violet-300">
                      <ShieldCheck className="h-3 w-3" />
                      Platform owned
                    </span>
                    {harnessDetails.severity && (
                      <span className="rounded-full border bg-background px-2.5 py-1 text-[10px] font-medium capitalize text-muted-foreground">
                        {harnessDetails.severity}
                      </span>
                    )}
                    {harnessDetails.classification && (
                      <span className="rounded-full border bg-background px-2.5 py-1 text-[10px] font-medium text-muted-foreground">
                        {harnessDetails.classification.replaceAll('_', ' ')}
                      </span>
                    )}
                    {harnessDetails.platform && (
                      <span className="rounded-full border border-violet-500/25 bg-violet-500/10 px-2.5 py-1 text-[10px] font-medium text-violet-700 dark:text-violet-300">
                        Affects {harnessDetails.platform.affected_workflows.length} workflow{harnessDetails.platform.affected_workflows.length === 1 ? '' : 's'}
                        {' · '}{harnessDetails.platform.seen_count} report{harnessDetails.platform.seen_count === 1 ? '' : 's'}
                      </span>
                    )}
                  </div>

                  <div className="mt-3 grid gap-3 lg:grid-cols-2">
                    <div className="rounded-lg border bg-background px-3 py-3">
                      <div className="text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">What failed</div>
                      <div className="mt-1.5 text-xs font-medium leading-5 text-foreground">
                        {harnessDetails.summary || finding.text}
                      </div>
                      {harnessDetails.impact && (
                        <div className="mt-2 text-[11px] leading-5 text-muted-foreground">
                          <b className="text-foreground">Impact:</b> {harnessDetails.impact}
                        </div>
                      )}
                      {harnessDetails.target_key && (
                        <code className="mt-2 block break-all rounded bg-muted px-2 py-1.5 text-[10px] text-muted-foreground">
                          {harnessDetails.target_key}
                        </code>
                      )}
                      {harnessDetails.platform && harnessDetails.platform.affected_workflows.length > 1 && (
                        <div className="mt-2 text-[10px] leading-5 text-muted-foreground">
                          Linked workflows: {harnessDetails.platform.affected_workflows.join(', ')}
                        </div>
                      )}
                    </div>

                    <div className="rounded-lg border bg-background px-3 py-3">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <div className="inline-flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                          <Terminal className="h-3.5 w-3.5" />
                          Safe reproduction
                        </div>
                        <span className={`rounded-full border px-2 py-0.5 text-[10px] font-medium ${
                          harnessDetails.reproduction.safe
                            ? 'border-emerald-500/25 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                            : 'border-amber-500/25 bg-amber-500/10 text-amber-700 dark:text-amber-300'
                        }`}>
                          {harnessDetails.reproduction.safe ? 'Side-effect free' : 'Not safe to rerun'}
                        </span>
                      </div>
                      {harnessDetails.reproduction.setup && (
                        <div className="mt-2 text-[11px] leading-5 text-muted-foreground">
                          <b className="text-foreground">Setup:</b> {harnessDetails.reproduction.setup}
                        </div>
                      )}
                      {harnessDetails.reproduction.action && (
                        <pre className="mt-2 whitespace-pre-wrap break-words rounded bg-muted px-2.5 py-2 font-mono text-[10px] leading-5 text-foreground">
                          {harnessDetails.reproduction.action}
                        </pre>
                      )}
                      {harnessDetails.reproduction.limitations && (
                        <div className="mt-2 text-[10px] leading-5 text-amber-700 dark:text-amber-300">
                          {harnessDetails.reproduction.limitations}
                        </div>
                      )}
                    </div>
                  </div>

                  {(harnessDetails.reproduction.expected || harnessDetails.reproduction.observed) && (
                    <div className="mt-3 grid gap-2 sm:grid-cols-2">
                      <div className="rounded-md border border-emerald-500/20 bg-emerald-500/[0.04] px-3 py-2.5">
                        <div className="text-[10px] font-semibold uppercase tracking-wide text-emerald-700 dark:text-emerald-300">Expected</div>
                        <div className="mt-1 text-[11px] leading-5 text-foreground">{harnessDetails.reproduction.expected || 'Not recorded'}</div>
                      </div>
                      <div className="rounded-md border border-red-500/20 bg-red-500/[0.04] px-3 py-2.5">
                        <div className="text-[10px] font-semibold uppercase tracking-wide text-red-700 dark:text-red-300">Observed</div>
                        <div className="mt-1 text-[11px] leading-5 text-foreground">{harnessDetails.reproduction.observed || 'Not recorded'}</div>
                      </div>
                    </div>
                  )}

                  {(harnessDetails.evidence?.length || harnessDetails.workaround) && (
                    <div className="mt-3 rounded-lg border bg-background px-3 py-3">
                      {harnessDetails.evidence && harnessDetails.evidence.length > 0 && (
                        <>
                          <div className="text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">Evidence</div>
                          <ul className="mt-1.5 space-y-1 text-[11px] leading-5 text-foreground">
                            {harnessDetails.evidence.map((item, evidenceIndex) => (
                              <li key={`${item}-${evidenceIndex}`} className="flex items-start gap-2">
                                <span className="mt-2 h-1 w-1 shrink-0 rounded-full bg-muted-foreground" />
                                <span className="break-words">{item}</span>
                              </li>
                            ))}
                          </ul>
                        </>
                      )}
                      {harnessDetails.workaround && (
                        <div className={`${harnessDetails.evidence?.length ? 'mt-2 border-t pt-2' : ''} text-[11px] leading-5 text-muted-foreground`}>
                          <b className="text-foreground">Temporary workaround:</b> {harnessDetails.workaround}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}

              {finding.fix_attempts.length > 0 && (
                <div className="space-y-2 border-b px-3 py-3 sm:px-4">
                  <div className="text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">Fix attempts</div>
                  {finding.fix_attempts.map((attempt) => {
                    const findingRef = attempt.findings?.[0]
                    return (
                      <div key={attempt.attempt_id} className="rounded-md border bg-muted/20 px-3 py-2.5">
                        <div className="flex flex-wrap items-center justify-between gap-2">
                          <span className="font-mono text-[10px] text-muted-foreground">{attempt.attempt_id}</span>
                          <span className="rounded-full border px-2 py-0.5 text-[10px] font-medium text-foreground">
                            {findingRef?.disposition?.replaceAll('_', ' ') || attempt.status.replaceAll('_', ' ')}
                          </span>
                        </div>
                        <div className="mt-1.5 text-xs leading-5 text-foreground">
                          {findingRef?.summary || attempt.summary}
                        </div>
                        {attempt.changed_files.length > 0 && (
                          <div className="mt-2 flex flex-wrap gap-1.5">
                            {attempt.changed_files.map((file) => (
                              <code key={file} className="rounded bg-muted px-1.5 py-1 text-[10px] text-muted-foreground">
                                {file}
                              </code>
                            ))}
                          </div>
                        )}
                        <div className="mt-2 text-[10px] text-muted-foreground">
                          Started {formatDate(attempt.started_at)}
                          {attempt.completed_at ? ` · completed ${formatDate(attempt.completed_at)}` : ''}
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}

              {finding.verifications.length > 0 && (
                <div className="space-y-2 px-3 py-3 sm:px-4">
                  <div className="text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">Verification</div>
                  {finding.verifications.map((verification, index) => {
                    const tone = verificationTone(verification)
                    const Icon = tone.Icon
                    return (
                      <div key={`${verification.check}-${index}`} className={`rounded-md border px-3 py-2.5 ${tone.className}`}>
                        <div className="flex items-start gap-2">
                          <Icon className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                          <div className="min-w-0">
                            <div className="text-xs font-medium">{verification.check}</div>
                            {(verification.expected || verification.observed) && (
                              <div className="mt-1 space-y-0.5 text-[11px] leading-5 opacity-90">
                                {verification.expected && <div><b>Expected:</b> {verification.expected}</div>}
                                {verification.observed && <div><b>Observed:</b> {verification.observed}</div>}
                              </div>
                            )}
                            {verification.verified_at && (
                              <div className="mt-1 text-[10px] opacity-70">{formatDate(verification.verified_at)}</div>
                            )}
                          </div>
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}

              {finding.events.length > 0 && (
                <div className="border-t px-3 py-3 sm:px-4">
                  <div className="text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">Lifecycle</div>
                  <div className="mt-2 space-y-2">
                    {finding.events.map((event, index) => (
                      <div key={`${event.event_type}-${event.recorded_at}-${index}`} className="flex items-start gap-2 text-xs">
                        <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-muted-foreground/60" />
                        <div className="min-w-0">
                          <span className="font-medium capitalize text-foreground">{event.event_type.replaceAll('_', ' ')}</span>
                          {event.summary && event.summary !== finding.text && (
                            <span className="text-muted-foreground"> · {event.summary}</span>
                          )}
                          <div className="mt-0.5 text-[10px] text-muted-foreground">
                            {formatDate(event.recorded_at)}
                            {event.pulse_run_id ? ` · ${event.pulse_run_id}` : ''}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {finding.fix_attempts.length === 0 && finding.verifications.length === 0 && (
                <div className="flex items-center gap-2 px-4 py-3 text-xs text-muted-foreground">
                  <CircleAlert className="h-3.5 w-3.5" />
                  Filed and awaiting a fix disposition.
                </div>
              )}
            </article>
          )
        })}
        {visibleFindings.length === 0 && (
          <div className="flex min-h-32 items-center justify-center rounded-lg border border-dashed px-4 text-center text-xs text-muted-foreground">
            No {filter} findings in this module.
          </div>
        )}
      </div>
    </section>
  )
}

export default PulseFindingLifecycle

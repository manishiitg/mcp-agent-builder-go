import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  ArrowUpRight,
  CheckCircle2,
  Clock3,
  Database,
  FileText,
  Loader2,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Sparkles,
  Wrench,
} from 'lucide-react'
import { agentApi } from '../../services/api'
import type {
  PulseFinalCommandState,
  PulseFindingLifecycle,
  PulseModuleState,
  PulseReviewRecord,
} from '../../services/api-types'
import { ReportHumanInputPanel } from './ReportHumanInputPanel'
import { SoulViewer } from './SoulViewer'
import { PulseModuleInspector } from './PulseModuleInspector'
import {
  buildPulseModuleActivity,
  acknowledgedReason,
  isPulseFindingClosed,
  summarizePulseModule,
} from './pulseModuleInspectorUtils'
import {
  buildPulseWorkspaceModuleSummaries,
  selectPulseWorkspaceModule,
  summarizePulseReviewStorage,
} from './pulseWorkspaceUtils'
import { WORKFLOW_LOG_REFRESH_EVENT } from './workflowEvents'
import {
  PULSE_FIXED_COMMANDS,
  PULSE_MODULE_COMMANDS,
} from './canvas/pulseSections'

function formatDate(value?: string): string {
  if (!value) return 'Not recorded'
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
}

function readable(value?: string): string {
  return (value || '').trim().replaceAll('_', ' ') || 'No data'
}

function statusTone(value?: string): string {
  const status = (value || '').toLowerCase().replace(/^last\s+/, '')
  if (['failed', 'blocked', 'timed_out', 'timed out'].includes(status)) {
    return 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300'
  }
  if (['changed', 'due', 'fixing', 'awaiting_verification'].includes(status)) {
    return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
  }
  if (['done', 'completed', 'clean', 'published', 'healthy'].includes(status)) {
    return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
  }
  return 'border-border bg-muted text-muted-foreground'
}

function moduleStateLabel(state?: PulseModuleState): string {
  return readable(state?.last_result || state?.last_gate_decision || state?.last_decision)
}

function finalCommandLabel(state?: PulseFinalCommandState): string {
  return readable(state?.status)
}

function FindingStatus({ finding }: { finding: PulseFindingLifecycle }) {
  const closed = isPulseFindingClosed(finding.status)
  const external = finding.status === 'external_action_required'
  const tone = external
    ? 'border-violet-500/25 bg-violet-500/10 text-violet-700 dark:text-violet-300'
    : closed
    ? 'border-emerald-500/25 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    : finding.status === 'fixing'
      ? 'border-sky-500/25 bg-sky-500/10 text-sky-700 dark:text-sky-300'
      : finding.status === 'awaiting_verification'
        ? 'border-amber-500/25 bg-amber-500/10 text-amber-700 dark:text-amber-300'
        : 'border-red-500/25 bg-red-500/10 text-red-700 dark:text-red-300'
  return (
    <span className={`rounded-full border px-2 py-0.5 text-[9px] font-semibold capitalize ${tone}`}>
      {external ? 'external action' : closed ? 'closed' : readable(finding.status)}
    </span>
  )
}

/** Which slice of the backlog the findings list is showing. */
type PulseFocus = 'all' | 'awaiting_user' | 'open' | 'blocked' | 'fixing' | 'awaiting_verification' | 'closed'

const FOCUS_TITLES: Record<PulseFocus, string> = {
  all: 'Needs attention',
  awaiting_user: 'Waiting on you',
  open: 'Pulse can fix these',
  blocked: 'Blocked',
  fixing: 'Being fixed now',
  awaiting_verification: 'Waiting for proof',
  closed: 'Closed',
}

const FOCUS_HINTS: Record<PulseFocus, string> = {
  all: 'Open findings across every review module',
  awaiting_user: 'Each one needs a decision only you can make',
  open: 'Queued for a fixer; nothing is blocking them',
  blocked: 'Diagnosed, but Pulse has no action available',
  fixing: 'A fixer holds these right now',
  awaiting_verification: 'Changed, but not proven until the next valid run',
  closed: 'Resolved, rejected, or handed to another owner',
}

function Metric({
  label,
  value,
  detail,
  tone,
  focus,
  activeFocus,
  onFocus,
}: {
  label: string
  value: number
  detail: string
  tone: string
  focus?: PulseFocus
  activeFocus?: PulseFocus
  onFocus?: (focus: PulseFocus) => void
}) {
  const body = (
    <>
      <div className="text-[10px] font-semibold uppercase tracking-wide opacity-75">{label}</div>
      <div className="mt-1 text-2xl font-semibold tabular-nums">{value}</div>
      <div className="mt-0.5 text-[10px] opacity-75">{detail}</div>
    </>
  )
  if (!focus || !onFocus) {
    return <div className={`rounded-xl border p-3 ${tone}`}>{body}</div>
  }
  const selected = activeFocus === focus
  // A count with nothing behind it should not look clickable — the empty list
  // it opens says less than the zero already does.
  const empty = value === 0
  return (
    <button
      type="button"
      aria-pressed={selected}
      disabled={empty}
      onClick={() => onFocus(selected ? 'all' : focus)}
      className={`rounded-xl border p-3 text-left transition ${tone} ${
        empty
          ? 'cursor-default opacity-60'
          : 'cursor-pointer hover:brightness-105 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-current'
      } ${selected ? 'ring-2 ring-current' : ''}`}
    >
      {body}
    </button>
  )
}

export function PulseWorkspace({
  workspacePath,
  monitorOn,
  moduleStates,
  finalCommandStates,
  statusLoading,
  statusError,
  onRefresh,
  onOpenDashboard,
}: {
  workspacePath: string
  monitorOn: boolean
  moduleStates: PulseModuleState[]
  finalCommandStates: PulseFinalCommandState[]
  statusLoading: boolean
  statusError: string | null
  onRefresh: () => void
  onOpenDashboard: () => void
}) {
  const [findings, setFindings] = useState<PulseFindingLifecycle[]>([])
  const [reviews, setReviews] = useState<PulseReviewRecord[]>([])
  const [selectedModule, setSelectedModule] = useState<string | null>(null)
  const [focus, setFocus] = useState<PulseFocus>('all')
  // Distinct from selectedModule on purpose. selectedModule always holds a
  // value because the inspector below needs something to render, and an effect
  // re-picks a default whenever it is empty — so using it to filter the list
  // meant Clear filter showed everything for one frame and then snapped back to
  // one module as that effect re-fired. This is only ever set by an explicit
  // click, and cleared means cleared.
  const [moduleFilter, setModuleFilter] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!workspacePath) return
    setLoading(true)
    setError(null)
    const [findingResult, reviewResult] = await Promise.allSettled([
      agentApi.getPulseFindings(workspacePath),
      agentApi.getPulseReviews(workspacePath),
    ])
    const errors: string[] = []
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
    if (reviewResult.status === 'fulfilled' && reviewResult.value.success) {
      setReviews(reviewResult.value.reviews || [])
    } else {
      setReviews([])
      errors.push(
        reviewResult.status === 'rejected'
          ? (reviewResult.reason instanceof Error ? reviewResult.reason.message : 'Could not load reviews.')
          : reviewResult.value.error || 'Could not load reviews.',
      )
    }
    setError(errors.length > 0 ? errors.join(' ') : null)
    setLoading(false)
  }, [workspacePath])

  useEffect(() => {
    setSelectedModule(null)
    setFindings([])
    setReviews([])
    void load()
  }, [load])

  useEffect(() => {
    const refresh = () => { void load() }
    window.addEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
    return () => window.removeEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
  }, [load])

  const summary = useMemo(() => summarizePulseModule(findings), [findings])
  const storageSummary = useMemo(() => summarizePulseReviewStorage(reviews), [reviews])
  const moduleSummaries = useMemo(
    () => buildPulseWorkspaceModuleSummaries(PULSE_MODULE_COMMANDS, findings, reviews),
    [findings, reviews],
  )
  const selectedDefinition = moduleSummaries.find((module) => module.id === selectedModule) || null
  useEffect(() => {
    if (selectedModule && moduleSummaries.some((module) => module.id === selectedModule)) return
    setSelectedModule(selectPulseWorkspaceModule(moduleSummaries))
  }, [moduleSummaries, selectedModule])

  const moduleStateByID = useMemo(
    () => new Map(moduleStates.map((state) => [state.module, state])),
    [moduleStates],
  )
  const finalCommandStateByID = useMemo(
    () => new Map(finalCommandStates.map((state) => [state.command, state])),
    [finalCommandStates],
  )
  const attentionFindings = useMemo(
    () => {
      const matchesFocus = (finding: PulseFindingLifecycle) => {
        switch (focus) {
          case 'all':
            return !isPulseFindingClosed(finding.status)
          case 'closed':
            return isPulseFindingClosed(finding.status)
          case 'fixing':
            return finding.status === 'fixing'
          case 'awaiting_verification':
            return finding.status === 'awaiting_verification'
          case 'awaiting_user':
            return finding.status === 'acknowledged' && acknowledgedReason(finding) === 'awaiting_user'
          case 'blocked':
            return finding.status === 'acknowledged' && acknowledgedReason(finding) === 'blocked'
          case 'open':
            return finding.status !== 'acknowledged'
              ? finding.status === 'open'
              : acknowledgedReason(finding) === 'other'
          default:
            return true
        }
      }
      const matched = findings
        .filter(matchesFocus)
        // Selecting a module narrows this list too, so the module grid and the
        // findings list are two views of one selection rather than two lists
        // that ignore each other.
        .filter((finding) => !moduleFilter || finding.module === moduleFilter)
        .sort((a, b) => {
        const statusPriority = (finding: PulseFindingLifecycle) => (
          finding.status === 'open' ? 3 : finding.status === 'awaiting_verification' ? 2 : 1
        )
        const priority = statusPriority(b) - statusPriority(a)
        return priority || (b.last_seen_at || '').localeCompare(a.last_seen_at || '')
      })
      // Unfiltered shows enough to scan a real backlog rather than a
      // six-item teaser; a chosen slice shows all of it, because the point of
      // clicking a count is to see everything it counted.
      return focus === 'all' ? matched.slice(0, 25) : matched
    },
    [findings, focus, moduleFilter],
  )
  const activity = useMemo(() => buildPulseModuleActivity(findings, 8), [findings])

  const health = summary.open > 0 || summary.failedChecks > 0
    ? {
        label: 'Action required',
        detail: `${summary.open} open finding${summary.open === 1 ? '' : 's'} need ownership`,
        tone: 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300',
        Icon: AlertTriangle,
      }
    : summary.awaitingVerification > 0 || summary.fixing > 0
      ? {
          label: 'Work in progress',
          detail: 'Fixes exist but the loop is not fully verified',
          tone: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
          Icon: Clock3,
        }
      : reviews.length > 0
        ? {
            label: 'No open findings',
            detail: 'Latest stored reviews have no active tracked issue',
            tone: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
            Icon: ShieldCheck,
          }
        : {
            label: 'Awaiting review evidence',
            detail: 'Run Pulse or a review slash command to establish health',
            tone: 'border-border bg-muted text-muted-foreground',
            Icon: Activity,
          }

  if (loading && findings.length === 0 && reviews.length === 0) {
    return (
      <div className="flex min-h-96 items-center justify-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading Pulse workspace…
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <SoulViewer workspacePath={workspacePath} pulseSummary />

      <section className="overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-background to-background">
        <div className="flex flex-wrap items-start justify-between gap-4 p-4 sm:p-5">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
              <Activity className="h-3.5 w-3.5" />
              Workflow health
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-semibold ${health.tone}`}>
                <health.Icon className="h-3.5 w-3.5" />
                {health.label}
              </span>
              <span className="text-xs text-muted-foreground">{health.detail}</span>
            </div>
            <div className="mt-2 text-[11px] text-muted-foreground">
              {reviews.length} stored review{reviews.length === 1 ? '' : 's'} · {findings.length} tracked finding{findings.length === 1 ? '' : 's'}
            </div>
            {storageSummary.migrated > 0 && (
              <div className="mt-2 inline-flex flex-wrap items-center gap-x-1.5 gap-y-1 rounded-md border border-emerald-500/25 bg-emerald-500/5 px-2 py-1 text-[10px] text-emerald-700 dark:text-emerald-300">
                <Database className="h-3 w-3 shrink-0" />
                <span className="font-semibold">SQLite migration verified</span>
                <span className="opacity-80">
                  {storageSummary.migrated} legacy review{storageSummary.migrated === 1 ? '' : 's'} imported with source provenance
                  {storageSummary.native > 0
                    ? ` · ${storageSummary.native} database-native review${storageSummary.native === 1 ? '' : 's'} since migration`
                    : ''}
                </span>
              </div>
            )}
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <button
              type="button"
              onClick={onRefresh}
              disabled={loading || statusLoading}
              className="inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-50"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${loading || statusLoading ? 'animate-spin' : ''}`} />
              Refresh
            </button>
            <button
              type="button"
              onClick={onOpenDashboard}
              className="inline-flex h-8 items-center gap-1.5 rounded-md border bg-background px-2.5 text-xs font-medium text-foreground hover:bg-muted"
            >
              <FileText className="h-3.5 w-3.5" />
              Full dashboard
              <ArrowUpRight className="h-3 w-3" />
            </button>
          </div>
        </div>
        {!monitorOn && (
          <div className="border-t border-dashed px-4 py-2.5 text-xs text-muted-foreground sm:px-5">
            Scheduled Pulse is off. Saved reviews and lifecycle history remain available.
          </div>
        )}
      </section>

      {(error || statusError) && (
        <div className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          Some Pulse data could not be loaded: {[statusError, error].filter(Boolean).join(' ')}
        </div>
      )}

      {/*
        Ordered by who has to act. "Needs you" leads because it is the only
        number the operator can move, and it was previously invisible: it was
        folded into a single count alongside blocked work, so a workflow with 6
        real items, 12 blocked, and 4 questions read as 25 outstanding problems
        and gave no way to tell a healthy workflow from a struggling one.
      */}
      <section className="grid grid-cols-2 gap-2 lg:grid-cols-6">
        <Metric
          label="Needs you"
          focus="awaiting_user"
          activeFocus={focus}
          onFocus={setFocus}
          value={summary.awaitingUser}
          detail={summary.awaitingUser > 0 ? 'waiting on your decision' : 'nothing waiting on you'}
          tone="border-fuchsia-500/25 bg-fuchsia-500/5 text-fuchsia-700 dark:text-fuchsia-300"
        />
        <Metric
          label="Pulse can fix"
          focus="open"
          activeFocus={focus}
          onFocus={setFocus}
          value={summary.open}
          detail={summary.recurring > 0 ? `${summary.recurring} seen before` : 'queued for a fixer'}
          tone="border-red-500/25 bg-red-500/5 text-red-700 dark:text-red-300"
        />
        <Metric
          label="Blocked"
          focus="blocked"
          activeFocus={focus}
          onFocus={setFocus}
          value={summary.blocked}
          detail="no action available to Pulse"
          tone="border-slate-500/25 bg-slate-500/5 text-slate-700 dark:text-slate-300"
        />
        <Metric
          label="Being fixed"
          focus="fixing"
          activeFocus={focus}
          onFocus={setFocus}
          value={summary.fixing}
          detail={`${summary.attempts} total attempt${summary.attempts === 1 ? '' : 's'}`}
          tone="border-sky-500/25 bg-sky-500/5 text-sky-700 dark:text-sky-300"
        />
        <Metric
          label="Needs proof"
          focus="awaiting_verification"
          activeFocus={focus}
          onFocus={setFocus}
          value={summary.awaitingVerification}
          detail={`${summary.inconclusiveChecks} inconclusive check${summary.inconclusiveChecks === 1 ? '' : 's'}`}
          tone="border-amber-500/25 bg-amber-500/5 text-amber-700 dark:text-amber-300"
        />
        <Metric
          label="Closed"
          focus="closed"
          activeFocus={focus}
          onFocus={setFocus}
          value={summary.closed + summary.externalAction}
          detail={summary.externalAction > 0
            ? `${summary.passedChecks} verified · ${summary.externalAction} handed off`
            : `${summary.passedChecks} passed verification${summary.passedChecks === 1 ? '' : 's'}`}
          tone="border-emerald-500/25 bg-emerald-500/5 text-emerald-700 dark:text-emerald-300"
        />
      </section>

      <ReportHumanInputPanel workspacePath={workspacePath} contentMode="pending" />

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(300px,0.65fr)]">
        <section className="overflow-hidden rounded-xl border bg-background">
          <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
            <div>
              <h3 className="text-sm font-semibold text-foreground">
                {FOCUS_TITLES[focus]}
                {moduleFilter && (
                  <span className="ml-1.5 font-normal text-muted-foreground">
                    in {moduleSummaries.find((m) => m.id === moduleFilter)?.label || moduleFilter}
                  </span>
                )}
              </h3>
              <p className="mt-0.5 text-[11px] text-muted-foreground">{FOCUS_HINTS[focus]}</p>
            </div>
            <div className="flex items-center gap-2">
              {(focus !== 'all' || moduleFilter) && (
                <button
                  type="button"
                  onClick={() => { setFocus('all'); setModuleFilter(null) }}
                  className="rounded-full border px-2 py-1 text-[10px] font-semibold text-muted-foreground hover:bg-muted"
                >
                  Clear filter
                </button>
              )}
              <span className="rounded-full bg-muted px-2 py-1 text-[10px] font-semibold text-muted-foreground">
                {attentionFindings.length} shown
              </span>
            </div>
          </div>
          {attentionFindings.length === 0 ? (
            <div className="flex min-h-40 flex-col items-center justify-center px-6 py-8 text-center">
              <CheckCircle2 className="h-5 w-5 text-emerald-500" />
              <div className="mt-2 text-sm font-medium text-foreground">No active tracked findings</div>
              <div className="mt-1 text-xs text-muted-foreground">Review evidence remains available in the module cards below.</div>
            </div>
          ) : (
            <div className="divide-y">
              {attentionFindings.map((finding) => {
                const module = moduleSummaries.find((item) => item.id === finding.module)
                return (
                  // A div, not a button: browsers suppress text selection inside
                  // buttons, and these rows carry the exact paths, fields and
                  // ids you need to paste elsewhere. The click still opens the
                  // module, but only when it was a click rather than the end of
                  // a drag-select.
                  <div
                    key={finding.fingerprint}
                    role="button"
                    tabIndex={0}
                    onClick={() => {
                      if ((window.getSelection()?.toString() || '').length > 0) return
                      if (finding.module) setSelectedModule(finding.module)
                    }}
                    onKeyDown={(event) => {
                      if (event.key !== 'Enter' && event.key !== ' ') return
                      event.preventDefault()
                      if (finding.module) setSelectedModule(finding.module)
                    }}
                    className="block w-full cursor-pointer select-text px-4 py-3 text-left transition-colors hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40"
                  >
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-[10px] font-semibold text-primary">{module?.label || finding.module || 'Review'}</span>
                      <FindingStatus finding={finding} />
                      {finding.seen_count > 1 && (
                        <span className="inline-flex items-center gap-1 text-[9px] font-medium text-amber-700 dark:text-amber-300">
                          <RotateCcw className="h-3 w-3" />
                          Seen {finding.seen_count} times
                        </span>
                      )}
                    </div>
                    <div className="mt-1.5 whitespace-pre-wrap break-words text-xs leading-5 text-foreground">{finding.text}</div>
                    <div className="mt-1 text-[10px] text-muted-foreground">
                      {finding.finding_id || finding.fingerprint} · {formatDate(finding.last_seen_at)}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </section>

        <section className="overflow-hidden rounded-xl border bg-background">
          <div className="border-b px-4 py-3">
            <h3 className="text-sm font-semibold text-foreground">Recent activity</h3>
            <p className="mt-0.5 text-[11px] text-muted-foreground">Filed, fixing, verified, closed, and reopened</p>
          </div>
          {activity.length === 0 ? (
            <div className="flex min-h-40 items-center justify-center px-5 text-center text-xs text-muted-foreground">
              Lifecycle activity will appear after findings are filed.
            </div>
          ) : (
            <div className="divide-y">
              {activity.map((event, index) => (
                <div key={`${event.fingerprint}-${event.recorded_at}-${index}`} className="flex gap-3 px-4 py-3">
                  <span className="mt-1 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
                    {event.event_type.includes('fix')
                      ? <Wrench className="h-3 w-3" />
                      : event.event_type === 'closed' || event.event_type === 'verified'
                        ? <CheckCircle2 className="h-3 w-3" />
                        : <Sparkles className="h-3 w-3" />}
                  </span>
                  <div className="min-w-0">
                    <div className="text-[10px] font-semibold capitalize text-foreground">{readable(event.event_type)}</div>
                    <div className="mt-0.5 line-clamp-2 text-[11px] leading-4 text-muted-foreground">
                      {event.summary || event.findingText}
                    </div>
                    <div className="mt-1 text-[9px] text-muted-foreground">{formatDate(event.recorded_at)}</div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>

      <section className="overflow-hidden rounded-xl border bg-background">
        <div className="border-b px-4 py-3">
          <h3 className="text-sm font-semibold text-foreground">Review modules</h3>
          <p className="mt-0.5 text-[11px] text-muted-foreground">Choose a module to inspect its verdict, findings, fixes, verification, and raw evidence</p>
        </div>
        <div className="grid gap-px bg-border sm:grid-cols-2 lg:grid-cols-4">
          {moduleSummaries.map((module) => {
            const state = moduleStateByID.get(module.id)
            const active = selectedModule === module.id
            const openWork = module.active + module.fixing + module.awaitingVerification
            return (
              <button
                key={module.id}
                type="button"
                onClick={() => { setSelectedModule(module.id); setModuleFilter(module.id) }}
                aria-pressed={active}
                className={`min-w-0 bg-background p-3 text-left transition-colors hover:bg-muted/40 ${active ? 'ring-2 ring-inset ring-primary/50' : ''}`}
              >
                <div className="flex items-start justify-between gap-2">
                  <span className="text-xs font-semibold text-foreground">{module.label}</span>
                  {openWork > 0 && (
                    <span className="flex h-5 min-w-5 items-center justify-center rounded-full bg-red-500/10 px-1.5 text-[9px] font-semibold text-red-700 dark:text-red-300">
                      {openWork}
                    </span>
                  )}
                </div>
                <div className="mt-1 line-clamp-2 min-h-8 text-[10px] leading-4 text-muted-foreground">{module.description}</div>
                <div className="mt-2 flex flex-wrap items-center gap-1.5">
                  <span className={`rounded-full border px-1.5 py-0.5 text-[9px] font-semibold capitalize ${statusTone(moduleStateLabel(state))}`}>
                    {moduleStateLabel(state)}
                  </span>
                  {module.recurring > 0 && (
                    <span className="text-[9px] font-medium text-amber-700 dark:text-amber-300">{module.recurring} recurring</span>
                  )}
                  {module.externalAction > 0 && (
                    <span className="text-[9px] font-medium text-violet-700 dark:text-violet-300">
                      {module.externalAction} external
                    </span>
                  )}
                </div>
                <div className="mt-2 line-clamp-2 text-[10px] leading-4 text-foreground">
                  {module.latestReview?.verdict || (module.latestReview ? 'Review recorded' : 'No stored review yet')}
                </div>
                <div className="mt-1 text-[9px] text-muted-foreground">
                  {module.latestReview ? formatDate(module.latestReview.recorded_at) : 'Awaiting evidence'}
                </div>
              </button>
            )
          })}
        </div>
      </section>

      {selectedDefinition && (
        <section className="overflow-hidden rounded-xl border bg-background">
          <div className="border-b px-4 py-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <h3 className="text-sm font-semibold text-foreground">{selectedDefinition.label}</h3>
                <p className="mt-0.5 text-[11px] text-muted-foreground">{selectedDefinition.description}</p>
              </div>
              <span className="text-[10px] text-muted-foreground">
                {selectedDefinition.findings} finding{selectedDefinition.findings === 1 ? '' : 's'} · {selectedDefinition.closed} closed
              </span>
            </div>
          </div>
          <PulseModuleInspector
            workspacePath={workspacePath}
            module={selectedDefinition.id}
            label={selectedDefinition.label}
          />
        </section>
      )}

      <section className="overflow-hidden rounded-xl border bg-background">
        <div className="border-b px-4 py-3">
          <h3 className="text-sm font-semibold text-foreground">Finalization</h3>
          <p className="mt-0.5 text-[11px] text-muted-foreground">Dashboard, backup, publish, and notification outcomes</p>
        </div>
        <div className="grid gap-px bg-border sm:grid-cols-2 lg:grid-cols-4">
          {PULSE_FIXED_COMMANDS.map((command) => {
            const state = finalCommandStateByID.get(command.id)
            return (
              <div key={command.id} className="min-w-0 bg-background p-3">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-xs font-semibold text-foreground">{command.label}</span>
                  <span className={`rounded-full border px-1.5 py-0.5 text-[9px] font-semibold capitalize ${statusTone(finalCommandLabel(state))}`}>
                    {finalCommandLabel(state)}
                  </span>
                </div>
                <div className="mt-1 line-clamp-2 text-[10px] leading-4 text-muted-foreground">
                  {state?.reason || command.description}
                </div>
                <div className="mt-2 text-[9px] text-muted-foreground">
                  {formatDate(state?.finished_at || state?.updated_at || state?.started_at)}
                </div>
              </div>
            )
          })}
        </div>
      </section>
    </div>
  )
}

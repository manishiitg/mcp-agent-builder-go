import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  ArrowUpRight,
  CheckCircle2,
  Clock3,
  FileText,
  Loader2,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Wrench,
} from 'lucide-react'
import { agentApi } from '../../services/api'
import type {
  PulseFinalCommandState,
  PulseFindingLifecycle,
  PulseImpactLedger,
  PulseAgentMetricRecord,
  PulseModuleState,
  PulseReviewRecord,
} from '../../services/api-types'
import { ReportHumanInputPanel } from './ReportHumanInputPanel'
import { SoulViewer } from './SoulViewer'
import { PulseModuleInspector } from './PulseModuleInspector'
import { PulseFindingCard } from './PulseFindingCard'
import {
  buildPulseModuleActivity,
} from './pulseModuleInspectorUtils'
import { pulseFindingPresentation, type PulseFindingQueue } from './pulseFindingPresentation'
import {
  buildPulseWorkspaceModuleSummaries,
  selectPulseWorkspaceModule,
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

const compactMetricNumber = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 })

function formatAgentDuration(milliseconds: number): string {
  if (!Number.isFinite(milliseconds) || milliseconds < 1) return '0s'
  const seconds = Math.round(milliseconds / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  if (hours > 0) return `${hours}h ${minutes % 60}m`
  return minutes > 0 ? `${minutes}m ${seconds % 60}s` : `${seconds}s`
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

/** Which slice of the backlog the findings list is showing. */
type PulseFocus = 'all' | PulseFindingQueue

const FOCUS_TITLES: Record<PulseFocus, string> = {
  all: 'Current work',
  needs_action: 'Pulse to fix',
  waiting_proof: 'Waiting on a run',
  decisions: 'Your decisions',
  proposals: 'Proposed improvements',
  platform: 'Platform team',
  resolved: 'Resolved',
  workflow_reported: 'Workflow evidence',
}

const FOCUS_HINTS: Record<PulseFocus, string> = {
  all: 'Open work, grouped by who or what can move it forward',
  needs_action: 'Issues Pulse can diagnose, repair, or reopen',
  waiting_proof: 'Fixes that need evidence from a future workflow run',
  decisions: 'Items that cannot continue without your approval or direction',
  proposals: 'Ideas Pulse recommends considering; these are not waiting for your answer',
  platform: 'Diagnosed work that must be fixed outside this workflow',
  resolved: 'Verified fixes and legitimate no-change closures',
  workflow_reported: 'Evidence filed by workflow steps, kept separate from Pulse\u2019s repair queue',
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
    <div className="flex min-w-0 items-center gap-2">
      <span className="text-base font-semibold tabular-nums">{value}</span>
      <span className="min-w-0">
        <span className="block text-[10px] font-semibold uppercase tracking-wide">{label}</span>
        <span className="block truncate text-[9px] opacity-70">{detail}</span>
      </span>
    </div>
  )
  if (!focus || !onFocus) {
    return <div className={`rounded-md border px-3 py-2 ${tone}`}>{body}</div>
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
      className={`rounded-md border px-3 py-2 text-left transition ${tone} ${
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
  const [agentMetrics, setAgentMetrics] = useState<PulseAgentMetricRecord[]>([])
  const [impact, setImpact] = useState<PulseImpactLedger>({ interventions: [], observations: [], assessments: [] })
  const [selectedModule, setSelectedModule] = useState<string | null>(null)
  const [focus, setFocus] = useState<PulseFocus>('all')
  // Distinct from selectedModule on purpose. selectedModule always holds a
  // value because the inspector below needs something to render, and an effect
  // re-picks a default whenever it is empty — so using it to filter the list
  // meant Clear filter showed everything for one frame and then snapped back to
  // one module as that effect re-fired. This is only ever set by an explicit
  // click, and cleared means cleared.
  const [moduleFilter, setModuleFilter] = useState<string | null>(null)
  const [expandedFinding, setExpandedFinding] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!workspacePath) return
    setLoading(true)
    setError(null)
    const [findingResult, reviewResult, impactResult, metricResult] = await Promise.allSettled([
      agentApi.getPulseFindings(workspacePath),
      agentApi.getPulseReviews(workspacePath),
      agentApi.getPulseImpact(workspacePath),
      agentApi.getPulseAgentMetrics(workspacePath),
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
    if (impactResult.status === 'fulfilled' && impactResult.value.success) {
      setImpact(impactResult.value.impact || { interventions: [], observations: [], assessments: [] })
    } else {
      setImpact({ interventions: [], observations: [], assessments: [] })
      errors.push(
        impactResult.status === 'rejected'
          ? (impactResult.reason instanceof Error ? impactResult.reason.message : 'Could not load goal impact.')
          : impactResult.value.error || 'Could not load goal impact.',
      )
    }
    if (metricResult.status === 'fulfilled' && metricResult.value.success) {
      setAgentMetrics(metricResult.value.metrics || [])
    } else {
      setAgentMetrics([])
      errors.push(
        metricResult.status === 'rejected'
          ? (metricResult.reason instanceof Error ? metricResult.reason.message : 'Could not load reviewer measurements.')
          : metricResult.value.error || 'Could not load reviewer measurements.',
      )
    }
    setError(errors.length > 0 ? errors.join(' ') : null)
    setLoading(false)
  }, [workspacePath])

  useEffect(() => {
    setSelectedModule(null)
    setFindings([])
    setReviews([])
    setAgentMetrics([])
    setImpact({ interventions: [], observations: [], assessments: [] })
    void load()
  }, [load])

  useEffect(() => {
    const refresh = () => { void load() }
    window.addEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
    return () => window.removeEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
  }, [load])

  const queueCounts = useMemo(() => {
    const counts: Record<PulseFindingQueue, number> = {
      needs_action: 0,
      waiting_proof: 0,
      decisions: 0,
      proposals: 0,
      platform: 0,
      resolved: 0,
      workflow_reported: 0,
    }
    findings.forEach((finding) => { counts[pulseFindingPresentation(finding).queue] += 1 })
    return counts
  }, [findings])
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
        const queue = pulseFindingPresentation(finding).queue
        if (focus === 'all') return !['resolved', 'workflow_reported'].includes(queue)
        return queue === focus
      }
      const matched = findings
        .filter(matchesFocus)
        // Selecting a module narrows this list too, so the module grid and the
        // findings list are two views of one selection rather than two lists
        // that ignore each other.
        .filter((finding) => !moduleFilter || finding.module === moduleFilter)
        .sort((a, b) => {
          const rank: Record<PulseFindingQueue, number> = {
            needs_action: 6,
            waiting_proof: 5,
            decisions: 4,
            proposals: 3,
            platform: 2,
            workflow_reported: 1,
            resolved: 0,
          }
          const priority = rank[pulseFindingPresentation(b).queue] - rank[pulseFindingPresentation(a).queue]
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
  const impactSummary = useMemo(() => {
    const latestBySeries = new Map<string, PulseImpactLedger['observations'][number]>()
    impact.observations.forEach((observation) => {
      const key = [observation.criterion_id, observation.metric, observation.route || '', observation.environment || ''].join('\u0000')
      if (!latestBySeries.has(key)) latestBySeries.set(key, observation)
    })
    const latestAssessmentByIntervention = new Map<string, PulseImpactLedger['assessments'][number]>()
    impact.assessments.forEach((assessment) => {
      if (!latestAssessmentByIntervention.has(assessment.intervention_id)) {
        latestAssessmentByIntervention.set(assessment.intervention_id, assessment)
      }
    })
    const currentAssessments = Array.from(latestAssessmentByIntervention.values())
    return {
      improved: currentAssessments.filter((item) => item.verdict === 'improved').length,
      regressed: currentAssessments.filter((item) => item.verdict === 'regressed').length,
      inconclusive: currentAssessments.filter((item) => ['inconclusive', 'confounded'].includes(item.verdict)).length,
      awaiting: impact.interventions.filter((item) => ['awaiting_evidence', 'measuring'].includes(item.status)).length,
      latest: Array.from(latestBySeries.values()).slice(0, 4),
    }
  }, [impact])
  const latestPassMetrics = useMemo(() => {
    const latestPulseRunID = agentMetrics.find((metric) => metric.pulse_run_id)?.pulse_run_id
    if (!latestPulseRunID) return null
    const rows = agentMetrics.filter((metric) => metric.pulse_run_id === latestPulseRunID)
    const captured = rows.filter((metric) => metric.usage_status === 'captured')
    const started = rows
      .map((metric) => Date.parse(metric.started_at || ''))
      .filter(Number.isFinite)
    const completed = rows
      .map((metric) => Date.parse(metric.completed_at || ''))
      .filter(Number.isFinite)
    return {
      pulseRunID: latestPulseRunID,
      reviewers: rows.filter((metric) => metric.role === 'reviewer').length,
      fixers: rows.filter((metric) => metric.role === 'fixer').length,
      agentTimeMS: rows.reduce((sum, metric) => sum + metric.duration_ms, 0),
      wallTimeMS: started.length > 0 && completed.length > 0
        ? Math.max(...completed) - Math.min(...started)
        : 0,
      calls: captured.reduce((sum, metric) => sum + metric.llm_call_count, 0),
      promptTokens: captured.reduce((sum, metric) => sum + metric.prompt_tokens, 0),
      cacheReadTokens: captured.reduce((sum, metric) => sum + metric.cache_read_tokens, 0),
      completionTokens: captured.reduce((sum, metric) => sum + metric.completion_tokens, 0),
      cost: captured.reduce((sum, metric) => sum + metric.total_cost_usd, 0),
      unavailable: rows.length - captured.length,
    }
  }, [agentMetrics])

  const health = queueCounts.needs_action > 0
    ? {
        label: 'Pulse work queued',
        detail: `${queueCounts.needs_action} issue${queueCounts.needs_action === 1 ? '' : 's'} for Pulse to diagnose or repair`,
        tone: 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300',
        Icon: AlertTriangle,
      }
    : queueCounts.decisions > 0
      ? {
          label: 'Your decision needed',
          detail: `${queueCounts.decisions} item${queueCounts.decisions === 1 ? '' : 's'} cannot continue without approval`,
          tone: 'border-fuchsia-500/30 bg-fuchsia-500/10 text-fuchsia-700 dark:text-fuchsia-300',
          Icon: Clock3,
        }
      : queueCounts.waiting_proof > 0
        ? {
          label: 'Waiting for proof',
          detail: `${queueCounts.waiting_proof} fix${queueCounts.waiting_proof === 1 ? '' : 'es'} need verification evidence`,
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
              <span className="font-medium text-foreground">Pulse:</span> {queueCounts.needs_action} to repair
              <span className="px-1.5">·</span>
              <span className="font-medium text-foreground">You:</span> {queueCounts.decisions} decision{queueCounts.decisions === 1 ? '' : 's'}
              <span className="px-1.5">·</span>
              <span className="font-medium text-foreground">Ideas:</span> {queueCounts.proposals} proposal{queueCounts.proposals === 1 ? '' : 's'}
              <span className="px-1.5">·</span>
              <span className="font-medium text-foreground">Next run:</span> {queueCounts.waiting_proof} verification{queueCounts.waiting_proof === 1 ? '' : 's'}
            </div>
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

      <section className="overflow-hidden rounded-xl border bg-background">
        <div className="flex flex-wrap items-start justify-between gap-3 border-b px-4 py-3">
          <div>
            <h3 className="text-sm font-semibold text-foreground">Goal impact</h3>
            <p className="mt-0.5 text-[11px] text-muted-foreground">
              Fixes and experiments linked to comparable success-criterion observations
            </p>
          </div>
          <div className="flex flex-wrap gap-1.5 text-[10px] font-semibold">
            <span className="rounded-full border border-emerald-500/25 bg-emerald-500/5 px-2 py-1 text-emerald-700 dark:text-emerald-300">{impactSummary.improved} improved</span>
            <span className="rounded-full border border-red-500/25 bg-red-500/5 px-2 py-1 text-red-700 dark:text-red-300">{impactSummary.regressed} regressed</span>
            <span className="rounded-full border border-amber-500/25 bg-amber-500/5 px-2 py-1 text-amber-700 dark:text-amber-300">{impactSummary.inconclusive} inconclusive</span>
            <span className="rounded-full border px-2 py-1 text-muted-foreground">{impactSummary.awaiting} awaiting evidence</span>
          </div>
        </div>
        {impactSummary.latest.length === 0 ? (
          <div className="flex items-start gap-2 px-4 py-3 text-xs text-muted-foreground">
            <Clock3 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-500" />
            <span>No comparable result yet. A producing workflow run is needed before Pulse can say whether these changes improved the goal.</span>
          </div>
        ) : (
          <div className="grid gap-px bg-border sm:grid-cols-2 lg:grid-cols-4">
            {impactSummary.latest.map((observation) => (
              <div key={observation.observation_id} className="bg-background px-4 py-3">
                <div className="truncate text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">{readable(observation.criterion_id)}</div>
                <div className="mt-1 text-sm font-semibold text-foreground">
                  {typeof observation.value === 'number'
                    ? `${observation.value}${observation.unit ? ` ${observation.unit}` : ''}`
                    : readable(observation.status)}
                </div>
                <div className="mt-1 truncate text-[10px] text-muted-foreground">{readable(observation.metric)} · {formatDate(observation.observed_at)}</div>
              </div>
            ))}
          </div>
        )}
      </section>

      {(error || statusError) && (
        <div className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          Some Pulse data could not be loaded: {[statusError, error].filter(Boolean).join(' ')}
        </div>
      )}

      {/* One lifecycle queue per card. These totals are mutually exclusive, so
          the operator can read the actual workload without double-counting a
          repair as open, fixing, and awaiting verification at the same time. */}
      <section className="grid grid-cols-2 gap-1.5 md:grid-cols-4 xl:grid-cols-7">
        <Metric
          label="Pulse to fix"
          focus="needs_action"
          activeFocus={focus}
          onFocus={setFocus}
          value={queueCounts.needs_action}
          detail="owned by Pulse"
          tone="border-red-500/25 bg-red-500/5 text-red-700 dark:text-red-300"
        />
        <Metric
          label="Waiting on run"
          focus="waiting_proof"
          activeFocus={focus}
          onFocus={setFocus}
          value={queueCounts.waiting_proof}
          detail="verify after workflow runs"
          tone="border-amber-500/25 bg-amber-500/5 text-amber-700 dark:text-amber-300"
        />
        <Metric
          label="Your decisions"
          focus="decisions"
          activeFocus={focus}
          onFocus={setFocus}
          value={queueCounts.decisions}
          detail="approval or direction needed"
          tone="border-fuchsia-500/25 bg-fuchsia-500/5 text-fuchsia-700 dark:text-fuchsia-300"
        />
        <Metric
          label="Proposed improvements"
          focus="proposals"
          activeFocus={focus}
          onFocus={setFocus}
          value={queueCounts.proposals}
          detail="ideas, no answer required"
          tone="border-blue-500/25 bg-blue-500/5 text-blue-700 dark:text-blue-300"
        />
        <Metric
          label="Platform team"
          focus="platform"
          activeFocus={focus}
          onFocus={setFocus}
          value={queueCounts.platform}
          detail="owned outside this workflow"
          tone="border-violet-500/25 bg-violet-500/5 text-violet-700 dark:text-violet-300"
        />
        <Metric
          label="Resolved"
          focus="resolved"
          activeFocus={focus}
          onFocus={setFocus}
          value={queueCounts.resolved}
          detail="verified or legitimately closed"
          tone="border-emerald-500/25 bg-emerald-500/5 text-emerald-700 dark:text-emerald-300"
        />
        <Metric
          label="Workflow evidence"
          focus="workflow_reported"
          activeFocus={focus}
          onFocus={setFocus}
          value={queueCounts.workflow_reported}
          detail="reported by workflow runs"
          tone="border-orange-500/25 bg-orange-500/5 text-orange-700 dark:text-orange-300"
        />
      </section>

      <ReportHumanInputPanel workspacePath={workspacePath} contentMode="pending" />

      <div className="grid gap-4">
        <section className="overflow-hidden rounded-xl border bg-background">
          <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
            <div>
              <h3 className="text-sm font-semibold text-foreground">Issues</h3>
              <p className="mt-0.5 text-[11px] text-muted-foreground">
                {FOCUS_TITLES[focus]}
                {moduleFilter && (
                  <span className="ml-1 font-normal">
                    in {moduleSummaries.find((m) => m.id === moduleFilter)?.label || moduleFilter}
                  </span>
                )}
                {' · '}{FOCUS_HINTS[focus]}
              </p>
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
              <div className="mt-2 text-sm font-medium text-foreground">
                {focus === 'resolved' ? 'No resolved issues yet' : 'Nothing in this queue'}
              </div>
              <div className="mt-1 text-xs text-muted-foreground">Choose another queue or inspect the review modules below.</div>
            </div>
          ) : (
            <div className="space-y-2 p-3">
              {attentionFindings.map((finding) => {
                const module = moduleSummaries.find((item) => item.id === finding.module)
                return (
                  <PulseFindingCard
                    key={finding.fingerprint}
                    finding={finding}
                    moduleLabel={module?.label}
                    expanded={expandedFinding === finding.fingerprint}
                    onToggle={() => setExpandedFinding(
                      expandedFinding === finding.fingerprint ? null : finding.fingerprint,
                    )}
                    onOpenModule={() => finding.module && setSelectedModule(finding.module)}
                  />
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
          <h3 className="text-sm font-semibold text-foreground">Reviewers</h3>
          <p className="mt-0.5 text-[11px] text-muted-foreground">Choose a reviewer to see its latest judgment or open the full forensic report</p>
		  {latestPassMetrics && (
		    <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-[10px] text-muted-foreground" title={latestPassMetrics.pulseRunID}>
		      <span className="font-medium text-foreground">Latest measured pass</span>
		      <span>{latestPassMetrics.reviewers} reviewer{latestPassMetrics.reviewers === 1 ? '' : 's'}{latestPassMetrics.fixers > 0 ? ` + ${latestPassMetrics.fixers} fixer` : ''}</span>
		      <span>{formatAgentDuration(latestPassMetrics.wallTimeMS)} wall</span>
		      <span>{formatAgentDuration(latestPassMetrics.agentTimeMS)} agent time</span>
		      <span>{latestPassMetrics.calls} calls</span>
		      <span>{compactMetricNumber.format(latestPassMetrics.promptTokens)} input</span>
		      <span>{compactMetricNumber.format(latestPassMetrics.cacheReadTokens)} cached</span>
		      <span>{compactMetricNumber.format(latestPassMetrics.completionTokens)} output</span>
		      <span>${latestPassMetrics.cost.toFixed(2)}</span>
		      {latestPassMetrics.unavailable > 0 && <span className="text-amber-700 dark:text-amber-300">{latestPassMetrics.unavailable} usage unavailable</span>}
		    </div>
		  )}
        </div>
        <div className="grid gap-px bg-border sm:grid-cols-2 lg:grid-cols-3">
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
				{module.latestReview?.metrics && (
				  <div className="mt-1 text-[9px] text-muted-foreground">
				    {formatAgentDuration(module.latestReview.metrics.duration_ms)}
				    {module.latestReview.metrics.usage_status === 'captured' && (
				      <> · {compactMetricNumber.format(module.latestReview.metrics.completion_tokens)} output · ${module.latestReview.metrics.total_cost_usd.toFixed(2)}</>
				    )}
				  </div>
				)}
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
            reviews={reviews.filter((review) => review.module === selectedDefinition.id)}
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

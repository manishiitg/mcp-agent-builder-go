import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Lightbulb,
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
  PulseContextRecord,
  PulseAgentMetricRecord,
  PulseModuleState,
  PulseRunMode,
  PulseReviewRecord,
  PulseReviewFocus,
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
  normalizePulseWorkspaceModule,
  selectPulseWorkspaceModule,
} from './pulseWorkspaceUtils'
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
  const text = (value || '').trim().replaceAll('_', ' ')
  return text ? text.charAt(0).toUpperCase() + text.slice(1) : 'No data'
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

type LatestWorkflowRun = {
  folder: string
  status: string
  created_at?: string
  completed_at?: string
  completed_steps: number
  total_steps: number
  is_running: boolean
}

const FOCUS_TITLES: Record<PulseFocus, string> = {
  all: 'Current work',
  needs_action: 'Pulse to fix',
  queued_repair: 'Queued for Pulse',
  waiting_proof: 'Waiting on a run',
  decisions: 'Your decisions',
  proposals: 'Proposed improvements',
  blocked: 'Paused',
  platform: 'Platform repair pending',
  resolved: 'Resolved',
  workflow_reported: 'Workflow evidence',
}

const FOCUS_HINTS: Record<PulseFocus, string> = {
  all: 'Open work, grouped by who or what can move it forward',
  needs_action: 'Issues Pulse can diagnose, repair, or reopen',
  queued_repair: 'Safe workflow repairs retained for a later Engineering pass',
  waiting_proof: 'Fixes that need evidence from a future workflow run',
  decisions: 'Items that cannot continue without your approval or direction',
  proposals: 'Ideas Pulse recommends considering; these are not waiting for your answer',
  blocked: 'Diagnosed issues with no safe action currently available',
  platform: 'Diagnosed runtime or product repairs outside this workflow',
  resolved: 'Verified fixes and legitimate no-change closures',
  workflow_reported: 'Evidence filed by workflow steps, kept separate from Pulse\u2019s repair queue',
}

export function PulseWorkspace({
  workspacePath,
  monitorOn,
  moduleStates,
  finalCommandStates,
  gateMode,
  reviewFocuses,
  statusLoading,
  statusError,
  onRefresh,
}: {
  workspacePath: string
  monitorOn: boolean
  moduleStates: PulseModuleState[]
  finalCommandStates: PulseFinalCommandState[]
  gateMode: PulseRunMode | null
  reviewFocuses: PulseReviewFocus[]
  statusLoading: boolean
  statusError: string | null
  onRefresh: () => void
}) {
  const [findings, setFindings] = useState<PulseFindingLifecycle[]>([])
  const [reviews, setReviews] = useState<PulseReviewRecord[]>([])
  const [agentMetrics, setAgentMetrics] = useState<PulseAgentMetricRecord[]>([])
  const [impact, setImpact] = useState<PulseImpactLedger>({ interventions: [], observations: [], assessments: [] })
  const [contextRecords, setContextRecords] = useState<PulseContextRecord[]>([])
  const [latestRun, setLatestRun] = useState<LatestWorkflowRun | null>(null)
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
  const [showCompleteBacklog, setShowCompleteBacklog] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!workspacePath) return
    setLoading(true)
    setError(null)
    const [findingResult, reviewResult, impactResult, contextResult, metricResult, runResult] = await Promise.allSettled([
      agentApi.getPulseFindings(workspacePath),
      agentApi.getPulseReviews(workspacePath),
      agentApi.getPulseImpact(workspacePath),
      agentApi.getPulseContext(workspacePath),
      agentApi.getPulseAgentMetrics(workspacePath),
      agentApi.getWorkflowsSummary([workspacePath]),
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
    if (contextResult.status === 'fulfilled' && contextResult.value.success) {
      setContextRecords(contextResult.value.records || [])
    } else {
      setContextRecords([])
      errors.push(
        contextResult.status === 'rejected'
          ? (contextResult.reason instanceof Error ? contextResult.reason.message : 'Could not load user context.')
          : contextResult.value.error || 'Could not load user context.',
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
    if (runResult.status === 'fulfilled' && runResult.value.success) {
      const workflow = runResult.value.workflows.find((item) => item.workspace_path === workspacePath)
        || runResult.value.workflows[0]
      const retainedRun = workflow?.latest_run
      // Older zero-step folders are bookkeeping artifacts, not useful run
      // outcomes. Showing one as the "latest run" is actively misleading.
      setLatestRun(retainedRun && (workflow.is_running || retainedRun.total_steps > 0)
        ? { ...retainedRun, is_running: workflow.is_running }
        : null)
    } else {
      setLatestRun(null)
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
    setContextRecords([])
    setLatestRun(null)
    void load()
  }, [load])

  const queueCounts = useMemo(() => {
    const counts: Record<PulseFindingQueue, number> = {
      needs_action: 0,
      queued_repair: 0,
      waiting_proof: 0,
      decisions: 0,
      proposals: 0,
      blocked: 0,
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
    () => new Map(moduleStates.map((state) => [normalizePulseWorkspaceModule(state.module), state])),
    [moduleStates],
  )
  const finalCommandStateByID = useMemo(
    () => new Map(finalCommandStates.map((state) => [state.command, state])),
    [finalCommandStates],
  )
  const matchingFindings = useMemo(
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
        .filter((finding) => (
          !moduleFilter
          || normalizePulseWorkspaceModule(finding.module) === moduleFilter
        ))
        .sort((a, b) => {
          const rank: Record<PulseFindingQueue, number> = {
            needs_action: 6,
            queued_repair: 5,
            waiting_proof: 4,
            decisions: 3,
            proposals: 2,
            blocked: 1,
            platform: 0,
            workflow_reported: 0,
            resolved: 0,
          }
          const priority = rank[pulseFindingPresentation(b).queue] - rank[pulseFindingPresentation(a).queue]
          return priority || (b.last_seen_at || '').localeCompare(a.last_seen_at || '')
        })
      return matched
    },
    [findings, focus, moduleFilter],
  )
  // Keep the initial dashboard scannable, but never hide the complete backlog
  // behind an unexplained cap. Queue and module filters always show every match;
  // the unfiltered backlog exposes an explicit one-click expansion.
  const attentionFindings = useMemo(
    () => focus === 'all' && !moduleFilter && !showCompleteBacklog
      ? matchingFindings.slice(0, 25)
      : matchingFindings,
    [focus, matchingFindings, moduleFilter, showCompleteBacklog],
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
      awaiting: impact.interventions.filter((item) => ['awaiting_evidence', 'proposed', 'approved', 'running', 'measuring', 'blocked'].includes(item.status)).length,
      strategyExperiments: impact.interventions
        .filter((item) => item.kind === 'strategy_experiment')
        .sort((left, right) => (right.updated_at || '').localeCompare(left.updated_at || '')),
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

      <section className="overflow-hidden rounded-xl border bg-gradient-to-br from-primary/7 via-background to-background">
        <div className="flex flex-wrap items-start justify-between gap-4 p-4 sm:p-5">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
              <Activity className="h-3.5 w-3.5" />
              Latest outcome
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-semibold ${health.tone}`}>
                <health.Icon className="h-3.5 w-3.5" />
                {health.label}
              </span>
              <span className="text-xs text-muted-foreground">{health.detail}</span>
            </div>
            <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-muted-foreground">
              <span>
                <span className="font-medium text-foreground">Latest retained run:</span>{' '}
                {latestRun
                  ? readable(latestRun.is_running ? 'running' : latestRun.status)
                  : 'Not recorded'}
              </span>
              {latestRun && latestRun.total_steps > 0 && (
                <span>
                  {latestRun.completed_steps}/{latestRun.total_steps} steps
                  {' · '}{formatDate(latestRun.completed_at || latestRun.created_at)}
                </span>
              )}
              {gateMode && (
                <span title={gateMode.reason}>
                  <span className="font-medium text-foreground">Pulse mode:</span>{' '}
                  {readable(gateMode.mode)}
                </span>
              )}
              <span><span className="font-medium text-foreground">Pulse owns:</span> {queueCounts.needs_action}</span>
              <span><span className="font-medium text-foreground">Queued for Pulse:</span> {queueCounts.queued_repair}</span>
              <span><span className="font-medium text-foreground">You own:</span> {queueCounts.decisions}</span>
              <span><span className="font-medium text-foreground">Waiting on runs:</span> {queueCounts.waiting_proof}</span>
              {queueCounts.platform > 0 && <span><span className="font-medium text-foreground">Platform repair pending:</span> {queueCounts.platform}</span>}
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
          </div>
        </div>
        {!monitorOn && (
          <div className="border-t border-dashed px-4 py-2.5 text-xs text-muted-foreground sm:px-5">
            Scheduled Pulse is off. Saved reviews and lifecycle history remain available.
          </div>
        )}
      </section>

      <ReportHumanInputPanel workspacePath={workspacePath} contentMode="all" providedImpact={impact} />

      <section className="overflow-hidden rounded-xl border bg-background">
        <div className="border-b px-4 py-3">
          <h3 className="text-sm font-semibold text-foreground">Work areas</h3>
          <p className="mt-0.5 text-[11px] text-muted-foreground">
            What Pulse found, who owns the next move, and the latest judgment
          </p>
        </div>
        <div className="grid gap-px bg-border lg:grid-cols-2">
          {[
            {
              id: 'technical_review',
              title: 'Technical review',
              icon: Wrench,
              description: 'Correctness, stores, runtime, orchestration, tools, models, cost, and execution efficiency',
              tone: 'text-sky-600 dark:text-sky-300',
            },
            {
              id: 'strategic_review',
              title: 'Strategic review',
              icon: Lightbulb,
              description: 'Hidden strategic mechanisms and materially different opportunities for the goal',
              tone: 'text-amber-600 dark:text-amber-300',
            },
          ].map((area) => {
            const areaModules = moduleSummaries.filter((module) => module.id === area.id)
            const decisions = areaModules.reduce((sum, module) => sum + module.awaitingUser, 0)
            const proposals = areaModules.reduce((sum, module) => sum + module.proposals, 0)
            const strategic = area.id === 'strategic_review'
            const actionable = strategic
              ? decisions + proposals
              : areaModules.reduce((sum, module) => sum + module.active + module.fixing + module.queuedForEngineering, 0)
            const waiting = areaModules.reduce((sum, module) => (
              sum + module.awaitingVerification + module.awaitingRun
            ), 0)
            const blocked = areaModules.reduce((sum, module) => sum + module.blocked, 0)
            const external = areaModules.reduce((sum, module) => sum + module.externalAction, 0)
            const latest = [...areaModules]
              .sort((a, b) => (b.latestReview?.recorded_at || '').localeCompare(a.latestReview?.recorded_at || ''))[0]
              ?.latestReview
            const latestFocus = reviewFocuses
              .filter((item) => normalizePulseWorkspaceModule(item.module) === area.id && item.last_reviewed_at)
              .sort((a, b) => (b.last_reviewed_at || '').localeCompare(a.last_reviewed_at || ''))[0]
            const upcomingFocuses = reviewFocuses
              .filter((item) => (
                normalizePulseWorkspaceModule(item.module) === area.id
                && item.focus_key !== latestFocus?.focus_key
              ))
              .slice(0, 3)
            const nextFocusKeys = latestFocus?.deferred_focuses?.length
              ? latestFocus.deferred_focuses.slice(0, 3)
              : upcomingFocuses.map((item) => item.focus_key)
            const moduleID = area.id
            return (
              <button
                key={area.id}
                type="button"
                onClick={() => {
                  setSelectedModule(moduleID)
                  setModuleFilter(moduleID)
                  if (strategic && (decisions > 0 || proposals > 0)) {
                    setFocus(decisions > 0 ? 'decisions' : 'proposals')
                  }
                  setShowCompleteBacklog(false)
                }}
                className="min-w-0 bg-background p-4 text-left transition-colors hover:bg-muted/35 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary/40"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex min-w-0 items-start gap-2.5">
                    <area.icon className={`mt-0.5 h-4 w-4 shrink-0 ${area.tone}`} />
                    <div className="min-w-0">
                      <h4 className="text-xs font-semibold text-foreground">{area.title}</h4>
                      <p className="mt-1 line-clamp-2 text-[10px] leading-4 text-muted-foreground">{area.description}</p>
                    </div>
                  </div>
                  {actionable > 0 && (
                    <span className="rounded-full border border-red-500/25 bg-red-500/5 px-2 py-0.5 text-[9px] font-semibold text-red-700 dark:text-red-300">
                      {actionable} {strategic ? 'recommendations' : 'to fix'}
                    </span>
                  )}
                </div>
                <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-[10px] text-muted-foreground">
                  {strategic ? (
                    <>
                      <span><span className="font-semibold text-foreground">{proposals}</span> ideas</span>
                      <span><span className="font-semibold text-foreground">{decisions}</span> decisions</span>
                    </>
                  ) : (
                    <>
                      <span><span className="font-semibold text-foreground">{actionable}</span> Pulse to fix</span>
                      <span><span className="font-semibold text-foreground">{waiting}</span> waiting on run</span>
                      {decisions > 0 && <span><span className="font-semibold text-foreground">{decisions}</span> decisions</span>}
                      {blocked > 0 && <span><span className="font-semibold text-foreground">{blocked}</span> blocked</span>}
                      {external > 0 && <span><span className="font-semibold text-foreground">{external}</span> platform</span>}
                      {proposals > 0 && <span><span className="font-semibold text-foreground">{proposals}</span> ideas</span>}
                    </>
                  )}
                </div>
                <div className="mt-3 border-t pt-2 text-[10px] leading-4 text-muted-foreground">
                  <span className="font-medium text-foreground">Latest:</span>{' '}
                  <span className="line-clamp-2">
                    {latest ? latest.verdict || 'Review recorded' : 'No stored review yet'}
                  </span>
                  {latestFocus ? (
                    <div className="mt-1.5 text-[10px] text-muted-foreground">
                      <span className="font-medium text-foreground">Last focus:</span>{' '}
                      {readable(latestFocus.focus_key)} · {formatDate(latestFocus.last_reviewed_at)}
                      {(latestFocus.review_count || 0) > 0 && (
                        <span> · reviewed {latestFocus.review_count} {latestFocus.review_count === 1 ? 'time' : 'times'}</span>
                      )}
                      {latestFocus.last_selection_reason && (
                        <span className="mt-0.5 block line-clamp-2">{latestFocus.last_selection_reason}</span>
                      )}
                      {nextFocusKeys.length > 0 && (
                        <span className="mt-0.5 block line-clamp-2">
                          Next focus candidates: {nextFocusKeys.map(readable).join(', ')}
                        </span>
                      )}
                    </div>
                  ) : upcomingFocuses.length > 0 ? (
                    <div className="mt-1.5 text-[10px] text-muted-foreground">
                      <span className="font-medium text-foreground">Next focus candidates:</span>{' '}
                      {upcomingFocuses.map((item) => readable(item.focus_key)).join(', ')}
                    </div>
                  ) : null}
                </div>
              </button>
            )
          })}
        </div>
      </section>

      {(error || statusError) && (
        <div className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          Some Pulse data could not be loaded: {[statusError, error].filter(Boolean).join(' ')}
        </div>
      )}

      <div className="grid gap-4">
        <section className="overflow-hidden rounded-xl border bg-background">
          <div className="border-b px-4 py-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold text-foreground">Issues and follow-through</h3>
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
                    onClick={() => { setFocus('all'); setModuleFilter(null); setShowCompleteBacklog(false) }}
                    className="rounded-full border px-2 py-1 text-[10px] font-semibold text-muted-foreground hover:bg-muted"
                  >
                    Clear filter
                  </button>
                )}
                {focus === 'all' && !moduleFilter && matchingFindings.length > 25 && (
                  <button
                    type="button"
                    onClick={() => setShowCompleteBacklog((shown) => !shown)}
                    className="rounded-full border px-2 py-1 text-[10px] font-semibold text-primary hover:bg-primary/10"
                  >
                    {showCompleteBacklog ? 'Show first 25' : `Show all ${matchingFindings.length}`}
                  </button>
                )}
                <span className="rounded-full bg-muted px-2 py-1 text-[10px] font-semibold text-muted-foreground">
                  {attentionFindings.length === matchingFindings.length
                    ? `${attentionFindings.length} shown`
                    : `${attentionFindings.length} of ${matchingFindings.length} shown`}
                </span>
              </div>
            </div>
            <div className="mt-3 flex flex-wrap gap-1.5" aria-label="Issue filters">
              {([
                ['all', 'Current', findings.filter((finding) => !['resolved', 'workflow_reported'].includes(pulseFindingPresentation(finding).queue)).length],
                ['needs_action', 'Pulse to fix', queueCounts.needs_action],
                ['queued_repair', 'Queued for Pulse', queueCounts.queued_repair],
                ['waiting_proof', 'Waiting on run', queueCounts.waiting_proof],
                ['decisions', 'Your decisions', queueCounts.decisions],
                ['proposals', 'Ideas', queueCounts.proposals],
                ['blocked', 'Paused', queueCounts.blocked],
                ['platform', 'Platform repair pending', queueCounts.platform],
                ['resolved', 'Resolved', queueCounts.resolved],
              ] as Array<[PulseFocus, string, number]>).map(([value, label, count]) => (
                <button
                  key={value}
                  type="button"
                  aria-pressed={focus === value}
                  onClick={() => { setFocus(value); setShowCompleteBacklog(false) }}
                  className={`rounded-full border px-2.5 py-1 text-[10px] font-semibold transition-colors ${
                    focus === value
                      ? 'border-primary/35 bg-primary/10 text-primary'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                  }`}
                >
                  {label} <span className="ml-1 tabular-nums opacity-75">{count}</span>
                </button>
              ))}
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
                const moduleID = normalizePulseWorkspaceModule(finding.module)
                const module = moduleSummaries.find((item) => item.id === moduleID)
                return (
                  <PulseFindingCard
                    key={finding.fingerprint}
                    finding={finding}
                    moduleLabel={module?.label}
                    expanded={expandedFinding === finding.fingerprint}
                    onToggle={() => setExpandedFinding(
                      expandedFinding === finding.fingerprint ? null : finding.fingerprint,
                    )}
                    onOpenModule={() => moduleID && setSelectedModule(moduleID)}
                  />
                )
              })}
            </div>
          )}
        </section>

        <section className="overflow-hidden rounded-xl border bg-background">
          <div className="border-b px-4 py-3">
            <h3 className="text-sm font-semibold text-foreground">Recent fixes and follow-through</h3>
            <p className="mt-0.5 text-[11px] text-muted-foreground">What changed after issues were filed</p>
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

        {contextRecords.length > 0 && (
          <section className="overflow-hidden rounded-xl border bg-background">
            <div className="border-b px-4 py-3">
              <h3 className="text-sm font-semibold text-foreground">User rules</h3>
              <p className="mt-0.5 text-[11px] text-muted-foreground">Confirmed context that future workflow runs must respect</p>
            </div>
            <div className="divide-y">
              {contextRecords.slice(0, 5).map((record) => (
                <div key={record.context_id} className="px-4 py-3">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">{record.section}</span>
                    <span className="shrink-0 text-[9px] text-muted-foreground">{formatDate(record.created_at)}</span>
                  </div>
                  <p className="mt-1 text-xs leading-5 text-foreground">{record.context_text}</p>
                  {record.example_note && <p className="mt-1 text-[10px] leading-4 text-muted-foreground">{record.example_note}</p>}
                </div>
              ))}
            </div>
          </section>
        )}
      </div>

      <section className="overflow-hidden rounded-xl border bg-background">
        <div className="border-b px-4 py-3">
          <h3 className="text-sm font-semibold text-foreground">Pulse activity</h3>
          <p className="mt-0.5 text-[11px] text-muted-foreground">
            What Gate selected, which review and repair turns ran, and how each sequence closed
          </p>
		  {latestPassMetrics && (
		    <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-[10px] text-muted-foreground" title={latestPassMetrics.pulseRunID}>
		      <span className="font-medium text-foreground">Latest measured pass</span>
		      <span>{latestPassMetrics.reviewers} review turn{latestPassMetrics.reviewers === 1 ? '' : 's'}{latestPassMetrics.fixers > 0 ? ` + ${latestPassMetrics.fixers} repair turn${latestPassMetrics.fixers === 1 ? '' : 's'}` : ' · no repair turn needed'}</span>
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
        <div className="grid gap-px bg-border sm:grid-cols-2 xl:grid-cols-3">
          {moduleSummaries.map((module) => {
            const state = moduleStateByID.get(module.id)
            const active = selectedModule === module.id
            const openWork = module.active + module.fixing + module.queuedForEngineering
            const waitingWork = module.awaitingVerification + module.awaitingRun
            return (
              <button
                key={module.id}
                type="button"
                onClick={() => { setSelectedModule(module.id); setModuleFilter(module.id); setShowCompleteBacklog(false) }}
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
                  {waitingWork > 0 && (
                    <span className="text-[9px] font-medium text-amber-700 dark:text-amber-300">{waitingWork} waiting</span>
                  )}
                  {module.awaitingUser > 0 && (
                    <span className="text-[9px] font-medium text-fuchsia-700 dark:text-fuchsia-300">{module.awaitingUser} decisions</span>
                  )}
                  {module.queuedForEngineering > 0 && (
                    <span className="text-[9px] font-medium text-sky-700 dark:text-sky-300">{module.queuedForEngineering} queued</span>
                  )}
                  {module.blocked > 0 && (
                    <span className="text-[9px] font-medium text-muted-foreground">{module.blocked} blocked</span>
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
                {(state?.last_result_reason || state?.last_reason) && (
                  <div className="mt-1 line-clamp-2 text-[9px] leading-4 text-muted-foreground">
                    {state.last_result_reason || state.last_reason}
                  </div>
                )}
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
            reviews={reviews.filter((review) => (
              normalizePulseWorkspaceModule(review.module) === selectedDefinition.id
            ))}
          />
        </section>
      )}

      <section className="overflow-hidden rounded-xl border bg-background">
        <div className="flex flex-wrap items-start justify-between gap-3 border-b px-4 py-3">
          <div>
            <h3 className="text-sm font-semibold text-foreground">Impact over time</h3>
            <p className="mt-0.5 text-[11px] text-muted-foreground">
              Whether Pulse changes improved the workflow’s actual success measures
            </p>
          </div>
          <div className="flex flex-wrap gap-1.5 text-[10px] font-semibold">
            <span className="rounded-full border border-emerald-500/25 bg-emerald-500/5 px-2 py-1 text-emerald-700 dark:text-emerald-300">{impactSummary.improved} improved</span>
            <span className="rounded-full border border-red-500/25 bg-red-500/5 px-2 py-1 text-red-700 dark:text-red-300">{impactSummary.regressed} regressed</span>
            <span className="rounded-full border border-amber-500/25 bg-amber-500/5 px-2 py-1 text-amber-700 dark:text-amber-300">{impactSummary.inconclusive} inconclusive</span>
            <span className="rounded-full border px-2 py-1 text-muted-foreground">{impactSummary.awaiting} awaiting evidence</span>
          </div>
        </div>
        {impactSummary.strategyExperiments.length > 0 && (
          <div className="divide-y border-b">
            {impactSummary.strategyExperiments.map((experiment) => (
              <div key={experiment.intervention_id} className="px-4 py-3 text-xs">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <div className="font-semibold text-foreground">Strategy experiment: {experiment.title}</div>
                    <div className="mt-1 text-[11px] text-muted-foreground">
                      {readable(experiment.metric)} · checkpoint {readable(experiment.checkpoint)}
                    </div>
                  </div>
                  <span className={`rounded-full border px-2 py-1 text-[10px] font-semibold ${statusTone(experiment.status)}`}>
                    {readable(experiment.status)}
                  </span>
                </div>
                {experiment.interference_domains?.length ? (
                  <div className="mt-2 text-[10px] leading-4 text-muted-foreground">Interference: {experiment.interference_domains.join(' · ')}</div>
                ) : null}
                {experiment.guardrails?.length ? (
                  <div className="mt-1 text-[10px] leading-4 text-muted-foreground">Guardrails: {experiment.guardrails.join(' · ')}</div>
                ) : null}
              </div>
            ))}
          </div>
        )}
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

import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  CheckCircle2,
  GitCompare,
  Lightbulb,
  Loader2,
  Wrench,
  X,
} from 'lucide-react'
import { agentApi } from '../../services/api'
import type {
  PulseFinalCommandState,
  PulseFindingLifecycle,
  PulseImpactLedger,
  PulseContextRecord,
  PulseModuleState,
  PulseReviewRecord,
  PulseReviewFocus,
} from '../../services/api-types'
import { ReportHumanInputPanel } from './ReportHumanInputPanel'
import { WORKFLOW_LOG_REFRESH_EVENT } from './workflowEvents'
import { SoulViewer } from './SoulViewer'
import { PulseFindingCard } from './PulseFindingCard'
import { pulseFindingPresentation, type PulseFindingQueue } from './pulseFindingPresentation'
import { isPulseOwnedFinding, pulseIssueForFinding } from './pulseModuleInspectorUtils'
import {
  buildPulseWorkspaceModuleSummaries,
  normalizePulseWorkspaceModule,
  pulseFindingReviewAreas,
  pulseFindingMatchesFocus,
  pulseWorkspaceQueueCounts,
  type PulseFocus,
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

function formatCheckBoundary(value?: string): string {
  if (!value) return ''
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    const date = new Date(`${value}T00:00:00`)
    return Number.isNaN(date.getTime())
      ? value
      : date.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
  }
  return formatDate(value)
}

function readable(value?: string): string {
  const text = (value || '').trim().replaceAll('_', ' ')
  return text ? text.charAt(0).toUpperCase() + text.slice(1) : 'No data'
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

function finalCommandLabel(state?: PulseFinalCommandState): string {
  return readable(state?.status)
}

type ReviewFocusLabel = {
  label: string
  relatedCount: number
}

const FOCUS_TITLES: Record<PulseFocus, string> = {
  all: 'Current work',
  needs_action: 'Pulse to fix',
  queued_repair: 'Queued for Pulse',
  waiting_proof: 'Waiting for evidence',
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
  waiting_proof: 'Fixes and recommendations waiting for a verification check or future evidence',
  decisions: 'Items that cannot continue without your approval or direction',
  proposals: 'Ideas Pulse recommends considering; these are not waiting for your answer',
  blocked: 'Diagnosed issues with no safe action currently available',
  platform: 'Diagnosed runtime or product repairs outside this workflow',
  resolved: 'Verified fixes and legitimate no-change closures',
  workflow_reported: 'Evidence filed by workflow steps, kept separate from Pulse\u2019s repair queue',
}

export function PulseWorkspace({
  workspacePath,
  moduleStates,
  finalCommandStates,
  reviewFocuses,
  reviewFocusSelections,
  statusError,
}: {
  workspacePath: string
  moduleStates: PulseModuleState[]
  finalCommandStates: PulseFinalCommandState[]
  reviewFocuses: PulseReviewFocus[]
  reviewFocusSelections: PulseReviewFocus[]
  statusError: string | null
}) {
  const [findings, setFindings] = useState<PulseFindingLifecycle[]>([])
  const [reviews, setReviews] = useState<PulseReviewRecord[]>([])
  const [impact, setImpact] = useState<PulseImpactLedger>({ interventions: [], observations: [], assessments: [] })
  const [contextRecords, setContextRecords] = useState<PulseContextRecord[]>([])
  const [focus, setFocus] = useState<PulseFocus>('all')
  const [moduleFilter, setModuleFilter] = useState<string | null>(null)
  const [expandedFinding, setExpandedFinding] = useState<string | null>(null)
  const [showCompleteBacklog, setShowCompleteBacklog] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async (showLoading = true) => {
    if (!workspacePath) return
    if (showLoading) setLoading(true)
    if (showLoading) setError(null)
    const [findingResult, reviewResult, impactResult, contextResult] = await Promise.allSettled([
      agentApi.getPulseFindings(workspacePath),
      agentApi.getPulseReviews(workspacePath),
      agentApi.getPulseImpact(workspacePath),
      agentApi.getPulseContext(workspacePath),
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
    setError(errors.length > 0 ? errors.join(' ') : null)
    setLoading(false)
  }, [workspacePath])

  useEffect(() => {
    setFocus('all')
    setModuleFilter(null)
    setExpandedFinding(null)
    setShowCompleteBacklog(false)
    setFindings([])
    setReviews([])
    setImpact({ interventions: [], observations: [], assessments: [] })
    setContextRecords([])
    void load()
  }, [load])

  useEffect(() => {
    const onRefresh = () => { void load(false) }
    window.addEventListener(WORKFLOW_LOG_REFRESH_EVENT, onRefresh)
    return () => window.removeEventListener(WORKFLOW_LOG_REFRESH_EVENT, onRefresh)
  }, [load])

  const areaFindings = useMemo(() => findings.filter((finding) => !moduleFilter
    || pulseFindingReviewAreas(finding, reviewFocusSelections).includes(moduleFilter)),
  [findings, moduleFilter, reviewFocusSelections])
  const queueCounts = useMemo(() => pulseWorkspaceQueueCounts(areaFindings), [areaFindings])
  const moduleSummaries = useMemo(
    () => buildPulseWorkspaceModuleSummaries(PULSE_MODULE_COMMANDS, findings, reviews, reviewFocusSelections),
    [findings, reviews, reviewFocusSelections],
  )
  const reviewFocusByIssueID = useMemo(() => {
    const labels = new Map<string, string[]>()
    reviewFocusSelections.forEach((selection) => {
      if (moduleFilter && normalizePulseWorkspaceModule(selection.module) !== moduleFilter) return
      const label = `${readable(normalizePulseWorkspaceModule(selection.module))} › ${readable(selection.focus_key)}`
      selection.issue_ids?.forEach((issueID) => {
        const normalizedID = issueID.trim().toUpperCase()
        if (!normalizedID) return
        const existing = labels.get(normalizedID) || []
        if (!existing.includes(label)) existing.push(label)
        labels.set(normalizedID, existing)
      })
    })
    return new Map<string, ReviewFocusLabel>(
      Array.from(labels, ([issueID, focusLabels]) => [
        issueID,
        { label: focusLabels[0], relatedCount: Math.max(0, focusLabels.length - 1) },
      ]),
    )
  }, [reviewFocusSelections, moduleFilter])

  const finalCommandStateByID = useMemo(
    () => new Map(finalCommandStates.map((state) => [state.command, state])),
    [finalCommandStates],
  )
  const matchingFindings = useMemo(
    () => {
      const matched = areaFindings
        .filter((finding) => pulseFindingMatchesFocus(finding, focus))
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
    [areaFindings, focus],
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

      <ReportHumanInputPanel workspacePath={workspacePath} contentMode="all" providedImpact={impact} />

      <section className="overflow-hidden rounded-xl border bg-background">
        <div className="border-b px-4 py-3">
          <h3 className="text-sm font-semibold text-foreground">Work areas</h3>
          <p className="mt-0.5 text-[11px] text-muted-foreground">
            What Pulse found, who owns the next move, and the latest judgment
          </p>
        </div>
        <div className="grid gap-px bg-border lg:grid-cols-3">
          {[
            {
              id: 'technical_review',
              title: 'Technical review',
              icon: Wrench,
              description: 'Execution health, plan integrity, stores, reports, evaluation, and model/cost fitness',
              tone: 'text-sky-600 dark:text-sky-300',
            },
            {
              id: 'strategic_review',
              title: 'Strategic review',
              icon: Lightbulb,
              description: 'Hidden strategic mechanisms and materially different opportunities for the goal',
              tone: 'text-amber-600 dark:text-amber-300',
            },
            {
              id: 'plan_drift_review',
              title: 'Plan drift review',
              icon: GitCompare,
              description: 'Steps flagged by a plan edit, checked for DB, report, learnings, KB, and validation_schema drift',
              tone: 'text-violet-600 dark:text-violet-300',
            },
          ].map((area) => {
            const areaModules = moduleSummaries.filter((module) => module.id === area.id)
            const decisions = areaModules.reduce((sum, module) => sum + module.awaitingUser, 0)
            const proposals = areaModules.reduce((sum, module) => sum + module.proposals, 0)
            const strategic = area.id === 'strategic_review'
            const actionable = strategic
              ? decisions + proposals
              : areaModules.reduce((sum, module) => sum + module.active + module.fixing, 0)
            const queued = areaModules.reduce((sum, module) => sum + module.queuedForEngineering, 0)
            const waiting = areaModules.reduce((sum, module) => (
              sum + module.awaitingVerification + module.awaitingRun
            ), 0)
            const blocked = areaModules.reduce((sum, module) => sum + module.blocked, 0)
            const external = areaModules.reduce((sum, module) => sum + module.externalAction, 0)
            const latest = [...areaModules]
              .sort((a, b) => (b.latestReview?.recorded_at || '').localeCompare(a.latestReview?.recorded_at || ''))[0]
              ?.latestReview
            const reviewedFocuses = reviewFocusSelections
              .filter((item) => normalizePulseWorkspaceModule(item.module) === area.id && item.last_reviewed_at)
              .sort((a, b) => (b.last_reviewed_at || '').localeCompare(a.last_reviewed_at || ''))
            const latestFocus = reviewedFocuses[0]
            const latestFocuses = latestFocus?.last_pulse_run_id
              ? reviewedFocuses.filter((item) => item.last_pulse_run_id === latestFocus.last_pulse_run_id)
              : latestFocus ? [latestFocus] : []
            const latestFocusKeys = new Set(latestFocuses.map((item) => item.focus_key))
            const upcomingFocuses = reviewFocuses
              .filter((item) => (
                normalizePulseWorkspaceModule(item.module) === area.id
                && !latestFocusKeys.has(item.focus_key)
              ))
              .slice(0, 3)
            const deferredFocuses = latestFocuses.flatMap((item) => item.deferred_focuses || [])
            const nextFocusKeys = deferredFocuses.length
              ? [...new Set(deferredFocuses)].slice(0, 3)
              : upcomingFocuses.map((item) => item.focus_key)
            const moduleID = area.id
            const moduleState = moduleStates.find((state) => (
              normalizePulseWorkspaceModule(state.module) === area.id
            ))
            const gateDecision = (moduleState?.last_gate_decision || '').trim().toLowerCase()
            const currentRunFocuses = moduleState?.last_pulse_run_id
              ? reviewedFocuses.filter((item) => item.last_pulse_run_id === moduleState.last_pulse_run_id)
              : []
            return (
              <button
                key={area.id}
                type="button"
                aria-pressed={moduleFilter === moduleID}
                onClick={() => {
                  setModuleFilter(moduleID)
                  setFocus('all')
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
                  <div className="flex shrink-0 flex-col items-end gap-1">
                    {gateDecision && (
                      <span className={`rounded-full border px-2 py-0.5 text-[9px] font-semibold ${gateDecision === 'due'
                        ? 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
                        : 'border-border bg-muted text-muted-foreground'
                      }`}>
                        {gateDecision === 'due' ? 'Due this run' : readable(gateDecision)}
                      </span>
                    )}
                    {actionable > 0 && (
                      <span className="rounded-full border border-red-500/25 bg-red-500/5 px-2 py-0.5 text-[9px] font-semibold text-red-700 dark:text-red-300">
                        {actionable} {strategic ? 'recommendations' : 'to fix'}
                      </span>
                    )}
                  </div>
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
                      {decisions > 0 && <span><span className="font-semibold text-foreground">{decisions}</span> decisions</span>}
                      {blocked > 0 && <span><span className="font-semibold text-foreground">{blocked}</span> blocked</span>}
                      {external > 0 && <span><span className="font-semibold text-foreground">{external}</span> platform</span>}
                      {proposals > 0 && <span><span className="font-semibold text-foreground">{proposals}</span> ideas</span>}
                    </>
                  )}
                  <span><span className="font-semibold text-foreground">{waiting}</span> waiting for evidence</span>
                  {queued > 0 && <span><span className="font-semibold text-foreground">{queued}</span> queued for Pulse</span>}
                </div>
                {moduleState && (gateDecision || moduleState.last_reason) && (
                  <div className="mt-3 rounded-md border bg-muted/25 px-2.5 py-2 text-[10px] leading-4 text-muted-foreground">
                    <div>
                      <span className="font-medium text-foreground">Gate decision:</span>{' '}
                      {gateDecision ? readable(gateDecision) : 'Recorded'}
                      {moduleState.next_check_at && (
                        <span> · Next check {formatCheckBoundary(moduleState.next_check_at)}</span>
                      )}
                      {!moduleState.next_check_at && moduleState.next_check_after_run_id && (
                        <span> · Recheck after the named workflow run</span>
                      )}
                    </div>
                    {moduleState.last_reason && (
                      <p className="mt-0.5 line-clamp-3">{moduleState.last_reason}</p>
                    )}
                    {area.id !== 'plan_drift_review' && (
                      <div className="mt-1">
                        <span className="font-medium text-foreground">
                          {gateDecision === 'skipped' ? 'Subcategories:' : 'Selected this run:'}
                        </span>{' '}
                        {gateDecision === 'skipped'
                          ? 'None — the review module was skipped.'
                          : currentRunFocuses.length > 0
                            ? currentRunFocuses.map((item) => readable(item.focus_key)).join(', ')
                            : 'Focus selection pending.'}
                      </div>
                    )}
                  </div>
                )}
                <div className="mt-3 border-t pt-2 text-[10px] leading-4 text-muted-foreground">
                  <span className="font-medium text-foreground">Latest:</span>{' '}
                  <span className="line-clamp-2">
                    {latest ? latest.verdict || 'Review recorded' : 'No stored review yet'}
                  </span>
                  {latestFocuses.length > 0 ? (
                    <div className="mt-1.5 text-[10px] text-muted-foreground">
                      <span className="font-medium text-foreground">Last {latestFocuses.length === 1 ? 'focus' : 'focuses'}:</span>{' '}
                      {latestFocuses.map((item) => (
                        `${readable(item.focus_key)}${item.route_scope ? ` · ${readable(item.route_scope)}` : ''}`
                      )).join(', ')} · {formatDate(latestFocuses[0].last_reviewed_at)}
                      {latestFocuses.length === 1 && (latestFocuses[0].review_count || 0) > 0 && (
                        <span> · reviewed {latestFocuses[0].review_count} {latestFocuses[0].review_count === 1 ? 'time' : 'times'}</span>
                      )}
                      {latestFocuses[0].last_selection_reason && (
                        <span className="mt-0.5 block line-clamp-2">{latestFocuses[0].last_selection_reason}</span>
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
            <div className="flex flex-wrap items-center justify-between gap-3">
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
              <div className="flex flex-wrap items-center gap-2">
                {moduleFilter && (
                  <button
                    type="button"
                    aria-label="Clear review area filter"
                    onClick={() => { setModuleFilter(null); setShowCompleteBacklog(false) }}
                    className="flex items-center gap-1 rounded-full border border-primary/35 bg-primary/10 px-2 py-1 text-[10px] font-semibold text-primary"
                  >
                    {moduleSummaries.find((module) => module.id === moduleFilter)?.label || readable(moduleFilter)}
                    <X className="h-3 w-3" />
                  </button>
                )}
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
                ['all', 'Current', queueCounts.all],
                ['needs_action', 'Pulse to fix', queueCounts.needs_action],
                ['queued_repair', 'Queued for Pulse', queueCounts.queued_repair],
                ['waiting_proof', 'Waiting for evidence', queueCounts.waiting_proof],
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
              <div className="mt-1 text-xs text-muted-foreground">Choose another queue{moduleFilter ? ' or clear the review area filter' : ' or inspect a review area above'}.</div>
            </div>
          ) : (
            <div className="space-y-2 p-3">
              {attentionFindings.map((finding) => {
                const moduleID = moduleFilter || pulseFindingReviewAreas(finding, reviewFocusSelections)[0]
                const module = moduleSummaries.find((item) => item.id === moduleID)
                const issueID = pulseIssueForFinding(finding).id.toUpperCase()
                const reviewFocus = reviewFocusByIssueID.get(issueID) || (isPulseOwnedFinding(finding)
                  ? { label: module ? `${module.label} › Unclassified` : 'Review area unassigned', relatedCount: 0 }
                  : undefined)
                return (
                  <PulseFindingCard
                    key={issueID}
                    finding={finding}
                    moduleLabel={module?.label}
                    reviewFocus={reviewFocus}
                    expanded={expandedFinding === issueID}
                    onToggle={() => setExpandedFinding(
                      expandedFinding === issueID ? null : issueID,
                    )}
                    onOpenModule={moduleID ? () => {
                      setModuleFilter(moduleID)
                      setFocus('all')
                      setShowCompleteBacklog(false)
                    } : undefined}
                  />
                )
              })}
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

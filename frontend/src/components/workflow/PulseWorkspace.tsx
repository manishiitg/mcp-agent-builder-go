import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  CheckCircle2,
  Lightbulb,
  Loader2,
  Wrench,
} from 'lucide-react'
import { agentApi } from '../../services/api'
import type {
  PulseFinalCommandState,
  PulseFindingLifecycle,
  PulseImpactLedger,
  PulseContextRecord,
  PulseReviewRecord,
  PulseReviewFocus,
} from '../../services/api-types'
import { ReportHumanInputPanel } from './ReportHumanInputPanel'
import { PulseEvalSummary } from './PulseEvalSummary'
import { SoulViewer } from './SoulViewer'
import { PulseFindingCard } from './PulseFindingCard'
import { pulseFindingPresentation, type PulseFindingQueue } from './pulseFindingPresentation'
import { isPulseOwnedFinding, pulseIssueForFinding } from './pulseModuleInspectorUtils'
import {
  buildPulseWorkspaceModuleSummaries,
  normalizePulseWorkspaceModule,
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

/** Which slice of the backlog the findings list is showing. */
type PulseFocus = 'all' | PulseFindingQueue

type ReviewFocusLabel = {
  label: string
  relatedCount: number
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
  finalCommandStates,
  reviewFocuses,
  reviewFocusSelections,
  statusError,
}: {
  workspacePath: string
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

  const load = useCallback(async () => {
    if (!workspacePath) return
    setLoading(true)
    setError(null)
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
    setFindings([])
    setReviews([])
    setImpact({ interventions: [], observations: [], assessments: [] })
    setContextRecords([])
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
  const reviewFocusByIssueID = useMemo(() => {
    const labels = new Map<string, string[]>()
    reviewFocusSelections.forEach((selection) => {
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
  }, [reviewFocusSelections])

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
        <div className="grid gap-px bg-border lg:grid-cols-2">
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
            return (
              <button
                key={area.id}
                type="button"
                onClick={() => {
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

      <PulseEvalSummary workspacePath={workspacePath} />

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
                const issueID = pulseIssueForFinding(finding).id.toUpperCase()
                const reviewFocus = reviewFocusByIssueID.get(issueID) || (isPulseOwnedFinding(finding)
                  ? { label: `${module?.label || readable(moduleID)} › Unclassified`, relatedCount: 0 }
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
                    onOpenModule={() => {
                      if (!moduleID) return
                      setModuleFilter(moduleID)
                      setFocus('all')
                      setShowCompleteBacklog(false)
                    }}
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

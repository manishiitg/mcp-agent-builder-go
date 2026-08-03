import React, { useEffect, useMemo, useState } from 'react'
import { ChevronDown, ChevronRight, CircleAlert } from 'lucide-react'
import type {
  PulseLoopClosureFinding,
  PulseShadowSignalObservation,
} from '../../services/api-types'
import {
  PULSE_LOOP_CLOSURE_PREVIEW_LIMIT,
  pulseLoopClosureReference,
  visiblePulseLoopClosureFindings,
} from './pulseLoopClosureNoticeUtils'
import { pulseLoopClosureHeading, summarizePulseLoopSignals } from './pulseHeaderStatus'

function formatPulseTimestamp(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function pulseLoopClosureKindLabel(kind: string): string {
  switch (kind) {
    case 'answer_not_applied':
      return 'Answer not applied'
    case 'decision_waiting_on_user':
      return 'Decision waiting'
    case 'concern_keeps_recurring':
      return 'Recurring concern'
    default:
      return 'Lifecycle follow-up'
  }
}

function pulseLoopClosureFindingKey(finding: PulseLoopClosureFinding, index: number): string {
  return `${finding.kind}-${finding.id || index}`
}

function pulseLoopClosureAge(ageDays: number): string {
  if (ageDays <= 0) return 'Today'
  return `${ageDays} day${ageDays === 1 ? '' : 's'}`
}

export function PulseLoopClosureFindingRow({
  finding,
  findingKey,
  expanded,
  onToggle,
}: {
  finding: PulseLoopClosureFinding
  findingKey: string
  expanded: boolean
  onToggle: () => void
}) {
  const reference = pulseLoopClosureReference(finding)
  const detailsId = `pulse-loop-closure-${findingKey.replace(/[^a-zA-Z0-9_-]/g, '-')}`

  return (
    <div>
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        aria-controls={detailsId}
        className="grid w-full gap-1 px-3 py-2.5 text-left transition-colors hover:bg-amber-500/[0.05] sm:grid-cols-[minmax(0,1fr)_auto] sm:px-4"
      >
        <div className="flex min-w-0 items-start gap-2">
          {expanded
            ? <ChevronDown className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            : <ChevronRight className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-[10px] font-semibold uppercase tracking-wide text-amber-700 dark:text-amber-300">
                {pulseLoopClosureKindLabel(finding.kind)}
              </span>
              <span className={`text-xs font-medium text-foreground ${expanded ? 'whitespace-normal break-words' : 'truncate'}`}>
                {finding.subject}
              </span>
            </div>
            {!expanded && (
              <p className="mt-0.5 line-clamp-2 text-[11px] leading-4 text-muted-foreground">
                {finding.detail}
              </p>
            )}
          </div>
        </div>
        <span className="ml-5 whitespace-nowrap text-[10px] font-medium text-muted-foreground sm:ml-0">
          {finding.age_days > 0 ? `${finding.age_days}d old` : 'today'}
        </span>
      </button>

      {expanded && (
        <div
          id={detailsId}
          role="region"
          aria-label={`${finding.subject} details`}
          className="space-y-3 border-t border-amber-500/10 bg-background/35 px-8 py-3 text-[11px] sm:px-10"
        >
          <div>
            <div className="font-semibold uppercase tracking-wide text-muted-foreground">Subject</div>
            <div className="mt-1 whitespace-pre-wrap break-words leading-5 text-foreground">{finding.subject}</div>
          </div>
          <div>
            <div className="font-semibold uppercase tracking-wide text-muted-foreground">Detail</div>
            <div className="mt-1 whitespace-pre-wrap break-words leading-5 text-foreground">{finding.detail}</div>
          </div>
          <div>
            <div className="font-semibold uppercase tracking-wide text-muted-foreground">Evidence</div>
            <div className="mt-1 whitespace-pre-wrap break-words leading-5 text-foreground">{finding.evidence}</div>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <div className="font-semibold uppercase tracking-wide text-muted-foreground">Age</div>
              <div className="mt-1 text-foreground">{pulseLoopClosureAge(finding.age_days)}</div>
            </div>
            <div>
              <div className="font-semibold uppercase tracking-wide text-muted-foreground">{reference.label}</div>
              <code className="mt-1 block break-all font-mono text-[10px] text-foreground">{reference.value}</code>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export function PulseLoopClosureNotice({ observation }: { observation: PulseShadowSignalObservation | null }) {
  const [showAll, setShowAll] = useState(false)
  const [expandedFindingKeys, setExpandedFindingKeys] = useState<Set<string>>(() => new Set())
  const observationIdentity = observation
    ? `${observation.pulse_run_id}:${observation.observed_at}`
    : ''

  useEffect(() => {
    setShowAll(false)
    setExpandedFindingKeys(new Set())
  }, [observationIdentity])

  const findings = useMemo(() => observation?.signals || [], [observation?.signals])
  const visibleFindings = useMemo(
    () => visiblePulseLoopClosureFindings(findings, showAll),
    [findings, showAll],
  )
  const signalSummary = useMemo(() => summarizePulseLoopSignals(findings), [findings])

  if (!observation) return null
  const coverageVerified = observation.coverage_status === 'verified'
  if (findings.length === 0 && coverageVerified) return null

  const toggleFinding = (findingKey: string) => {
    setExpandedFindingKeys((current) => {
      const next = new Set(current)
      if (next.has(findingKey)) next.delete(findingKey)
      else next.add(findingKey)
      return next
    })
  }

  return (
    <section
      className="mb-4 overflow-hidden rounded-lg border border-amber-500/30 bg-amber-500/[0.07]"
      aria-label="Pulse lifecycle follow-up"
    >
      <div className="flex items-start gap-3 px-3 py-3 sm:px-4">
        <CircleAlert className="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-300" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-sm font-semibold text-foreground">
              {findings.length > 0
                ? pulseLoopClosureHeading(signalSummary)
                : 'Loop-closure evidence is incomplete'}
            </h3>
            <span className={`rounded-full border px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wide ${
              coverageVerified
                ? 'border-emerald-500/25 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                : 'border-amber-500/25 bg-amber-500/10 text-amber-700 dark:text-amber-300'
            }`}>
              {observation.coverage_status.replaceAll('_', ' ')}
            </span>
          </div>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            Read-only evidence observed {formatPulseTimestamp(observation.observed_at) || 'during the latest Pulse'}.
            Pulse may weigh it with other facts; it does not force a repair or bypass approval.
          </p>
          {!coverageVerified && observation.coverage_reason && (
            <p className="mt-1 whitespace-pre-wrap break-words text-[11px] leading-4 text-amber-800/80 dark:text-amber-200/80">
              Coverage: {observation.coverage_reason}
            </p>
          )}
        </div>
      </div>
      {findings.length > 0 && (
        <div className="divide-y divide-amber-500/15 border-t border-amber-500/20">
          {visibleFindings.map((finding, index) => {
            const findingKey = pulseLoopClosureFindingKey(finding, index)
            return (
              <PulseLoopClosureFindingRow
                key={findingKey}
                finding={finding}
                findingKey={findingKey}
                expanded={expandedFindingKeys.has(findingKey)}
                onToggle={() => toggleFinding(findingKey)}
              />
            )
          })}
          {findings.length > PULSE_LOOP_CLOSURE_PREVIEW_LIMIT && (
            <button
              type="button"
              onClick={() => setShowAll((current) => !current)}
              aria-expanded={showAll}
              className="flex w-full items-center gap-1.5 px-3 py-2 text-left text-[11px] font-medium text-muted-foreground transition-colors hover:bg-amber-500/[0.05] hover:text-foreground sm:px-4"
            >
              {showAll
                ? <ChevronDown className="h-3.5 w-3.5" />
                : <ChevronRight className="h-3.5 w-3.5" />}
              {showAll
                ? 'Show fewer stalled loops'
                : `+${findings.length - PULSE_LOOP_CLOSURE_PREVIEW_LIMIT} more — show complete list`}
            </button>
          )}
        </div>
      )}
    </section>
  )
}

export default PulseLoopClosureNotice

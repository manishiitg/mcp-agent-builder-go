import type { PulseLoopClosureFinding } from '../../services/api-types'

export type PulseLoopSignalSummary = {
  needsUser: number
  awaitingPulse: number
  recurring: number
  other: number
}

export function summarizePulseLoopSignals(signals: PulseLoopClosureFinding[]): PulseLoopSignalSummary {
  const summary: PulseLoopSignalSummary = {
    needsUser: 0,
    awaitingPulse: 0,
    recurring: 0,
    other: 0,
  }
  signals.forEach((signal) => {
    if (signal.kind === 'decision_waiting_on_user') summary.needsUser += 1
    else if (signal.kind === 'answer_not_applied') summary.awaitingPulse += 1
    else if (signal.kind === 'concern_keeps_recurring') summary.recurring += 1
    else summary.other += 1
  })
  return summary
}

export function pulseLoopClosureHeading(summary: PulseLoopSignalSummary): string {
  const total = summary.needsUser + summary.awaitingPulse + summary.recurring + summary.other
  if (total === 0) return 'No lifecycle follow-up is pending'
  if (summary.needsUser === total) return `${total} decision${total === 1 ? '' : 's'} need your answer`
  if (summary.awaitingPulse === total) return `${total} answered decision${total === 1 ? '' : 's'} awaiting Pulse`
  if (summary.recurring === total) return `${total} recurring issue${total === 1 ? '' : 's'} need Pulse follow-up`
  return `${total} lifecycle item${total === 1 ? '' : 's'} need follow-through`
}

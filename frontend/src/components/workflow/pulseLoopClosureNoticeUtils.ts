import type { PulseLoopClosureFinding } from '../../services/api-types'

export const PULSE_LOOP_CLOSURE_PREVIEW_LIMIT = 4

export function pulseLoopClosureReference(finding: PulseLoopClosureFinding): {
  label: string
  value: string
} {
  const isDecision = finding.kind === 'answer_not_applied'
    || finding.kind === 'decision_waiting_on_user'
  return {
    label: isDecision ? 'Linked decision' : 'Linked finding',
    value: finding.id?.trim() || 'No durable record ID',
  }
}

export function visiblePulseLoopClosureFindings(
  findings: PulseLoopClosureFinding[],
  showAll: boolean,
): PulseLoopClosureFinding[] {
  return showAll
    ? findings
    : findings.slice(0, PULSE_LOOP_CLOSURE_PREVIEW_LIMIT)
}

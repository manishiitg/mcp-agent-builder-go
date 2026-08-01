import type {
  PulseFindingEvent,
  PulseFindingLifecycle,
} from '../../services/api-types'

export type PulseModuleSummary = {
  total: number
  open: number
  /** Acknowledged and waiting on the operator to decide something. */
  awaitingUser: number
  /** Acknowledged but Pulse has no way to act — nothing here is yours to do. */
  blocked: number
  fixing: number
  awaitingVerification: number
  closed: number
  externalAction: number
  recurring: number
  harnessIssues: number
  attempts: number
  passedChecks: number
  failedChecks: number
  inconclusiveChecks: number
}

/**
 * An acknowledged finding is triaged, not outstanding, and the three reasons it
 * can be acknowledged demand opposite responses: blocked means Pulse cannot
 * act, awaiting_user means only you can, and a recorded proposal means neither
 * is urgent. The status column flattens all three, so counting them as one
 * number made rtslatency read as "25 need action" when 12 were blocked, 4 were
 * questions for the operator, and only 6 were work Pulse could pick up.
 *
 * The reason lives in the finding's events rather than its status, so the most
 * recent one that carries a reason decides.
 */
export function acknowledgedReason(finding: PulseFindingLifecycle): 'blocked' | 'awaiting_user' | 'other' {
  for (let index = finding.events.length - 1; index >= 0; index -= 1) {
    const eventType = finding.events[index]?.event_type
    if (eventType === 'blocked' || eventType === 'awaiting_user') return eventType
  }
  return 'other'
}

export type PulseModuleActivity = PulseFindingEvent & {
  fingerprint: string
  findingID?: string
  findingText: string
}

export function isPulseFindingClosed(status: string): boolean {
  return status === 'resolved'
    || status === 'rejected'
    || status === 'external_action_required'
}

export function summarizePulseModule(findings: PulseFindingLifecycle[]): PulseModuleSummary {
  const summary: PulseModuleSummary = {
    total: findings.length,
    open: 0,
    fixing: 0,
    awaitingVerification: 0,
    closed: 0,
    externalAction: 0,
    recurring: 0,
    harnessIssues: 0,
    attempts: 0,
    passedChecks: 0,
    failedChecks: 0,
    inconclusiveChecks: 0,
    awaitingUser: 0,
    blocked: 0,
  }
  findings.forEach((finding) => {
    if (finding.status === 'external_action_required') summary.externalAction++
    else if (isPulseFindingClosed(finding.status)) summary.closed++
    else if (finding.status === 'fixing') summary.fixing++
    else if (finding.status === 'awaiting_verification') summary.awaitingVerification++
    else if (finding.status === 'acknowledged') {
      const reason = acknowledgedReason(finding)
      if (reason === 'awaiting_user') summary.awaitingUser++
      else if (reason === 'blocked') summary.blocked++
      else summary.open++
    } else summary.open++
    if (finding.seen_count > 1 && finding.status !== 'external_action_required') summary.recurring++
    if (finding.details?.issue_kind === 'harness_issue') summary.harnessIssues++
    summary.attempts += finding.fix_attempts.length
    finding.verifications.forEach((verification) => {
      if (verification.verdict === 'passed') summary.passedChecks++
      else if (verification.verdict === 'failed') summary.failedChecks++
      else summary.inconclusiveChecks++
    })
  })
  return summary
}

export function buildPulseModuleActivity(
  findings: PulseFindingLifecycle[],
  limit = 8,
): PulseModuleActivity[] {
  return findings
    .flatMap((finding) => finding.events.map((event) => ({
      ...event,
      fingerprint: finding.fingerprint,
      findingID: finding.finding_id || event.finding_id,
      findingText: finding.text,
    })))
    .sort((a, b) => b.recorded_at.localeCompare(a.recorded_at))
    .slice(0, Math.max(0, limit))
}

import type {
  PulseFindingEvent,
  PulseFindingLifecycle,
} from '../../services/api-types'

export type PulseModuleSummary = {
  total: number
  open: number
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
  }
  findings.forEach((finding) => {
    if (finding.status === 'external_action_required') summary.externalAction++
    else if (isPulseFindingClosed(finding.status)) summary.closed++
    else if (finding.status === 'fixing') summary.fixing++
    else if (finding.status === 'awaiting_verification') summary.awaitingVerification++
    else summary.open++
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

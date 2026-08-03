import type {
  PulseFindingLifecycle,
  PulseFindingVerification,
  PulseFixAttempt,
} from '../../services/api-types'
import {
  acknowledgedReason,
  isPulseFindingClosed,
  isPulseOwnedFinding,
  pulseIssueForFinding,
} from './pulseModuleInspectorUtils'

export type PulseFindingQueue =
  | 'needs_action'
  | 'waiting_proof'
  | 'decisions'
  | 'platform'
  | 'resolved'
  | 'workflow_reported'

export type PulseFindingTone = 'danger' | 'warning' | 'info' | 'success' | 'decision' | 'neutral'

export type PulseFindingPresentation = {
  label: string
  queue: PulseFindingQueue
  tone: PulseFindingTone
  nextAction: string
}

export type PulseFindingProgressStep = {
  label: string
  state: 'done' | 'current' | 'pending'
}

function readable(value?: string): string {
  return (value || '').trim().replaceAll('_', ' ')
}

export function pulseFindingDisposition(finding: PulseFindingLifecycle): string {
  for (const event of finding.events) {
    const disposition = event.metadata?.disposition
    if (typeof disposition === 'string' && disposition.trim()) return disposition.trim()
  }
  for (const attempt of finding.fix_attempts) {
    const reference = attempt.findings?.find((candidate) => (
      candidate.fingerprint === finding.fingerprint
      || (finding.finding_id && candidate.finding_id === finding.finding_id)
    ))
    if (reference?.disposition?.trim()) return reference.disposition.trim()
  }
  return ''
}

function closedLabel(finding: PulseFindingLifecycle): string {
  const disposition = pulseFindingDisposition(finding)
  if (disposition === 'fixed_verified') return 'Fixed and verified'
  if (disposition === 'verified_no_change') return 'Closed · no fix needed'
  if (finding.status === 'rejected') return 'Closed · not proceeding'
  return 'Resolved'
}

export function pulseFindingPresentation(finding: PulseFindingLifecycle): PulseFindingPresentation {
  if (!isPulseOwnedFinding(finding)) {
    return {
      label: 'Reported by workflow run',
      queue: 'workflow_reported',
      tone: 'neutral',
      nextAction: finding.resolution_note?.trim()
        || 'Triage this run evidence and link it to a canonical Pulse issue when it describes the same behavior.',
    }
  }

  if (finding.status === 'external_action_required') {
    const owner = readable(finding.external_owner) || 'platform owner'
    return {
      label: 'Platform action required',
      queue: 'platform',
      tone: 'neutral',
      nextAction: finding.resolution_note?.trim()
        || (finding.reopen_condition?.trim()
          ? `Owned by ${owner}. Reopen when ${finding.reopen_condition.trim()}`
          : `Owned by ${owner}; Pulse cannot repair it inside this workflow.`),
    }
  }

  if (isPulseFindingClosed(finding.status)) {
    return {
      label: closedLabel(finding),
      queue: 'resolved',
      tone: 'success',
      nextAction: finding.resolution_note?.trim() || 'No further action is required unless new evidence reopens the issue.',
    }
  }

  if (finding.status === 'fixing') {
    return {
      label: 'Fix in progress',
      queue: 'needs_action',
      tone: 'info',
      nextAction: finding.resolution_note?.trim()
        || finding.fix_attempts[0]?.summary?.trim()
        || 'Pulse is applying the current repair attempt.',
    }
  }

  if (finding.status === 'awaiting_verification') {
    const failed = finding.verifications.some((verification) => verification.verdict === 'failed')
    return {
      label: failed ? 'Verification failed' : 'Fix applied · needs verification',
      queue: failed ? 'needs_action' : 'waiting_proof',
      tone: failed ? 'danger' : 'warning',
      nextAction: finding.resolution_note?.trim()
        || (failed
          ? 'Review the failed check, reopen the repair, and run verification again.'
          : 'Run the required post-change check before closing this issue.'),
    }
  }

  if (finding.status === 'awaiting_run') {
    return {
      label: 'Waiting for workflow run',
      queue: 'waiting_proof',
      tone: 'warning',
      nextAction: finding.resolution_note?.trim()
        || 'Run the producing workflow again so Pulse can verify the changed behavior.',
    }
  }

  if (finding.status === 'acknowledged') {
    const reason = acknowledgedReason(finding)
    if (reason === 'awaiting_user') {
      return {
        label: 'Your decision needed',
        queue: 'decisions',
        tone: 'decision',
        nextAction: finding.resolution_note?.trim() || 'Choose the requested option before Pulse continues.',
      }
    }
    if (reason === 'proposal') {
      return {
        label: 'Proposed improvement',
        queue: 'decisions',
        tone: 'decision',
        nextAction: finding.resolution_note?.trim() || 'Review this proposal before Pulse changes strategy, policy, or success criteria.',
      }
    }
    if (reason === 'blocked') {
      return {
        label: 'Blocked · no available action',
        queue: 'platform',
        tone: 'neutral',
        nextAction: finding.resolution_note?.trim()
          || (finding.reopen_condition?.trim()
            ? `Reopen when ${finding.reopen_condition.trim()}`
            : 'Pulse has diagnosed this issue but has no safe repair path.'),
      }
    }
  }

  return {
    label: 'New',
    queue: 'needs_action',
    tone: 'danger',
    nextAction: finding.resolution_note?.trim() || 'Pulse needs to diagnose this issue and assign a repair.',
  }
}

export function pulseFindingProgress(finding: PulseFindingLifecycle): PulseFindingProgressStep[] {
  const presentation = pulseFindingPresentation(finding)
  const disposition = pulseFindingDisposition(finding)
  const diagnosed = finding.fix_attempts.length > 0
    || finding.events.some((event) => !['filed', 'rediscovered'].includes(event.event_type))
    || presentation.queue === 'decisions'
    || presentation.queue === 'platform'
    || presentation.queue === 'resolved'
  const fixApplied = finding.fix_attempts.some((attempt) => attempt.changed_files.length > 0)
    || ['changed_unverified', 'fixed_verified'].includes(disposition)
  const verified = finding.verifications.some((verification) => verification.verdict === 'passed')
    || ['fixed_verified', 'verified_no_change'].includes(disposition)
  const closed = presentation.queue === 'resolved'
  const flags = [true, diagnosed, fixApplied, verified, closed]
  const labels = ['Found', 'Diagnosed', 'Fix applied', 'Verified', 'Closed']
  const firstPending = flags.findIndex((flag) => !flag)
  return labels.map((label, index) => ({
    label,
    state: flags[index] ? 'done' : index === firstPending ? 'current' : 'pending',
  }))
}

export function pulseVerificationLevel(verification: PulseFindingVerification): string {
  const text = `${verification.check} ${verification.expected || ''} ${verification.observed || ''}`.toLowerCase()
  if (/\b(static|structural|configuration|config inspection|schema|plan validation|file check)\b/.test(text)) {
    return 'Static check'
  }
  if (/\b(replay|fixture|deterministic|inert|model cases?)\b/.test(text)) {
    return 'Deterministic check'
  }
  if (/\b(runtime|producing|workflow run|consumer behavior|publish|linkedin|evaluation runtime)\b/.test(text)) {
    return 'Producing-run check'
  }
  return 'Verification check'
}

export function pulseFixAttemptIsIncomplete(
  finding: PulseFindingLifecycle,
  attempt: PulseFixAttempt,
): boolean {
  if (attempt.status !== 'fixing' || attempt.completed_at) return false
  return finding.status !== 'fixing'
    || finding.events.some((event) => event.attempt_id === attempt.attempt_id && event.event_type !== 'fix_started')
}

export function pulseFindingImpact(finding: PulseFindingLifecycle): string {
  const issue = pulseIssueForFinding(finding)
  return finding.details?.impact?.trim() || issue.description?.trim() || ''
}

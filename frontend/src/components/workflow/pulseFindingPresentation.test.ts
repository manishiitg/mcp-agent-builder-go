import { describe, expect, it } from 'vitest'
import type { PulseFindingLifecycle, PulseFixAttempt } from '../../services/api-types'
import {
  pulseFindingPresentation,
  pulseFindingProgress,
  pulseFindingReporter,
  pulseFixAttemptIsIncomplete,
  pulseVerificationLevel,
} from './pulseFindingPresentation'

function finding(overrides: Partial<PulseFindingLifecycle> = {}): PulseFindingLifecycle {
  return {
    fingerprint: 'finding-1',
    finding_id: 'PUL-1',
    module: 'bug_review',
    step_id: 'bug_review',
    phase: 'review',
    text: 'The collector writes an incorrect total.',
    status: 'open',
    first_seen_at: '2026-08-01T10:00:00Z',
    last_seen_at: '2026-08-02T10:00:00Z',
    seen_count: 1,
    fix_attempts: [],
    verifications: [],
    events: [],
    ...overrides,
  }
}

function attempt(overrides: Partial<PulseFixAttempt> = {}): PulseFixAttempt {
  return {
    attempt_id: 'attempt-1',
    module: 'bug_review',
    pulse_run_id: 'run-1',
    summary: 'Correct the aggregation.',
    status: 'fixing',
    intended_files: ['db/query.sql'],
    changed_files: ['db/query.sql'],
    before_refs: [],
    after_refs: [],
    started_at: '2026-08-02T10:10:00Z',
    ...overrides,
  }
}

describe('Pulse finding presentation', () => {
  it('names the reviewer that originally generated the finding', () => {
    expect(pulseFindingReporter(finding({
      module: 'strategy_auditor',
      step_id: 'strategy_auditor',
    }), 'Strategy Auditor')).toBe('Strategy Auditor')

    expect(pulseFindingReporter(finding({
      module: 'workflow_review',
      step_id: 'bug_review',
    }), 'Workflow review')).toBe('Bug Review')

    expect(pulseFindingReporter(finding({
      module: undefined,
      phase: 'prevalidation',
      step_id: 'collect-data',
    }))).toBe('Workflow step · collect-data')
  })

  it('routes an applied fix to proof instead of action', () => {
    expect(pulseFindingPresentation(finding({ status: 'awaiting_verification' }))).toMatchObject({
      queue: 'waiting_proof',
      label: 'Fix applied · needs verification',
    })
  })

  it('routes failed verification back to action', () => {
    expect(pulseFindingPresentation(finding({
      status: 'awaiting_verification',
      verifications: [{ check: 'Replay corrected rows', verdict: 'failed' }],
    }))).toMatchObject({
      queue: 'needs_action',
      label: 'Verification failed',
    })
  })

  it('separates decisions, platform gaps, and workflow evidence', () => {
    expect(pulseFindingPresentation(finding({
      status: 'acknowledged',
      events: [{
        event_type: 'awaiting_user',
        summary: '',
        metadata: { human_input_id: 'decision-1' },
        recorded_at: '2026-08-02T10:00:00Z',
      }],
    })).queue).toBe('decisions')

    expect(pulseFindingPresentation(finding({
      status: 'acknowledged',
      events: [{ event_type: 'proposal_recorded', summary: '', recorded_at: '2026-08-02T10:00:00Z' }],
    })).queue).toBe('proposals')

    expect(pulseFindingPresentation(finding({
      status: 'acknowledged',
      events: [{ event_type: 'awaiting_user', summary: '', recorded_at: '2026-08-02T10:00:00Z' }],
    }))).toMatchObject({ queue: 'needs_action', label: 'Decision request missing' })

    expect(pulseFindingPresentation(finding({
      status: 'external_action_required',
      external_owner: 'scheduler platform',
    })).queue).toBe('platform')

    expect(pulseFindingPresentation(finding({
      status: 'acknowledged',
      events: [{ event_type: 'blocked', summary: '', recorded_at: '2026-08-02T10:00:00Z' }],
    }))).toMatchObject({ queue: 'blocked', label: 'Blocked · no available action' })

    expect(pulseFindingPresentation(finding({
      phase: 'prevalidation',
      step_id: 'collect-data',
    })).queue).toBe('workflow_reported')
  })

  it('uses the terminal disposition for a precise resolved label', () => {
    expect(pulseFindingPresentation(finding({
      status: 'resolved',
      events: [{
        event_type: 'closed',
        summary: 'Replay passed.',
        metadata: { disposition: 'fixed_verified' },
        recorded_at: '2026-08-02T10:20:00Z',
      }],
    }))).toMatchObject({
      queue: 'resolved',
      label: 'Fixed and verified',
    })
  })

  it('marks a stale fixing attempt as incomplete after the finding advances', () => {
    const staleAttempt = attempt()
    expect(pulseFixAttemptIsIncomplete(
      finding({ status: 'awaiting_run', fix_attempts: [staleAttempt] }),
      staleAttempt,
    )).toBe(true)
    expect(pulseFixAttemptIsIncomplete(
      finding({ status: 'fixing', fix_attempts: [staleAttempt] }),
      staleAttempt,
    )).toBe(false)
  })

  it('explains verification strength in human terms', () => {
    expect(pulseVerificationLevel({ check: 'Inspect plan configuration', verdict: 'passed' })).toBe('Static check')
    expect(pulseVerificationLevel({ check: 'Replay all stored rows', verdict: 'passed' })).toBe('Deterministic check')
    expect(pulseVerificationLevel({ check: 'Next workflow run publishes the value', verdict: 'inconclusive' })).toBe('Producing-run check')
  })

  it('shows the unresolved lifecycle step instead of implying closure', () => {
    const steps = pulseFindingProgress(finding({
      status: 'awaiting_verification',
      fix_attempts: [attempt({ completed_at: '2026-08-02T10:15:00Z', status: 'completed' })],
    }))
    expect(steps.map((step) => step.state)).toEqual(['done', 'done', 'done', 'current', 'pending'])
  })
})

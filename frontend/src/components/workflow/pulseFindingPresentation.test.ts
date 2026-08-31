import { describe, expect, it } from 'vitest'
import type { PulseFindingLifecycle } from '../../services/api-types'
import { pulseFindingPresentation, pulseFindingProgress } from './pulseFindingPresentation'

function finding(overrides: Partial<PulseFindingLifecycle>): PulseFindingLifecycle {
  return {
    finding_id: 'PUL-1',
    phase: 'review',
    step_id: 'workflow_review',
    text: 'Finding',
    status: 'open',
    seen_count: 1,
    fix_attempts: [],
    verifications: [],
    events: [],
    ...overrides,
  }
}

describe('pulse finding action lanes', () => {
  it('keeps a deferred safe repair in Pulse’s queue', () => {
    expect(pulseFindingPresentation(finding({ status: 'queued_for_engineering' }))).toMatchObject({
      queue: 'queued_repair', label: 'Queued for Pulse',
    })
  })

  it('labels a migrated missing decision request as Pulse-owned repair work', () => {
    expect(pulseFindingPresentation(finding({
      status: 'queued_for_engineering',
      events: [{
        event_type: 'decision_request_missing',
        summary: 'Pulse must create and link an answerable decision.',
        recorded_at: '2026-08-25T00:00:00Z',
      }],
    }))).toMatchObject({ queue: 'queued_repair', label: 'Decision request missing' })
  })

  it('renders old deferred blocked records as queued work during migration', () => {
    expect(pulseFindingPresentation(finding({
      status: 'acknowledged',
      events: [{
        event_type: 'blocked',
        summary: 'Browser snapshot overflow not attempted this pass; deferred to a future Engineering pass.',
        recorded_at: '2026-08-08T00:00:00Z',
      }],
    }))).toMatchObject({ queue: 'queued_repair', label: 'Queued for Pulse' })
  })

  it('renders old reproduce-on-next-run records as waiting for evidence', () => {
    expect(pulseFindingPresentation(finding({
      status: 'acknowledged',
      events: [{
        event_type: 'blocked',
        summary: 'Needs triage on the next daily-bid run before attempting a fix.',
        recorded_at: '2026-08-08T00:00:00Z',
      }],
    }))).toMatchObject({ queue: 'waiting_proof', label: 'Waiting for next run' })
  })

  it('uses the latest verification instead of a historical pass', () => {
    const record = finding({
      verifications: [
        {
          check: 'Global learning skill size and purity',
          verdict: 'failed',
          expected: 'The learning skill stays within its compact, pure-skill contract.',
          observed: 'The file still exceeds the contract.',
          verified_at: '2026-08-06T00:00:00Z',
        },
        {
          check: 'Contradictory claims removed',
          verdict: 'passed',
          verified_at: '2026-08-05T00:00:00Z',
        },
      ],
    })

    expect(pulseFindingPresentation(record)).toMatchObject({
      queue: 'needs_action', label: 'Verification failed',
    })
    expect(pulseFindingProgress(record).find((step) => step.label === 'Verified')?.state).not.toBe('done')
  })

  it('does not present a recurrence after an applied fix as a new issue', () => {
    const record = finding({
      status: 'open',
      seen_count: 2,
      fix_attempts: [{
        attempt_id: 'attempt-1',
        module: 'technical_review',
        pulse_run_id: 'pulse-1',
        summary: 'Applied the plan repair.',
        status: 'completed',
        intended_files: ['planning/plan.json'],
        changed_files: ['planning/plan.json'],
        before_refs: [],
        after_refs: [],
        findings: [],
        started_at: '2026-08-28T10:00:00Z',
        completed_at: '2026-08-28T10:01:00Z',
      }],
      events: [{
        event_type: 'reopened',
        summary: 'The same concern was observed on a later workflow run.',
        recorded_at: '2026-08-29T10:00:00Z',
      }],
    })

    expect(pulseFindingPresentation(record)).toMatchObject({
      queue: 'needs_action', label: 'Reopened after fix',
    })
    expect(pulseFindingProgress(record).at(-1)).toMatchObject({
      label: 'Reopened', state: 'current',
    })
  })
})

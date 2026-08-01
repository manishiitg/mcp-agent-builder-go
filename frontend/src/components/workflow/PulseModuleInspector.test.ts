import { describe, expect, it } from 'vitest'
import type { PulseFindingLifecycle } from '../../services/api-types'
import { buildPulseModuleActivity, summarizePulseModule } from './pulseModuleInspectorUtils'

function finding(overrides: Partial<PulseFindingLifecycle>): PulseFindingLifecycle {
  return {
    fingerprint: 'fp',
    step_id: 'bug_review',
    phase: 'review',
    text: 'Finding',
    status: 'open',
    seen_count: 1,
    fix_attempts: [],
    verifications: [],
    events: [],
    ...overrides,
  }
}

describe('PulseModuleInspector data summaries', () => {
  it('separates action, fixing, proof, and closed states', () => {
    const summary = summarizePulseModule([
      finding({ fingerprint: 'open', seen_count: 3 }),
      finding({
        fingerprint: 'fixing',
        status: 'fixing',
        fix_attempts: [{
          attempt_id: 'a1',
          module: 'bug_review',
          pulse_run_id: 'p1',
          summary: 'Fix',
          status: 'running',
          intended_files: [],
          changed_files: [],
          before_refs: [],
          after_refs: [],
          started_at: '2026-07-31T01:00:00Z',
        }],
      }),
      finding({
        fingerprint: 'proof',
        status: 'awaiting_verification',
        verifications: [{ check: 'next run', verdict: 'inconclusive' }],
      }),
      finding({
        fingerprint: 'closed',
        status: 'resolved',
        verifications: [
          { check: 'regression', verdict: 'passed' },
          { check: 'old attempt', verdict: 'failed' },
        ],
      }),
      finding({
        fingerprint: 'external',
        status: 'external_action_required',
        seen_count: 5,
        external_owner: 'platform',
        reason_code: 'missing_plan_editor',
        reopen_condition: 'A compatible plan-editing tool becomes available.',
      }),
    ])

    expect(summary).toMatchObject({
      total: 5,
      open: 1,
      fixing: 1,
      awaitingVerification: 1,
      closed: 1,
      externalAction: 1,
      recurring: 1,
      harnessIssues: 0,
      attempts: 1,
      passedChecks: 1,
      failedChecks: 1,
      inconclusiveChecks: 1,
    })
  })

  it('counts structured harness issues as platform-owned findings', () => {
    const summary = summarizePulseModule([
      finding({
        details: {
          issue_kind: 'harness_issue',
          finding_id: 'HARNESS-1',
          target_key: 'harness:plan-editor:legacy-type',
          summary: 'The updater rejected its effective runtime type.',
          reproduction: {
            safe: true,
            expected: 'Edit is applied.',
            observed: 'Edit is rejected.',
          },
          platform: {
            issue_key: 'harness:plan-editor:legacy-type',
            affected_workflows: ['Workflow/a', 'Workflow/b'],
            seen_count: 3,
          },
        },
      }),
      finding({ fingerprint: 'workflow-bug' }),
    ])

    expect(summary.harnessIssues).toBe(1)
    expect(summary.open).toBe(2)
  })

  it('builds a newest-first event feed with finding context', () => {
    const activity = buildPulseModuleActivity([
      finding({
        fingerprint: 'one',
        finding_id: 'BUG-1',
        text: 'First',
        events: [{ event_type: 'filed', summary: 'First filed', recorded_at: '2026-07-31T01:00:00Z' }],
      }),
      finding({
        fingerprint: 'two',
        text: 'Second',
        events: [{ event_type: 'reopened', summary: 'Second reopened', recorded_at: '2026-07-31T02:00:00Z' }],
      }),
    ])

    expect(activity.map((event) => event.fingerprint)).toEqual(['two', 'one'])
    expect(activity[1]).toMatchObject({ findingID: 'BUG-1', findingText: 'First' })
  })
})

describe('acknowledged findings are split by who must act', () => {
  const finding = (status: string, eventTypes: string[]): PulseFindingLifecycle => ({
    fingerprint: `fp-${status}-${eventTypes.join('-')}`,
    step_id: 'step', phase: 'review', text: 't', status,
    seen_count: 1, fix_attempts: [], verifications: [],
    events: eventTypes.map((event_type) => ({ event_type, summary: '', recorded_at: '' })),
  } as unknown as PulseFindingLifecycle)

  it('counts blocked separately from work Pulse can pick up', () => {
    // rtslatency's real shape: one number said 25 needed action when only 6
    // were Pulse's to do, 12 were blocked, and 4 were questions for the operator.
    const summary = summarizePulseModule([
      finding('acknowledged', ['filed', 'blocked']),
      finding('acknowledged', ['filed', 'blocked']),
      finding('acknowledged', ['filed', 'awaiting_user']),
      finding('open', ['filed']),
    ])
    expect(summary.blocked).toBe(2)
    expect(summary.awaitingUser).toBe(1)
    expect(summary.open).toBe(1)
  })

  it('uses the most recent reason when a finding was blocked and then handed to the user', () => {
    const summary = summarizePulseModule([finding('acknowledged', ['blocked', 'awaiting_user'])])
    expect(summary.awaitingUser).toBe(1)
    expect(summary.blocked).toBe(0)
  })
})

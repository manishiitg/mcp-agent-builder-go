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
    ])

    expect(summary).toMatchObject({
      total: 4,
      open: 1,
      fixing: 1,
      awaitingVerification: 1,
      closed: 1,
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

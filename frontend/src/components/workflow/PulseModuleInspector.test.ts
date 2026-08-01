import { describe, expect, it } from 'vitest'
import type { PulseFindingLifecycle } from '../../services/api-types'
import { buildPulseModuleActivity, pulseIssueForFinding, summarizePulseModule } from './pulseModuleInspectorUtils'

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
      finding('acknowledged', ['blocked', 'filed']),
      finding('acknowledged', ['blocked', 'filed']),
      finding('acknowledged', ['awaiting_user', 'filed']),
      finding('open', ['filed']),
    ])
    expect(summary.blocked).toBe(2)
    expect(summary.awaitingUser).toBe(1)
    expect(summary.open).toBe(1)
  })

  it('uses the most recent reason when a finding was blocked and then handed to the user', () => {
    // Lifecycle events are newest first.
    const summary = summarizePulseModule([finding('acknowledged', ['awaiting_user', 'blocked'])])
    expect(summary.awaitingUser).toBe(1)
    expect(summary.blocked).toBe(0)
  })
})

describe('compact Pulse issue projection', () => {
  it('uses the server projection without exposing the fingerprint as the issue id', () => {
    const issue = pulseIssueForFinding(finding({
      fingerprint: '0123456789abcdef',
      issue: {
        id: 'PUL-01234567',
        title: 'Selector repeats the same accounts',
        status: 'backlog',
        priority: 'high',
        module: 'bug_review',
        seen_count: 4,
      },
    }))

    expect(issue).toMatchObject({
      id: 'PUL-01234567',
      title: 'Selector repeats the same accounts',
      status: 'backlog',
      priority: 'high',
      seen_count: 4,
    })
    expect(issue.id).not.toContain('0123456789abcdef')
  })

  it('projects older API responses into the same minimal issue shape', () => {
    const issue = pulseIssueForFinding(finding({
      fingerprint: 'abcdef0123456789',
      finding_id: '',
      module: 'bug_review',
      text: 'Broken scheduler status',
      status: 'awaiting_verification',
      seen_count: 2,
      details: {
        severity: 'critical',
        summary: 'Scheduler marks a partial run successful',
        reproduction: { safe: false },
      },
    }))

    expect(issue).toMatchObject({
      id: 'PUL-ABCDEF01',
      title: 'Scheduler marks a partial run successful',
      description: 'Broken scheduler status',
      status: 'in_review',
      priority: 'urgent',
      module: 'bug_review',
      seen_count: 2,
    })
  })
})

describe('acknowledged findings route by reason, not by fallthrough', () => {
  const event = (event_type: string) => ({
    event_type,
    summary: '',
    recorded_at: '2026-08-01T12:30:00Z',
  })

  // Regression: `summary.open` backs the "Pulse can fix" metric and used to be a
  // fallthrough `else`. acknowledgedReason only recognised blocked and
  // awaiting_user, so proposal_only findings landed in it — social-media showed
  // 109 when 105 were actionable and 4 were proposals Pulse had deliberately
  // declined to repair.
  it('counts a recorded proposal as a proposal, never as work Pulse can fix', () => {
    const summary = summarizePulseModule([
      finding({ fingerprint: 'actionable' }),
      finding({
        fingerprint: 'proposal',
        status: 'acknowledged',
        events: [event('proposal_recorded'), event('filed')],
      }),
    ])

    expect(summary.proposals).toBe(1)
    expect(summary.open).toBe(1)
  })

  it('keeps blocked and awaiting_user out of the actionable count', () => {
    const summary = summarizePulseModule([
      finding({ fingerprint: 'b', status: 'acknowledged', events: [event('blocked')] }),
      finding({ fingerprint: 'u', status: 'acknowledged', events: [event('awaiting_user')] }),
    ])

    expect(summary.blocked).toBe(1)
    expect(summary.awaitingUser).toBe(1)
    expect(summary.open).toBe(0)
  })

  // A status this build does not model must not swell the metric the operator is
  // most likely to act on — that is the mechanism that hid the proposal bug.
  it('sends an unmodelled status to unclassified rather than to open', () => {
    const summary = summarizePulseModule([
      finding({ fingerprint: 'known' }),
      finding({ fingerprint: 'future', status: 'awaiting_third_party_sync' }),
    ])

    expect(summary.open).toBe(1)
    expect(summary.unclassified).toBe(1)
  })
})

describe('workflow-reported concerns are not Pulse’s queue', () => {
  // Regression: run_concerns holds Pulse reviewer findings (phase 'review')
  // alongside concerns the workflow's own steps file while running. Counting
  // the latter under "Pulse can fix" made social-media read 105 when Pulse
  // owned 10 — and every one of the 95 was something Pulse was never asked to
  // fix, in a module that was not even due.
  it('keeps step-reported concerns out of the actionable count', () => {
    const summary = summarizePulseModule([
      finding({ fingerprint: 'pulse', phase: 'review' }),
      finding({ fingerprint: 'prevalidation', phase: 'prevalidation', step_id: 'execute-find-opportunities' }),
      finding({ fingerprint: 'execution', phase: 'execution', step_id: 'execute-allocate' }),
      finding({ fingerprint: 'sequence', phase: 'message-sequence', step_id: 'execute-actions' }),
    ])

    expect(summary.open).toBe(1)
    expect(summary.workflowReported).toBe(3)
  })

  // Compatibility: an older backend may not send `phase`. Hiding those findings
  // would be worse than counting them, so an absent phase stays Pulse-owned.
  it('treats a missing phase as Pulse-owned rather than hiding it', () => {
    const summary = summarizePulseModule([finding({ fingerprint: 'legacy', phase: '' })])

    expect(summary.open).toBe(1)
    expect(summary.workflowReported).toBe(0)
  })
})

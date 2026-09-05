import { describe, expect, it } from 'vitest'
import type {
  PulseFindingLifecycle,
  PulseReviewRecord,
  PulseReviewFocus,
} from '../../services/api-types'
import {
  buildPulseWorkspaceModuleSummaries,
  normalizePulseWorkspaceModule,
  selectPulseWorkspaceModule,
  pulseFindingReviewAreas,
  pulseWorkspaceQueueCounts,
  pulseFindingMatchesFocus,
  type PulseFocus,
} from './pulseWorkspaceUtils'

const definitions = [
  { id: 'technical_review', label: 'Technical review', description: 'Correctness and operations' },
  { id: 'strategic_review', label: 'Strategic review', description: 'Strategy' },
]

function finding(
  module: string,
  status: string,
  seenCount = 1,
): PulseFindingLifecycle {
  return {
    module,
    step_id: 'step-1',
    phase: 'review',
    text: `${module} ${status}`,
    status,
    seen_count: seenCount,
    fix_attempts: [],
    verifications: [],
    events: [],
  }
}

function review(module: string, recordedAt: string): PulseReviewRecord {
  return {
    id: Date.parse(recordedAt),
    module,
    review_run_id: `${recordedAt}_${module}`,
    finding_count: 0,
    verification_count: 0,
    recorded_at: recordedAt,
  }
}

describe('Pulse workspace model', () => {
  const selections: PulseReviewFocus[] = [{
    workspace_path: 'Workflow/substack', module: 'technical_review', focus_key: 'execution_health',
    updated_at: '2026-09-05', issue_ids: [' pul-queued ', 'PUL-PLATFORM'],
  }]
  const queued = { ...finding('step-revise-draft', 'queued_for_engineering'), finding_id: 'PUL-QUEUED' }
  const platform = { ...finding('step-growth-baseline', 'external_action_required'), finding_id: 'PUL-PLATFORM' }
  const evidence = { ...finding('strategic_review', 'acknowledged'), finding_id: 'PUL-EVIDENCE',
    details: { recommended_route: 'evidence_wait', next_check: 'After ten growth days', reproduction: { safe: true } },
    events: [{ event_type: 'proposal_recorded', summary: 'Old proposal', recorded_at: '2026-08-01' }],
  }
  const records: PulseFindingLifecycle[] = [queued, platform, evidence,
    finding('technical_review', 'open'), finding('plan_drift_review', 'resolved'),
    { ...finding('technical_review', 'acknowledged'), events: [{ event_type: 'awaiting_user',
      metadata: { human_input_id: 'decision-1' }, summary: 'Choose', recorded_at: '2026-09-05' }] },
    { ...finding('strategic_review', 'acknowledged'), events: [{ event_type: 'proposal_recorded', summary: 'Idea', recorded_at: '2026-09-05' }] },
    { ...finding('technical_review', 'acknowledged'), events: [{ event_type: 'blocked', summary: 'No safe action', recorded_at: '2026-09-05' }] },
    { ...finding('technical_review', 'open'), kind: 'observation' },
  ]

  it('uses explicit reviewer associations without losing reporting-step identity', () => {
    expect(pulseFindingReviewAreas(queued, selections)).toEqual(['technical_review'])
    expect(queued.module).toBe('step-revise-draft')
    expect(pulseFindingReviewAreas({ ...queued, finding_id: 'PUL-UNASSIGNED' }, selections)).toEqual([])
    expect(pulseFindingReviewAreas({ ...queued, issue: { id: 'PUL-QUEUED', title: 'Queued', status: 'backlog', priority: 'high', seen_count: 1 } }, selections))
      .toEqual(['technical_review'])
  })

  it('deduplicates aliases and supports multiple explicitly linked review areas', () => {
    expect(pulseFindingReviewAreas({ ...queued, module: 'workflow_review' }, [
      ...selections, { ...selections[0], module: 'llm_ops_review' },
      { ...selections[0], module: 'plan_drift_review' },
    ])).toEqual(['technical_review', 'plan_drift_review'])
  })

  it.each([null, 'technical_review', 'strategic_review', 'plan_drift_review'])(
  'keeps all nine queue counts equal to results within %s', (area) => {
    const scoped = records.filter((record) => !area || pulseFindingReviewAreas(record, selections).includes(area))
    const counts = pulseWorkspaceQueueCounts(scoped)
    const filters: PulseFocus[] = ['all', 'needs_action', 'queued_repair', 'waiting_proof', 'decisions', 'proposals', 'blocked', 'platform', 'resolved']
    for (const focus of filters) {
      expect(counts[focus]).toBe(scoped.filter((record) => pulseFindingMatchesFocus(record, focus)).length)
    }
    if (area === 'technical_review') expect(counts).toMatchObject({ queued_repair: 1, platform: 1, proposals: 0 })
    if (area === 'strategic_review') expect(counts).toMatchObject({ waiting_proof: 1, proposals: 1, platform: 0 })
    if (area === 'plan_drift_review') expect(counts).toMatchObject({ all: 0, resolved: 1 })
    if (area === null) expect(counts.all).toBe(7) // excludes resolved and workflow observations
  })

  it('uses the same area membership and classification in work-area totals', () => {
    const summary = buildPulseWorkspaceModuleSummaries(definitions, records, [], selections)
    expect(summary[0]).toMatchObject({ queuedForEngineering: 1, externalAction: 1, active: 1, awaitingUser: 1, blocked: 1 })
    expect(summary[1]).toMatchObject({ proposals: 1, awaitingRun: 1, active: 0 })
  })

  it('keeps only canonical module identities', () => {
    expect(normalizePulseWorkspaceModule('workflow_review')).toBe('technical_review')
    expect(normalizePulseWorkspaceModule('strategy_auditor')).toBe('strategic_review')
    expect(normalizePulseWorkspaceModule('goal_advisor')).toBe('strategic_review')
  })

  it('summarizes module lifecycle state and keeps the latest review', () => {
    const blocked = finding('workflow_review', 'acknowledged')
    blocked.events = [{
      event_type: 'blocked',
      summary: 'No safe repair path.',
      recorded_at: '2026-07-31T09:00:00Z',
    }]
    const awaitingUser = finding('workflow_review', 'acknowledged')
    awaitingUser.events = [{
      event_type: 'awaiting_user',
      summary: 'Decision requested.',
      metadata: { human_input_id: 'decision-1' },
      recorded_at: '2026-07-31T09:00:00Z',
    }]
    const summaries = buildPulseWorkspaceModuleSummaries(
      definitions,
      [
        finding('workflow_review', 'open', 3),
        finding('workflow_review', 'fixing'),
        finding('workflow_review', 'awaiting_verification'),
        finding('workflow_review', 'awaiting_run'),
        finding('workflow_review', 'queued_for_engineering'),
        blocked,
        awaitingUser,
        finding('workflow_review', 'resolved'),
        finding('workflow_review', 'external_action_required', 5),
      ],
      [
        review('workflow_review', '2026-07-31T10:00:00Z'),
        review('workflow_review', '2026-07-30T10:00:00Z'),
      ],
    )

    expect(summaries[0]).toMatchObject({
      findings: 9,
      active: 1,
      fixing: 1,
      awaitingVerification: 1,
      awaitingRun: 1,
      queuedForEngineering: 1,
      awaitingUser: 1,
      blocked: 1,
      closed: 1,
      externalAction: 1,
      recurring: 1,
    })
    expect(summaries[0].latestReview?.recorded_at).toBe('2026-07-31T10:00:00Z')
  })

  it('selects the module with unresolved work before a merely recent clean review', () => {
    const summaries = buildPulseWorkspaceModuleSummaries(
      definitions,
      [finding('workflow_review', 'open')],
      [review('llm_ops_review', '2026-07-31T12:00:00Z')],
    )

    expect(selectPulseWorkspaceModule(summaries)).toBe('technical_review')
  })

})

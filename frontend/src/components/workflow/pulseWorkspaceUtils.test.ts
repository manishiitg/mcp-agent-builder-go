import { describe, expect, it } from 'vitest'
import type {
  PulseFindingLifecycle,
  PulseReviewRecord,
} from '../../services/api-types'
import {
  buildPulseWorkspaceModuleSummaries,
  normalizePulseWorkspaceModule,
  selectPulseWorkspaceModule,
} from './pulseWorkspaceUtils'

const definitions = [
  { id: 'workflow_review', label: 'Engineering review', description: 'Correctness' },
  { id: 'llm_ops_review', label: 'Ops review', description: 'Operations' },
  { id: 'strategic_review', label: 'Strategic review', description: 'Strategy' },
]

function finding(
  module: string,
  status: string,
  seenCount = 1,
): PulseFindingLifecycle {
  return {
    fingerprint: `${module}-${status}-${seenCount}`,
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
  it('keeps only canonical module identities', () => {
    expect(normalizePulseWorkspaceModule('workflow_review')).toBe('workflow_review')
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

    expect(selectPulseWorkspaceModule(summaries)).toBe('workflow_review')
  })

})

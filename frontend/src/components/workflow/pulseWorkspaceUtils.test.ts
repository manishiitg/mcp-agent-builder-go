import { describe, expect, it } from 'vitest'
import type {
  PulseFindingLifecycle,
  PulseReviewRecord,
} from '../../services/api-types'
import {
  buildPulseWorkspaceModuleSummaries,
  selectPulseWorkspaceModule,
  summarizePulseReviewStorage,
} from './pulseWorkspaceUtils'

const definitions = [
  { id: 'bug_review', label: 'Bug review', description: 'Correctness' },
  { id: 'llm_ops_review', label: 'Ops review', description: 'Operations' },
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
    artifact_kind: 'review',
    artifact_bytes: 10,
    recorded_at: recordedAt,
  }
}

describe('Pulse workspace model', () => {
  it('summarizes module lifecycle state and keeps the latest review', () => {
    const summaries = buildPulseWorkspaceModuleSummaries(
      definitions,
      [
        finding('bug_review', 'open', 3),
        finding('bug_review', 'fixing'),
        finding('bug_review', 'awaiting_verification'),
        finding('bug_review', 'resolved'),
        finding('bug_review', 'external_action_required', 5),
      ],
      [
        review('bug_review', '2026-07-31T10:00:00Z'),
        review('bug_review', '2026-07-30T10:00:00Z'),
      ],
    )

    expect(summaries[0]).toMatchObject({
      findings: 5,
      active: 1,
      fixing: 1,
      awaitingVerification: 1,
      closed: 1,
      externalAction: 1,
      recurring: 1,
    })
    expect(summaries[0].latestReview?.recorded_at).toBe('2026-07-31T10:00:00Z')
  })

  it('selects the module with unresolved work before a merely recent clean review', () => {
    const summaries = buildPulseWorkspaceModuleSummaries(
      definitions,
      [finding('bug_review', 'open')],
      [review('llm_ops_review', '2026-07-31T12:00:00Z')],
    )

    expect(selectPulseWorkspaceModule(summaries)).toBe('bug_review')
  })

  it('separates migrated file history from SQLite-native reviews', () => {
    const migrated = {
      ...review('bug_review', '2026-07-30T10:00:00Z'),
      legacy_source_path: 'pulse/reviews/legacy-run/bug_review.md',
    }
    const native = review('bug_review', '2026-07-31T10:00:00Z')

    expect(summarizePulseReviewStorage([migrated, native])).toEqual({
      total: 2,
      migrated: 1,
      native: 1,
    })
  })
})

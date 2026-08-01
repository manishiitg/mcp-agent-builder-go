import { describe, expect, it } from 'vitest'
import { collectPulseReviewArtifacts, pulseReviewRunDate } from './pulseReviewArtifactUtils'

describe('collectPulseReviewArtifacts', () => {
  const response = {
    success: true,
    data: [{
      filepath: 'Workflow/example/pulse/reviews',
      type: 'folder',
      children: [
        {
          filepath: 'Workflow/example/pulse/reviews/2026-07-31T03-00-00.000Z_run-2',
          type: 'folder',
          children: [
            {
              filepath: 'Workflow/example/pulse/reviews/2026-07-31T03-00-00.000Z_run-2/bug_review.md',
              type: 'file',
            },
          ],
        },
        {
          filepath: 'pulse/reviews/2026-07-30T03-00-00.000Z_run-1',
          type: 'folder',
          children: [
            {
              filepath: 'pulse/reviews/2026-07-30T03-00-00.000Z_run-1/bug_review.md',
              type: 'file',
            },
            {
              filepath: 'pulse/reviews/2026-07-30T03-00-00.000Z_run-1/eval_health.md',
              type: 'file',
            },
          ],
        },
      ],
    }],
  }

  it('finds one module and sorts newest first', () => {
    const artifacts = collectPulseReviewArtifacts(response, 'Workflow/example', 'bug_review')

    expect(artifacts.map((artifact) => artifact.reviewRunId)).toEqual([
      '2026-07-31T03-00-00.000Z_run-2',
      '2026-07-30T03-00-00.000Z_run-1',
    ])
    expect(artifacts[1].path).toBe(
      'Workflow/example/pulse/reviews/2026-07-30T03-00-00.000Z_run-1/bug_review.md',
    )
  })

  it('includes retired reviewer files under their merged module', () => {
    const opsResponse = {
      data: [{
        filepath: '2026-07-29T03-00-00.000Z_run/cost_llm_time.md',
        type: 'file',
      }],
    }

    expect(collectPulseReviewArtifacts(opsResponse, 'Workflow/example', 'llm_ops_review'))
      .toEqual([expect.objectContaining({ module: 'cost_llm_time' })])
  })
})

describe('pulseReviewRunDate', () => {
  it('parses the filesystem-safe UTC timestamp prefix', () => {
    expect(pulseReviewRunDate('2026-07-31T03-34-37.411Z_schedule')?.toISOString())
      .toBe('2026-07-31T03:34:37.411Z')
  })
})

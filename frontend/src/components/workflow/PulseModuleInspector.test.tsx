import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { PulseReviewRecord } from '../../services/api-types'

vi.mock('./PulseReviewHistory', () => ({
  PulseReviewHistory: () => <div>Review history</div>,
}))

import { PulseModuleInspector } from './PulseModuleInspector'

const reviews: PulseReviewRecord[] = [{
  id: 7,
  module: 'workflow_review',
  review_run_id: 'review-run-1',
  verdict: 'The collector fix held on the latest producing run.',
  status: 'completed',
  finding_count: 2,
  verification_count: 1,
  recorded_at: '2026-08-03T10:40:00Z',
}]

describe('PulseModuleInspector', () => {
  it('keeps reviewer details distinct from the canonical issue tracker', () => {
    const html = renderToStaticMarkup(
      <PulseModuleInspector
        workspacePath="/workspace/workflow"
        module="workflow_review"
        label="Workflow review"
        reviews={reviews}
      />,
    )

    expect(html).toContain('Latest judgment')
    expect(html).toContain('The collector fix held on the latest producing run.')
    expect(html).toContain('Review history')
    expect(html).not.toContain('Findings')
    expect(html).not.toContain('Needs attention')
    expect(html).not.toContain('Recent lifecycle activity')
  })
})

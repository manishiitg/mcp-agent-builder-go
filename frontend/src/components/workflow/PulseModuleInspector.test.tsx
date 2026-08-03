import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type { PulseReviewRecord } from '../../services/api-types'

vi.mock('./PulseReviewArtifacts', () => ({
  PulseReviewArtifacts: () => <div>Forensic report</div>,
}))

import { PulseModuleInspector } from './PulseModuleInspector'

const reviews: PulseReviewRecord[] = [{
  id: 7,
  module: 'workflow_review',
  review_run_id: 'review-run-1',
  verdict: 'The collector fix held on the latest producing run.',
  status: 'completed',
  artifact_kind: 'review',
  artifact_bytes: 1200,
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
    expect(html).toContain('Full report')
    expect(html).not.toContain('Findings')
    expect(html).not.toContain('Needs attention')
    expect(html).not.toContain('Recent lifecycle activity')
  })
})

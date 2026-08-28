import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'

vi.mock('../../services/api', () => ({
  agentApi: {},
  getApiBaseUrl: () => 'http://127.0.0.1:99999',
}))

import { PulseWorkspace } from './PulseWorkspace'

describe('PulseWorkspace information hierarchy', () => {
  it('leads with work areas, leaving reviewer mechanics for later', () => {
    const html = renderToStaticMarkup(
      <PulseWorkspace
        workspacePath="/workspace/example"
        finalCommandStates={[]}
        reviewFocuses={[
          {
            workspace_path: '/workspace/example',
            module: 'strategic_review',
            focus_key: 'feedback_loops_bias',
            updated_at: '2026-08-21T10:00:00Z',
            review_count: 0,
          },
        ]}
        reviewFocusSelections={[
          {
            workspace_path: '/workspace/example',
            module: 'technical_review',
            focus_key: 'execution_health',
            last_pulse_run_id: 'pulse-1',
            last_reviewed_at: '2026-08-21T10:00:00Z',
            last_selection_reason: 'The latest run exceeded its cadence.',
            updated_at: '2026-08-21T10:00:00Z',
            review_count: 2,
            route_scope: 'daily-execution/large-route',
            deferred_focuses: ['plan_orchestration_integrity'],
          },
          {
            workspace_path: '/workspace/example',
            module: 'technical_review',
            focus_key: 'plan_orchestration_integrity',
            last_pulse_run_id: 'pulse-1',
            last_reviewed_at: '2026-08-21T10:00:00Z',
            last_selection_reason: 'A separate small route missed its schedule.',
            route_scope: 'daily-execution/small-route',
            updated_at: '2026-08-21T10:00:00Z',
            review_count: 1,
          },
        ]}
        statusError={null}
      />,
    )

    const workAreas = html.indexOf('Work areas')
    const issues = html.indexOf('Issues and follow-through')

    expect(html).not.toContain('Pulse work queued')
    expect(html).not.toContain('Latest outcome')
    expect(workAreas).toBeGreaterThan(-1)
    expect(issues).toBeGreaterThan(workAreas)
    expect(html).not.toContain('Impact over time')
    expect(html).not.toContain('Pulse activity')
    expect(html).not.toContain('Recent fixes and follow-through')
    expect(html).toContain('Technical review')
    expect(html).toContain('Strategic review')
    expect(html).toContain('Last focuses:')
    expect(html).toContain('Execution health')
    expect(html).toContain('Plan orchestration integrity')
    expect(html).toContain('Daily-execution/large-route')
    expect(html).toContain('Next focus candidates:')
    expect(html).not.toContain('Focused review')
    expect(html).toContain('Feedback loops bias')
    expect(html).not.toContain('Model cost fitness')
    expect(html).not.toContain('Experiment impact')
    expect(html).not.toContain('PUL-3880D006')
    expect(html).not.toContain('Latest judgment')
    expect(html).not.toContain('Review history')
    expect(html).not.toContain('Report Health')
    expect(html).not.toContain('Eval Health')
    expect(html).not.toContain('Stores Health')
    expect(html).not.toContain('Artifact Review')
  })
})

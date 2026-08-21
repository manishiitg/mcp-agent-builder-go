import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'

vi.mock('../../services/api', () => ({
  agentApi: {},
  getApiBaseUrl: () => 'http://127.0.0.1:99999',
}))

import { PulseWorkspace } from './PulseWorkspace'

describe('PulseWorkspace information hierarchy', () => {
  it('leads with outcomes and ownership, leaving reviewer mechanics for later', () => {
    const html = renderToStaticMarkup(
      <PulseWorkspace
        workspacePath="/workspace/example"
        monitorOn
        moduleStates={[]}
        finalCommandStates={[]}
        gateMode={null}
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
            focus_key: 'execution_efficiency',
            last_pulse_run_id: 'pulse-1',
            last_reviewed_at: '2026-08-21T10:00:00Z',
            last_selection_reason: 'The latest run exceeded its cadence.',
            updated_at: '2026-08-21T10:00:00Z',
            review_count: 2,
            route_scope: 'daily-execution/large-route',
            deferred_focuses: ['tool_runtime_reliability'],
          },
          {
            workspace_path: '/workspace/example',
            module: 'technical_review',
            focus_key: 'schedule_capacity_recovery',
            last_pulse_run_id: 'pulse-1',
            last_reviewed_at: '2026-08-21T10:00:00Z',
            last_selection_reason: 'A separate small route missed its schedule.',
            route_scope: 'daily-execution/small-route',
            updated_at: '2026-08-21T10:00:00Z',
            review_count: 1,
          },
        ]}
        statusLoading={false}
        statusError={null}
        onRefresh={() => undefined}
      />,
    )

    const latestOutcome = html.indexOf('Latest outcome')
    const workAreas = html.indexOf('Work areas')
    const issues = html.indexOf('Issues and follow-through')
    const activity = html.indexOf('Pulse activity')
    const impact = html.indexOf('Impact over time')

    expect(latestOutcome).toBeGreaterThan(-1)
    expect(workAreas).toBeGreaterThan(latestOutcome)
    expect(issues).toBeGreaterThan(workAreas)
    expect(activity).toBeGreaterThan(issues)
    expect(impact).toBeGreaterThan(activity)
    expect(html).toContain('Technical review')
    expect(html).toContain('Strategic review')
    expect(html).toContain('Last focuses:')
    expect(html).toContain('Execution efficiency')
    expect(html).toContain('Schedule capacity recovery')
    expect(html).toContain('Daily-execution/large-route')
    expect(html).toContain('Next focus candidates:')
    expect(html).toContain('Tool runtime reliability')
    expect(html).toContain('Feedback loops bias')
    expect(html).not.toContain('Report Health')
    expect(html).not.toContain('Eval Health')
    expect(html).not.toContain('Stores Health')
    expect(html).not.toContain('Artifact Review')
  })
})

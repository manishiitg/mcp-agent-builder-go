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
    expect(html).toContain('Engineering issues')
    expect(html).toContain('Operations')
    expect(html).toContain('Product improvements')
    expect(html).not.toContain('Report Health')
    expect(html).not.toContain('Eval Health')
    expect(html).not.toContain('Stores Health')
    expect(html).not.toContain('Artifact Review')
  })
})

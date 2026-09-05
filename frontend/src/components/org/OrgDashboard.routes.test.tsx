// @vitest-environment happy-dom
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import type { OrgDashboardNotification } from '../../services/api-types'

vi.mock('../../services/api', () => ({ agentApi: {
  getOrgDashboardNotifications: vi.fn(), listReportHumanInputsAggregate: vi.fn(),
} }))
import { agentApi } from '../../services/api'
import { OrgDashboard } from './OrgDashboard'

describe('Activity route summaries', () => {
  it('keeps a quiet blocked route visible beside a busy completed route and opens its own details', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    const quiet: OrgDashboardNotification = { id: 'old-digest', workspace_path: 'Workflow/demo', kind: 'run_summary',
      status: 'blocked', title: 'Research needs access', message: 'No research output produced.', created_at: '2026-09-01T00:00:00Z' }
    const busy: OrgDashboardNotification = { ...quiet, id: 'new-digest', status: 'completed', title: 'Publishing completed',
      message: 'Published three articles.', created_at: '2026-09-05T00:00:00Z' }
    vi.mocked(agentApi.listReportHumanInputsAggregate).mockResolvedValue({ success: true, inputs: [] })
    vi.mocked(agentApi.getOrgDashboardNotifications).mockResolvedValue({ success: true, workflows: [{
      workspace_path: 'Workflow/demo', run_summary: busy, recent: [busy], by_route: [
        { routing_step_id: 'router-a', route_id: 'daily', label: 'Research', run_summary: quiet },
        { routing_step_id: 'router-b', route_id: 'daily', label: 'Publishing', run_summary: busy },
      ],
    }] })
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    try {
      await act(async () => root.render(<OrgDashboard workflows={[{ workspacePath: 'Workflow/demo', label: 'Demo' }]} onOpenWorkflow={vi.fn()} />))
      expect(container.textContent).toContain('Attention 1')
      expect(container.textContent).toContain('Needs attention')
      const research = container.querySelector('[aria-label="Demo · Research"]')!
      const publishing = container.querySelector('[aria-label="Demo · Publishing"]')!
      expect(research.textContent).toContain('Research needs access')
      expect(research.textContent).toContain('No pulse summary recorded yet')
      expect(research.textContent).not.toContain('Publishing completed')
      expect(publishing.textContent).toContain('Publishing completed')
      await act(async () => research.querySelector('button')!.click())
      expect(document.body.textContent).toContain('No research output produced.')
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })
})

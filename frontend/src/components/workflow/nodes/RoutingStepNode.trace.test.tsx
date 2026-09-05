// @vitest-environment happy-dom
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import type { RoutingStepNodeData } from '../hooks/usePlanToFlow'
vi.mock('@xyflow/react', () => ({ Handle: () => null, Position: { Top: 'top', Bottom: 'bottom' } }))
import { RoutingStepNode } from './RoutingStepNode'

describe('routing card trace control', () => {
  it('exposes a pressed button and stops opening the step inspector while preserving the executed route', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    const host = document.createElement('div')
    document.body.append(host)
    const root = createRoot(host)
    const trace = vi.fn(), inspect = vi.fn()
    const data = { id: 'router', title: 'Pipeline', stepIndex: 0, status: 'completed', tracedRouteId: 'weekly', onTraceRoute: trace,
      routes: [{ route_id: 'daily', route_name: 'Daily', next_step_id: 'a' }, { route_id: 'weekly', route_name: 'Weekly', next_step_id: 'b' }],
      step: { type: 'routing', id: 'router', selected_route_id: 'daily' },
    } as unknown as RoutingStepNodeData
    try {
      await act(async () => root.render(<div onClick={inspect}><RoutingStepNode data={data} /></div>))
      const button = host.querySelector<HTMLButtonElement>('[aria-label="Trace route: Weekly"]')!
      expect(button.type).toBe('button')
      expect(button.getAttribute('aria-pressed')).toBe('true')
      expect(host.querySelector('[aria-label="Trace route: Daily"]')?.getAttribute('aria-pressed')).toBe('false')
      await act(async () => button.click())
      expect(trace).toHaveBeenCalledWith('weekly')
      expect(inspect).not.toHaveBeenCalled()
      expect(data.step).toMatchObject({ selected_route_id: 'daily' })
    } finally {
      await act(async () => root.unmount())
      host.remove()
    }
  })
})

import { describe, expect, it } from 'vitest'
import { collectRouteTrace, routeTraceFromEdge, traceRouteGraph } from './routeTrace'
import type { WorkflowEdge, WorkflowNode } from '../hooks/usePlanToFlow'
const edge = (id: string, source: string, target: string, sourceHandle?: string): WorkflowEdge => ({ id, source, target, sourceHandle })
const edges = [edge('a', 'router', 'a', 'route-daily'), edge('b', 'router', 'b', 'route-weekly'),
  edge('nested', 'a', 'gate'), edge('branch', 'gate', 'join', 'route-done'),
  edge('handoff', 'gate', 'b', 'handoff-publish'), edge('b-join', 'b', 'join'),
  edge('end', 'join', 'end'), edge('loop', 'join', 'router'), edge('dep-a-other', 'a', 'other')]
describe('route trace', () => {
  it('resolves clicked routing and handoff lines by their exact source and choice', () => {
    const nodes = ['routing', 'branch'].map(type => ({ id: `${type}-node`, type,
      position: { x: 0, y: 0 }, data: { routes: [{ route_id: 'same-route-id', next_step_id: 'end' }] },
    })) as WorkflowNode[]
    expect(routeTraceFromEdge(nodes, edge('any-id', 'routing-node', 'end', 'route-same-route-id')))
      .toEqual({ nodeId: 'routing-node', routeId: 'same-route-id' })
    expect(routeTraceFromEdge(nodes, edge('any-id', 'branch-node', 'end', 'handoff-same-route-id')))
      .toEqual({ nodeId: 'branch-node', routeId: 'same-route-id' })
    expect(routeTraceFromEdge(nodes, edge('other', 'routing-node', 'end'))).toBeNull()
    expect(routeTraceFromEdge(nodes, edge('stale', 'routing-node', 'end', 'route-removed'))).toBeNull()
    expect(routeTraceFromEdge(nodes, edge('missing', 'missing', 'end', 'route-same-route-id'))).toBeNull()
  })
  it('follows branches, handoffs and joins without opening siblings on loops or following dependencies', () => {
    const focus = collectRouteTrace(edges, { nodeId: 'router', routeId: 'daily' })!
    expect([...focus.nodeIds].sort()).toEqual(['a', 'b', 'end', 'gate', 'join', 'router'])
    expect(focus.edgeIds.has('handoff')).toBe(true)
    expect(focus.edgeIds.has('b')).toBe(false)
    expect(focus.edgeIds.has('dep-a-other')).toBe(false)
  })
  it('does not walk upstream into siblings through a shared join', () => {
    const focus = collectRouteTrace(edges, { nodeId: 'router', routeId: 'weekly' })!
    expect(focus.nodeIds.has('a')).toBe(false)
    expect(focus.nodeIds.has('join')).toBe(true)
  })
  it('uses router plus route identity and supports end and missing choices', () => {
    const choices = [edge('one', 'one', 'end', 'route-done'), edge('two', 'two', 'b', 'route-done')]
    expect([...collectRouteTrace(choices, { nodeId: 'one', routeId: 'done' })!.nodeIds]).toEqual(['one', 'end'])
    expect(collectRouteTrace(choices, { nodeId: 'one', routeId: 'missing' })).toBeNull()
  })
  it('fades nodes and labels without mutating runtime choices and restores original styles', () => {
    const nodes = ['router', 'a', 'b'].map(id => ({ id, position: { x: 0, y: 0 }, data: { step: { selected_route_id: 'weekly' } } })) as WorkflowNode[]
    const graph = traceRouteGraph(nodes, edges, { nodeId: 'router', routeId: 'weekly' })
    expect(graph.nodes.find(n => n.id === 'a')?.style?.opacity).toBe(0.14)
    expect(graph.nodes.find(n => n.id === 'b')?.style?.opacity).toBe(1)
    expect(graph.edges.find(e => e.id === 'a')?.labelStyle?.opacity).toBe(0.08)
    expect(nodes[0].data.step).toEqual({ selected_route_id: 'weekly' })
    expect(nodes[1].style).toBeUndefined()
    expect(traceRouteGraph(nodes, edges, null)).toEqual({ nodes, edges })
  })
})

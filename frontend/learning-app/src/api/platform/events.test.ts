// The mapper is fed events shaped exactly like the agent server's polling
// and SSE frames (captured from a real SparkQuill turn), and must produce
// the preview events and TurnResult the SparkQuill UI consumes.
import { describe, expect, it } from 'vitest'
import type { TurnStreamEvent } from '../familyApi'
import { TurnCollector, bareToolName, familyRelativePath, isMainEvent, type PlatformEvent } from './events'

const SID = 'product-379ea1f1'
const main = (type: string, data: Record<string, unknown>, extra: Partial<PlatformEvent> = {}): PlatformEvent => ({
  id: `${SID}_${type}_${Math.random()}`, type, session_id: SID, execution_kind: 'main_agent', execution_id: `main:${SID}`,
  data: { type, hierarchy_level: 3, component: 'agent', data }, ...extra,
})

describe('TurnCollector', () => {
  it('maps a real parent turn into preview events and a result', () => {
    const seen: TurnStreamEvent[] = []
    const c = new TurnCollector(SID, (e) => seen.push(e))
    c.feed(main('streaming_chunk', { content: 'Got it', is_delta: true }))
    c.feed(main('tool_call_start', { tool_name: 'mcp__api-bridge__execute_shell_command', tool_call_id: 't1', tool_params: { arguments: '{"command":"ls"}' } }))
    c.feed(main('tool_call_end', { tool_name: 'mcp__api-bridge__execute_shell_command', tool_call_id: 't1', result: 'ok', duration: 12 }))
    c.feed(main('tool_call_start', { tool_name: 'set_child_profile', tool_call_id: 't2', tool_params: { arguments: '{"name":"Maya"}' } }))
    c.feed(main('product_interaction', { product: 'sparkquill', kind: 'family_updated', payload: { child: { name: 'Maya', grade: '6', board: 'CBSE' } } }))
    c.feed(main('tool_call_end', { tool_name: 'set_child_profile', tool_call_id: 't2', result: '{"status":"ok"}', duration: 3 }))
    c.feed(main('product_interaction', { product: 'sparkquill', kind: 'family_updated', payload: { parent_label: 'mom' } }))
    c.feed(main('presentation_updated', { presentation_id: 'p1', kind: 'document.file', title: 'progress.html', payload: { path: '_users/default/Chats/SparkQuill/reports/progress.html', focus: 'q2' } }))
    c.feed(main('presentation_updated', { presentation_id: 'p2', kind: 'sparkquill.activity', title: 'Fractions', payload: { dir: '_users/default/Chats/SparkQuill/activities/2026-09-03-fractions' } }))
    c.feed(main('product_interaction', { product: 'sparkquill', kind: 'suggestions', payload: { actions: [{ label: 'How is she doing?', message: 'progress' }, { label: '', message: 'x' }] } }))
    // A delegated sub-agent's completion must not end the turn.
    c.feed(main('unified_completion', { final_result: 'sub-agent done', status: 'completed' }, { execution_kind: 'delegation', execution_id: 'delegation-1' }))
    expect(c.done).toBe(false)
    c.feed(main('llm_generation_end', { content: 'Got it, mom — Maya is all set.' }))
    c.feed(main('unified_completion', { final_result: 'Got it, mom — Maya is all set.', status: 'completed' }))
    expect(c.done).toBe(true)

    expect(seen[0]).toEqual({ type: 'delta', text: 'Got it' })
    expect(seen.filter((e) => e.type === 'status').map((e) => e.text)).toEqual(['Working in the workspace', '', ''])
    const calls = seen.filter((e) => e.type === 'tool_call').map((e) => `${e.tool_call?.tool_name}:${e.tool_call?.status}`)
    expect(calls).toEqual(['execute_shell_command:running', 'execute_shell_command:completed', 'set_child_profile:running', 'set_child_profile:completed'])

    const r = c.result()
    expect(r.reply).toBe('Got it, mom — Maya is all set.')
    expect(r.error).toBeUndefined()
    expect(r.suggestions).toEqual([{ label: 'How is she doing?', message: 'progress' }])
    expect(r.tool_events).toEqual([
      { tool: 'set_child_profile', name: 'Maya', grade: '6', board: 'CBSE' },
      { tool: 'set_parent_label', parent_label: 'mom' },
      { tool: 'open_file', path: 'reports/progress.html', focus: 'q2' },
      { tool: 'open_activity', path: 'activities/2026-09-03-fractions' },
    ])
    expect(r.tool_calls?.map((t) => t.tool_name)).toEqual(['execute_shell_command', 'set_child_profile'])
    expect(r.tool_calls?.[0].arguments).toBe('{"command":"ls"}')
  })

  it('surfaces a child turn celebration and scene, and errors', () => {
    const c = new TurnCollector(SID, () => {})
    c.feed(main('product_interaction', { kind: 'celebrate', payload: { stars: 2, reason: 'nice!' } }))
    c.feed(main('product_interaction', { kind: 'scene', payload: { html: '<b>hi</b>' } }))
    c.feed(main('tool_call_error', { tool_name: 'read_image', tool_call_id: 't9', error: 'no file' }))
    c.feed(main('unified_completion', { status: 'error', error: 'boom' }))
    const r = c.result()
    expect(r.scene).toBe('<b>hi</b>')
    expect(r.tool_events).toEqual([{ tool: 'celebrate', stars: 2, reason: 'nice!' }])
    expect(r.error).toBe('boom')
    expect(r.tool_calls?.[0].status).toBe('failed')
    expect(c.done).toBe(true)
  })

  it('ignores events from other sessions and delegation components', () => {
    const c = new TurnCollector(SID, () => {})
    c.feed(main('unified_completion', { final_result: 'other', status: 'completed' }, { session_id: 'someone-else' }))
    c.feed(main('unified_completion', { final_result: 'w', status: 'completed' }, { data: { type: 'unified_completion', component: 'workshop-step', data: { final_result: 'w', status: 'completed' } } }))
    expect(c.done).toBe(false)
    expect(isMainEvent({ id: 'e1', type: 'agent_end', execution_kind: 'main_agent', execution_id: 'main:x', session_id: 'x' } as PlatformEvent, 'x')).toBe(true)
    expect(bareToolName('mcp__api-bridge__execute_shell_command')).toBe('execute_shell_command')
    expect(bareToolName('celebrate')).toBe('celebrate')
    expect(familyRelativePath('_users/u1/Chats/SparkQuill/activities/a')).toBe('activities/a')
    expect(familyRelativePath('reports/x.html')).toBe('reports/x.html')
  })
})

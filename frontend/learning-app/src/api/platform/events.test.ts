// The mapper is fed events shaped exactly like the agent server's polling
// and SSE frames (captured from a real SparkQuill turn), and must produce
// the preview events and TurnResult the SparkQuill UI consumes.
import { describe, expect, it } from 'vitest'
import type { TurnStreamEvent } from '../familyApi'
import { TurnCollector, bareToolName, familyRelativePath, isMainEvent, messagesFromEvents, type PlatformEvent } from './events'

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
    c.feed(main('llm_generation_end', { content: 'Got it, mom — Maya is all set.', metadata: { assistant_turn_text: 'Saving that now.\n\nGot it, mom — Maya is all set.' } }))
    c.feed(main('unified_completion', { final_result: 'Got it, mom — Maya is all set.', status: 'completed' }))
    expect(c.done).toBe(true)

    expect(seen[0]).toEqual({ type: 'replace', text: 'Got it' })
    expect(seen.filter((e) => e.type === 'status').map((e) => e.text)).toEqual(['Working in the workspace', '', ''])
    const calls = seen.filter((e) => e.type === 'tool_call').map((e) => `${e.tool_call?.tool_name}:${e.tool_call?.status}`)
    expect(calls).toEqual(['execute_shell_command:running', 'execute_shell_command:completed', 'set_child_profile:running', 'set_child_profile:completed'])

    const r = c.result()
    expect(r.reply).toBe('Saving that now.\n\nGot it, mom — Maya is all set.')
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

describe('TurnCollector streaming preview', () => {
  // The exact "Thinking" bug: the claude-code pane capture (source "terminal")
  // was appended to the preview as prose. The platform's classification rules
  // (same as AgentWorks' chat store) must apply here too.
  it('never shows terminal captures as prose, turns tool markers into status, joins block chunks', () => {
    const seen: TurnStreamEvent[] = []
    const c = new TurnCollector(SID, (e) => seen.push(e))
    c.feed(main('streaming_chunk', { content: '⏺ Two more options are ready for you. 🌟\n✻ Churned for 3s · done\n❯ Reply with exactly one short friendly line', chunk_index: 0, is_delta: false, source: 'terminal' }))
    c.feed(main('streaming_chunk', { content: 'api-bridge - execute_shell_command (MCP)', chunk_index: 1, is_delta: false, source: 'transcript' }))
    c.feed(main('streaming_chunk', { content: 'All good, mom! 😊', chunk_index: 2, is_delta: false, source: 'transcript' }))
    c.feed(main('streaming_chunk', { content: 'All good, mom! 😊', chunk_index: 2, is_delta: false, source: 'transcript' })) // poll overlap
    c.feed(main('streaming_chunk', { content: 'Two buttons are up for you. 🌟', chunk_index: 3, is_delta: false, source: 'transcript' }))
    expect(seen.filter((e) => e.type === 'delta')).toEqual([])
    expect(seen.filter((e) => e.type === 'replace').map((e) => e.text)).toEqual(['All good, mom! 😊', 'All good, mom! 😊\n\nTwo buttons are up for you. 🌟'])
    const statuses = seen.filter((e) => e.type === 'status').map((e) => e.text)
    expect(statuses[0]).toBe('Working')
    expect(statuses[1]).toMatch(/execute|shell|working/i)
    expect(statuses.at(-1)).toBe('')
    // A fresh generation (chunk 0) restarts the preview instead of appending.
    c.feed(main('streaming_chunk', { content: 'Second turn', chunk_index: 0, is_delta: true, source: 'content' }))
    expect(seen.at(-1)).toEqual({ type: 'replace', text: 'Second turn' })
  })
})

describe('messagesFromEvents', () => {
  // Shaped like /api/chat-history/sessions/{id} for a claude-code turn: the
  // narration, the tool call and the closing line are separate "ai" messages.
  it('rebuilds a restored claude-code conversation with the whole turn text', async () => {
    const { conversationToRestoredEvents } = await import('../../../../shared/session')
    const conversation = {
      session_id: SID,
      conversation_history: [
        { Role: 'system', Parts: [{ Type: 'text', Text: 'You are Quill.' }] },
        { Role: 'human', Parts: [{ Type: 'text', Text: 'hi, what can you help me with today?' }] },
        { Role: 'ai', Parts: [{ Type: 'text', Text: "Hi! I'm here to help you support Maya's grade 6 CBSE learning." }, { Type: 'function' }] },
        { Role: 'tool', Parts: [{ Type: 'text', Text: '{"ok":true}' }] },
        { Role: 'ai', Parts: [{ Type: 'text', Text: 'Three options are ready below to get started! 🌟' }] },
        { Role: 'human', Parts: [{ Type: 'text', Text: 'thanks' }] },
        { Role: 'ai', Parts: [{ Type: 'text', Text: 'Anytime!' }] },
      ],
    }
    const msgs = messagesFromEvents(conversationToRestoredEvents(conversation) as PlatformEvent[], SID)
    expect(msgs).toEqual([
      { role: 'user', text: 'hi, what can you help me with today?' },
      { role: 'assistant', text: "Hi! I'm here to help you support Maya's grade 6 CBSE learning.\n\nThree options are ready below to get started! 🌟" },
      { role: 'user', text: 'thanks' },
      { role: 'assistant', text: 'Anytime!' },
    ])
  })

  it('prefers the recorded turn text on live events and keeps product cards', () => {
    const events = [
      main('user_message', { content: 'celebrate her' }),
      main('llm_generation_end', { content: 'Done!', metadata: { assistant_turn_text: 'Cheering now.\n\nDone!' } }),
      main('product_interaction', { product: 'sparkquill', kind: 'celebrate', payload: { stars: 3, reason: 'fractions' } }),
      main('unified_completion', { final_result: 'Done!', status: 'completed' }),
    ]
    expect(messagesFromEvents(events, SID)).toEqual([
      { role: 'user', text: 'celebrate her' },
      { role: 'tool', tool: 'celebrate', stars: 3, reason: 'fractions' },
      { role: 'assistant', text: 'Cheering now.\n\nDone!' },
    ])
  })
})

import { describe, it, expect } from 'vitest'
import {
  pairToolCalls,
  selectTerminalEvents,
  buildTranscriptItems,
  collapseCompletedLifecycleStarts,
  type TranscriptItem,
} from './terminalEventTranscript'
import type { PollingEvent, TerminalSnapshot } from '../services/api-types'

// The rail is the hierarchy: every agent and sub-agent owns its own terminal.
// selectTerminalEvents is the contract between the rail and the transcript, and
// it fails SILENTLY when wrong — you get another agent's conversation rendered
// under this terminal, which looks like working software. These pin that,
// against the SAME owner-key derivation the (now-removed) tree used, not a
// narrower reimplementation of it.

function evt(partial: Partial<PollingEvent> & { id: string; session_id: string }): PollingEvent {
  return {
    type: 'agent_message',
    timestamp: '2026-07-25T10:00:00Z',
    ...partial,
  } as PollingEvent
}

function terminal(partial: Partial<TerminalSnapshot> & { session_id: string; owner_id: string }): TerminalSnapshot {
  return {
    terminal_id: `${partial.session_id}:${partial.owner_id}`,
    content: '',
    rows: [],
    chunk_index: 0,
    active: true,
    status: 'running',
    created_at: '',
    updated_at: '',
    ...partial,
  } as TerminalSnapshot
}

describe('selectTerminalEvents — owned terminal (workflow step, message-sequence item, ...)', () => {
  it('matches an event whose execution_id equals the terminal owner id', () => {
    const t = terminal({ session_id: 's1', owner_id: 'exec-1' })
    const events = [
      evt({ id: 'a', session_id: 's1', execution_id: 'exec-1' }),
      evt({ id: 'b', session_id: 's1', execution_id: 'exec-2' }),
    ]
    expect(selectTerminalEvents(events, t).map(e => e.id)).toEqual(['a'])
  })

  it('matches via the workflow-step composite id AND its short step-id alias', () => {
    // This is exactly the case that broke in production: a message-sequence
    // item terminal (owner_id = "msgseq-dummy-sequence-1-d-1-1784986266486292000")
    // whose agent events carry the PARENT step's execution id, not the item's
    // own id. getOwnedTerminalOwnerKeys derives a short alias from a
    // workflow-step composite id; matching on that is what makes this terminal
    // show its real conversation instead of falling back to an empty pane.
    const t = terminal({ session_id: 's1', owner_id: 'dummy-sequence-1' })
    const events = [
      evt({
        id: 'a', session_id: 's1',
        data: { data: { execution_id: 'workflow-step:exec-dummy-sequence-1-123:dummy-sequence-1' } } as never,
      }),
    ]
    expect(selectTerminalEvents(events, t).map(e => e.id)).toEqual(['a'])
  })

  it('does not leak a sibling sub-agent conversation into this terminal', () => {
    const t = terminal({ session_id: 's1', owner_id: 'exec-1' })
    const events = [
      evt({ id: 'mine', session_id: 's1', execution_id: 'exec-1' }),
      evt({ id: 'sibling', session_id: 's1', execution_id: 'exec-2' }),
    ]
    expect(selectTerminalEvents(events, t).map(e => e.id)).toEqual(['mine'])
  })

  it('keeps child workflow-step events out of their full-workflow parent transcript', () => {
    const parent = terminal({
      session_id: 's1',
      owner_id: 'workflow-full-1',
      execution_id: 'workflow-full-1',
      execution_kind: 'sub_agent',
    })
    const child = terminal({
      session_id: 's1',
      owner_id: 'workflow-step:workflow-full-1:eval-quality',
      execution_id: 'workflow-full-1',
      execution_kind: 'workflow_step',
      step_id: 'eval-quality',
    })
    const events = [
      evt({ id: 'parent', session_id: 's1', execution_id: 'workflow-full-1' }),
      evt({
        id: 'child',
        session_id: 's1',
        execution_id: 'workflow-full-1',
        data: {
          execution_id: 'workflow-full-1',
          metadata: {
            execution_id: 'workflow-step:workflow-full-1:eval-quality',
          },
        } as never,
      }),
    ]

    expect(selectTerminalEvents(events, parent, [parent, child]).map(e => e.id)).toEqual(['parent'])
    expect(selectTerminalEvents(events, child, [parent, child]).map(e => e.id)).toEqual(['child'])
  })

  it('matches via delegation_id / background_agent_id / agent_id, not just execution_id', () => {
    const t = terminal({ session_id: 's1', owner_id: 'delegation-9' })
    const events = [
      evt({ id: 'a', session_id: 's1', data: { delegation_id: 'delegation-9' } as never }),
    ]
    expect(selectTerminalEvents(events, t).map(e => e.id)).toEqual(['a'])
  })

  it('sorts chronologically so out-of-order arrivals do not read as scrambled', () => {
    const t = terminal({ session_id: 's1', owner_id: 'exec-1' })
    const events = [
      evt({ id: 'late', session_id: 's1', execution_id: 'exec-1', timestamp: '2026-07-25T10:00:05Z' }),
      evt({ id: 'early', session_id: 's1', execution_id: 'exec-1', timestamp: '2026-07-25T10:00:01Z' }),
    ]
    expect(selectTerminalEvents(events, t).map(e => e.id)).toEqual(['early', 'late'])
  })

  it('returns nothing for an empty or missing terminal', () => {
    const events = [evt({ id: 'a', session_id: 's1', execution_id: 'exec-1' })]
    expect(selectTerminalEvents(events, terminal({ session_id: 's1', owner_id: '' }))).toEqual([])
    expect(selectTerminalEvents(events, null)).toEqual([])
    expect(selectTerminalEvents(undefined, terminal({ session_id: 's1', owner_id: 'exec-1' }))).toEqual([])
  })
})

describe('selectTerminalEvents — main-agent terminal', () => {
  it('excludes events claimed by a sibling owned terminal when the sibling list is given', () => {
    const main = terminal({ session_id: 's1', owner_id: 'main:s1', execution_kind: 'main_agent' })
    const sub = terminal({ session_id: 's1', owner_id: 'exec-1' })
    const events = [
      evt({ id: 'mainEvt', session_id: 's1' }),
      evt({ id: 'subEvt', session_id: 's1', execution_id: 'exec-1' }),
    ]
    expect(selectTerminalEvents(events, main, [main, sub]).map(e => e.id)).toEqual(['mainEvt'])
  })

  it('is permissive (shows everything in-session) when no sibling list is supplied', () => {
    // Documented fallback: hiding a live turn because the sibling list was not
    // threaded through somewhere is worse than occasionally over-showing.
    const main = terminal({ session_id: 's1', owner_id: 'main:s1', execution_kind: 'main_agent' })
    const events = [
      evt({ id: 'mainEvt', session_id: 's1' }),
      evt({ id: 'subEvt', session_id: 's1', execution_id: 'exec-1' }),
    ]
    expect(selectTerminalEvents(events, main).map(e => e.id).sort()).toEqual(['mainEvt', 'subEvt'])
  })

  it('never shows another session entirely, sibling list or not', () => {
    const main = terminal({ session_id: 's1', owner_id: 'main:s1', execution_kind: 'main_agent' })
    const events = [evt({ id: 'other-session', session_id: 's2' })]
    expect(selectTerminalEvents(events, main)).toEqual([])
  })
})

describe('buildTranscriptItems', () => {
  it('replaces a finished background-agent start card with its one completion card', () => {
    const events = [
      evt({
        id: 'started', session_id: 's1', type: 'background_agent_started',
        data: { data: { agent_id: 'math-probe', name: 'Math Solver Probe' } } as never,
      }),
      evt({
        id: 'completed', session_id: 's1', type: 'background_agent_completed',
        data: { data: { agent_id: 'math-probe', name: 'Math Solver Probe', status: 'completed' } } as never,
      }),
    ]

    expect(collapseCompletedLifecycleStarts(events).map(event => event.id)).toEqual(['completed'])
    expect(buildTranscriptItems(events).map(item => item.key)).toEqual(['completed'])
  })

  it('keeps the background-agent start card until the agent finishes', () => {
    const events = [
      evt({
        id: 'started', session_id: 's1', type: 'background_agent_started',
        data: { data: { agent_id: 'math-probe', name: 'Math Solver Probe' } } as never,
      }),
    ]

    expect(buildTranscriptItems(events).map(item => item.key)).toEqual(['started'])
  })

  it('collapses generic and duplicate agent starts into the richest lifecycle card', () => {
    const events = [
      evt({
        id: 'background-start', session_id: 's1', execution_id: 'review-cost-1',
        type: 'background_agent_started',
        data: { data: { agent_id: 'review-cost-1', name: 'Review Cost Review' } } as never,
      }),
      evt({
        id: 'sparse-agent-start', session_id: 's1', execution_id: 'review-cost-1',
        type: 'orchestrator_agent_start',
        data: { data: { agent_name: 'Review Cost Review', agent_type: 'workshop-background-task' } } as never,
      }),
      evt({
        id: 'rich-agent-start', session_id: 's1', execution_id: 'review-cost-1',
        type: 'orchestrator_agent_start',
        data: {
          data: {
            agent_name: 'Review Cost Review',
            agent_type: 'workshop-background-task',
            provider: 'claude-code',
            model_id: 'claude-opus-5',
          },
        } as never,
      }),
    ]

    expect(buildTranscriptItems(events).map(item => item.key)).toEqual(['rich-agent-start'])
  })

  it('replaces all aliased start events with one completion event', () => {
    const events = [
      evt({
        id: 'background-start', session_id: 's1', execution_id: 'review-cost-1',
        type: 'background_agent_started',
        data: { data: { agent_id: 'review-cost-1', name: 'Review Cost Review' } } as never,
      }),
      evt({
        id: 'agent-start', session_id: 's1', execution_id: 'review-cost-1',
        type: 'orchestrator_agent_start',
        data: { data: { agent_name: 'Review Cost Review', provider: 'claude-code' } } as never,
      }),
      evt({
        id: 'agent-end', session_id: 's1', execution_id: 'review-cost-1',
        type: 'orchestrator_agent_end',
        data: { data: { agent_name: 'Review Cost Review', success: true } } as never,
      }),
    ]

    expect(buildTranscriptItems(events).map(item => item.key)).toEqual(['agent-end'])
  })

  it('does not collapse different sequence items sharing one execution', () => {
    const events = [
      evt({
        id: 'first', session_id: 's1', execution_id: 'sequence-1',
        type: 'orchestrator_agent_start',
        data: { data: { agent_name: 'Load Inputs' } } as never,
      }),
      evt({
        id: 'second', session_id: 's1', execution_id: 'sequence-1',
        type: 'orchestrator_agent_start',
        data: { data: { agent_name: 'Validate Result' } } as never,
      }),
    ]

    expect(buildTranscriptItems(events).map(item => item.key)).toEqual(['first', 'second'])
  })

  it('does not let one completed agent hide a different live sibling', () => {
    const events = [
      evt({ id: 'a-start', session_id: 's1', type: 'background_agent_started', data: { data: { agent_id: 'a', name: 'A' } } as never }),
      evt({ id: 'b-start', session_id: 's1', type: 'background_agent_started', data: { data: { agent_id: 'b', name: 'B' } } as never }),
      evt({ id: 'a-end', session_id: 's1', type: 'background_agent_completed', data: { data: { agent_id: 'a', name: 'A' } } as never }),
    ]

    expect(buildTranscriptItems(events).map(item => item.key)).toEqual(['b-start', 'a-end'])
  })

  it('folds a consecutive tool run into one collapsible group', () => {
    const items = buildTranscriptItems([
      evt({ id: 'm1', session_id: 's1', type: 'agent_message' }),
      evt({ id: 't1', session_id: 's1', type: 'tool_call_start' }),
      evt({ id: 't2', session_id: 's1', type: 'tool_call_end' }),
      evt({ id: 't3', session_id: 's1', type: 'tool_call_start' }),
      evt({ id: 't4', session_id: 's1', type: 'tool_call_end' }),
      evt({ id: 'm2', session_id: 's1', type: 'agent_message' }),
    ])
    expect(items.map(i => i.kind)).toEqual(['event', 'tools', 'event'])
    expect((items[1] as Extract<TranscriptItem, { kind: 'tools' }>).toolCount).toBe(2)
  })

  it('omits token usage because cost and context have dedicated UI', () => {
    const items = buildTranscriptItems([
      evt({ id: 'm1', session_id: 's1', type: 'agent_message' }),
      evt({ id: 'u1', session_id: 's1', type: 'token_usage' }),
      evt({ id: 'm2', session_id: 's1', type: 'agent_message' }),
    ])
    expect(items.map(i => i.kind)).toEqual(['event', 'event'])
    expect(items.map(i => i.key)).toEqual(['m1', 'm2'])
  })

  it('omits repeated provider status lines because the terminal footer owns them', () => {
    const items = buildTranscriptItems([
      evt({ id: 'm1', session_id: 's1', type: 'agent_message' }),
      evt({ id: 's1', session_id: 's1', type: 'status_line' }),
      evt({ id: 's2', session_id: 's1', type: 'status_line' }),
      evt({ id: 'm2', session_id: 's1', type: 'agent_message' }),
    ])
    expect(items.map(i => i.key)).toEqual(['m1', 'm2'])
  })

  it('does not show token usage beside collapsed tool calls', () => {
    const items = buildTranscriptItems([
      evt({ id: 't1', session_id: 's1', type: 'tool_call_start' }),
      evt({ id: 't2', session_id: 's1', type: 'tool_call_end' }),
      evt({ id: 'u1', session_id: 's1', type: 'token_usage' }),
      evt({ id: 'm1', session_id: 's1', type: 'agent_message' }),
    ])
    expect(items.map(i => i.kind)).toEqual(['tools', 'event'])
    expect(items.map(i => i.key)).toEqual(['t1', 'm1'])
  })

  it('does not split a batch when delegation events interleave', () => {
    const items = buildTranscriptItems([
      evt({ id: 't1', session_id: 's1', type: 'tool_call_start' }),
      evt({ id: 'd1', session_id: 's1', type: 'delegation_start' }),
      evt({ id: 'd2', session_id: 's1', type: 'delegation_end' }),
      evt({ id: 't2', session_id: 's1', type: 'tool_call_end' }),
    ])
    expect(items.map(i => i.kind)).toEqual(['tools'])
  })

  it('preserves message order around tool groups', () => {
    const items = buildTranscriptItems([
      evt({ id: 'm1', session_id: 's1', type: 'agent_message' }),
      evt({ id: 't1', session_id: 's1', type: 'tool_call_start' }),
      evt({ id: 'm2', session_id: 's1', type: 'agent_message' }),
    ])
    expect(items.map(i => i.key)).toEqual(['m1', 't1', 'm2'])
  })

  it('handles an empty list', () => {
    expect(buildTranscriptItems([])).toEqual([])
  })
})

describe('buildTranscriptItems — duplicate execution_prompt suppression', () => {
  it('drops an execution_prompt user_message when the preceding orchestrator_agent_start already carries the same content', () => {
    const items = buildTranscriptItems([
      evt({
        id: 'start', session_id: 's1', type: 'orchestrator_agent_start',
        data: { data: { user_message: 'Do the thing' } } as never,
      }),
      evt({
        id: 'prompt', session_id: 's1', type: 'user_message',
        data: { data: { content: 'Do the thing', metadata: { source: 'execution_prompt' } } } as never,
      }),
      evt({ id: 'reply', session_id: 's1', type: 'agent_message' }),
    ])
    expect(items.map(i => i.key)).toEqual(['start', 'reply'])
  })

  it('drops an execution_prompt user_message when the preceding background_agent_started already carries the instruction', () => {
    const items = buildTranscriptItems([
      evt({
        id: 'start', session_id: 's1', type: 'background_agent_started',
        data: { data: { instruction: 'Count the folders' } } as never,
      }),
      evt({
        id: 'prompt', session_id: 's1', type: 'user_message',
        data: { data: { content: 'Count the folders', metadata: { source: 'execution_prompt' } } } as never,
      }),
    ])
    expect(items.map(i => i.key)).toEqual(['start'])
  })

  it('keeps the execution_prompt message when the preceding start event has no content to duplicate', () => {
    // Some execution kinds' own start event never carries the kickoff text —
    // dropping here would silently lose the only copy. Never guess from the tag alone.
    const items = buildTranscriptItems([
      evt({
        id: 'start', session_id: 's1', type: 'orchestrator_agent_start',
        data: { data: { user_message: '' } } as never,
      }),
      evt({
        id: 'prompt', session_id: 's1', type: 'user_message',
        data: { data: { content: 'Only copy of this text', metadata: { source: 'execution_prompt' } } } as never,
      }),
    ])
    expect(items.map(i => i.key)).toEqual(['start', 'prompt'])
  })

  it('keeps an execution_prompt message with no preceding lifecycle-start event at all', () => {
    const items = buildTranscriptItems([
      evt({
        id: 'prompt', session_id: 's1', type: 'user_message',
        data: { data: { content: 'Kickoff', metadata: { source: 'execution_prompt' } } } as never,
      }),
    ])
    expect(items.map(i => i.key)).toEqual(['prompt'])
  })

  it('never touches an [AUTO-NOTIFICATION] user_message even after a content-bearing start event', () => {
    const items = buildTranscriptItems([
      evt({
        id: 'start', session_id: 's1', type: 'orchestrator_agent_start',
        data: { data: { user_message: 'Do the thing' } } as never,
      }),
      evt({
        id: 'notice', session_id: 's1', type: 'user_message',
        data: { data: { content: '[AUTO-NOTIFICATION] Agent X completed' } } as never,
      }),
    ])
    expect(items.map(i => i.key)).toEqual(['start', 'notice'])
  })
})

describe('pairToolCalls', () => {
  const tool = (id: string, type: string, extra: Record<string, unknown> = {}) =>
    evt({ id, session_id: 's1', type, data: { data: { tool_call_id: 'call-1', ...extra } } as never })

  it('collapses a start/end pair into ONE row instead of two', () => {
    const pairs = pairToolCalls([
      tool('a', 'tool_call_start', { tool_name: 'ToolSearch' }),
      tool('b', 'tool_call_end', { tool_name: 'ToolSearch', duration: 1200 }),
    ])
    expect(pairs).toHaveLength(1)
    expect(pairs[0].name).toBe('ToolSearch')
    expect(pairs[0].status).toBe('ok')
    expect(pairs[0].durationMs).toBe(1200)
    expect(pairs[0].events).toHaveLength(2)
  })

  it('strips the mcp__server__tool wire name and keeps the server separately', () => {
    const pairs = pairToolCalls([
      tool('a', 'tool_call_start', { tool_name: 'mcp__api-bridge__agent_browser' }),
    ])
    expect(pairs[0].name).toBe('agent_browser')
    expect(pairs[0].server).toBe('api-bridge')
  })

  it('marks a call still running when only the start has arrived', () => {
    expect(pairToolCalls([tool('a', 'tool_call_start', { tool_name: 'x' })])[0].status).toBe('running')
  })

  it('an error anywhere in the pair wins over a later success', () => {
    const pairs = pairToolCalls([
      tool('a', 'tool_call_start', { tool_name: 'x' }),
      tool('b', 'tool_call_error', { tool_name: 'x' }),
      tool('c', 'tool_call_end', { tool_name: 'x' }),
    ])
    expect(pairs).toHaveLength(1)
    expect(pairs[0].status).toBe('error')
  })

  it('keeps interleaved calls separate rather than merging them', () => {
    const a = (id: string, type: string, name: string) =>
      evt({ id, session_id: 's1', type, data: { data: { tool_call_id: 'A', tool_name: name } } as never })
    const b = (id: string, type: string, name: string) =>
      evt({ id, session_id: 's1', type, data: { data: { tool_call_id: 'B', tool_name: name } } as never })
    const pairs = pairToolCalls([
      a('1', 'tool_call_start', 'first'),
      b('2', 'tool_call_start', 'second'),
      a('3', 'tool_call_end', 'first'),
      b('4', 'tool_call_end', 'second'),
    ])
    expect(pairs.map(p => p.name)).toEqual(['first', 'second'])
    expect(pairs.every(p => p.status === 'ok')).toBe(true)
  })

  it('never drops an event that has no tool_call_id to pair on', () => {
    const orphan = evt({ id: 'o', session_id: 's1', type: 'tool_call_end', data: { data: { tool_name: 'lonely' } } as never })
    const pairs = pairToolCalls([orphan])
    expect(pairs).toHaveLength(1)
    expect(pairs[0].name).toBe('lonely')
  })
})

describe('pairToolCalls — args and result surfaced on the pair', () => {
  it('takes arguments from the START event and the result from the END event', () => {
    // This split is the whole reason a per-event card was misleading: neither
    // event alone can render a complete tool call.
    const start = evt({
      id: 'a', session_id: 's1', type: 'tool_call_start',
      data: { data: { tool_call_id: 'c1', tool_name: 'agent_browser', tool_params: { arguments: '{"action":"status"}' } } } as never,
    })
    const end = evt({
      id: 'b', session_id: 's1', type: 'tool_call_end',
      data: { data: { tool_call_id: 'c1', tool_name: 'agent_browser', result: 'CDP connected', duration: 1200 } } as never,
    })
    const [pair] = pairToolCalls([start, end])
    expect(pair.args).toBe('{"action":"status"}')
    expect(pair.result).toBe('CDP connected')
    expect(pair.durationMs).toBe(1200)
    expect(pair.status).toBe('ok')
  })

  it('leaves args/result undefined when the provider sent neither', () => {
    const [pair] = pairToolCalls([
      evt({ id: 'a', session_id: 's1', type: 'tool_call_start', data: { data: { tool_call_id: 'c1', tool_name: 'x' } } as never }),
    ])
    expect(pair.args).toBeUndefined()
    expect(pair.result).toBeUndefined()
  })
})

describe('final answer visibility (CDP step regression)', () => {
  const ev = (id: string, type: string, content?: string): any => ({
    id,
    type,
    data: { data: { ...(content !== undefined ? { content } : {}), tool_name: 'agent_browser' } },
  })

  // A browser/CDP step ends on a tool call and then answers. The answer event
  // must not be swallowed by the adjacent tool batch: nothing else in the
  // transcript renders that text (EventDispatcher has no streaming_chunk case).
  it('keeps a content-carrying llm_generation_end out of an adjacent tool batch', () => {
    const items = buildTranscriptItems([
      ev('t1', 'tool_call_start'),
      ev('t2', 'tool_call_end'),
      ev('answer', 'llm_generation_end', 'Snapshot captured. The page shows 3 results.'),
    ])

    const rendered = items.find((i) => i.kind === 'event' && (i as any).event.id === 'answer')
    expect(rendered).toBeDefined()
  })

  // An empty one is still just noise closing a generation, and should stay
  // absorbed rather than splitting the batch into two collapsed groups.
  it('still absorbs an empty llm_generation_end into the tool batch', () => {
    const items = buildTranscriptItems([
      ev('t1', 'tool_call_start'),
      ev('t2', 'tool_call_end'),
      ev('empty', 'llm_generation_end', '   '),
    ])

    expect(items.some((i) => i.kind === 'event' && (i as any).event.id === 'empty')).toBe(false)
    expect(items.some((i) => i.kind === 'tools')).toBe(true)
  })
})

describe('answer shown exactly once', () => {
  const ANSWER =
    'Confirmed CDP is available: status reported the endpoint reachable, and a live snapshot succeeded.'
  const gen = (content: string): any => ({
    id: 'gen', type: 'llm_generation_end', data: { data: { content } },
  })
  const completion = (final_result: string): any => ({
    id: 'done', type: 'unified_completion', data: { data: { final_result, status: 'completed' } },
  })

  it('drops the generation-end copy when a completion card repeats it', () => {
    const items = buildTranscriptItems([gen(ANSWER), completion(ANSWER)])
    const ids = items.filter(i => i.kind === 'event').map(i => (i as any).event.id)
    expect(ids).toContain('done')
    expect(ids).not.toContain('gen')
  })

  it('still keeps generation-end when no completion card carries the answer', () => {
    const items = buildTranscriptItems([gen(ANSWER)])
    const ids = items.filter(i => i.kind === 'event').map(i => (i as any).event.id)
    expect(ids).toContain('gen')
  })

  it('keeps both when the completion card carries a different answer', () => {
    const items = buildTranscriptItems([gen(ANSWER), completion('Something else entirely happened here.')])
    const ids = items.filter(i => i.kind === 'event').map(i => (i as any).event.id)
    expect(ids).toContain('gen')
    expect(ids).toContain('done')
  })
})

describe('full-run container rows', () => {
  const ev = (id: string, type: string, execution_kind?: string): any => ({
    id, type, execution_kind, data: { data: { name: 'full-run [Toptal Bid / iteration-0]' } },
  })

  // A full run has no conversation of its own -- the backend already keeps it
  // out of the rail for that reason. Its lifecycle card just restated the panel
  // header before the first real event.
  it('drops full_run lifecycle rows from the transcript', () => {
    const items = buildTranscriptItems([
      ev('fullrun', 'background_agent_started', 'full_run'),
      ev('routing', 'orchestrator_agent_start', 'orchestrator'),
    ])
    const ids = items.filter(i => i.kind === 'event').map(i => (i as any).event.id)
    expect(ids).not.toContain('fullrun')
    expect(ids).toContain('routing')
  })

  it('keeps non-lifecycle events even when tagged full_run', () => {
    const items = buildTranscriptItems([ev('msg', 'user_message', 'full_run')])
    const ids = items.filter(i => i.kind === 'event').map(i => (i as any).event.id)
    expect(ids).toContain('msg')
  })
})

describe('lifecycle alias dedupe', () => {
  const start = (id: string, type: string, name: string, execution_id = 'pulse-review-2026-07-27-bug'): any => ({
    id, type, execution_id, data: { data: { name, provider: 'claude-code', model_id: 'opus' } },
  })

  // Real production names for ONE reviewer: the server and the delegated agent
  // decorate the same execution differently, so the agent rendered twice.
  it('collapses background/orchestrator starts for the same execution', () => {
    const items = buildTranscriptItems([
      start('bg', 'background_agent_started', 'Pulse reviewer: pulse 2026 07 27 bug review'),
      start('orch', 'orchestrator_agent_start', 'Background: Pulse reviewer - pulse-2026-07-27-bug-review'),
    ])
    const ids = items.filter(i => i.kind === 'event').map(i => (i as any).event.id)
    expect(ids).toHaveLength(1)
    expect(ids).toContain('orch') // the richer payload wins
  })

  // Siblings that legitimately share one execution id must stay apart. An eval
  // step emits several orchestrator starts under the same id; repeats WITHIN a
  // family are the signal that these are distinct agents, not aliases.
  it('keeps repeated starts from the same family distinct', () => {
    const items = buildTranscriptItems([
      start('v2', 'orchestrator_agent_start', 'step-1-execution-evaluate-search-route-val-2', 'workflow-step:eval'),
      start('v3', 'orchestrator_agent_start', 'step-1-execution-evaluate-search-route-val-3', 'workflow-step:eval'),
    ])
    const ids = items.filter(i => i.kind === 'event').map(i => (i as any).event.id)
    expect(ids).toHaveLength(2)
  })

  // Mixed families where one family repeats: the repeats are real agents, so
  // nothing may collapse on execution id alone.
  it('does not alias-collapse when a family repeats on that execution', () => {
    const items = buildTranscriptItems([
      start('a', 'agent_start', 'eval', 'workflow-step:eval'),
      start('v2', 'orchestrator_agent_start', 'route-val-2', 'workflow-step:eval'),
      start('v3', 'orchestrator_agent_start', 'route-val-3', 'workflow-step:eval'),
    ])
    const ids = items.filter(i => i.kind === 'event').map(i => (i as any).event.id)
    expect(ids).toContain('v2')
    expect(ids).toContain('v3')
  })

  // Different executions are never merged, however similar the names.
  it('never merges across executions', () => {
    const items = buildTranscriptItems([
      start('bug', 'background_agent_started', 'Pulse reviewer: pulse bug review', 'exec-bug'),
      start('art', 'background_agent_started', 'Pulse reviewer: pulse artifact review', 'exec-artifact'),
    ])
    const ids = items.filter(i => i.kind === 'event').map(i => (i as any).event.id)
    expect(ids).toHaveLength(2)
  })
})

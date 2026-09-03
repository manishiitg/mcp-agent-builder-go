import { describe, expect, it } from 'vitest'
import type { PollingEvent } from '../../services/api-types'
import { coalesceThinkingDeltas } from '../thinkingDeltas'

function thinking(id: string, text: string, opts: { delta?: boolean; turn?: number; nested?: boolean } = {}): PollingEvent {
  const payload = { thinking: text, turn: opts.turn ?? 1, ...(opts.delta ? { is_delta: true } : {}) }
  return {
    id,
    type: 'conversation_thinking',
    timestamp: '2026-09-03T00:00:00Z',
    data: opts.nested === false ? payload : { data: payload },
  } as unknown as PollingEvent
}

function toolCall(id: string): PollingEvent {
  return { id, type: 'tool_call_start', timestamp: '2026-09-03T00:00:01Z', data: { tool_name: 'read_file' } } as unknown as PollingEvent
}

function textOf(event: PollingEvent): string {
  const env = event.data as Record<string, unknown>
  const inner = env.data as Record<string, unknown> | undefined
  return String(inner?.thinking ?? env.thinking)
}

describe('coalesceThinkingDeltas', () => {
  it('folds a run of cursor thinking fragments into the first event of the span', () => {
    const out = coalesceThinkingDeltas([
      thinking('a', 'Reading', { delta: true }),
      thinking('b', ' soul.md first.', { delta: true }),
      thinking('c', ' Determining the current', { delta: true }),
      thinking('d', ' phase from plan.json', { delta: true }),
    ])
    expect(out).toHaveLength(1)
    expect(out[0].id).toBe('a')
    expect(textOf(out[0])).toBe('Reading soul.md first. Determining the current phase from plan.json')
  })

  it('starts a new block after a non-thinking event and across turns', () => {
    const out = coalesceThinkingDeltas([
      thinking('a', 'Plan', { delta: true }),
      thinking('b', 'ning', { delta: true }),
      toolCall('t'),
      thinking('c', 'Next', { delta: true }),
      thinking('d', ' step', { delta: true }),
      thinking('e', 'Turn two', { delta: true, turn: 2 }),
    ])
    expect(out.map(e => e.id)).toEqual(['a', 't', 'c', 'e'])
    expect(textOf(out[0])).toBe('Planning')
    expect(textOf(out[2])).toBe('Next step')
  })

  it('leaves whole-block thinking events (no is_delta) as separate cards', () => {
    const input = [thinking('a', 'First block'), thinking('b', 'Second block')]
    const out = coalesceThinkingDeltas(input)
    expect(out).toBe(input)
  })

  it('is idempotent and does not mutate the input events', () => {
    const first = thinking('a', 'Read', { delta: true })
    const input = [first, thinking('b', 'ing', { delta: true })]
    const once = coalesceThinkingDeltas(input)
    expect(textOf(first)).toBe('Read')
    expect(coalesceThinkingDeltas(once)).toEqual(once)
    expect(coalesceThinkingDeltas([...once, thinking('c', ' more', { delta: true })])).toHaveLength(1)
  })

  it('handles the flat envelope shape too', () => {
    const out = coalesceThinkingDeltas([
      thinking('a', 'Flat', { delta: true, nested: false }),
      thinking('b', ' text', { delta: true, nested: false }),
    ])
    expect(out).toHaveLength(1)
    expect(textOf(out[0])).toBe('Flat text')
  })
})

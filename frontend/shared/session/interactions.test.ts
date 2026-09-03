import { describe, expect, it } from 'vitest'
import { latestSuggestions, parseProductInteraction } from './interactions'
import type { PollingEvent } from './types'

const ev = (type: string, data: Record<string, unknown>) => ({ id: `${type}-${Math.random()}`, type, session_id: 's', data: { type, data } }) as unknown as PollingEvent

describe('product interactions', () => {
  it('parses an interaction and keeps only the latest reply\'s suggestions', () => {
    const s1 = ev('product_interaction', { product: 'p', kind: 'suggestions', payload: { actions: [{ label: 'A', message: 'a' }] } })
    expect(parseProductInteraction(s1)?.kind).toBe('suggestions')
    expect(parseProductInteraction(ev('user_message', { content: 'x' }))).toBeNull()
    const events = [s1, ev('user_message', { content: 'next' }), ev('product_interaction', { kind: 'suggestions', payload: { actions: [{ label: 'B', message: 'b' }, { label: '', message: 'skip' }] } })]
    expect(latestSuggestions(events)).toEqual([{ label: 'B', message: 'b' }])
    expect(latestSuggestions([s1, ev('user_message', { content: 'later' })])).toEqual([])
  })
})

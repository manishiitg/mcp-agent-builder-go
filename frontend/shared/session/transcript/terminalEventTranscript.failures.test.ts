import { describe, expect, it } from 'vitest'
import { collapseTurnFailures, turnFailureText, type TranscriptItem } from './terminalEventTranscript'
import type { PollingEvent } from '../../../src/services/api-types'

const SID = 'product-1'
const raw = "all LLMs failed (primary + 0 fallbacks): codex-cli/gpt-6-astra [quota_exhausted]: Codex input remained unconfirmed after 8s"

function event(type: string, data: Record<string, unknown>, id = `${type}-${Math.random()}`): PollingEvent {
  return { id, type, session_id: SID, execution_kind: 'main_agent', execution_id: `main:${SID}`, data: { type, data } } as unknown as PollingEvent
}
const item = (e: PollingEvent): TranscriptItem => ({ kind: 'event', key: e.id, event: e })

describe('turnFailureText', () => {
  it('reads the error off each failure-bearing event and nothing else', () => {
    expect(turnFailureText(event('llm_generation_error', { error: raw }))).toBe(raw)
    expect(turnFailureText(event('conversation_error', { error: raw }))).toBe(raw)
    expect(turnFailureText(event('unified_completion', { status: 'error', final_result: raw }))).toBe(raw)
    expect(turnFailureText(event('unified_completion', { status: 'completed', final_result: 'Hello' }))).toBe('')
    expect(turnFailureText(event('llm_generation_end', { content: 'Hello' }))).toBe('')
  })
})

describe('collapseTurnFailures', () => {
  it('keeps only the last failure of a turn, and one per turn', () => {
    const items = [
      item(event('user_message', { content: 'hi' })),
      item(event('llm_generation_error', { error: raw })),
      item(event('conversation_error', { error: raw })),
      item(event('unified_completion', { status: 'error', final_result: raw }, 'completion-1')),
      item(event('user_message', { content: 'again' })),
      item(event('llm_generation_error', { error: 'second turn failed' }, 'second-1')),
    ]
    const out = collapseTurnFailures(items)
    expect(out.map((i) => i.kind === 'event' ? i.event.type : i.kind)).toEqual([
      'user_message', 'unified_completion', 'user_message', 'llm_generation_error',
    ])
    expect(out[1].key).toBe('completion-1')
    expect(out[3].key).toBe('second-1')
  })

  it('leaves successful turns and non-failure events untouched', () => {
    const items = [
      item(event('user_message', { content: 'hi' })),
      item(event('tool_call_start', { tool_name: 'x' })),
      item(event('unified_completion', { status: 'completed', final_result: 'done' })),
    ]
    expect(collapseTurnFailures(items)).toEqual(items)
  })
})

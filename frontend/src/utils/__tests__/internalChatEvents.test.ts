import { describe, expect, it } from 'vitest'
import type { PollingEvent } from '../../services/api-types'
import { autoNotificationDedupKey, isInternalAutoNotificationEvent } from '../internalChatEvents'

function userMessage(id: string, content: string): PollingEvent {
  return {
    id,
    type: 'user_message',
    timestamp: new Date().toISOString(),
    data: { type: 'user_message', data: { content } },
  } as PollingEvent
}

describe('autoNotificationDedupKey', () => {
  it('keys an auto-notification by its content, ignoring the transport event id', () => {
    const a = userMessage('steer-message-1', "[AUTO-NOTIFICATION] Agent 'X [default]' completed — status=completed")
    const b = userMessage('streaming-chunk-99', "[AUTO-NOTIFICATION] Agent 'X [default]' completed — status=completed")
    const key = autoNotificationDedupKey(a)
    expect(key).toBeTruthy()
    expect(autoNotificationDedupKey(b)).toBe(key)
  })

  it('keeps distinct completions of the same step distinct (different Result payloads)', () => {
    const run1 = userMessage('a', "[AUTO-NOTIFICATION] Agent 'X [default]' completed\nResult: {\"n\": 1}")
    const run2 = userMessage('b', "[AUTO-NOTIFICATION] Agent 'X [default]' completed\nResult: {\"n\": 2}")
    expect(autoNotificationDedupKey(run1)).not.toBe(autoNotificationDedupKey(run2))
  })

  it('returns null for an ordinary user message so real repeats are never collapsed', () => {
    const a = userMessage('u1', 'run it again')
    const b = userMessage('u2', 'run it again')
    expect(autoNotificationDedupKey(a)).toBeNull()
    expect(autoNotificationDedupKey(b)).toBeNull()
    expect(isInternalAutoNotificationEvent(a)).toBe(false)
  })

  it('reads content from the flat data shape as well as the nested one', () => {
    const flat = {
      id: 'x', type: 'user_message', timestamp: '', data: { content: '[AUTO-NOTIFICATION] Agent Y completed' },
    } as unknown as PollingEvent
    expect(isInternalAutoNotificationEvent(flat)).toBe(true)
    expect(autoNotificationDedupKey(flat)).toContain('[AUTO-NOTIFICATION] Agent Y completed')
  })

  it('is not fooled by a non-user_message event carrying the text', () => {
    const chunk = {
      id: 'c', type: 'streaming_chunk', timestamp: '',
      data: { type: 'streaming_chunk', data: { content: "❯ [AUTO-NOTIFICATION] Agent 'X' completed" } },
    } as unknown as PollingEvent
    expect(autoNotificationDedupKey(chunk)).toBeNull()
  })
})

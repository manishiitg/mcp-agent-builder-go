import { describe, expect, it } from 'vitest'
import type { PollingEvent } from '../services/api-types'
import { retainEventInSessionWorkingSet } from './sessionEventWorkingSet'

const sessionId = 'session-1'

function event(type: string, terminalId?: string, ownerId?: string): PollingEvent {
  return {
    id: `${type}-${terminalId || 'legacy'}`,
    type,
    timestamp: '2026-08-02T00:00:00Z',
    data: {},
    terminal_id: terminalId,
    terminal_owner_id: ownerId,
  } as PollingEvent
}

describe('retainEventInSessionWorkingSet', () => {
  it('retains legacy events that do not have canonical ownership', () => {
    expect(retainEventInSessionWorkingSet(sessionId, event('tool_call_end'))).toBe(true)
  })

  it('retains detailed main-agent events', () => {
    const main = event('tool_call_end', `${sessionId}:main:${sessionId}`, `main:${sessionId}`)
    expect(retainEventInSessionWorkingSet(sessionId, main)).toBe(true)
  })

  it.each(['tool_call_start', 'tool_call_end', 'llm_generation_end', 'streaming_chunk', 'user_message'])(
    'does not retain child %s transcript details',
    type => {
      const child = event(type, `${sessionId}:background:reviewer`, 'background:reviewer')
      expect(retainEventInSessionWorkingSet(sessionId, child)).toBe(false)
    },
  )

  it.each(['background_agent_started', 'background_agent_completed', 'workflow_step_started'])(
    'retains child %s control events',
    type => {
      const child = event(type, `${sessionId}:background:reviewer`, 'background:reviewer')
      expect(retainEventInSessionWorkingSet(sessionId, child)).toBe(true)
    },
  )
})

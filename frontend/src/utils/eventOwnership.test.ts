import { describe, it, expect } from 'vitest'
import { getOwnedTerminalOwnerKeys, getTerminalOwnerPayload } from './eventOwnership'
import type { PollingEvent } from '../services/api-types'

function evt(partial: Partial<PollingEvent> & { id: string; session_id: string }): PollingEvent {
  return {
    type: 'tool_call_start',
    timestamp: '2026-07-26T10:00:00Z',
    ...partial,
  } as PollingEvent
}

const keysFor = (event: PollingEvent) => getOwnedTerminalOwnerKeys(event, getTerminalOwnerPayload(event))

describe('getOwnedTerminalOwnerKeys — delegate-tool background agent content', () => {
  it('resolves a delegation-shaped event to its background agent via parent_execution_id', () => {
    // Mirrors what DelegationEventObserver actually forwards for a background
    // delegation: correlation_id = the delegationID, parent_execution_id =
    // the background agent's own id (inherited from delegation_start per
    // internal/events/event_store.go's findParentExecutionOwnership). No
    // field carries the background agent's id directly.
    const event = evt({
      id: 'a',
      session_id: 's1',
      execution_id: undefined,
      parent_execution_id: 'chat-0001',
      correlation_id: 'delegation-0-1785039217239436000',
    } as never)
    expect(keysFor(event)).toContain('s1:chat-0001')
  })

  it('does not fall back to parent_execution_id for a non-delegation-shaped event', () => {
    // A todo_task-dispatched sub-agent's event carrying parent_execution_id
    // pointing at its orchestrator must NOT also match the orchestrator's own
    // terminal — that would duplicate the sub-agent's tool calls into both
    // panes. The delegation-shape guard (correlation_id starting with
    // "delegation-") is what prevents this.
    const event = evt({
      id: 'b',
      session_id: 's1',
      execution_id: 'sub-agent-7',
      parent_execution_id: 'orchestrator-1',
      correlation_id: 'todo-task-step-3',
    } as never)
    const keys = keysFor(event)
    expect(keys).toContain('s1:sub-agent-7')
    expect(keys).not.toContain('s1:orchestrator-1')
  })
})

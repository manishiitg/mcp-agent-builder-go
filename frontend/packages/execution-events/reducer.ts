import type { ExecutionEvent } from './types'

export type ExecutionEventsAction =
  | { type: 'replace'; events: ExecutionEvent[] }
  | { type: 'upsert'; event: ExecutionEvent }
  | { type: 'clear' }

export function executionEventsReducer(state: ExecutionEvent[], action: ExecutionEventsAction): ExecutionEvent[] {
  if (action.type === 'clear') return []
  if (action.type === 'replace') return deduplicate(action.events)
  const index = state.findIndex((event) => event.id === action.event.id)
  return index < 0
    ? deduplicate([...state, action.event])
    : state.map((event, eventIndex) => eventIndex === index ? action.event : event)
}

function deduplicate(events: ExecutionEvent[]) {
  const byID = new Map(events.map((event) => [event.id, event]))
  return [...byID.values()].sort((left, right) => {
    const time = Date.parse(left.createdAt) - Date.parse(right.createdAt)
    return time || left.id.localeCompare(right.id)
  })
}

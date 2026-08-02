import type { PollingEvent } from '../services/api-types'

// Child transcript payloads are fetched from /api/terminals/{id}/events only
// while that terminal is selected. Keeping these high-volume records in the
// session-wide Zustand store would retain every unopened agent's tool
// arguments/results in Electron memory and defeat lazy loading.
const CHILD_TRANSCRIPT_DETAIL_EVENT_TYPES = new Set<string>([
  'tool_call_start',
  'tool_call_end',
  'tool_call_error',
  'llm_generation_start',
  'llm_generation_end',
  'llm_generation_error',
  'streaming_start',
  'streaming_chunk',
  'streaming_end',
  'status_line',
  'token_usage',
  'system_prompt',
  'user_message',
])

// Low-volume lifecycle and workflow-control events remain session-scoped for
// status, canvas, badges, and completion handling. Legacy events without a
// canonical terminal identity fail open so an older retained run is never
// silently hidden.
export function retainEventInSessionWorkingSet(sessionId: string, event: PollingEvent): boolean {
  const terminalId = event.terminal_id?.trim()
  if (!terminalId) return true

  const ownerId = event.terminal_owner_id?.trim()
  const isMain = ownerId === `main:${sessionId}` || terminalId === `${sessionId}:main:${sessionId}`
  return isMain || !CHILD_TRANSCRIPT_DETAIL_EVENT_TYPES.has(event.type || '')
}

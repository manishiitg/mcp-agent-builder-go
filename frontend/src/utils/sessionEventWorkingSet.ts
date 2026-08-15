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

/**
 * eventBelongsToSession is the session-ownership boundary. An event is only
 * ever stored under the session that produced it.
 *
 * This is deliberately separate from the working-set volume filter below. That
 * filter classifies events by COST (child transcript detail vs. session
 * lifecycle) and fails open for anything without a terminal identity — which
 * meant an event owned by session A, arriving under a response envelope
 * labelled B, was written into B's bucket and rendered as if it answered the
 * user's unrelated question in B.
 *
 * An event that declares no owning session is accepted: legacy and locally
 * synthesized (optimistic) records carry no `session_id`, and rejecting those
 * would silently drop the user's own messages. Only a DISAGREEING owner is
 * rejected, which is the case that can cross a session boundary.
 */
export function eventBelongsToSession(sessionId: string, event: PollingEvent): boolean {
  const owner = event.session_id?.trim()
  if (!owner) return true
  return owner === sessionId.trim()
}

// Low-volume lifecycle and workflow-control events remain session-scoped for
// status, canvas, badges, and completion handling. Legacy events without a
// canonical terminal identity fail open so an older retained run is never
// silently hidden.
//
// Ownership is checked FIRST and never fails open: a foreign-session event is
// rejected regardless of its type or terminal identity.
export function retainEventInSessionWorkingSet(sessionId: string, event: PollingEvent): boolean {
  if (!eventBelongsToSession(sessionId, event)) return false

  const terminalId = event.terminal_id?.trim()
  if (!terminalId) return true

  const ownerId = event.terminal_owner_id?.trim()
  const isMain = ownerId === `main:${sessionId}` || terminalId === `${sessionId}:main:${sessionId}`
  return isMain || !CHILD_TRANSCRIPT_DETAIL_EVENT_TYPES.has(event.type || '')
}

import { useMemo } from 'react'
import { useChatStore } from '../../stores/useChatStore'
import { parsePresentationUpdatedEvent, type PresentationUpdatedEvent } from '../../../shared/session/presentations'
export { PRESENTATION_UPDATED_EVENT_TYPE, parsePresentationUpdatedEvent } from '../../../shared/session/presentations'
export type { PresentationUpdatedEvent } from '../../../shared/session/presentations'

/**
 * Live updates for anything a product tool has presented in this session —
 * a video today, a report or any other kind a future product declares
 * tomorrow. Reads from the same tabEvents stream the chat transcript already
 * renders from (useChatStore.connectSSE); it does not open a second
 * connection.
 *
 * Filter by `kinds` to only react to the kinds a given surface renders (e.g.
 * Video Studio's panel only cares about "media.video"), the same way the
 * frontend's presentation renderer registry dispatches by kind rather than
 * by which product or tool produced the event.
 */
export function usePresentationEvents(sessionId: string | undefined, kinds?: string[]): PresentationUpdatedEvent[] {
  const events = useChatStore((state) => (sessionId ? state.tabEvents[sessionId] : undefined))
  return useMemo(() => {
    if (!events || events.length === 0) return []
    const kindFilter = kinds && kinds.length > 0 ? new Set(kinds) : null
    const parsed: PresentationUpdatedEvent[] = []
    for (const event of events) {
      const presentation = parsePresentationUpdatedEvent(event)
      if (presentation && (!kindFilter || kindFilter.has(presentation.kind))) parsed.push(presentation)
    }
    return parsed
  }, [events, kinds])
}

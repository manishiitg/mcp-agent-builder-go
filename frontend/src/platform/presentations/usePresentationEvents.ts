import { useMemo } from 'react'
import { useChatStore } from '../../stores/useChatStore'
import type { PollingEvent } from '../../services/api-types'
import { getTypedEventData } from '../../generated/event-types'

export const PRESENTATION_UPDATED_EVENT_TYPE = 'presentation_updated'

export type PresentationUpdatedEvent = {
  presentationId: string
  kind: string
  title: string
  workspacePath: string
	payload: Record<string, unknown>
	activity: {
		label: string
		destination: string
		detail: string
	}
}

function asString(value: string | undefined): string {
  return typeof value === 'string' ? value : ''
}

// PresentationUpdatedEvent is a real registered event type (cmd/schema-gen ->
// EventDataUnion/EventRegistry/UnifiedEvent -> generated/events-bridge.ts),
// backed by pkg/orchestrator/events.PresentationUpdatedEvent on the Go side.
// getTypedEventData narrows PollingEvent to it and reads event.data.data at
// the depth every other typed event uses -- no defensive unwrapping, because
// the depth is no longer a guess: it is what a real EventData value
// serializes to, confirmed against a live emission.
//
// An earlier version of this file used an ad hoc map[string]interface{} on
// the Go side and walked event.data up to four levels deep here to find it,
// because that path wrapped one level deeper (GenericEventData's own "data"
// field) than a typed event does. Registering a real type removed the guess
// instead of hardening it.
export function parsePresentationUpdatedEvent(event: PollingEvent): PresentationUpdatedEvent | null {
  const data = getTypedEventData(event, PRESENTATION_UPDATED_EVENT_TYPE)
  if (!data || !data.presentation_id || !data.kind) return null
  return {
    presentationId: data.presentation_id,
    kind: data.kind,
    title: asString(data.title),
		workspacePath: asString(data.workspace_path),
		payload: (data.payload as Record<string, unknown> | undefined) ?? {},
		activity: {
			label: asString(data.activity?.label),
			destination: asString(data.activity?.destination),
			detail: asString(data.activity?.detail),
		},
  }
}

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

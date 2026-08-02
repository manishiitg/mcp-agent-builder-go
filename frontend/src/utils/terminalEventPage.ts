import type { PollingEvent } from '../services/api-types'

function eventKey(event: PollingEvent, index: number): string {
  if (event.id) return `id:${event.id}`
  if (typeof event.sequence === 'number' && event.sequence > 0) return `sequence:${event.sequence}`
  return `fallback:${event.type || ''}:${event.timestamp || ''}:${index}`
}

function compareTerminalEvents(a: PollingEvent, b: PollingEvent): number {
  const aSequence = typeof a.sequence === 'number' ? a.sequence : 0
  const bSequence = typeof b.sequence === 'number' ? b.sequence : 0
  if (aSequence > 0 && bSequence > 0 && aSequence !== bSequence) return aSequence - bSequence

  const aTime = Date.parse(a.timestamp || '')
  const bTime = Date.parse(b.timestamp || '')
  if (Number.isFinite(aTime) && Number.isFinite(bTime) && aTime !== bTime) return aTime - bTime
  return aSequence - bSequence
}

// Cursor pages can overlap when a live event lands between requests. Merge by
// durable identity and keep chronological order so prepend and append use the
// same code path.
export function mergeTerminalEventPages(
  current: PollingEvent[],
  incoming: PollingEvent[],
): PollingEvent[] {
  const merged = new Map<string, PollingEvent>()
  current.forEach((event, index) => merged.set(eventKey(event, index), event))
  incoming.forEach((event, index) => merged.set(eventKey(event, current.length + index), event))
  return Array.from(merged.values()).sort(compareTerminalEvents)
}

export function terminalEventSequenceBounds(events: PollingEvent[]): {
  oldestSequence?: number
  latestSequence?: number
} {
  let oldestSequence: number | undefined
  let latestSequence: number | undefined
  for (const event of events) {
    if (typeof event.sequence !== 'number' || event.sequence <= 0) continue
    oldestSequence = oldestSequence === undefined ? event.sequence : Math.min(oldestSequence, event.sequence)
    latestSequence = latestSequence === undefined ? event.sequence : Math.max(latestSequence, event.sequence)
  }
  return { oldestSequence, latestSequence }
}

interface LoadedTerminalEventWindow {
  events: PollingEvent[]
  hasOlder: boolean
  oldestSequence?: number
  latestSequence?: number
}

interface NewerTerminalEventPage {
  events?: PollingEvent[]
  has_older: boolean
}

// An after_sequence response describes its position in the server's complete
// transcript, so has_older is normally true whenever any transcript preceded
// the incremental page. It does not mean the client has unseen older history.
// Only the initial/latest request and before_sequence requests may change that
// client-side pagination fact.
export function mergeNewerTerminalEventPage(
  current: LoadedTerminalEventWindow,
  incoming: NewerTerminalEventPage,
): LoadedTerminalEventWindow {
  const events = mergeTerminalEventPages(current.events, incoming.events || [])
  const bounds = terminalEventSequenceBounds(events)
  return {
    events,
    hasOlder: current.hasOlder,
    oldestSequence: bounds.oldestSequence ?? current.oldestSequence,
    latestSequence: bounds.latestSequence ?? current.latestSequence,
  }
}

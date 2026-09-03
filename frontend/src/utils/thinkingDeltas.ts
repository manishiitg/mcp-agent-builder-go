import type { PollingEvent } from '../services/api-types'
import { appendStreamingText } from './streamingStatus'

// Token-streaming coding agents (Cursor) emit one conversation_thinking event
// per fragment of a thinking span, flagged is_delta by the backend the same
// way StreamingChunkEvent fragments are. Rendering each fragment as its own
// event produced a column of one-line "Thinking" cards ("Reading soul.md
// first." / "Determining the current" / "phase from plan.json"). Fold a delta
// into the thinking event immediately before it when both belong to the same
// turn; whole-block providers (Claude Code) never set the flag and are left
// untouched. Idempotent: re-running over an already folded list is a no-op.

type Envelope = Record<string, unknown>

function asRecord(value: unknown): Envelope | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Envelope) : undefined
}

// The typed payload lives either directly on event.data or one level down
// (event.data.data) depending on the envelope the transport used; return the
// record that actually carries `thinking` so the fold writes back in place.
function thinkingPayload(event: PollingEvent): Envelope | undefined {
  const envelope = asRecord(event.data)
  if (!envelope) return undefined
  const inner = asRecord(envelope.data)
  if (inner && typeof inner.thinking === 'string') return inner
  if (typeof envelope.thinking === 'string') return envelope
  return inner ?? envelope
}

function isThinkingEvent(event: PollingEvent | undefined): event is PollingEvent {
  return event?.type === 'conversation_thinking'
}

export function thinkingEventIsDelta(event: PollingEvent): boolean {
  return thinkingPayload(event)?.is_delta === true
}

function thinkingTurn(event: PollingEvent): unknown {
  return thinkingPayload(event)?.turn
}

function withAppendedThinking(head: PollingEvent, fragment: string): PollingEvent {
  const envelope = asRecord(head.data)
  if (!envelope) return head
  const inner = asRecord(envelope.data)
  if (inner && typeof inner.thinking === 'string') {
    return {
      ...head,
      data: { ...envelope, data: { ...inner, thinking: appendStreamingText(inner.thinking, fragment, true) } },
    } as PollingEvent
  }
  const current = typeof envelope.thinking === 'string' ? envelope.thinking : ''
  return { ...head, data: { ...envelope, thinking: appendStreamingText(current, fragment, true) } } as PollingEvent
}

export function coalesceThinkingDeltas(events: PollingEvent[]): PollingEvent[] {
  if (events.length < 2) return events
  let out: PollingEvent[] | null = null
  for (let i = 0; i < events.length; i++) {
    const event = events[i]
    const previous = out ? out[out.length - 1] : events[i - 1]
    if (
      i > 0 &&
      isThinkingEvent(event) &&
      thinkingEventIsDelta(event) &&
      isThinkingEvent(previous) &&
      thinkingTurn(previous) === thinkingTurn(event)
    ) {
      if (!out) out = events.slice(0, i)
      const fragment = thinkingPayload(event)?.thinking
      out[out.length - 1] = withAppendedThinking(previous, typeof fragment === 'string' ? fragment : '')
      continue
    }
    if (out) out.push(event)
  }
  return out ?? events
}

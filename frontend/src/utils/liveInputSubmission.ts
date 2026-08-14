import { createRequestCoalescer } from '../services/requestCoalescer'

export type LiveInputSubmissionCoordinator = <T>(
  sessionKey: string,
  message: string,
  submit: () => Promise<T>,
) => Promise<T>

// A rapid Enter/double-click can invoke ChatInput twice before the first
// /live-input response clears the draft. Share the complete submission promise
// for that exact session + message so the HTTP mutation, optimistic event, and
// fallback decision all execute once. A later intentional repeat is allowed
// after the first submission settles.
export function createLiveInputSubmissionCoordinator(): LiveInputSubmissionCoordinator {
  const coalesce = createRequestCoalescer()
  return <T>(sessionKey: string, message: string, submit: () => Promise<T>) => {
    const key = JSON.stringify([sessionKey, message.trim()])
    return coalesce(key, submit)
  }
}

export const liveInputSubmissionCoordinator = createLiveInputSubmissionCoordinator()

// The live-input endpoint records the durable user_message before returning its
// acknowledgement. SSE can therefore deliver that event while the submitter is
// still awaiting HTTP. Only add the optimistic copy when the acknowledged
// message ID has not already reached the local event store. Matching by the
// backend ID preserves intentional repeated messages with identical text.
export function shouldAppendOptimisticLiveInputMessage(
  events: unknown[],
  acknowledgedMessageId: string | undefined,
): boolean {
  const wanted = acknowledgedMessageId?.trim()
  if (!wanted) return true

  return !events.some(eventValue => {
    if (!eventValue || typeof eventValue !== 'object') return false
    const event = eventValue as Record<string, unknown>
    if (event.type !== 'user_message') return false
    const outer = event.data && typeof event.data === 'object'
      ? event.data as Record<string, unknown>
      : undefined
    const payload = outer?.data && typeof outer.data === 'object'
      ? outer.data as Record<string, unknown>
      : outer
    const metadata = payload?.metadata && typeof payload.metadata === 'object'
      ? payload.metadata as Record<string, unknown>
      : undefined
    return metadata?.message_id === wanted
  })
}

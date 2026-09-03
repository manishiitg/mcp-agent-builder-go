// Rebuilds a conversation's event stream from the persisted chat history
// (`GET /api/chat-history/sessions/{id}`), which is the durable record: the
// live event store is in memory and empty after a server restart. Moved here
// from AgentWorks' utils/sessionRestore.ts so every product surface restores a
// chat with the same rules (coding-CLI narration kept, transcript artifacts
// dropped, a bounded UI trace merged without duplicating user/final carriers).
import type { ChatHistoryMessage, PollingEvent, RestorableConversation } from './types'
import { isProviderTranscriptArtifact } from './transcript/restoredConversationFilter'

export function getMessageRole(message: ChatHistoryMessage): string {
  return String(message.Role || message.role || '').toLowerCase()
}

export function getMessageText(message: ChatHistoryMessage): string {
  const parts = message.Parts || message.parts || []
  const texts = parts
    .map(part => {
      if (!part || typeof part !== 'object') return ''
      return part.Text || part.text || part.Content || part.content || ''
    })
    .filter(text => typeof text === 'string' && text.trim().length > 0)
  return texts.join('\n\n')
}

export function makeRestoredEvent(
  sessionId: string,
  type: string,
  data: Record<string, unknown>,
  index: number,
): PollingEvent {
  const timestamp = typeof data.timestamp === 'string' ? data.timestamp : new Date().toISOString()
  return {
    id: `restored-${sessionId}-${index}-${type}`,
    type,
    timestamp,
    session_id: sessionId,
    event_index: index,
    data: {
      type,
      timestamp,
      session_id: sessionId,
      data: {
        timestamp,
        session_id: sessionId,
        ...data,
      },
    },
  } as PollingEvent
}

export function conversationToRestoredEvents(conversation: RestorableConversation): PollingEvent[] {
  const sessionId = conversation.session_id
  const messages = conversation.conversation_history || []
  const traceTimes = (conversation.ui_events || [])
    .map(event => Date.parse(event.timestamp || ''))
    .filter((timestamp): timestamp is number => Number.isFinite(timestamp))
  const traceStart = traceTimes.length > 0 ? Math.min(...traceTimes) : undefined
  const traceEnd = traceTimes.length > 0 ? Math.max(...traceTimes) : undefined
  const sourceMessageCount = conversation.history_source_message_count || Math.max(
    messages.length,
    ...messages.map(message => Number(message.resume_source_message_count) || 0),
  )
  // Conversation history does not carry provider timestamps, while its saved
  // UI trace does. The source order lets a restored user/update/final message
  // remain in the right place among tool calls instead of appearing after the
  // whole trace just because it was rebuilt at restore time.
  const restoredMessageTimestamp = (message: ChatHistoryMessage): string | undefined => {
    const order = Number(message.resume_order)
    if (!Number.isFinite(order) || traceStart === undefined || traceEnd === undefined || sourceMessageCount <= 0) {
      return undefined
    }
    const position = Math.max(0, Math.min(1, (order + 1) / (sourceMessageCount + 1)))
    return new Date(traceStart + ((traceEnd - traceStart) * position)).toISOString()
  }
  // Page identity is durable across "Load earlier" requests. Without this,
  // every page restarts at restored-…-0 and the event store/UI deduplicates
  // distinct older turns as if they were the newest page.
  const eventIndexBase = Math.max(0, conversation.history_pagination?.start_turn ?? 0) * 2
  const events: PollingEvent[] = [
    makeRestoredEvent(sessionId, 'conversation_resumed', {
      previous_event_count: messages.length,
      has_more_history: conversation.history_pagination?.has_more === true,
      restored_from: 'workspace_chat_history',
    }, eventIndexBase),
  ]

  let turn = 0
  let currentQuestion = ''
  let pendingAssistant: Array<{ content: string; message: ChatHistoryMessage }> = []
  const flushAssistant = () => {
    if (pendingAssistant.length === 0) return
    // A coding CLI can persist several ordinary assistant messages within one
    // user turn (progress, a finding, then the final reply). Restoring only
    // the last one made a 183-message chat appear almost empty. Preserve every
    // readable update; only the last gets the completion carrier that settles
    // the turn, so the transcript still has exactly one final response.
    pendingAssistant.forEach(({ content, message }, index) => {
      const final = index === pendingAssistant.length - 1
      const timestamp = restoredMessageTimestamp(message)
      events.push(makeRestoredEvent(sessionId, 'llm_generation_end', {
        status: 'completed',
        question: currentQuestion,
        content,
        result: content,
        turns: turn,
        restored_intermediate_update: !final,
        ...(timestamp ? { timestamp } : {}),
      }, eventIndexBase + events.length))
      if (final) {
        events.push(makeRestoredEvent(sessionId, 'unified_completion', {
          status: 'completed',
          question: currentQuestion,
          final_result: content,
          result: content,
          turns: turn,
          ...(timestamp ? { timestamp } : {}),
        }, eventIndexBase + events.length))
      }
    })
    pendingAssistant = []
  }

  for (const message of messages) {
    const role = getMessageRole(message)
    if (role === 'system' || role === 'tool') continue

    const content = getMessageText(message)
    if (!content) continue

    if (role === 'human' || role === 'user') {
      flushAssistant()
      turn += 1
      currentQuestion = content
      const timestamp = restoredMessageTimestamp(message)
      events.push(makeRestoredEvent(sessionId, 'user_message', {
        content,
        role: 'user',
        turn,
        ...(timestamp ? { timestamp } : {}),
      }, eventIndexBase + events.length))
    } else if (role === 'ai' || role === 'assistant') {
      if (isProviderTranscriptArtifact(content)) continue
      // Coding providers persist commentary and tool markers as separate AI
      // messages. The final ordinary AI message before the next user message
      // is the completed reply that belongs in the resumed chat.
      pendingAssistant.push({ content, message })
    }
  }
  flushAssistant()

  // A scheduled run has two durable representations:
  //
  // - conversation_history is the complete parent conversation, with a real
  //   page cursor for older turns;
  // - ui_events is a bounded, displayable trace containing tool calls and
  //   child-agent activity.
  //
  // The latter used to replace the former. That meant navigating to a running
  // workflow from Global Monitor could show only the small retained trace even
  // though its full parent conversation was on disk. Keep the conversation as
  // the transcript backbone and append only trace records that do not duplicate
  // a persisted user/final-answer carrier.
  if (conversation.ui_events && conversation.ui_events.length > 0) {
    return mergePersistedUIEvents(
      events,
      conversation.ui_events as PollingEvent[],
      sessionId,
      eventIndexBase + events.length,
    )
  }

  return events
}

function restoredEventText(event: PollingEvent): string {
  const outer = event.data && typeof event.data === 'object' ? event.data as Record<string, unknown> : {}
  const nested = outer.data && typeof outer.data === 'object' ? outer.data as Record<string, unknown> : outer
  for (const field of ['content', 'final_result', 'result']) {
    const value = nested[field]
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

function transcriptCarrierKey(event: PollingEvent): string | undefined {
  const type = event.type || ''
  const content = restoredEventText(event).replace(/\s+/g, ' ').trim().toLowerCase()
  if (!content) return undefined

  if (type === 'user_message') return `user:${content}`
  // The transport can retain a final reply as either a generation-end event
  // or a unified completion. Durable chat history synthesizes both carriers,
  // while a just-completed live session can return either one. They describe
  // the same reader-visible reply, so their identity is the answer itself,
  // not the protocol event type.
  if (type === 'llm_generation_end' || type === 'unified_completion') {
    return `assistant:${content}`
  }
  return undefined
}

export function filterDuplicateTranscriptEvents(
  existingEvents: PollingEvent[],
  incomingEvents: PollingEvent[],
): PollingEvent[] {
  const knownIDs = new Set(existingEvents.map(event => event.id).filter(Boolean))
  const knownCarriers = new Set(existingEvents
    .map(transcriptCarrierKey)
    .filter((key): key is string => !!key))

  return incomingEvents.filter((event) => {
    if (event.id && knownIDs.has(event.id)) return false
    const carrier = transcriptCarrierKey(event)
    if (carrier && knownCarriers.has(carrier)) return false
    if (event.id) knownIDs.add(event.id)
    if (carrier) knownCarriers.add(carrier)
    return true
  })
}

function mergePersistedUIEvents(
  conversationEvents: PollingEvent[],
  persistedUIEvents: PollingEvent[],
  sessionId: string,
  eventIndexBase: number,
): PollingEvent[] {
  const knownIDs = new Set(conversationEvents.map(event => event.id).filter(Boolean))
  const knownCarriers = new Set(conversationEvents
    .map(transcriptCarrierKey)
    .filter((key): key is string => !!key))

  const trace = persistedUIEvents
    .map((event, index) => markPersistedRestoreTrace(event, sessionId, eventIndexBase + index + 1))
    .filter((event) => {
      if (event.id && knownIDs.has(event.id)) return false
      const carrier = transcriptCarrierKey(event)
      if (carrier && knownCarriers.has(carrier)) return false
      if (event.id) knownIDs.add(event.id)
      if (carrier) knownCarriers.add(carrier)
      return true
    })

  return [...conversationEvents, ...trace]
}

function markPersistedRestoreTrace(event: PollingEvent, parentSessionId: string, eventIndex: number): PollingEvent {
  const outer = event.data && typeof event.data === 'object' ? event.data as Record<string, unknown> : {}
  const nested = outer.data && typeof outer.data === 'object' ? outer.data as Record<string, unknown> : {}
  const metadata = nested.metadata && typeof nested.metadata === 'object' ? nested.metadata as Record<string, unknown> : {}
  return {
    ...event,
    // Persisted UI events are recorded under the parent session even when the
    // nested event belongs to a background child. Preserve that ownership so
    // one restored schedule remains one tab/timeline.
    session_id: event.session_id || parentSessionId,
    event_index: typeof event.event_index === 'number' ? event.event_index : eventIndex,
    data: {
      ...outer,
      data: {
        ...nested,
        metadata: { ...metadata, restored_persisted_trace: true },
      },
    },
  } as PollingEvent
}

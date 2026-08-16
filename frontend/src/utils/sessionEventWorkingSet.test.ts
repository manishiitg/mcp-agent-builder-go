import { describe, expect, it } from 'vitest'
import type { PollingEvent } from '../services/api-types'
import {
  eventBelongsToSession,
  retainEventInSessionWorkingSet,
  sessionOwnsGlobalChatIndicators,
} from './sessionEventWorkingSet'

const sessionId = 'session-1'

function event(type: string, terminalId?: string, ownerId?: string): PollingEvent {
  return {
    id: `${type}-${terminalId || 'legacy'}`,
    type,
    timestamp: '2026-08-02T00:00:00Z',
    data: {},
    terminal_id: terminalId,
    terminal_owner_id: ownerId,
  } as PollingEvent
}

function ownedEvent(type: string, owningSessionId: string): PollingEvent {
  return { ...event(type), id: `${type}-${owningSessionId}`, session_id: owningSessionId } as PollingEvent
}

describe('retainEventInSessionWorkingSet', () => {
  it('retains legacy events that do not have canonical ownership', () => {
    expect(retainEventInSessionWorkingSet(sessionId, event('tool_call_end'))).toBe(true)
  })

  it('retains detailed main-agent events', () => {
    const main = event('tool_call_end', `${sessionId}:main:${sessionId}`, `main:${sessionId}`)
    expect(retainEventInSessionWorkingSet(sessionId, main)).toBe(true)
  })

  it.each(['tool_call_start', 'tool_call_end', 'llm_generation_end', 'streaming_chunk', 'user_message'])(
    'does not retain child %s transcript details',
    type => {
      const child = event(type, `${sessionId}:background:reviewer`, 'background:reviewer')
      expect(retainEventInSessionWorkingSet(sessionId, child)).toBe(false)
    },
  )

  it.each(['background_agent_started', 'background_agent_completed', 'workflow_step_started'])(
    'retains child %s control events',
    type => {
      const child = event(type, `${sessionId}:background:reviewer`, 'background:reviewer')
      expect(retainEventInSessionWorkingSet(sessionId, child)).toBe(true)
    },
  )
})

// PLAT-106: a Schedule event was rendered under the Chat tab. Ownership is a
// hard boundary and must be enforced before any volume/type classification.
describe('session ownership boundary', () => {
  const scheduleSession = 'schedule-cron--51af4f19_1786764627816018000'

  it('rejects an event owned by a different session', () => {
    expect(eventBelongsToSession(sessionId, ownedEvent('unified_completion', scheduleSession))).toBe(false)
    expect(retainEventInSessionWorkingSet(sessionId, ownedEvent('unified_completion', scheduleSession))).toBe(false)
  })

  it('accepts an event owned by this session', () => {
    expect(retainEventInSessionWorkingSet(sessionId, ownedEvent('unified_completion', sessionId))).toBe(true)
  })

  // These types are absent from CHILD_TRANSCRIPT_DETAIL_EVENT_TYPES, so before
  // the ownership check they passed straight into the wrong session's bucket —
  // which is exactly how a finished Schedule answer surfaced inside Chat.
  it.each(['unified_completion', 'agent_end', 'conversation_end'])(
    'rejects foreign %s even though its type is not volume-filtered',
    type => {
      expect(retainEventInSessionWorkingSet(sessionId, ownedEvent(type, scheduleSession))).toBe(false)
    },
  )

  it('still accepts events that declare no owning session', () => {
    // Optimistic/local records and legacy events carry no session_id; dropping
    // them would silently discard the user's own messages.
    expect(retainEventInSessionWorkingSet(sessionId, event('user_message'))).toBe(true)
    expect(eventBelongsToSession(sessionId, event('unified_completion'))).toBe(true)
  })

  it('does not fail open for a foreign event that has no terminal identity', () => {
    const foreign = ownedEvent('unified_completion', scheduleSession)
    expect(foreign.terminal_id).toBeUndefined()
    expect(retainEventInSessionWorkingSet(sessionId, foreign)).toBe(false)
  })
})

// The app-wide isStreaming/isCompleted/hasActiveChat fields are globals, but
// event responses are processed for every polled session. Without an ownership
// check a background session overwrote the foreground indicator each poll and
// the composer alternated between "working" and idle.
describe('global chat indicator ownership', () => {
  const foreground = 'chat-visible'
  const background = 'schedule-cron--background'

  it('lets the session the user is viewing drive the global indicators', () => {
    expect(sessionOwnsGlobalChatIndicators(foreground, foreground)).toBe(true)
  })

  it('refuses a background session that is not on screen', () => {
    expect(sessionOwnsGlobalChatIndicators(background, foreground)).toBe(false)
  })

  it('refuses a response with no owning session while one is selected', () => {
    // Ambiguous ownership must not win over a known foreground conversation.
    expect(sessionOwnsGlobalChatIndicators(undefined, foreground)).toBe(false)
    expect(sessionOwnsGlobalChatIndicators('   ', foreground)).toBe(false)
  })

  it('allows any response when no session is selected', () => {
    // Nothing is on screen to protect, and this preserves first-load behaviour
    // before a tab has bound its session.
    expect(sessionOwnsGlobalChatIndicators(background, undefined)).toBe(true)
    expect(sessionOwnsGlobalChatIndicators(undefined, undefined)).toBe(true)
  })

  it('does not flip ownership as concurrent sessions interleave', () => {
    // The reported symptom: alternating polls from two sessions. Only the
    // foreground one may ever write, regardless of arrival order.
    const arrivals = [background, foreground, background, background, foreground]
    const winners = arrivals.filter(id => sessionOwnsGlobalChatIndicators(id, foreground))
    expect(winners).toEqual([foreground, foreground])
  })
})

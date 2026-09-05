import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { PollingEvent } from '../services/api-types'

// One background-step completion is steered into the live coding CLI once, but
// the live transport can surface that same [AUTO-NOTIFICATION] user_message as
// several distinct-ID events within the session window. The event store dedups
// by event.id, so before this fix each distinct ID drew its own duplicate
// "Automation update" row (Dominion, 2026-09-04/05). The store must collapse
// them by content while leaving ordinary repeated messages and distinct
// completions alone.

const SESSION = 'chat-auto-notif'

const createMemoryStorage = (): Storage => {
  const values = new Map<string, string>()
  return {
    get length() { return values.size },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => Array.from(values.keys())[index] ?? null,
    removeItem: (key) => { values.delete(key) },
    setItem: (key, value) => { values.set(key, value) },
  }
}

function autoNotif(id: string, content: string): PollingEvent {
  return {
    id,
    type: 'user_message',
    timestamp: '2026-09-05T09:00:00Z',
    session_id: SESSION,
    data: { type: 'user_message', data: { content } },
  } as PollingEvent
}

function userMsg(id: string, content: string): PollingEvent {
  return {
    id,
    type: 'user_message',
    timestamp: '2026-09-05T09:00:00Z',
    session_id: SESSION,
    data: { type: 'user_message', data: { content } },
  } as PollingEvent
}

async function loadStore() {
  const mod = await import('./useChatStore')
  return mod.useChatStore
}

const COMPLETION = "[AUTO-NOTIFICATION] Agent 'TEST - Alpaca Price Data Validation [default]' completed — status=completed"

describe('auto-notification dedup in the event store', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.stubGlobal('localStorage', createMemoryStorage())
  })
  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('collapses the same steered completion arriving under several distinct IDs', async () => {
    const useChatStore = await loadStore()
    const store = useChatStore.getState()

    store._addTabEventsImmediate(SESSION, [
      autoNotif('steer-message-1788591590', COMPLETION),
      autoNotif('c308ea30_user_message_1788591590196129235_133332', COMPLETION),
      autoNotif('catchup-poll-42', COMPLETION),
      autoNotif('sse-redelivery-7', COMPLETION),
    ])

    const notifRows = useChatStore.getState().getTabEvents(SESSION)
      .filter(e => e.type === 'user_message')
    expect(notifRows).toHaveLength(1)
    expect(notifRows[0].id).toBe('steer-message-1788591590')
  })

  it('collapses across separate add calls too (live batches trickle in)', async () => {
    const useChatStore = await loadStore()
    const store = useChatStore.getState()

    store._addTabEventsImmediate(SESSION, [autoNotif('a', COMPLETION)])
    store._addTabEventsImmediate(SESSION, [autoNotif('b', COMPLETION)])
    store._addTabEventsImmediate(SESSION, [autoNotif('c', COMPLETION)])

    expect(useChatStore.getState().getTabEvents(SESSION)).toHaveLength(1)
  })

  it('keeps distinct completions of the same step (different Result payloads)', async () => {
    const useChatStore = await loadStore()
    const store = useChatStore.getState()

    store._addTabEventsImmediate(SESSION, [
      autoNotif('run1-a', `${COMPLETION}\nResult: {"n": 1}`),
      autoNotif('run1-b', `${COMPLETION}\nResult: {"n": 1}`),
      autoNotif('run2-a', `${COMPLETION}\nResult: {"n": 2}`),
    ])

    expect(useChatStore.getState().getTabEvents(SESSION)).toHaveLength(2)
  })

  it('never collapses ordinary user messages with identical text', async () => {
    const useChatStore = await loadStore()
    const store = useChatStore.getState()

    store._addTabEventsImmediate(SESSION, [
      userMsg('u1', 'run it again'),
      userMsg('u2', 'run it again'),
    ])

    expect(useChatStore.getState().getTabEvents(SESSION)).toHaveLength(2)
  })
})

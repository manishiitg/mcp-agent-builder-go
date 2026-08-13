import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  setTabEvents: vi.fn(),
  setTabLastEventIndex: vi.fn(),
  setTabHasMoreOlderEvents: vi.fn(),
  getRecentSessionEvents: vi.fn(),
  getChatHistoryConversation: vi.fn(),
}))

vi.mock('../stores/useChatStore', () => ({
  useChatStore: {
    getState: () => ({
      setTabEvents: mocks.setTabEvents,
      setTabLastEventIndex: mocks.setTabLastEventIndex,
      setTabHasMoreOlderEvents: mocks.setTabHasMoreOlderEvents,
    }),
  },
}))

vi.mock('../stores/useModeStore', () => ({
  useModeStore: { getState: () => ({ setModeCategory: vi.fn() }) },
}))

vi.mock('../services/api', () => ({
  agentApi: {
    getRecentSessionEvents: mocks.getRecentSessionEvents,
    getChatHistoryConversation: mocks.getChatHistoryConversation,
  },
}))

import { hydrateTabEvents } from './sessionRestore'

describe('hydrateTabEvents restored chat fallback', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('prefers complete persisted history when reopening a chat', async () => {
    const persistedEvent = {
      id: 'persisted-1',
      type: 'conversation_end',
      session_id: 'restored-session',
      timestamp: '2026-08-05T00:00:00Z',
      data: {},
    }
    mocks.getRecentSessionEvents.mockResolvedValue({
      events: [{ ...persistedEvent, id: 'volatile-tail-only' }],
      session_status: 'completed',
      last_processed_index: -1,
      has_more: false,
    })
    mocks.getChatHistoryConversation.mockResolvedValue({
      session_id: 'restored-session',
      conversation_history: [],
      ui_events: [persistedEvent],
    })

    await hydrateTabEvents('restored-session', {
      workspacePath: '/workspace/workflow',
      fallbackToChatHistory: true,
      preferChatHistory: true,
    })

    expect(mocks.getChatHistoryConversation).toHaveBeenCalledWith(
      'restored-session',
      '/workspace/workflow',
    )
    expect(mocks.setTabEvents).toHaveBeenCalledWith('restored-session', [persistedEvent])
    expect(mocks.setTabLastEventIndex).toHaveBeenCalledWith('restored-session', 0)
    expect(mocks.setTabHasMoreOlderEvents).toHaveBeenCalledWith('restored-session', false)
    expect(mocks.getRecentSessionEvents).not.toHaveBeenCalled()
  })
})

import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  setTabEvents: vi.fn(),
  setTabLastEventIndex: vi.fn(),
  setTabHasMoreOlderEvents: vi.fn(),
  setTabHistoryPagination: vi.fn(),
  getRecentSessionEvents: vi.fn(),
  getChatHistoryConversation: vi.fn(),
  getChatHistoryResumeConversation: vi.fn(),
}))

vi.mock('../stores/useChatStore', () => ({
  useChatStore: {
    getState: () => ({
      setTabEvents: mocks.setTabEvents,
      setTabLastEventIndex: mocks.setTabLastEventIndex,
      setTabHasMoreOlderEvents: mocks.setTabHasMoreOlderEvents,
      setTabHistoryPagination: mocks.setTabHistoryPagination,
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
    getChatHistoryResumeConversation: mocks.getChatHistoryResumeConversation,
  },
}))

import { hydrateTabEvents } from './sessionRestore'

describe('hydrateTabEvents restored chat fallback', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('prefers complete persisted history when reopening a chat', async () => {
    mocks.getRecentSessionEvents.mockResolvedValue({
      events: [{ id: 'volatile-tail-only', type: 'conversation_end' }],
      session_status: 'completed',
      last_processed_index: -1,
      has_more: false,
    })
    mocks.getChatHistoryResumeConversation.mockResolvedValue({
      session_id: 'restored-session',
      conversation_history: [
        { Role: 'human', Parts: [{ Text: 'Hello' }] },
        { Role: 'ai', Parts: [{ Text: 'I will inspect that.' }] },
        { Role: 'ai', Parts: [{ Text: '[Previous tool call: exec({})]' }] },
        { Role: 'ai', Parts: [{ Text: 'Hi there' }] },
      ],
    })

    await hydrateTabEvents('restored-session', {
      workspacePath: '/workspace/workflow',
      fallbackToChatHistory: true,
      preferChatHistory: true,
    })

    expect(mocks.getChatHistoryResumeConversation).toHaveBeenCalledWith(
      'restored-session',
      '/workspace/workflow',
    )
    expect(mocks.setTabEvents).toHaveBeenCalledWith(
      'restored-session',
      expect.arrayContaining([
        expect.objectContaining({ type: 'user_message' }),
        expect.objectContaining({
          type: 'llm_generation_end',
          data: expect.objectContaining({
            data: expect.objectContaining({ content: 'Hi there', result: 'Hi there' }),
          }),
        }),
        expect.objectContaining({
          type: 'unified_completion',
          data: expect.objectContaining({
            data: expect.objectContaining({ final_result: 'Hi there', result: 'Hi there' }),
          }),
        }),
      ]),
    )
    expect(mocks.setTabLastEventIndex).toHaveBeenCalledWith('restored-session', 3)
    expect(mocks.setTabHasMoreOlderEvents).toHaveBeenCalledWith('restored-session', false)
    expect(mocks.setTabHistoryPagination).toHaveBeenCalledWith('restored-session', null)
    expect(mocks.getRecentSessionEvents).toHaveBeenCalledWith('restored-session')
  })
})

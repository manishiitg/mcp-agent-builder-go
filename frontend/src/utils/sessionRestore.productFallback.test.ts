import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const getRecentSessionEvents = vi.fn()
const getChatHistoryConversation = vi.fn()

vi.mock('../services/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../services/api')>()
  return {
    ...actual,
    agentApi: {
      ...actual.agentApi,
      getRecentSessionEvents,
      getChatHistoryConversation,
    },
  }
})

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

describe('session restore chat-history fallback', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.stubGlobal('localStorage', createMemoryStorage())
    getRecentSessionEvents.mockReset()
    getChatHistoryConversation.mockReset()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('hydrates a persisted product session when runtime events are empty', async () => {
    getRecentSessionEvents.mockResolvedValue({
      events: [],
      session_status: 'completed',
      has_running_background_agents: false,
      is_synthetic_turn: false,
      can_steer: false,
    })
    getChatHistoryConversation.mockResolvedValue({
      session_id: 'video-studio:project:launch',
      conversation_history: [
        { Role: 'user', Parts: [{ Text: 'Create the launch teaser.' }] },
        { Role: 'assistant', Parts: [{ Text: 'The finished teaser is ready.' }] },
      ],
    })

    const { useChatStore, waitForChatStoreHydration } = await import('../stores/useChatStore')
    const { restoreSession } = await import('./sessionRestore')
    await waitForChatStoreHydration()
    const workspacePath = 'Chats/Video Studio/projects/launch'
    const tabId = await useChatStore.getState().createChatTab('Launch teaser', {
      mode: 'multi-agent',
      agentProfileId: 'video-studio',
      agentProfileVersion: 1,
      agentProfileWorkspace: workspacePath,
    }, 'video-studio:project:launch')

    await expect(restoreSession('video-studio:project:launch', {
      source: 'video-project-open',
    })).resolves.toBe(tabId)

    expect(getChatHistoryConversation).toHaveBeenCalledWith(
      'video-studio:project:launch',
      workspacePath,
    )
    expect(useChatStore.getState().getTabEvents('video-studio:project:launch').map((event) => event.type)).toEqual([
      'conversation_resumed',
      'user_message',
      'conversation_end',
    ])
    expect(useChatStore.getState().chatTabs[tabId]?.isStreaming).toBe(false)
  })
})

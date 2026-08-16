import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const createMemoryStorage = (): Storage => {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => Array.from(values.keys())[index] ?? null,
    removeItem: (key) => {
      values.delete(key)
    },
    setItem: (key, value) => {
      values.set(key, value)
    },
  }
}

describe('useChatStore hydration bootstrap', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.stubGlobal('localStorage', createMemoryStorage())
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('finishes synchronous storage hydration before callers need the backstop', async () => {
    const chatStore = await import('./useChatStore')

    await chatStore.waitForChatStoreHydration()

    expect(chatStore.getChatStoreHydrationSnapshot()).toEqual({
      status: 'hydrated',
      error: null,
    })
  }, 15_000)

  it('does not persist streaming chunks or an obsolete tree preference', async () => {
    vi.useFakeTimers()
    const storage = createMemoryStorage()
    const setItem = vi.spyOn(storage, 'setItem')
    vi.stubGlobal('localStorage', storage)
    const chatStore = await import('./useChatStore')
    await chatStore.waitForChatStoreHydration()
    expect(chatStore.normalizeEventViewMode('tree')).toBe('formatted')
    chatStore.useChatStore.setState({ eventViewModePreference: 'formatted' })
    vi.advanceTimersByTime(250)
    setItem.mockClear()

    for (let index = 1; index <= 1_000; index += 1) {
      chatStore.useChatStore.getState().appendStreamingChunk('session-1', index, `chunk-${index}`)
    }
    vi.advanceTimersByTime(250)
    expect(setItem).not.toHaveBeenCalled()

    chatStore.useChatStore.setState({ eventViewModePreference: 'tree' })
    chatStore.useChatStore.setState({ eventViewModePreference: 'formatted' })
    chatStore.useChatStore.setState({ eventViewModePreference: 'tree' })
    vi.advanceTimersByTime(250)

    expect(setItem).not.toHaveBeenCalled()
  })

  it('coalesces repeated workflow switches into one durable write', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-13T00:00:00Z'))
    const storage = createMemoryStorage()
    const setItem = vi.spyOn(storage, 'setItem')
    vi.stubGlobal('localStorage', storage)
    const chatStore = await import('./useChatStore')
    await chatStore.waitForChatStoreHydration()

    const firstTab = await chatStore.useChatStore.getState().createChatTab('First workflow', {
      mode: 'workflow',
      phaseId: 'workflow-builder',
      presetQueryId: 'workflow-one',
    })
    vi.setSystemTime(new Date('2026-07-13T00:00:00.001Z'))
    const secondTab = await chatStore.useChatStore.getState().createChatTab('Second workflow', {
      mode: 'workflow',
      phaseId: 'workflow-builder',
      presetQueryId: 'workflow-two',
    })
    vi.advanceTimersByTime(250)
    setItem.mockClear()

    for (let index = 0; index < 100; index += 1) {
      chatStore.useChatStore.getState().switchTab(index % 2 === 0 ? firstTab : secondTab)
    }
    vi.advanceTimersByTime(250)

    expect(setItem).toHaveBeenCalledTimes(1)
    expect(chatStore.useChatStore.getState().activeTabId).toBe(secondTab)
  })

  it('keeps the Chief of Staff chat independent from a running schedule tab', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-04T09:30:00Z'))
    const chatStore = await import('./useChatStore')
    await chatStore.waitForChatStoreHydration()

    const scheduleTabId = await chatStore.useChatStore.getState().createChatTab('Schedule', {
      mode: 'multi-agent',
      isViewOnly: true,
      isScheduledRun: true,
    })
    const interactiveTabId = await chatStore.useChatStore.getState().createChatTab('Chief of Staff', {
      mode: 'multi-agent',
    })
    const reusedInteractiveTabId = await chatStore.useChatStore.getState().createChatTab('Chief of Staff', {
      mode: 'multi-agent',
    })

    expect(interactiveTabId).not.toBe(scheduleTabId)
    expect(reusedInteractiveTabId).toBe(interactiveTabId)
    expect(Object.keys(chatStore.useChatStore.getState().chatTabs)).toHaveLength(2)
  })

  it('keeps Chief of Staff and product-profile project sessions in separate lanes', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-07T10:00:00Z'))
    const chatStore = await import('./useChatStore')
    await chatStore.waitForChatStoreHydration()

    const chiefTabId = await chatStore.useChatStore.getState().createChatTab('Chief of Staff', { mode: 'multi-agent' })
    const firstProjectTabId = await chatStore.useChatStore.getState().createChatTab('Launch film', {
      mode: 'multi-agent',
      agentProfileId: 'video-studio',
      agentProfileVersion: 1,
      agentProfileWorkspace: 'Chats/Video Studio/projects/launch-film',
    }, 'video-studio:project:launch-film')
    const reusedProjectTabId = await chatStore.useChatStore.getState().createChatTab('Launch film', {
      mode: 'multi-agent',
      agentProfileId: 'video-studio',
      agentProfileVersion: 1,
      agentProfileWorkspace: 'Chats/Video Studio/projects/launch-film',
    })
    const secondProjectTabId = await chatStore.useChatStore.getState().createChatTab('Customer story', {
      mode: 'multi-agent',
      agentProfileId: 'video-studio',
      agentProfileVersion: 1,
      agentProfileWorkspace: 'Chats/Video Studio/projects/customer-story',
    }, 'video-studio:project:customer-story')

    expect(firstProjectTabId).not.toBe(chiefTabId)
    expect(reusedProjectTabId).toBe(firstProjectTabId)
    expect(secondProjectTabId).not.toBe(firstProjectTabId)
    expect(Object.keys(chatStore.useChatStore.getState().chatTabs)).toHaveLength(3)
  })

  it('restores the existing persisted chat-store envelope', async () => {
    const storage = createMemoryStorage()
    const createdAt = Date.now()
    storage.setItem('chat-store', JSON.stringify({
      state: {
        chatTabs: {
          'legacy-tab': {
            tabId: 'legacy-tab',
            name: 'Existing workflow chat',
            sessionId: 'legacy-session',
            isStreaming: false,
            isCompleted: false,
            hasRunningBgAgents: false,
            isSyntheticTurn: false,
            canSteer: false,
            hideToolCalls: true,
            viewMode: 'terminal',
            config: {
              inputText: '',
              useCodeExecutionMode: true,
              selectedServers: [],
              selectedSkills: [],
              selectedSecrets: [],
              llmConfig: { provider: 'codex-cli', model_id: 'gpt-5.6-sol' },
              fileContext: [],
              browserMode: 'none',
              workflowContext: [],
              queuedMessages: [],
            },
            createdAt,
            lastAccessedAt: createdAt,
            lastViewedEventCount: 0,
            lastViewedEventCounts: { micro: 0 },
            metadata: { mode: 'workflow', presetQueryId: 'existing-workflow' },
          },
        },
        activeTabId: 'legacy-tab',
        eventViewModePreference: 'terminal',
      },
      version: 0,
    }))
    vi.stubGlobal('localStorage', storage)

    const chatStore = await import('./useChatStore')
    await chatStore.waitForChatStoreHydration()

    expect(chatStore.useChatStore.getState().activeTabId).toBe('legacy-tab')
    expect(chatStore.useChatStore.getState().chatTabs['legacy-tab']).toMatchObject({
      name: 'Existing workflow chat',
      sessionId: 'legacy-session',
      metadata: { mode: 'workflow', presetQueryId: 'existing-workflow' },
    })
  })
})

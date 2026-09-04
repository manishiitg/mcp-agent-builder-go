import { describe, expect, it, vi } from 'vitest'
import type { ChatTab } from '../stores/useChatStore'
import { resolveWorkflowTabForSession, reusableScheduleTabId, scheduleLaneKey } from './workflowTabResolution'

describe('scheduleLaneKey', () => {
  it('is the schedule segment of a scheduler-minted session id', () => {
    expect(scheduleLaneKey('schedule-cron--42eca39a_1788453558390126411')).toBe('42eca39a')
  })

  it('is the same for a cron fire and a "Run now" of the same schedule', () => {
    expect(scheduleLaneKey('schedule-cron--42eca39a_100')).toBe(scheduleLaneKey('schedule-manual--42eca39a_200'))
  })

  it('differs between two schedules of one workflow', () => {
    expect(scheduleLaneKey('schedule-cron--42eca39a_100')).not.toBe(scheduleLaneKey('schedule-cron--86683998_100'))
  })

  it("gives the toolbar's one-off Pulse its own lane", () => {
    expect(scheduleLaneKey('schedule-manual--manual-p_100')).toBe('manual-p')
  })

  it('keeps an underscore inside the schedule id and still strips the timestamp', () => {
    expect(scheduleLaneKey('schedule-cron--ab_cd_100')).toBe('ab_cd')
  })

  it('is null for anything the scheduler did not mint', () => {
    expect(scheduleLaneKey('fae1da55-e1e0-4b27-81e7-54d3424760da')).toBeNull()
    expect(scheduleLaneKey('bot-whatsapp-1')).toBeNull()
    expect(scheduleLaneKey('')).toBeNull()
    expect(scheduleLaneKey(null)).toBeNull()
  })
})

describe('reusableScheduleTabId', () => {
  const finishedDailyTab = {
    tabId: 'daily-tab',
    sessionId: 'schedule-cron--daily123_1',
    isStreaming: false,
    metadata: { mode: 'workflow' as const, isScheduledRun: true, isViewOnly: true, presetQueryId: 'workflow-upwork' },
  }

  it('reuses the finished tab of the same schedule instead of opening another per run', () => {
    expect(reusableScheduleTabId({ a: finishedDailyTab }, 'workflow-upwork', 'schedule-cron--daily123_2'))
      .toBe('daily-tab')
  })

  it('reuses it across triggers -- a "Run now" continues the cron lane', () => {
    expect(reusableScheduleTabId({ a: finishedDailyTab }, 'workflow-upwork', 'schedule-manual--daily123_2'))
      .toBe('daily-tab')
  })

  it('never lets a different schedule of the same workflow take the tab', () => {
    expect(reusableScheduleTabId({ a: finishedDailyTab }, 'workflow-upwork', 'schedule-cron--weekly45_2'))
      .toBeNull()
  })

  it("never lets a one-off Pulse take a named schedule's tab (the title-flip bug)", () => {
    expect(reusableScheduleTabId({ a: finishedDailyTab }, 'workflow-upwork', 'schedule-manual--manual-p_2'))
      .toBeNull()
  })

  it('never displaces a live run', () => {
    expect(reusableScheduleTabId(
      { a: { ...finishedDailyTab, isStreaming: true } }, 'workflow-upwork', 'schedule-cron--daily123_2',
    )).toBeNull()
  })

  it('never recycles a tab the user promoted to an interactive chat', () => {
    expect(reusableScheduleTabId(
      { a: { ...finishedDailyTab, metadata: { ...finishedDailyTab.metadata, userInteractiveContinuation: true } } },
      'workflow-upwork', 'schedule-cron--daily123_2',
    )).toBeNull()
  })

  it('never crosses workflows', () => {
    expect(reusableScheduleTabId({ a: finishedDailyTab }, 'workflow-social', 'schedule-cron--daily123_2')).toBeNull()
  })

  it('leaves an ordinary Chat tab alone', () => {
    expect(reusableScheduleTabId(
      { a: { ...finishedDailyTab, metadata: { mode: 'workflow' as const } } },
      'workflow-upwork', 'schedule-cron--daily123_2',
    )).toBeNull()
  })

  it('has nothing to reuse for a session the scheduler did not mint', () => {
    expect(reusableScheduleTabId({ a: finishedDailyTab }, 'workflow-upwork', 'fae1da55')).toBeNull()
  })
})

describe('resolveWorkflowTabForSession', () => {
  const tab = (overrides: Partial<ChatTab>): ChatTab => ({
    tabId: 'tab', name: 'Daily', sessionId: 'schedule-cron--daily123_1',
    isStreaming: false, isCompleted: false, hasRunningBgAgents: false, isSyntheticTurn: false,
    canSteer: false, hideToolCalls: true, viewMode: 'terminal', config: {} as ChatTab['config'],
    createdAt: 1, lastAccessedAt: 1, lastViewedEventCount: 0, lastViewedEventCounts: { micro: 0 },
    metadata: { mode: 'workflow', presetQueryId: 'workflow-upwork', isScheduledRun: true, isViewOnly: true },
    ...overrides,
  })
  const scheduled = { mode: 'workflow' as const, presetQueryId: 'workflow-upwork', isScheduledRun: true, isViewOnly: true }
  const builder = { mode: 'workflow' as const, presetQueryId: 'workflow-upwork', phaseId: 'workflow-builder' }

  const run = (tabs: ChatTab[], sessionId: string, metadata: NonNullable<ChatTab['metadata']>) => {
    const createChatTab = vi.fn(async () => 'new-tab')
    const updateTabSessionId = vi.fn()
    const byId = Object.fromEntries(tabs.map(t => [t.tabId, t]))
    const result = resolveWorkflowTabForSession({
      getTabs: () => byId, presetQueryId: 'workflow-upwork', sessionId, name: 'n', metadata,
      createChatTab, updateTabSessionId,
    })
    return { result, createChatTab, updateTabSessionId }
  }

  it('returns the tab already bound to the session and opens nothing', async () => {
    const { result, createChatTab, updateTabSessionId } = run([tab({})], 'schedule-cron--daily123_1', scheduled)
    expect(await result).toEqual({ tabId: 'tab', via: 'existing' })
    expect(createChatTab).not.toHaveBeenCalled()
    expect(updateTabSessionId).not.toHaveBeenCalled()
  })

  it("rebinds the same schedule's finished tab to the new run instead of opening a tab", async () => {
    const { result, createChatTab, updateTabSessionId } = run([tab({})], 'schedule-cron--daily123_2', scheduled)
    expect(await result).toEqual({ tabId: 'tab', via: 'lane' })
    expect(updateTabSessionId).toHaveBeenCalledWith('tab', 'schedule-cron--daily123_2')
    expect(createChatTab).not.toHaveBeenCalled()
  })

  it("opens a new tab for a Pulse run rather than taking a named schedule's", async () => {
    const { result, createChatTab } = run([tab({})], 'schedule-manual--manual-p_2', scheduled)
    expect(await result).toEqual({ tabId: 'new-tab', via: 'created' })
    expect(createChatTab).toHaveBeenCalledTimes(1)
  })

  it('never rebinds a lane for a Builder chat -- those are user conversations', async () => {
    const { result, updateTabSessionId } = run([tab({})], 'fae1da55', builder)
    expect(await result).toEqual({ tabId: 'new-tab', via: 'created' })
    expect(updateTabSessionId).not.toHaveBeenCalled()
  })

  it('picks the schedule tab, not the Builder chat, when both share a session id', async () => {
    const chat = tab({ tabId: 'chat', metadata: { ...builder } })
    const lane = tab({ tabId: 'lane' })
    const { result } = run([chat, lane], 'schedule-cron--daily123_1', scheduled)
    expect((await result).tabId).toBe('lane')
    const forChat = run([chat, lane], 'schedule-cron--daily123_1', builder)
    expect((await forChat.result).tabId).toBe('chat')
  })

  it('reads tabs live, so a tab created by an earlier await is seen', async () => {
    const byId: Record<string, ChatTab> = {}
    const createChatTab = vi.fn(async () => 'new-tab')
    const first = await resolveWorkflowTabForSession({
      getTabs: () => byId, presetQueryId: 'workflow-upwork', sessionId: 'schedule-cron--daily123_1',
      name: 'n', metadata: scheduled, createChatTab, updateTabSessionId: vi.fn(),
    })
    byId['new-tab'] = tab({ tabId: 'new-tab' })
    const second = await resolveWorkflowTabForSession({
      getTabs: () => byId, presetQueryId: 'workflow-upwork', sessionId: 'schedule-cron--daily123_1',
      name: 'n', metadata: scheduled, createChatTab, updateTabSessionId: vi.fn(),
    })
    expect(first.via).toBe('created')
    expect(second).toEqual({ tabId: 'new-tab', via: 'existing' })
    expect(createChatTab).toHaveBeenCalledTimes(1)
  })
})

import { describe, expect, it } from 'vitest'
import type { RunningWorkflowInfo } from '../../services/api-types'
import type { ChatTab } from '../../stores/useChatStore'
import { convertObservedWorkflowTabToInteractive } from './workflowChatTabConversion'
import {
  reconcileWorkflowRuntimeTab,
  reusableScheduleTabId,
  shouldCatchUpRunningWorkflowTranscript,
  shouldDisplayWorkflowTab,
  workflowRuntimeTabProjection,
} from './workflowRuntimeTabProjection'

function runtime(overrides: Partial<RunningWorkflowInfo>): RunningWorkflowInfo {
  return {
    session_id: 'session-1',
    status: 'running',
    ...overrides,
  } as RunningWorkflowInfo
}

describe('workflowRuntimeTabProjection', () => {
  it('projects a running schedule beside Chat without auto-activating it', () => {
    const projected = workflowRuntimeTabProjection(runtime({
      session_id: 'schedule-manual--daily_123',
      triggered_by: 'cron',
      title: 'Daily execution',
    }), 'workflow-social')

    expect(projected).toEqual({
      name: 'Daily execution',
      metadata: {
        mode: 'workflow',
        presetQueryId: 'workflow-social',
        isViewOnly: true,
        isScheduledRun: true,
        scheduledJobName: 'Daily execution',
      },
      autoActivate: false,
    })
  })

  it('falls back to the generic "Schedule" label when no job name is available', () => {
    const projected = workflowRuntimeTabProjection(runtime({
      session_id: 'schedule-manual--unnamed_1',
      triggered_by: 'cron',
    }), 'workflow-social')

    expect(projected?.name).toBe('Schedule')
    expect(projected?.metadata.scheduledJobName).toBe('Schedule')
  })

  it('keeps an interactive builder run eligible for automatic selection', () => {
    const projected = workflowRuntimeTabProjection(runtime({
      phase_id: 'workflow-builder',
      phase_name: 'Workflow Builder',
      triggered_by: 'user',
    }), 'workflow-social')

    expect(projected?.name).toBe('Automation Builder')
    expect(projected?.metadata.isViewOnly).toBeUndefined()
    expect(projected?.autoActivate).toBe(true)
  })

  it('does not auto-project bot lanes', () => {
    expect(workflowRuntimeTabProjection(runtime({
      session_id: 'bot-slack-123',
      triggered_by: 'bot-slack',
    }), 'workflow-social')).toBeNull()
  })

  it('does not turn a converted Schedule back into a read-only tab on later reconciliation ticks', () => {
    const observed = {
      tabId: 'schedule-tab',
      name: 'Schedule',
      sessionId: 'schedule-manual--daily_123',
      isStreaming: true,
      isCompleted: false,
      hasRunningBgAgents: false,
      isSyntheticTurn: false,
      canSteer: true,
      hideToolCalls: false,
      viewMode: 'tree',
      config: {} as ChatTab['config'],
      createdAt: 1,
      lastAccessedAt: 1,
      lastViewedEventCount: 0,
      lastViewedEventCounts: { micro: 0 },
      metadata: {
        mode: 'workflow' as const,
        presetQueryId: 'workflow-social',
        isViewOnly: true,
        isScheduledRun: true,
        scheduledJobName: 'Daily execution',
      },
    } satisfies ChatTab
    const projection = workflowRuntimeTabProjection(runtime({
      session_id: observed.sessionId,
      triggered_by: 'cron',
      title: 'Daily execution',
    }), 'workflow-social')

    expect(projection).not.toBeNull()
    const converted = convertObservedWorkflowTabToInteractive(observed)
    const firstTick = reconcileWorkflowRuntimeTab(converted, projection!)
    const secondTick = reconcileWorkflowRuntimeTab(firstTick, projection!)

    expect(secondTick.tabId).toBe(observed.tabId)
    expect(secondTick.sessionId).toBe(observed.sessionId)
    expect(secondTick.name).toBe('Automation Builder')
    expect(secondTick.metadata).toMatchObject({
      phaseId: 'workflow-builder',
      phaseName: 'Automation Builder',
      isViewOnly: false,
      isScheduledRun: false,
      userInteractiveContinuation: true,
    })
    expect(secondTick.metadata?.scheduledJobName).toBeUndefined()
  })

  it('continues refreshing ordinary Schedule tabs from runtime state', () => {
    const tab = {
      tabId: 'schedule-tab',
      name: 'Old name',
      sessionId: 'schedule-manual--daily_123',
      metadata: { mode: 'workflow' as const, presetQueryId: 'old-preset' },
    } as ChatTab
    const projection = workflowRuntimeTabProjection(runtime({
      session_id: 'schedule-manual--daily_123',
      triggered_by: 'cron',
      title: 'Daily execution',
    }), 'workflow-social')!

    const reconciled = reconcileWorkflowRuntimeTab(tab, projection)

    expect(reconciled.name).toBe('Daily execution')
    expect(reconciled.metadata).toMatchObject({
      presetQueryId: 'workflow-social',
      isViewOnly: true,
      isScheduledRun: true,
      scheduledJobName: 'Daily execution',
    })
  })
})

describe('shouldCatchUpRunningWorkflowTranscript', () => {
  it('loads already-emitted events when a running tab appears in formatted mode', () => {
    expect(shouldCatchUpRunningWorkflowTranscript('formatted', 0)).toBe(true)
  })

  it('leaves a hydrated transcript and terminal restoration alone', () => {
    expect(shouldCatchUpRunningWorkflowTranscript('formatted', 1)).toBe(false)
    expect(shouldCatchUpRunningWorkflowTranscript('terminal', 0)).toBe(false)
  })
})

describe('shouldDisplayWorkflowTab', () => {
  const scheduleTab = (overrides: Partial<ChatTab>): ChatTab => ({
    tabId: 'schedule-tab', name: 'Daily execution', sessionId: 'schedule-session',
    isStreaming: false, isCompleted: false, hasRunningBgAgents: false, isSyntheticTurn: false,
    canSteer: false, hideToolCalls: true, viewMode: 'terminal', config: {} as ChatTab['config'],
    createdAt: 1, lastAccessedAt: 1, lastViewedEventCount: 0, lastViewedEventCounts: { micro: 0 },
    metadata: { mode: 'workflow', presetQueryId: 'workflow-1', isViewOnly: true, isScheduledRun: true },
    ...overrides,
  })

  // Explicit product decision: a tab, once opened, is closed only by the
  // user -- never auto-hidden because its run finished or lost focus. This
  // was previously conditional (visible only while active/streaming/running
  // background agents); reverted on purpose, accepting that finished
  // Schedule tabs accumulate until manually closed.
  it('keeps a LIVE Schedule visible after the user switches to Chat', () => {
    expect(shouldDisplayWorkflowTab(scheduleTab({ isStreaming: true }), 'chat-tab')).toBe(true)
  })

  it('keeps a Schedule whose background agents are still running', () => {
    expect(shouldDisplayWorkflowTab(scheduleTab({ hasRunningBgAgents: true }), 'chat-tab')).toBe(true)
  })

  it('keeps the Schedule you are actually looking at, finished or not', () => {
    expect(shouldDisplayWorkflowTab(scheduleTab({}), 'schedule-tab')).toBe(true)
  })

  it('keeps a finished Schedule you are not looking at -- user closes it, not auto-hide', () => {
    expect(shouldDisplayWorkflowTab(scheduleTab({}), 'chat-tab')).toBe(true)
  })
})

describe('reusableScheduleTabId', () => {
  const finishedScheduleTab = {
    tabId: 'schedule-tab',
    sessionId: 'schedule-cron--old_1',
    isStreaming: false,
    metadata: { mode: 'workflow' as const, isScheduledRun: true, isViewOnly: true, presetQueryId: 'workflow-social' },
  }

  it('reuses a finished Schedule lane instead of opening another tab per run', () => {
    expect(reusableScheduleTabId({ a: finishedScheduleTab }, 'workflow-social', 'schedule-cron--new_2'))
      .toBe('schedule-tab')
  })

  it('never displaces a live run — the scheduler lease means it still owns the lane', () => {
    expect(reusableScheduleTabId(
      { a: { ...finishedScheduleTab, isStreaming: true } }, 'workflow-social', 'schedule-cron--new_2',
    )).toBeNull()
  })

  it('never recycles a tab the user promoted to an interactive chat', () => {
    expect(reusableScheduleTabId(
      { a: { ...finishedScheduleTab, metadata: { ...finishedScheduleTab.metadata, userInteractiveContinuation: true } } },
      'workflow-social', 'schedule-cron--new_2',
    )).toBeNull()
  })

  it('never crosses workflows', () => {
    expect(reusableScheduleTabId({ a: finishedScheduleTab }, 'workflow-upwork', 'schedule-cron--new_2')).toBeNull()
  })

  it('leaves an ordinary Chat tab alone', () => {
    expect(reusableScheduleTabId(
      { a: { ...finishedScheduleTab, metadata: { mode: 'workflow' as const } } },
      'workflow-social', 'schedule-cron--new_2',
    )).toBeNull()
  })
})

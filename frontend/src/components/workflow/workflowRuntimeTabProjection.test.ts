import { describe, expect, it } from 'vitest'
import type { RunningWorkflowInfo } from '../../services/api-types'
import type { ChatTab } from '../../stores/useChatStore'
import { convertObservedWorkflowTabToInteractive } from './workflowChatTabConversion'
import {
  reconcileWorkflowRuntimeTab,
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
      name: 'Schedule',
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

    expect(reconciled.name).toBe('Schedule')
    expect(reconciled.metadata).toMatchObject({
      presetQueryId: 'workflow-social',
      isViewOnly: true,
      isScheduledRun: true,
      scheduledJobName: 'Daily execution',
    })
  })
})

describe('shouldDisplayWorkflowTab', () => {
  it('keeps a Schedule visible after the user switches to Chat', () => {
    expect(shouldDisplayWorkflowTab({
      tabId: 'schedule-tab',
      name: 'Schedule',
      sessionId: 'schedule-session',
      isStreaming: false,
      isCompleted: false,
      hasRunningBgAgents: false,
      isSyntheticTurn: false,
      canSteer: false,
      hideToolCalls: true,
      viewMode: 'terminal',
      config: {} as ChatTab['config'],
      createdAt: 1,
      lastAccessedAt: 1,
      lastViewedEventCount: 0,
      lastViewedEventCounts: { micro: 0 },
      metadata: {
        mode: 'workflow',
        presetQueryId: 'workflow-1',
        isViewOnly: true,
        isScheduledRun: true,
      },
    }, 'chat-tab')).toBe(true)
  })
})

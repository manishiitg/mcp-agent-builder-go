import { describe, expect, it } from 'vitest'
import type { RunningWorkflowInfo } from '../../services/api-types'
import type { ChatTab } from '../../stores/useChatStore'
import { shouldDisplayWorkflowTab, workflowRuntimeTabProjection } from './workflowRuntimeTabProjection'

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

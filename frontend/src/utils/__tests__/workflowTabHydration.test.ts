import { describe, expect, it } from 'vitest'
import { isReadOnlyWorkflowRunTab, workflowTabsNeedingHydration } from '../workflowTabHydration'
import type { PollingEvent } from '../../services/api-types'

const tab = (tabId: string, sessionId: string | undefined, metadata: Record<string, unknown>) =>
  ({ tabId, sessionId, metadata } as unknown as Parameters<typeof workflowTabsNeedingHydration>[0][number])

describe('workflowTabsNeedingHydration', () => {
  const events: Record<string, PollingEvent[]> = {
    'schedule-cron--d4007648_1': [],
    'schedule-cron--5227790a_2': [{ id: 'e1', type: 'user_message' } as unknown as PollingEvent],
    'chat-interactive': [],
  }
  const getTabEvents = (sessionId: string) => events[sessionId] ?? []

  it('includes a read-only scheduled-run tab whose events are gone (the stuck "Restoring previous session" case)', () => {
    const scheduled = tab('t1', 'schedule-cron--d4007648_1', { mode: 'workflow', isViewOnly: true, isScheduledRun: true, presetQueryId: 'wf_1' })
    const result = workflowTabsNeedingHydration([scheduled], getTabEvents)
    expect(result.map(t => t.tabId)).toEqual(['t1'])
  })

  it('skips tabs that already hold events, and tabs without a session', () => {
    const hydrated = tab('t2', 'schedule-cron--5227790a_2', { mode: 'workflow', isViewOnly: true, isScheduledRun: true })
    const blank = tab('t3', undefined, { mode: 'workflow', phaseId: 'workflow-builder' })
    const interactive = tab('t4', 'chat-interactive', { mode: 'workflow', phaseId: 'workflow-builder' })
    const result = workflowTabsNeedingHydration([hydrated, blank, interactive], getTabEvents)
    expect(result.map(t => t.tabId)).toEqual(['t4'])
  })

  it('ignores non-workflow tabs', () => {
    const multi = tab('t5', 'chat-interactive', { mode: 'multi-agent' })
    expect(workflowTabsNeedingHydration([multi], getTabEvents)).toEqual([])
  })
})

describe('isReadOnlyWorkflowRunTab', () => {
  it('is true only for a view-only workflow tab', () => {
    expect(isReadOnlyWorkflowRunTab({ metadata: { mode: 'workflow', isViewOnly: true } } as never)).toBe(true)
    expect(isReadOnlyWorkflowRunTab({ metadata: { mode: 'workflow' } } as never)).toBe(false)
    expect(isReadOnlyWorkflowRunTab({ metadata: { mode: 'multi-agent', isViewOnly: true } } as never)).toBe(false)
    expect(isReadOnlyWorkflowRunTab({ metadata: undefined } as never)).toBe(false)
  })
})

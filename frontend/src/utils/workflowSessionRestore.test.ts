import { describe, expect, it } from 'vitest'
import { isScheduledWorkflowSession, pickWorkflowActiveSession } from './workflowSessionRestore'
import type { ActiveSessionInfo, RunningWorkflowInfo } from '../services/api-types'

const session = (overrides: Partial<ActiveSessionInfo> = {}): ActiveSessionInfo => ({
  session_id: 'wfrun_1',
  observer_id: '',
  agent_mode: 'workflow',
  status: 'running',
  last_activity: '2026-06-30T00:00:00Z',
  created_at: '2026-06-30T00:00:00Z',
  ...overrides,
})

const runningWorkflow = (overrides: Partial<RunningWorkflowInfo> = {}): RunningWorkflowInfo => ({
  query_id: 'query-1',
  session_id: 'wfrun_1',
  workspace_path: 'Workflow/hetznerssh',
  triggered_by: 'manual',
  started_at: '2026-06-30T00:00:00Z',
  ...overrides,
})

describe('workflow session restore classification', () => {
  it('treats cron-triggered workflow runs as scheduled read-only runs', () => {
    expect(isScheduledWorkflowSession(session({ triggered_by: 'cron' }))).toBe(true)
    expect(isScheduledWorkflowSession(session(), runningWorkflow({ triggered_by: 'cron' }))).toBe(true)
  })

  it('does not classify ordinary workflow runs as scheduled', () => {
    expect(isScheduledWorkflowSession(session({ triggered_by: 'manual' }))).toBe(false)
    expect(isScheduledWorkflowSession(session(), runningWorkflow({ triggered_by: 'manual' }))).toBe(false)
  })

  it('keeps the Pulse parent selected instead of its newer internal reviewer', () => {
    const parent = session({
      session_id: 'pulse-root-1',
      preset_query_id: 'rtslatency',
      workspace_path: 'Workflow/rtslatency',
      last_activity: '2026-08-03T09:00:00Z',
    })
    const child = session({
      session_id: 'pulse-reviewer-1',
      parent_session_id: parent.session_id,
      session_kind: 'pulse_reviewer',
      preset_query_id: 'rtslatency',
      workspace_path: 'Workflow/rtslatency',
      has_retained_tmux_session: true,
      last_activity: '2026-08-03T09:05:00Z',
    })
    const preset = {
      id: 'rtslatency',
      label: 'RTS latency',
      selectedFolder: { filepath: 'Workflow/rtslatency' },
    } as Parameters<typeof pickWorkflowActiveSession>[1]

    expect(pickWorkflowActiveSession([child, parent], preset, {})?.session_id).toBe(parent.session_id)
  })
})

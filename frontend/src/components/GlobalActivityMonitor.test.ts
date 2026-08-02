import { describe, expect, it } from 'vitest'
import type { ActiveSessionInfo } from '../services/api-types'
import { headerStatusLabel, statusTone } from '../utils/globalActivityMonitorStatus'

function retainedWorkflowWithRuntime(phase: 'running' | 'completed'): ActiveSessionInfo {
  return {
    session_id: 'rtslatency-session',
    observer_id: '',
    agent_mode: 'workflow',
    status: 'completed',
    display_status: 'stopped',
    workspace_path: 'Workflow/rtslatency',
    workflow_name: 'rtslatency',
    has_retained_tmux_session: true,
    has_running_background_agents: false,
    running_background_agent_count: 0,
    created_at: new Date().toISOString(),
    last_activity: new Date().toISOString(),
    runtime_state: {
      session_id: 'rtslatency-session',
      generation: 2,
      revision: 922,
      phase,
      reason: phase === 'completed' ? 'session completed' : 'session running',
      raw_session_status: phase === 'completed' ? 'completed' : 'running',
      foreground_turn: {
        busy: phase === 'running',
        has_cancel: phase === 'running',
        can_steer: phase === 'running',
        synthetic: false,
      },
      child_executions: [],
      background_agents: [],
      background_live: false,
      terminals: [],
      terminal_busy: phase === 'running',
      waiting_for_user: false,
      last_progress_at: new Date().toISOString(),
      started_at: new Date().toISOString(),
      observed_at: new Date().toISOString(),
    },
  } as ActiveSessionInfo
}

describe('global activity monitor status', () => {
  it('shows a clock for the completed rtslatency session with a retained idle CLI', () => {
    const session = retainedWorkflowWithRuntime('completed')

    expect(headerStatusLabel(session)).toBe('idle')
    expect(statusTone(session)).toBe('idle')
  })

  it('keeps a genuinely busy retained CLI on the running spinner', () => {
    const session = retainedWorkflowWithRuntime('running')

    expect(headerStatusLabel(session)).toBe('running')
    expect(statusTone(session)).toBe('running')
  })
})

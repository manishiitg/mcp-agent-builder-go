import { describe, expect, it } from 'vitest'
import type { ActiveSessionInfo } from '../services/api-types'
import { currentActiveSession, currentSessionId, headerStatusLabel, statusTone } from '../utils/globalActivityMonitorStatus'

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

// currentSessionId/currentActiveSession are the single definition of "which
// session is the user currently looking at", shared by GlobalActivityMonitor
// (which excludes it from pills) and ModePresetBar's current-workflow
// selector (which must show its live status instead). Before this was
// shared, the selector had no equivalent lookup at all and only ever showed
// a static dot — a workflow could be visibly running with its status
// invisible in both places at once.
describe('currentSessionId', () => {
  const tabs = {
    'tab-1': { sessionId: 'session-1', metadata: { mode: 'workflow' } },
    'tab-2': { sessionId: null, metadata: { mode: 'workflow' } },
    'tab-3': { sessionId: 'session-3', metadata: { mode: 'multi-agent' } },
  }

  it('returns the session id for the active tab in the current mode', () => {
    expect(currentSessionId('tab-1', tabs, 'workflow', false)).toBe('session-1')
  })

  it('returns null while the Organization overview is showing', () => {
    expect(currentSessionId('tab-1', tabs, 'workflow', true)).toBeNull()
  })

  it('returns null with no active tab', () => {
    expect(currentSessionId(null, tabs, 'workflow', false)).toBeNull()
  })

  it('returns null when the active tab belongs to a different mode', () => {
    // tab-3 is a multi-agent tab; a workflow-mode caller must not adopt it.
    expect(currentSessionId('tab-3', tabs, 'workflow', false)).toBeNull()
  })

  it('returns null when the active tab has no session yet', () => {
    expect(currentSessionId('tab-2', tabs, 'workflow', false)).toBeNull()
  })
})

describe('currentActiveSession', () => {
  function minimalSession(overrides: Partial<ActiveSessionInfo> = {}): ActiveSessionInfo {
    return {
      session_id: 'session-1',
      observer_id: '',
      agent_mode: 'workflow',
      status: 'running',
      display_status: 'busy',
      workspace_path: 'Workflow/rtslatency',
      workflow_name: 'rtslatency',
      has_retained_tmux_session: false,
      has_running_background_agents: false,
      running_background_agent_count: 0,
      created_at: new Date().toISOString(),
      last_activity: new Date().toISOString(),
      ...overrides,
    } as ActiveSessionInfo
  }

  it('resolves the matching session from the cache', () => {
    const cache = [minimalSession({ session_id: 'session-1' }), minimalSession({ session_id: 'session-2' })]
    expect(currentActiveSession(cache, 'session-1')?.session_id).toBe('session-1')
  })

  it('returns null when no id is given', () => {
    expect(currentActiveSession([minimalSession()], null)).toBeNull()
  })

  it('returns null when the id matches nothing in the cache', () => {
    expect(currentActiveSession([minimalSession({ session_id: 'session-1' })], 'session-missing')).toBeNull()
  })

  it('excludes an internal child session even if the id matches', () => {
    const cache = [minimalSession({ session_id: 'session-1', parent_session_id: 'parent-session' })]
    expect(currentActiveSession(cache, 'session-1')).toBeNull()
  })
})

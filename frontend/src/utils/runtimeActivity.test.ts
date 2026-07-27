import { describe, expect, it } from 'vitest'
import type { ActiveSessionInfo, RuntimePhase, RuntimeSnapshot, TerminalSnapshot } from '../services/api-types'
import {
  executionTreeRuntimeStatus,
  reconcileTerminalRuntimeState,
  runtimeCanSteer,
  runtimeHasBackgroundAgents,
  runtimeNeedsUserInput,
  sessionRuntimeStatus,
} from './runtimeActivity'

function runtime(phase: RuntimePhase, overrides: Partial<RuntimeSnapshot> = {}): RuntimeSnapshot {
  return {
    session_id: 'session-1', generation: 1, revision: 7, phase,
    foreground_turn: { busy: false, has_cancel: false, can_steer: false, synthetic: false },
    background_live: false, terminal_busy: false, waiting_for_user: false,
    last_progress_at: '2026-07-17T00:00:00Z', started_at: '2026-07-17T00:00:00Z',
    observed_at: '2026-07-17T00:00:00Z',
    ...overrides,
  }
}

function session(state: RuntimeSnapshot): ActiveSessionInfo {
  return {
    session_id: state.session_id, observer_id: '', agent_mode: 'workflow', status: 'completed',
    last_activity: state.last_progress_at, created_at: state.started_at, runtime_state: state,
    // Deliberately contradictory legacy values prove runtime_state wins.
    display_status: 'stopped', has_running_background_agents: false, can_steer: false,
  }
}

function terminal(overrides: Partial<TerminalSnapshot> = {}): TerminalSnapshot {
  return {
    terminal_id: 'session-1:pulse-review-learn-health-123',
    session_id: 'session-1',
    execution_id: 'pulse-review-learn-health-123',
    content: '',
    rows: [],
    chunk_index: 0,
    active: true,
    state: 'running',
    status: {},
    created_at: '2026-07-17T00:00:00Z',
    updated_at: '2026-07-17T00:00:00Z',
    ...overrides,
  }
}

describe('authoritative runtime activity selector', () => {
  it.each([
    ['starting', 'busy'], ['running', 'busy'], ['waiting', 'idle'], ['idle', 'idle'],
    ['completed', 'stopped'], ['failed', 'stopped'], ['canceled', 'stopped'],
  ] as const)('maps %s to %s consistently', (phase, expected) => {
    expect(sessionRuntimeStatus(session(runtime(phase)))).toBe(expected)
  })

  it('uses the same runtime revision for execution-tree status', () => {
    const state = runtime('running')
    expect(executionTreeRuntimeStatus({
      session_id: state.session_id,
      root: { execution_id: 'root', session_id: state.session_id, kind: 'session', name: 'Session', status: 'running', started_at: state.started_at },
      summary: {
        session_id: state.session_id, session_status: 'completed', display_status: 'stopped', is_session_busy: false,
        running_count: 0, completed_count: 2, failed_count: 0, canceled_count: 0,
        has_running_main_agent: false, has_running_background_agents: false, has_running_tracked_executions: false,
      },
      runtime_state: state,
    })).toBe('busy')
  })

  it('takes background, waiting, and steering signals only from runtime_state when present', () => {
    const value = session(runtime('waiting', {
      background_live: true,
      waiting_for_user: true,
      foreground_turn: { busy: false, has_cancel: false, can_steer: true, synthetic: false },
    }))
    expect(runtimeHasBackgroundAgents(value)).toBe(true)
    expect(runtimeNeedsUserInput(value)).toBe(true)
    expect(runtimeCanSteer(value)).toBe(true)
  })

  it('does not expose stale live children after a terminal runtime boundary', () => {
    const value = session(runtime('canceled', { background_live: true, terminal_busy: true }))
    expect(runtimeHasBackgroundAgents(value)).toBe(false)
    expect(sessionRuntimeStatus(value)).toBe('stopped')
  })

  it('keeps only the child that the runtime ledger still reports as active', () => {
    const state = runtime('running', {
      raw_session_status: 'completed',
      background_live: true,
      child_executions: [
        {
          execution_id: 'pulse-review-learn-health-123',
          kind: 'pulse_reviewer',
          status: 'running',
          started_at: '2026-07-17T00:01:00Z',
        },
        {
          execution_id: 'pulse-review-llm-ops-456',
          kind: 'pulse_reviewer',
          status: 'completed',
          started_at: '2026-07-17T00:00:00Z',
          completed_at: '2026-07-17T00:02:00Z',
        },
      ],
    })

    expect(reconcileTerminalRuntimeState(terminal(), state)).toMatchObject({
      active: true,
      state: 'running',
    })
    expect(reconcileTerminalRuntimeState(terminal({
      terminal_id: 'session-1:pulse-review-llm-ops-456',
      execution_id: 'pulse-review-llm-ops-456',
    }), state)).toMatchObject({
      active: false,
      state: 'completed',
    })
  })

  it('settles wrapper terminals whose IDs use the same reviewer slug', () => {
    const state = runtime('running', {
      raw_session_status: 'completed',
      child_executions: [{
        execution_id: 'review-costs-44000',
        kind: 'workflow_builder_task',
        status: 'completed',
        started_at: '2026-07-17T00:00:00Z',
        completed_at: '2026-07-17T00:02:00Z',
      }],
    })

    expect(reconcileTerminalRuntimeState(terminal({
      terminal_id: 'session-1:agent:workshop-review-costs-99999',
      owner_id: 'agent:workshop-review-costs-99999',
      execution_id: 'agent:workshop-review-costs-99999',
    }), state)).toMatchObject({
      active: false,
      state: 'completed',
    })
  })

  it('does not settle a newly launching terminal while the foreground is busy', () => {
    const state = runtime('running', {
      raw_session_status: 'completed',
      foreground_turn: { busy: true, has_cancel: true, can_steer: true, synthetic: false },
    })

    expect(reconcileTerminalRuntimeState(terminal(), state)).toMatchObject({
      active: true,
      state: 'running',
    })
  })

  it('settles a completed child even while the main agent continues working', () => {
    const state = runtime('running', {
      raw_session_status: 'running',
      foreground_turn: { busy: true, has_cancel: true, can_steer: true, synthetic: true },
      child_executions: [{
        execution_id: 'pulse-review-llm-ops-456',
        kind: 'pulse_reviewer',
        status: 'completed',
        started_at: '2026-07-17T00:00:00Z',
        completed_at: '2026-07-17T00:02:00Z',
      }],
    })

    expect(reconcileTerminalRuntimeState(terminal({
      terminal_id: 'session-1:pulse-review-llm-ops-456',
      execution_id: 'pulse-review-llm-ops-456',
    }), state)).toMatchObject({
      active: false,
      state: 'completed',
    })
  })
})

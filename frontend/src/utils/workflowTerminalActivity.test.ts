import { describe, expect, it } from 'vitest'
import type { TerminalSnapshot } from '../services/api-types'
import {
  activeSessionFromWorkflowTerminal,
  isLiveWorkflowTerminal,
  liveWorkflowTerminalSessionForPreset,
} from './workflowTerminalActivity'

function terminal(overrides: Partial<TerminalSnapshot>): TerminalSnapshot {
  return {
    terminal_id: 'session:main:session',
    session_id: 'session',
    owner_id: 'main:session',
    execution_id: 'main:session',
    execution_kind: 'main_agent',
    label: 'main',
    scope: 'execution',
    workflow_path: 'Workflow/example',
    active: false,
    state: 'starting',
    content: '',
    rows: [],
    chunk_index: 0,
    status: {},
    created_at: '2026-07-14T00:00:00Z',
    updated_at: '2026-07-14T00:00:00Z',
    ...overrides,
  }
}

describe('isLiveWorkflowTerminal', () => {
  it('does not treat a provisional startup row as an attached terminal', () => {
    expect(isLiveWorkflowTerminal(terminal({ state: 'starting' }))).toBe(false)
  })

  it('retains a completed terminal while its tmux pane still exists', () => {
    expect(isLiveWorkflowTerminal(terminal({
      state: 'completed',
      tmux_session: 'mlp-claude-code-session',
    }))).toBe(true)
  })

  it('does not retain a terminal after the backend marks its pane stale', () => {
    expect(isLiveWorkflowTerminal(terminal({
      state: 'stale',
      tmux_session: 'mlp-claude-code-session',
    }))).toBe(false)
  })

  it('does not misreport foreground terminal activity as a background agent', () => {
    const session = activeSessionFromWorkflowTerminal(terminal({
      active: true,
      state: 'running',
      tmux_session: 'mlp-codex-cli-session',
    }))
    expect(session.has_running_background_agents).toBeUndefined()
    expect(session.has_retained_tmux_session).toBe(true)
  })
})

describe('liveWorkflowTerminalSessionForPreset', () => {
  const preset = {
    id: 'example',
    label: 'Example',
    selectedFolder: { filepath: 'Workflow/example' },
  } as Parameters<typeof liveWorkflowTerminalSessionForPreset>[1]

  it('opens the canonical main agent instead of a newer active child', () => {
    const main = terminal({
      session_id: 'workflow-root',
      terminal_id: 'workflow-root:main:workflow-root',
      owner_id: 'main:workflow-root',
      execution_kind: 'main_agent',
      label: 'Main agent',
      tmux_session: 'main-tmux',
      state: 'completed',
      active: false,
      updated_at: '2026-08-10T08:00:00Z',
    })
    const child = terminal({
      session_id: 'workflow-root',
      terminal_id: 'workflow-root:workflow-step:review',
      owner_id: 'workflow-step:review',
      execution_kind: 'workflow_step',
      label: 'Child reviewer',
      tmux_session: 'child-tmux',
      state: 'running',
      active: true,
      updated_at: '2026-08-10T08:05:00Z',
    })

    const restored = liveWorkflowTerminalSessionForPreset([child, main], preset, 'Example')
    expect(restored?.session_id).toBe(main.session_id)
    expect(restored?.current_execution_name).toBe('Main agent')
  })

  it('does not use a child terminal as the workflow landing session', () => {
    const child = terminal({
      session_id: 'child-only',
      terminal_id: 'child-only:workflow-step:review',
      owner_id: 'workflow-step:review',
      execution_kind: 'workflow_step',
      active: true,
      state: 'running',
    })

    expect(liveWorkflowTerminalSessionForPreset([child], preset, 'Example')).toBeUndefined()
  })
})

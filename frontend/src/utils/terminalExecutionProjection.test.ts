import { describe, expect, it } from 'vitest'
import type {
  SessionExecutionTreeNode,
  SessionExecutionTreeResponse,
  TerminalSnapshot,
} from '../services/api-types'
import { projectExecutionTreeTerminals } from './terminalExecutionProjection'

const node = (
  executionID: string,
  overrides: Partial<SessionExecutionTreeNode> = {},
): SessionExecutionTreeNode => ({
  execution_id: executionID,
  session_id: 'session-1',
  kind: 'background_agent',
  name: executionID,
  status: 'running',
  started_at: '2026-08-04T16:00:00Z',
  ...overrides,
})

const tree = (children: SessionExecutionTreeNode[]): SessionExecutionTreeResponse => ({
  session_id: 'session-1',
  root: node('session:session-1', {
    kind: 'session_root',
    name: 'Session',
    children,
  }),
  summary: {
    session_id: 'session-1',
    session_status: 'running',
    display_status: 'busy',
    is_session_busy: true,
    running_count: 1,
    completed_count: 1,
    failed_count: 0,
    canceled_count: 0,
    has_running_main_agent: false,
    has_running_background_agents: true,
    has_running_tracked_executions: true,
  },
})

const terminal = (overrides: Partial<TerminalSnapshot> = {}): TerminalSnapshot => ({
  terminal_id: 'session-1:child-1',
  session_id: 'session-1',
  execution_id: 'child-1',
  execution_kind: 'background_agent',
  agent_name: 'Trending radar',
  content: '',
  rows: [],
  chunk_index: 1,
  active: false,
  state: 'completed',
  status: {},
  created_at: '2026-08-04T16:00:00Z',
  updated_at: '2026-08-04T16:00:01Z',
  ...overrides,
})

describe('live execution-tree terminal projection', () => {
  it('shows a running asynchronous child when no terminal exists yet', () => {
    const result = projectExecutionTreeTerminals([], tree([
      node('main:session-1', {
        kind: 'main_agent',
        status: 'completed',
        name: 'Route execution',
      }),
      node('child-1', {
        parent_execution_id: 'main:session-1',
        name: 'Execute trending radar',
      }),
    ]))

    expect(result).toHaveLength(1)
    expect(result[0]).toMatchObject({
      execution_id: 'child-1',
      parent_execution_id: 'main:session-1',
      agent_name: 'Execute trending radar',
      active: true,
      state: 'running',
      execution_tree_placeholder: true,
    })
  })

  it('replaces the placeholder identity with the real terminal without duplicating it', () => {
    const retained = terminal()
    const result = projectExecutionTreeTerminals([retained], tree([
      node('child-1', {
        parent_execution_id: 'orchestrator-1',
        name: 'Execute trending radar',
      }),
    ]))

    expect(result).toHaveLength(1)
    expect(result[0]).toMatchObject({
      terminal_id: retained.terminal_id,
      parent_execution_id: 'orchestrator-1',
      active: true,
      state: 'running',
    })
    expect(result[0].execution_tree_placeholder).toBeUndefined()
  })

  it('uses the published parent step terminal for a live message-sequence item', () => {
    const publishedStepTerminal = terminal({
      terminal_id: 'session-1:workflow-step:exec-discover-1:discover',
      execution_id: 'workflow-step:exec-discover-1:discover',
      owner_id: 'workflow-step:exec-discover-1:discover',
      active: true,
      state: 'running',
    })

    const result = projectExecutionTreeTerminals([publishedStepTerminal], tree([
      node('msgseq-discover-audit-123', {
        parent_execution_id: 'exec-discover-1',
        kind: 'message_sequence_item',
        name: 'Audit candidate pool',
      }),
    ]))

    // The item shares its structured agent and event stream with the parent
    // workflow-step terminal. A second placeholder would have no terminal or
    // event endpoint to load.
    expect(result).toHaveLength(1)
    expect(result[0]).toMatchObject({
      terminal_id: publishedStepTerminal.terminal_id,
      execution_id: publishedStepTerminal.execution_id,
      active: true,
      state: 'running',
    })
    expect(result[0].execution_tree_placeholder).toBeUndefined()
  })

  it('does not create retained UI rows for completed children', () => {
    const result = projectExecutionTreeTerminals([], tree([
      node('child-1', { status: 'completed', completed_at: '2026-08-04T16:01:00Z' }),
    ]))

    expect(result).toEqual([])
  })
})

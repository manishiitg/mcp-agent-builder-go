import { describe, expect, it } from 'vitest'
import type { TerminalSnapshot } from '../types'
import { isMainAgentTerminal, preferredTerminalForContext } from './terminalIdentity'

const terminal = (overrides: Partial<TerminalSnapshot>): TerminalSnapshot => ({
  terminal_id: 'session-1:main:session-1',
  session_id: 'session-1',
  active: true,
  state: 'running',
  ...overrides,
} as TerminalSnapshot)

describe('isMainAgentTerminal', () => {
  it('recognizes the canonical main owner', () => {
    expect(isMainAgentTerminal(terminal({
      owner_id: 'main:session-1',
      execution_kind: 'main_agent',
    }))).toBe(true)
  })

  it('does not promote a child with an inherited main kind', () => {
    expect(isMainAgentTerminal(terminal({
      terminal_id: 'session-1:pulse-reviewer-eval-789',
      owner_id: 'pulse-reviewer-eval-789',
      execution_kind: 'main_agent',
    }))).toBe(false)
  })
})

describe('preferredTerminalForContext', () => {
  it('waits for the main agent instead of opening a workflow child', () => {
    const child = terminal({
      terminal_id: 'session-1:workflow-step:review',
      owner_id: 'workflow-step:review',
      execution_kind: 'workflow_step',
    })
    expect(preferredTerminalForContext(null, [child], true)).toBeNull()
  })

  it('still permits standalone non-workflow agents to open directly', () => {
    const agent = terminal({
      terminal_id: 'session-1:background-review',
      owner_id: 'background-review',
      execution_kind: 'background_agent',
    })
    expect(preferredTerminalForContext(null, [agent], false)).toBe(agent)
  })

  // PLAT-107: an execution-tree placeholder has no published terminal behind
  // it, so auto-selecting one renders an unexplained blank pane. The workflow
  // branch already refused this; a Schedule session is not a workflow context.
  it('never auto-selects an unpublished placeholder outside workflow context', () => {
    const placeholder = terminal({
      terminal_id: 'session-1:pulse-finalizer-1',
      owner_id: 'pulse-finalizer-1',
      execution_kind: 'background_agent',
      execution_tree_placeholder: true,
    })
    expect(preferredTerminalForContext(null, [placeholder], false)).toBeNull()
  })

  it('prefers a real terminal over a placeholder that appeared first', () => {
    const placeholder = terminal({
      terminal_id: 'session-1:pulse-finalizer-1',
      owner_id: 'pulse-finalizer-1',
      execution_tree_placeholder: true,
    })
    const real = terminal({
      terminal_id: 'session-1:background-review',
      owner_id: 'background-review',
      execution_kind: 'background_agent',
    })
    expect(preferredTerminalForContext(null, [placeholder, real], false)).toBe(real)
  })
})

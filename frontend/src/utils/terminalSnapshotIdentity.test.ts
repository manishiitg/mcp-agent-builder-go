import { describe, expect, it } from 'vitest'
import type { TerminalSnapshot } from '../services/api-types'
import {
  mergeTerminalSnapshotBody,
  reconcileTerminalSnapshots,
  resolveTerminalFormattedView,
  shouldHydrateMainTerminalEvents,
  shouldLoadTerminalEvents,
  shouldStreamTerminal,
} from './terminalSnapshotIdentity'

const terminal = (id: string, overrides: Partial<TerminalSnapshot> = {}): TerminalSnapshot => ({
  terminal_id: id,
  session_id: 'session-1',
  content: '',
  rows: [],
  chunk_index: 1,
  active: true,
  status: { provider_label: 'Codex', input_tokens: 10 },
  created_at: '2026-07-13T00:00:00Z',
  updated_at: '2026-07-13T00:00:01Z',
  ...overrides,
})

describe('reconcileTerminalSnapshots', () => {
  it('preserves the array for repeated empty polls', () => {
    const current: TerminalSnapshot[] = []

    expect(reconcileTerminalSnapshots(current, [])).toBe(current)
  })

  it('preserves the array and objects for semantically identical polls', () => {
    const current = [terminal('one'), terminal('two')]
    const incoming = current.map(item => ({
      ...item,
      rows: [...item.rows],
      status: { ...item.status },
    }))

    const result = reconcileTerminalSnapshots(current, incoming)

    expect(result).toBe(current)
    expect(result[0]).toBe(current[0])
  })

  it('does not churn state for 20 unchanged active terminals', () => {
    const current = Array.from({ length: 20 }, (_, index) => terminal(`terminal-${index}`))
    const incoming = current.map(item => ({
      ...item,
      rows: [...item.rows],
      status: { ...item.status },
    }))

    expect(reconcileTerminalSnapshots(current, incoming)).toBe(current)
  })

  it('ignores backend ordering changes when the terminal set is unchanged', () => {
    const current = [terminal('one'), terminal('two')]
    const incoming = [{ ...current[1] }, { ...current[0] }]

    expect(reconcileTerminalSnapshots(current, incoming)).toBe(current)
  })

  it('reuses unchanged objects while applying changed snapshots', () => {
    const current = [terminal('one'), terminal('two')]
    const changed = terminal('two', { chunk_index: 2, updated_at: '2026-07-13T00:00:02Z' })

    const result = reconcileTerminalSnapshots(current, [{ ...current[0] }, changed])

    expect(result).not.toBe(current)
    expect(result[0]).toBe(current[0])
    expect(result[1]).toBe(changed)
  })

  it('applies additions and removals', () => {
    const current = [terminal('one'), terminal('two')]
    const added = terminal('three')

    expect(reconcileTerminalSnapshots(current, [current[0], added])).toEqual([current[0], added])
    expect(reconcileTerminalSnapshots(current, [current[0]])).toEqual([current[0]])
  })
})

describe('mergeTerminalSnapshotBody', () => {
  it('does not let stale detail metadata downgrade a live terminal', () => {
    const base = terminal('main', {
      state: 'running',
      active: true,
      process_state: 'live',
      content: '',
    })
    const detail = terminal('main', {
      state: 'completed',
      active: false,
      process_state: 'closed',
      content: 'captured transcript',
    })

    expect(mergeTerminalSnapshotBody(base, detail)).toMatchObject({
      state: 'running',
      active: true,
      process_state: 'live',
      content: 'captured transcript',
    })
  })
})

describe('shouldStreamTerminal', () => {
  it('keeps an interactive CLI attached after its current turn completes', () => {
    expect(shouldStreamTerminal(terminal('main', {
      state: 'completed',
      active: false,
      process_state: 'live',
      snapshot_kind: 'live',
      tmux_session: 'tmux-main',
    }))).toBe(true)
  })

  it('does not stream an archived or closed pane', () => {
    expect(shouldStreamTerminal(terminal('main', {
      state: 'completed',
      active: false,
      process_state: 'closed',
      snapshot_kind: 'archived',
      tmux_session: 'tmux-main',
    }))).toBe(false)
  })
})

describe('shouldLoadTerminalEvents', () => {
  it('does not request an unpublished execution-tree placeholder', () => {
    expect(shouldLoadTerminalEvents(terminal('child', {
      execution_tree_placeholder: true,
      state: 'running',
      active: true,
    }), false, true)).toBe(false)
  })

  it('does not load a real child transcript while Raw is selected', () => {
    expect(shouldLoadTerminalEvents(terminal('child', {
      state: 'running',
      active: true,
    }), false, false)).toBe(false)
  })

  it('loads events once Formatted is selected for a real child terminal', () => {
    expect(shouldLoadTerminalEvents(terminal('child', {
      state: 'running',
      active: true,
    }), false, true)).toBe(true)
  })
})

describe('shouldHydrateMainTerminalEvents', () => {
  it('keeps restored Schedule main-agent history unloaded in Raw mode', () => {
    expect(shouldHydrateMainTerminalEvents(true, false, 0)).toBe(false)
  })

  it('hydrates an empty restored Schedule main-agent history on Formatted', () => {
    expect(shouldHydrateMainTerminalEvents(true, true, 0, true, false)).toBe(true)
  })

  it('hydrates a restored Schedule history even if a newer live event has arrived', () => {
    expect(shouldHydrateMainTerminalEvents(true, true, 1, true, false)).toBe(true)
  })

  it('does not hydrate the same restored Schedule history twice', () => {
    expect(shouldHydrateMainTerminalEvents(true, true, 12, true, true)).toBe(false)
  })

  it('does not reload main-agent history that is already present', () => {
    expect(shouldHydrateMainTerminalEvents(true, true, 12)).toBe(false)
  })
})

describe('resolveTerminalFormattedView', () => {
  it('defaults every terminal to raw mode even when structured events exist', () => {
    expect(resolveTerminalFormattedView(true)).toBe(false)
  })

  it('respects an explicit raw or formatted choice', () => {
    expect(resolveTerminalFormattedView(true, false)).toBe(false)
    expect(resolveTerminalFormattedView(true, true)).toBe(true)
  })

  it('never shows an unavailable formatted transcript', () => {
    expect(resolveTerminalFormattedView(false, true)).toBe(false)
  })
})

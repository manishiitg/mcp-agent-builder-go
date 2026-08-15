import type { TerminalSnapshot } from '../services/api-types'
import { isMainAgentTerminal } from './terminalIdentity'

const isRecord = (value: unknown): value is Record<string, unknown> => (
  value !== null && typeof value === 'object' && !Array.isArray(value)
)

const jsonValueEqual = (left: unknown, right: unknown): boolean => {
  if (Object.is(left, right)) return true
  if (Array.isArray(left) || Array.isArray(right)) {
    if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) return false
    return left.every((value, index) => jsonValueEqual(value, right[index]))
  }
  if (!isRecord(left) || !isRecord(right)) return false

  const leftKeys = Object.keys(left)
  const rightKeys = Object.keys(right)
  if (leftKeys.length !== rightKeys.length) return false
  return leftKeys.every(key => (
    Object.prototype.hasOwnProperty.call(right, key) && jsonValueEqual(left[key], right[key])
  ))
}

/**
 * Reuses terminal snapshots from the previous poll when their JSON payload is
 * unchanged. Poll ordering is not treated as a change because the terminal
 * rail applies its own stable sort before rendering.
 */
export function reconcileTerminalSnapshots(
  current: TerminalSnapshot[],
  incoming: TerminalSnapshot[],
): TerminalSnapshot[] {
  if (current.length === 0) return incoming.length === 0 ? current : incoming
  if (incoming.length === 0) return incoming

  const currentByID = new Map(current.map(terminal => [terminal.terminal_id, terminal]))
  let changed = current.length !== incoming.length
  const reconciled = incoming.map(terminal => {
    const existing = currentByID.get(terminal.terminal_id)
    if (existing && jsonValueEqual(existing, terminal)) return existing
    changed = true
    return terminal
  })

  if (!changed) return current
  return reconciled
}

/**
 * Detail endpoints provide the terminal body, but their lifecycle metadata may
 * have been captured before the latest metadata poll. Preserve the poll's
 * authoritative runtime state so loading history cannot briefly downgrade a
 * live pane to completed and force a WebSocket reconnect.
 */
export function mergeTerminalSnapshotBody(
  base: TerminalSnapshot,
  detail: TerminalSnapshot,
): TerminalSnapshot {
  return {
    ...base,
    content: detail.content || base.content || '',
    content_source: detail.content_source || base.content_source,
    rows: Array.isArray(detail.rows) && detail.rows.length > 0 ? detail.rows : base.rows,
  }
}

export function shouldStreamTerminal(terminal: TerminalSnapshot | null): boolean {
  if (!terminal || !terminal.tmux_session) return false

  const processState = (terminal.process_state || '').trim().toLowerCase()
  const snapshotKind = (terminal.snapshot_kind || '').trim().toLowerCase()
  if (processState === 'closed' || snapshotKind === 'archived') return false

  // Interactive CLIs remain available for follow-up messages after a turn is
  // marked completed. The live process is the authoritative signal here.
  if (processState === 'live') return true

  const state = (terminal.state || '').trim().toLowerCase()
  if (state === 'completed' || state === 'failed' || state === 'closing' || state === 'stale') {
    return false
  }
  return terminal.active || state === 'running' || state === 'idle'
}

/**
 * Main-agent terminal IDs can change when a retained conversation starts its
 * next turn. Keep the reader's Raw/Formatted choice on the durable session;
 * child terminals remain independently selectable by terminal ID.
 */
export function terminalViewPreferenceKey(terminal: TerminalSnapshot | null): string | null {
  if (!terminal?.terminal_id) return null
  const sessionID = (terminal.session_id || '').trim()
  if (sessionID && isMainAgentTerminal(terminal)) return `session:${sessionID}`
  return `terminal:${terminal.terminal_id}`
}

/**
 * Execution-tree placeholders are navigation entries, not published terminal
 * transcripts. Fetching /events for one necessarily returns 404 until the
 * runtime replaces it with a real terminal snapshot.
 */
export function shouldLoadTerminalEvents(
  terminal: TerminalSnapshot | null,
  usesSessionEvents: boolean,
  formattedViewRequested: boolean,
): boolean {
  return Boolean(
    formattedViewRequested &&
    terminal &&
    !usesSessionEvents &&
    !terminal.execution_tree_placeholder,
  )
}

/** Main-agent transcripts live in the session event store rather than the
 * per-terminal event endpoint. Hydrate one bounded durable page when the
 * formatted conversation is opened. A non-empty live tail is not proof that
 * the beginning was loaded: a reconnect can contain tools and the answer while
 * omitting the opening user message.
 */
export function shouldHydrateMainTerminalEvents(
  usesSessionEvents: boolean,
  formattedViewRequested: boolean,
  _loadedEventCount: number,
  _restoredHistoryRequired = false,
  durableHistoryLoaded = false,
): boolean {
  return Boolean(
    usesSessionEvents &&
    formattedViewRequested &&
    !durableHistoryLoaded,
  )
}

/** The formatted conversation is the user-facing default whenever structured
 * events are available. Raw tmux remains an explicit diagnostic choice and an
 * explicit choice must always win.
 */
export function resolveTerminalFormattedView(
  canShowFormattedView: boolean,
  explicitPreference?: boolean,
): boolean {
  if (!canShowFormattedView) return false
  return explicitPreference ?? true
}

/** A terminal can offer Raw/Formatted even after its live tmux process is
 * released. Retained raw bytes and main-session events are durable views; a
 * live tmux_session is only one possible source, not the eligibility rule.
 */
export function canToggleTerminalView(
  terminal: TerminalSnapshot | null,
  isSynthetic: boolean,
  hasRawContent: boolean,
  usesSessionEvents: boolean,
): boolean {
  return Boolean(
    terminal?.terminal_id &&
    !isSynthetic &&
    (terminal.tmux_session || hasRawContent || usesSessionEvents),
  )
}

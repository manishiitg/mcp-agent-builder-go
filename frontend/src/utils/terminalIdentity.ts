import type { TerminalSnapshot } from '../services/api-types'

export function isMainAgentTerminal(terminal: TerminalSnapshot): boolean {
  const kind = (terminal.execution_kind || '').trim().toLowerCase()
  const ownerID = (terminal.owner_id || '').trim().toLowerCase()
  const sessionID = (terminal.session_id || '').trim().toLowerCase()
  const terminalID = (terminal.terminal_id || '').trim().toLowerCase()
  const canonicalOwner = ownerID.startsWith('main:') || (!!sessionID && ownerID === sessionID)
  const canonicalTerminalID = terminalID.includes(':main:')

  // An explicit non-main owner is authoritative. Provider callbacks can
  // inherit main_agent from their parent while describing a child pane.
  if (ownerID && !canonicalOwner) return false

  return canonicalOwner || canonicalTerminalID || kind === 'main_agent' || kind === 'main' || kind === 'chat'
}

export function preferredTerminalForContext(
  mainTerminal: TerminalSnapshot | null,
  fallbackTerminals: Array<TerminalSnapshot | null | undefined>,
  isWorkflowContext: boolean,
): TerminalSnapshot | null {
  if (mainTerminal) return mainTerminal
  // Workflow navigation always lands on the main agent. A child may appear
  // before the main terminal snapshot is published, but it must only become
  // the active pane after the user explicitly selects it from the rail.
  if (isWorkflowContext) return null
  return fallbackTerminals.find((terminal): terminal is TerminalSnapshot => Boolean(terminal)) || null
}

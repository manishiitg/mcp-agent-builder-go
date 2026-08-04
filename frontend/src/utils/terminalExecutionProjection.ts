import type {
  SessionExecutionTreeNode,
  SessionExecutionTreeResponse,
  TerminalSnapshot,
} from '../services/api-types'

const HIDDEN_EXECUTION_KINDS = new Set([
  'session_root',
  'main_agent',
  'synthetic_turn',
])

function normalizedExecutionStatus(status?: string): string {
  return (status || '').trim().toLowerCase()
}

function isLiveExecution(node: SessionExecutionTreeNode): boolean {
  return ['starting', 'queued', 'pending', 'running', 'waiting'].includes(
    normalizedExecutionStatus(node.status),
  )
}

function isVisibleChildExecution(node: SessionExecutionTreeNode, rootID: string): boolean {
  if (!node.execution_id || node.execution_id === rootID) return false
  if (node.execution_id.startsWith('main:')) return false
  return !HIDDEN_EXECUTION_KINDS.has((node.kind || '').trim().toLowerCase())
}

function flattenExecutionTree(root: SessionExecutionTreeNode): SessionExecutionTreeNode[] {
  const nodes: SessionExecutionTreeNode[] = []
  const pending = [root]
  const seen = new Set<string>()
  while (pending.length > 0) {
    const node = pending.shift()
    if (!node || !node.execution_id || seen.has(node.execution_id)) continue
    seen.add(node.execution_id)
    nodes.push(node)
    pending.push(...(node.children || []))
  }
  return nodes
}

function terminalMatchesExecution(terminal: TerminalSnapshot, node: SessionExecutionTreeNode): boolean {
  if (terminal.session_id !== node.session_id) return false
  return terminal.execution_id === node.execution_id ||
    terminal.owner_id === node.execution_id ||
    terminal.terminal_id === `${node.session_id}:${node.execution_id}`
}

function runningTerminalState(node: SessionExecutionTreeNode): TerminalSnapshot['state'] {
  const status = normalizedExecutionStatus(node.status)
  if (status === 'failed') return 'failed'
  if (status === 'canceled' || status === 'cancelled') return 'completed'
  if (isLiveExecution(node)) return 'running'
  return 'completed'
}

/**
 * Project live execution-tree children into the terminal rail while their
 * detailed terminal snapshot is still being created. The projection is
 * deliberately ephemeral: as soon as a real terminal with the same execution
 * identity appears, it is enriched in place and the placeholder disappears.
 */
export function projectExecutionTreeTerminals(
  terminals: TerminalSnapshot[],
  tree?: SessionExecutionTreeResponse | null,
): TerminalSnapshot[] {
  if (!tree?.root) return terminals

  let projected = terminals
  const nodes = flattenExecutionTree(tree.root)
  const nodesByID = new Map(nodes.map(node => [node.execution_id, node]))

  for (const node of nodes) {
    if (!isVisibleChildExecution(node, tree.root.execution_id)) continue

    const matchingIndexes: number[] = []
    projected.forEach((terminal, index) => {
      if (terminalMatchesExecution(terminal, node)) matchingIndexes.push(index)
    })

    if (matchingIndexes.length > 0) {
      // Clone only when execution-tree metadata adds information or corrects a
      // stale retained "completed" snapshot for an execution that is live.
      const next = [...projected]
      for (const index of matchingIndexes) {
        const terminal = next[index]
        const live = isLiveExecution(node)
        next[index] = {
          ...terminal,
          parent_execution_id: node.parent_execution_id || terminal.parent_execution_id,
          execution_kind: terminal.execution_kind || node.kind,
          agent_name: terminal.agent_name || node.name,
          display_title: terminal.display_title || node.name,
          ...(live ? {
            active: true,
            state: 'running',
            process_state: terminal.process_state || 'live',
            snapshot_kind: terminal.snapshot_kind || 'live',
          } : {}),
        }
      }
      projected = next
      continue
    }

    // Completed historical tree nodes do not need another retained UI row.
    // Only a currently live child can otherwise become invisible.
    if (!isLiveExecution(node)) continue

    const parent = node.parent_execution_id
      ? nodesByID.get(node.parent_execution_id)
      : undefined
    const startedAt = node.started_at || new Date(0).toISOString()
    projected = [...projected, {
      terminal_id: `${node.session_id}:${node.execution_id}`,
      session_id: node.session_id,
      owner_id: node.execution_id,
      execution_id: node.execution_id,
      parent_execution_id: node.parent_execution_id,
      execution_kind: node.kind || 'background_agent',
      agent_name: node.name || 'Background agent',
      display_title: node.name || 'Background agent',
      display_meta: parent?.name ? `Child of ${parent.name}` : 'Asynchronous child',
      content_source: 'execution_tree',
      execution_tree_placeholder: true,
      content: '',
      rows: [],
      chunk_index: 0,
      active: true,
      state: runningTerminalState(node),
      process_state: 'live',
      snapshot_kind: 'live',
      status: {
        status_text: 'Running',
      },
      created_at: startedAt,
      updated_at: startedAt,
    }]
  }

  return projected
}

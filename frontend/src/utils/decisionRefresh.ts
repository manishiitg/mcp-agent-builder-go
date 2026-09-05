import type { PollingEvent } from '../services/api-types'
import { getTypedEventData } from '../generated/event-types'

const MUTATIONS = new Set([
  'create_human_input_request',
  'answer_human_input_request',
  'mark_human_input_consumed',
  'dismiss_duplicate_human_input_request',
])
const SAVED = new Set(['created', 'answered', 'consumed', 'duplicate_dismissed'])

// Refresh from a committed typed receipt, even while the agent continues its
// turn. Discussion, starts, failures and writes to another workflow do not
// invalidate the decision cards currently on screen.
export function decisionMutationNeedsRefresh(event: PollingEvent, workspacePath: string): boolean {
  if (!workspacePath) return false
  const result = getTypedEventData(event, 'tool_call_end')
    ?? getTypedEventData(event, 'tool_execution')
  if (!result?.tool_name || !MUTATIONS.has(result.tool_name) || !result.result) return false
  try {
    const receipt = JSON.parse(result.result)
    return SAVED.has(receipt?.status)
      && typeof receipt?.input?.id === 'string'
      && receipt.input.workspace_path === workspacePath
  } catch {
    return false
  }
}

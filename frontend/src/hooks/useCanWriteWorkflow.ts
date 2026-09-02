import { useAuthStore } from '../stores/useAuthStore'
import { hasWorkflowWriteAccess } from '../utils/workflowPermissions'

/** Tooltip for a control that is disabled because the user has read-only access. */
export const READ_ONLY_TITLE = 'Read-only access'

/**
 * Whether the current user may change workflow-affecting state.
 *
 * The capabilities panel hides its Save button and the toolbar hides the
 * capability cluster on this, but the section components underneath write
 * account-wide state immediately (bot routes, API keys, secrets, skills, MCP
 * connections) and several are reachable from surfaces that never pass
 * through either gate. Every one of those mutating controls disables on this
 * so a read-only user can't mutate through the side door.
 */
export function useCanWriteWorkflow(): boolean {
  return useAuthStore(state => hasWorkflowWriteAccess(state.user, state.isMultiUserMode))
}

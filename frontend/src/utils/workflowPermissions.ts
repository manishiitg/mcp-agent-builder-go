import type { AuthUser } from '../services/api'

export function hasWorkflowWriteAccess(user: AuthUser | null | undefined, isMultiUserMode: boolean): boolean {
  if (user?.can_write_workflows !== undefined) {
    return user.can_write_workflows
  }
  if (!isMultiUserMode) {
    return true
  }
  return user?.workflow_access === 'write' || user?.workflow_access === 'owner'
}

export function hasWorkflowOwnerAccess(user: AuthUser | null | undefined, isMultiUserMode: boolean): boolean {
  if (user?.can_manage_workflow_access !== undefined) {
    return user.can_manage_workflow_access
  }
  if (!isMultiUserMode) {
    return true
  }
  return user?.workflow_access === 'owner'
}

// True only once the backend has actually confirmed non-write access (PLAT-262
// read tier) — never a guess from the absence of a signal, which is why this
// is `!hasWorkflowWriteAccess` rather than checking `workflow_access === 'read'`
// directly (that field can be absent even when can_write_workflows is set).
export function isWorkflowReadOnly(user: AuthUser | null | undefined, isMultiUserMode: boolean): boolean {
  return !hasWorkflowWriteAccess(user, isMultiUserMode)
}

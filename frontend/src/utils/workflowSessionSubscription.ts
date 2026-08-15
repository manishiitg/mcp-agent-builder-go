export type WorkflowSessionActivity = {
  isStreaming: boolean
  hasRunningBackgroundAgents: boolean
  isBackendActive: boolean
}

/**
 * Keep a workflow transcript connected while any authoritative activity signal
 * remains. In particular, the backend can still deliver child completions after
 * the foreground turn and the local background-agent flag have settled.
 */
export function shouldKeepWorkflowSessionSubscribed(activity: WorkflowSessionActivity): boolean {
  return (
    activity.isStreaming ||
    activity.hasRunningBackgroundAgents ||
    activity.isBackendActive
  )
}

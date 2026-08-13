import type { RunningWorkflowInfo } from '../../services/api-types'
import type { ChatTab } from '../../stores/useChatStore'
import { isExternalReadOnlyWorkflowSession, isScheduledSession } from '../../utils/workflowSessionKinds'

export interface WorkflowRuntimeTabProjection {
  name: string
  metadata: NonNullable<ChatTab['metadata']>
  autoActivate: boolean
}

/** Describe the top-level tab for one live workflow execution. */
export function workflowRuntimeTabProjection(
  running: RunningWorkflowInfo,
  presetQueryId: string,
): WorkflowRuntimeTabProjection | null {
  const identity = {
    sessionId: running.session_id,
    triggeredBy: running.triggered_by,
  }
  const scheduled = isScheduledSession(identity)
  const external = isExternalReadOnlyWorkflowSession(identity)

  // Bot lanes retain their explicit open-from-activity behavior. A scheduled
  // run is different: it is a first-class parallel lane beside Chat.
  if (external && !scheduled) return null

  if (scheduled) {
    return {
      name: 'Schedule',
      metadata: {
        mode: 'workflow',
        presetQueryId,
        isViewOnly: true,
        isScheduledRun: true,
        scheduledJobName: running.title || running.preset_name || running.phase_name || 'Schedule',
      },
      // Runtime discovery must not pull the user away from their live Chat.
      autoActivate: false,
    }
  }

  const phaseName = running.phase_name || running.title || running.preset_name || 'Running automation'
  const tabName = running.phase_id === 'workflow-builder' ? 'Automation Builder' : phaseName
  return {
    name: tabName,
    metadata: {
      mode: 'workflow',
      phaseId: running.phase_id || undefined,
      phaseName,
      presetQueryId,
    },
    autoActivate: true,
  }
}

import type { RunningWorkflowInfo } from '../../services/api-types'
import type { ChatTab } from '../../stores/useChatStore'
import { isExternalReadOnlyWorkflowSession, isScheduledSession } from '../../utils/workflowSessionKinds'
import { scheduleTabLabel } from '../../utils/scheduleTabLabel'

export interface WorkflowRuntimeTabProjection {
  name: string
  metadata: NonNullable<ChatTab['metadata']>
  autoActivate: boolean
}

/**
 * Apply a live-runtime projection without undoing an explicit user promotion.
 *
 * Runtime discovery owns whether a session is still running. It does not own
 * the presentation of a Schedule/Bot tab after the user has deliberately
 * converted that exact conversation into an interactive Builder chat. The
 * userInteractiveContinuation marker is therefore the higher-precedence fact.
 */
export function reconcileWorkflowRuntimeTab(
  tab: ChatTab,
  projection: WorkflowRuntimeTabProjection,
): ChatTab {
  if (tab.metadata?.userInteractiveContinuation === true) {
    return {
      ...tab,
      metadata: {
        ...tab.metadata,
        mode: 'workflow',
        presetQueryId: projection.metadata.presetQueryId ?? tab.metadata.presetQueryId,
        isViewOnly: false,
        isScheduledRun: false,
        scheduledJobName: undefined,
        isBotRun: false,
        botPlatform: undefined,
        userInteractiveContinuation: true,
      },
    }
  }

  return {
    ...tab,
    name: projection.name,
    metadata: {
      ...tab.metadata,
      ...projection.metadata,
    },
  }
}

/**
 * A Schedule is a first-class parallel lane, not a temporary observer that
 * disappears when Chat becomes active. Keep it in the tab strip until the user
 * closes it. Other read-only lanes retain the old active/running visibility
 * rule so stale bot observations do not accumulate in the toolbar.
 */
export function shouldDisplayWorkflowTab(tab: ChatTab, activeTabId: string | null): boolean {
  if (!tab.metadata?.isViewOnly) return true
  if (tab.metadata?.isScheduledRun) return true
  return tab.tabId === activeTabId || tab.isStreaming || tab.hasRunningBgAgents
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
    const scheduledJobName = running.title || running.preset_name || running.phase_name || 'Schedule'
    return {
      // Label with the schedule's own name: several scheduled runs can be open
      // at once and "Schedule" made them indistinguishable without hovering.
      name: scheduleTabLabel(scheduledJobName),
      metadata: {
        mode: 'workflow',
        presetQueryId,
        isViewOnly: true,
        isScheduledRun: true,
        scheduledJobName,
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

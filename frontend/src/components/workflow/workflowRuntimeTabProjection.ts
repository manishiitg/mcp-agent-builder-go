import type { RunningWorkflowInfo } from '../../services/api-types'
import type { ChatTab } from '../../stores/useChatStore'
import { isExternalReadOnlyWorkflowSession, isScheduledSession } from '../../utils/workflowSessionKinds'

export interface WorkflowRuntimeTabProjection {
  name: string
  metadata: NonNullable<ChatTab['metadata']>
  autoActivate: boolean
}

/**
 * A runtime-projected tab can be discovered after its session has already
 * emitted events. Formatted mode needs an explicit history catch-up before its
 * live SSE subscription can take over; terminal mode has its own retained
 * terminal restoration path.
 */
export function shouldCatchUpRunningWorkflowTranscript(
  viewMode: ChatTab['viewMode'],
  eventCount: number,
): boolean {
  return viewMode === 'formatted' && eventCount === 0
}


/**
 * reusableScheduleTabId finds a finished Schedule lane that a newly-discovered
 * run should take over, instead of opening yet another tab.
 *
 * The scheduler holds a durable per-workflow lease — `runningScheduleInSetLocked`
 * refuses to start a second schedule while one owns the workflow — so at most one
 * scheduled run per workflow exists at a time. The UI keyed its dedupe purely on
 * backend session id, and every run mints a new session, so each run opened a
 * fresh tab and none was ever reclaimed: a workflow scheduled three times a day
 * accumulated a row of identical "Schedule" tabs for runs that had long finished.
 *
 * Only a lane that is genuinely free is reused:
 *  - it belongs to this workflow (same preset), so runs never cross workflows;
 *  - it is still a view-only scheduled run. A tab the user promoted to an
 *    interactive Builder chat is user-owned state and must never be recycled
 *    underneath them (the same precedence reconcileWorkflowRuntimeTab honours);
 *  - its own run is over. A streaming lane is a live run, and the lease means a
 *    new run should not be displacing it.
 */
export function reusableScheduleTabId(
  // Derived from ChatTab rather than restated: a hand-written shape drifted
  // from the real one (ChatTab.sessionId is string | null, not string).
  tabs: Record<string, Pick<ChatTab, 'tabId' | 'sessionId' | 'isStreaming' | 'metadata'>>,
  presetQueryId: string,
  incomingSessionId: string,
): string | null {
  for (const tab of Object.values(tabs)) {
    if (!tab || tab.sessionId === incomingSessionId) continue
    const meta = tab.metadata
    if (!meta || meta.mode !== 'workflow') continue
    if (!meta.isScheduledRun || !meta.isViewOnly) continue
    if (meta.userInteractiveContinuation) continue
    if (meta.presetQueryId !== presetQueryId) continue
    if (tab.isStreaming) continue
    return tab.tabId
  }
  return null
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
 * A tab, once opened, is closed only by the user — never auto-hidden because
 * its run finished or it fell out of focus.
 *
 * This was previously conditional for view-only Schedule lanes: hidden once
 * a run finished unless it was the active tab, streaming, or had running
 * background agents. That auto-hide was itself a reaction to an earlier,
 * unconditional "until the user closes it" version that let finished runs
 * pile up in the strip and reappear on every reload (tabs persist 24h with
 * isStreaming reset to false). Explicit product decision: revert to
 * user-closes-only and accept that a workflow scheduled several times a day
 * accumulates that many tabs until manually closed, including across a
 * reload — see selectDurableChatState's persistence filter, which no longer
 * excludes finished Schedule tabs either.
 */
export function shouldDisplayWorkflowTab(_tab: ChatTab, _activeTabId: string | null): boolean {
  return true
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
      // The tab label shows the actual schedule's name (e.g. "Daily
      // execution") rather than the generic "Schedule" -- with several
      // schedules per workflow, a row of identically-labeled tabs was not
      // distinguishable at a glance. The scheduled/bot "make interactive"
      // icon (WorkflowChatTabs) still signals it's a schedule lane
      // independent of the label.
      name: scheduledJobName,
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

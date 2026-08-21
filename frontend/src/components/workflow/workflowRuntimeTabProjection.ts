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
 * A Schedule lane exists to watch a run that is happening. It is shown while
 * that run is live, and while it is the tab you are actually looking at.
 *
 * It used to be shown unconditionally, "until the user closes it", so that
 * switching to Chat mid-run could not make the lane you were watching vanish.
 * That intent is preserved by the streaming/bg-agent checks below, but the
 * unconditional form kept FINISHED runs in the strip forever — and, because
 * tabs persist for 24h with isStreaming reset to false, brought them back on
 * every reload. The result was a row of dead Schedule tabs for runs that had
 * long since ended. The very same comment guarded bot lanes against exactly
 * that ("so stale bot observations do not accumulate in the toolbar"); a
 * schedule is no different once its run is over.
 *
 * A run you are actively viewing still stays put, because activeTabId wins.
 */
export function shouldDisplayWorkflowTab(tab: ChatTab, activeTabId: string | null): boolean {
  if (!tab.metadata?.isViewOnly) return true
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

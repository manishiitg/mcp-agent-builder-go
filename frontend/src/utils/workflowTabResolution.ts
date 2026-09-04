import type { ChatTab } from '../stores/useChatStore'

/**
 * The one place that decides which workflow tab a backend session lands in.
 *
 * Three callers used to answer "does this running/restored session already
 * have a tab, and if not, do I open one?" on their own: the 10s running-
 * workflow reconciler and the boot/preset-switch reconnect in
 * WorkflowLayout, and the Global Activity Monitor's open-session path in
 * workflowSessionRestore. Each keyed purely on exact backend session id --
 * and every scheduled run mints a fresh session id -- so each run opened a
 * new tab. The one caller that did try to reuse a finished lane keyed that
 * reuse on the *workflow*, not the *schedule*, so a Pulse run would take
 * over a named schedule's tab (its title flipping to "Manual Pulse") and
 * the cron fire would take it back. Keep the invariant here and nowhere
 * else; do not add a fourth caller with its own lookup.
 */

type TabMetadata = NonNullable<ChatTab['metadata']>

// Minted by SchedulerService.newScheduleSessionID (agent_go/cmd/server/
// scheduler.go): "schedule-<trigger>--<schedule id, first 8 chars>_<unixnano>".
// The trigger is "cron" or "manual"; the timestamp is what makes every run a
// new session. The middle segment is the only stable part.
const SCHEDULE_SESSION_ID = /^schedule-[^-]+--(.+)_\d+$/i

/**
 * The schedule a session belongs to, independent of which run it is or what
 * triggered it. A cron fire and a "Run now" of the same schedule share a
 * lane; two different schedules of one workflow do not; the toolbar's
 * one-off Pulse ("manual-pulse" -> "manual-p") is its own lane. Null for
 * anything that isn't a scheduler-minted session.
 */
export function scheduleLaneKey(sessionId?: string | null): string | null {
  const match = SCHEDULE_SESSION_ID.exec((sessionId || '').trim())
  return match ? match[1].toLowerCase() : null
}

/**
 * A finished tab of the *same schedule* that a newly-discovered run should
 * take over instead of opening another tab.
 *
 * The scheduler holds a per-workflow lease (`runningScheduleInSetLocked`),
 * so at most one scheduled run per workflow is live at a time -- but a
 * workflow can have several schedules plus one-off Pulse runs, and their
 * finished tabs all sit in the strip together. Only a lane that is
 * genuinely this run's own is reused:
 *  - same schedule (lane key), so Pulse never hijacks a named schedule's
 *    tab and two schedules never fight over one;
 *  - same workflow (preset), so runs never cross workflows;
 *  - still a view-only scheduled run. A tab the user promoted to an
 *    interactive Builder chat is user-owned and never recycled;
 *  - its own run is over. A streaming lane is a live run.
 */
export function reusableScheduleTabId(
  tabs: Record<string, Pick<ChatTab, 'tabId' | 'sessionId' | 'isStreaming' | 'metadata'>>,
  presetQueryId: string,
  incomingSessionId: string,
): string | null {
  const lane = scheduleLaneKey(incomingSessionId)
  if (!lane) return null
  for (const tab of Object.values(tabs)) {
    if (!tab || tab.sessionId === incomingSessionId) continue
    const meta = tab.metadata
    if (!meta || meta.mode !== 'workflow') continue
    if (!meta.isScheduledRun || !meta.isViewOnly) continue
    if (meta.userInteractiveContinuation) continue
    if (meta.presetQueryId !== presetQueryId) continue
    if (tab.isStreaming) continue
    if (scheduleLaneKey(tab.sessionId) !== lane) continue
    return tab.tabId
  }
  return null
}

/** Whether an existing tab is the same *kind* of lane the session is. A
 * scheduled run and the Builder child execution it spawns can share a
 * session id; the schedule tab must not be mistaken for the chat, or vice
 * versa. */
function sameLaneKind(tab: Pick<ChatTab, 'metadata'>, metadata: TabMetadata): boolean {
  const meta = tab.metadata
  if (!meta) return false
  if (metadata.isScheduledRun) return meta.isScheduledRun === true
  if (metadata.isBotRun) return meta.isBotRun === true
  return !meta.isViewOnly
}

export type WorkflowTabResolution = {
  tabId: string
  /** existing: a tab already bound to this session. lane: took over a
   * finished tab of the same schedule. created: opened a new tab. */
  via: 'existing' | 'lane' | 'created'
}

export interface ResolveWorkflowTabArgs {
  /** Read live, not a snapshot: the exact-session check must see tabs the
   * previous await created, or two discoverers of one session both open
   * a tab for it. */
  getTabs: () => Record<string, ChatTab>
  presetQueryId: string
  sessionId: string
  name: string
  metadata: TabMetadata
  createChatTab: (name: string, metadata: TabMetadata, sessionId: string) => Promise<string>
  updateTabSessionId: (tabId: string, sessionId: string) => void
}

/**
 * Resolve the tab for a workflow session: an existing tab bound to it, else
 * (for scheduled runs) a finished tab of the same schedule to take over,
 * else a new tab.
 */
export async function resolveWorkflowTabForSession(args: ResolveWorkflowTabArgs): Promise<WorkflowTabResolution> {
  const tabs = args.getTabs()
  const boundToSession = Object.values(tabs).filter(tab =>
    tab.metadata?.mode === 'workflow' && tab.sessionId === args.sessionId,
  )
  const existing = boundToSession.find(tab => sameLaneKind(tab, args.metadata)) ?? boundToSession[0]
  if (existing) return { tabId: existing.tabId, via: 'existing' }

  if (args.metadata.isScheduledRun) {
    const laneTabId = reusableScheduleTabId(tabs, args.presetQueryId, args.sessionId)
    if (laneTabId) {
      args.updateTabSessionId(laneTabId, args.sessionId)
      return { tabId: laneTabId, via: 'lane' }
    }
  }

  const tabId = await args.createChatTab(args.name, args.metadata, args.sessionId)
  return { tabId, via: 'created' }
}

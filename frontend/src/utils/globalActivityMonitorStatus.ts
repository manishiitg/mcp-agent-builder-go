import type { ActiveSessionInfo, RunningWorkflowInfo } from '../services/api-types'
import {
  hasIdleAliveCodingAgent,
  hasLiveBackgroundAgents,
  isVisibleActivitySession,
  normalizedActivityStatus,
} from './activitySessions'
import { runtimeNeedsUserInput, sessionRuntimeStatus } from './runtimeActivity'
import { isInternalChildSession } from './workflowSessionKinds'

// The global monitor is the user's live activity indicator. A 30-second cache
// made a newly-started schedule look idle for an entire turn of attention even
// though the backend had already marked it busy. Keep this bounded and explicit;
// the monitor force-refreshes on this cadence while other consumers may retain
// the broader active-session cache for cheaper background checks.
export const GLOBAL_ACTIVITY_REFRESH_MS = 5_000

function isWorkflowSession(session: ActiveSessionInfo): boolean {
  return session.agent_mode?.toLowerCase().includes('workflow') ?? false
}

export function headerStatusLabel(session: ActiveSessionInfo, workflow?: RunningWorkflowInfo): string {
  if (runtimeNeedsUserInput(session)) return 'waiting for input'
  const hasBackgroundAgents = hasLiveBackgroundAgents(session)
  if (session.runtime_state) {
    const status = sessionRuntimeStatus(session)
    if (status === 'idle') return hasBackgroundAgents ? 'waiting for background agents' : 'idle'
    // A completed foreground turn can retain its coding CLI tmux so the user
    // can continue the conversation. The authoritative runtime is correctly
    // stopped, but the retained pane is idle—not active work. Resolve this
    // before the generic stopped branch so the header renders a clock instead
    // of a spinner. A genuinely busy runtime still falls through to running.
    if (status === 'stopped' && hasIdleAliveCodingAgent(session)) return 'idle'
    if (status === 'stopped') return 'stopped'
    return hasBackgroundAgents && !session.runtime_state.foreground_turn.busy
      ? 'background running'
      : 'running'
  }
  const status = normalizedActivityStatus(workflow?.status || session.status)
  if (status === 'paused') return 'paused'
  if (status === 'idle') return 'idle'
  if ((status === 'waiting' || status === 'waiting_feedback') && hasBackgroundAgents) return 'waiting for background agents'
  if (status === 'waiting' || status === 'waiting_feedback') return 'waiting'
  if ((status === 'completed' || status === 'idle') && hasBackgroundAgents) return 'background running'
  // Idle-but-alive coding CLI (backend marked it completed once the turn ended,
  // but the tmux agent is still up waiting for input): show it as idle (clock),
  // never as a spinner. A genuinely-running session keeps status "running".
  if ((status === 'completed' || status === 'idle') && hasIdleAliveCodingAgent(session)) return 'idle'
  if (status === 'completed' && isWorkflowSession(session)) return 'idle'
  return status || 'running'
}

// The narrow slice of a ChatTab this needs. Kept local instead of importing
// the store's ChatTab type to avoid coupling this pure-logic module to
// useChatStore.
interface ActiveTabForCurrentSession {
  sessionId: string | null
  metadata?: { mode?: string }
}

/**
 * Identifies which session, if any, the user is currently looking at, so a
 * header component can render its live status.
 *
 * This must stay the single definition of "current session": before it
 * existed, GlobalActivityMonitor computed this inline to exclude the current
 * session from its pills, and the header's plain workflow-name selector had
 * no equivalent — so a session could be visibly running while the header's
 * live-status pills genuinely never showed it anywhere, on the theory that
 * the plain selector already covered it. It never carried live status.
 * Two independent inline copies of this same lookup is exactly the failure
 * mode that produced that gap; callers must share this one.
 */
export function currentSessionId(
  activeTabId: string | null,
  chatTabs: Record<string, ActiveTabForCurrentSession>,
  selectedModeCategory: string | null,
  isOrganizationView: boolean,
): string | null {
  if (isOrganizationView || !activeTabId) return null
  const activeTab = chatTabs[activeTabId]
  if (!activeTab || activeTab.metadata?.mode !== selectedModeCategory) return null
  return activeTab.sessionId ?? null
}

/**
 * Resolves the full ActiveSessionInfo for currentSessionId(...), applying the
 * same visibility rules GlobalActivityMonitor's pill list applies, so a
 * session that would never render as a pill (internal child, not otherwise
 * visible) also does not render a spurious status on the header selector.
 */
export function currentActiveSession(
  activeSessionsCache: ActiveSessionInfo[],
  sessionId: string | null,
): ActiveSessionInfo | null {
  if (!sessionId) return null
  return activeSessionsCache.find(session =>
    session.session_id === sessionId &&
    !isInternalChildSession({ parentSessionId: session.parent_session_id, sessionKind: session.session_kind }) &&
    isVisibleActivitySession(session)
  ) ?? null
}

/**
 * The global monitor renders every visible session, including the one currently
 * open in the chat surface. A workflow may have an interactive chat and a
 * scheduled run at the same time; they have independent session IDs,
 * lifecycles, and destinations. Collapsing them by workspace hides real work.
 *
 * The current session used to be excluded here, because the workflow selector
 * carried its own live status indicator and showing both was duplication. That
 * indicator has been removed — run status belongs in one place — so excluding
 * the current session left it with no status anywhere: a running workflow you
 * were looking at appeared to be doing nothing at all. The monitor is now that
 * one place, so it must be complete.
 */
export function visibleActivitySessions(
  activeSessions: ActiveSessionInfo[],
  _currentSessionId: string | null,
): ActiveSessionInfo[] {
  return activeSessions
}

export function statusTone(
  session: ActiveSessionInfo,
  workflow?: RunningWorkflowInfo,
): 'running' | 'needs-input' | 'paused' | 'background' | 'idle' {
  const status = headerStatusLabel(session, workflow)
  if (status === 'waiting for input') return 'needs-input'
  if (status === 'idle' || status === 'waiting') return 'idle'
  if (status === 'paused') return 'paused'
  if (status === 'background running' || status === 'waiting for background agents') return 'background'
  return 'running'
}

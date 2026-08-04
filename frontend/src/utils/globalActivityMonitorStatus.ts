import type { ActiveSessionInfo, RunningWorkflowInfo } from '../services/api-types'
import {
  hasIdleAliveCodingAgent,
  hasLiveBackgroundAgents,
  isVisibleActivitySession,
  normalizedActivityStatus,
} from './activitySessions'
import { runtimeNeedsUserInput, sessionRuntimeStatus } from './runtimeActivity'
import { isInternalChildSession } from './workflowSessionKinds'

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
 * Identifies which workflow a session belongs to, for de-duplicating pills
 * that point at the same workflow from different sessions — e.g. an open
 * chat tab and a background Pulse schedule for that same workflow.
 */
export function sessionWorkflowKey(session: ActiveSessionInfo): string {
  return session.workflow_name || session.workflow_label || session.workspace_path || ''
}

/**
 * The pills GlobalActivityMonitor should render: every visible session
 * except the current one, with same-workflow siblings collapsed to the most
 * useful row.
 *
 * Excluding the current session by session_id alone left a real gap: a
 * sibling session for the SAME workflow (a background Pulse schedule, a
 * second observing tab) has a different session_id, so it survived into the
 * pill list while the header's current-workflow selector also showed that
 * workflow's status — the one workflow rendered twice at once, violating
 * "the selected running workflow must appear exactly once". Excluding by the
 * current session's workflow identity, not just its id, is what actually
 * satisfies that.
 */
export function visibleActivitySessions(
  activeSessions: ActiveSessionInfo[],
  currentSessionId: string | null,
  currentSessionWorkflowKey: string,
): ActiveSessionInfo[] {
  const filtered = activeSessions.filter(session => {
    if (session.session_id === currentSessionId) return false
    if (currentSessionWorkflowKey && isWorkflowSession(session) && sessionWorkflowKey(session) === currentSessionWorkflowKey) {
      return false
    }
    return true
  })

  const byWorkflow = new Map<string, ActiveSessionInfo>()
  const nonWorkflow: ActiveSessionInfo[] = []
  const rank = (s: ActiveSessionInfo) => {
    const st = sessionRuntimeStatus(s)
    let score = 0
    if (st === 'busy') score += 30
    if (st === 'idle') score += 10
    if (hasLiveBackgroundAgents(s)) score += 20
    if (runtimeNeedsUserInput(s)) score += 15
    if (s.has_retained_tmux_session) score += 50
    return score
  }
  const timestamp = (s: ActiveSessionInfo) => Date.parse(s.last_activity || s.created_at || '') || 0

  for (const session of filtered) {
    const key = isWorkflowSession(session) ? sessionWorkflowKey(session) : ''
    if (!key) {
      nonWorkflow.push(session)
      continue
    }
    const existing = byWorkflow.get(key)
    if (!existing) {
      byWorkflow.set(key, session)
      continue
    }
    const rankDelta = rank(session) - rank(existing)
    if (rankDelta > 0 || (rankDelta === 0 && timestamp(session) > timestamp(existing))) {
      byWorkflow.set(key, session)
    }
  }

  return [...byWorkflow.values(), ...nonWorkflow]
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

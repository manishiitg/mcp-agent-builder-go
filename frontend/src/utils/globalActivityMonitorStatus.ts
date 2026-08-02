import type { ActiveSessionInfo, RunningWorkflowInfo } from '../services/api-types'
import {
  hasIdleAliveCodingAgent,
  hasLiveBackgroundAgents,
  normalizedActivityStatus,
} from './activitySessions'
import { runtimeNeedsUserInput, sessionRuntimeStatus } from './runtimeActivity'

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

import { createRequestCoalescer } from '../services/requestCoalescer'

export type LiveInputSubmissionCoordinator = <T>(
  sessionKey: string,
  message: string,
  submit: () => Promise<T>,
) => Promise<T>

// A rapid Enter/double-click can invoke ChatInput twice before the first
// /live-input response clears the draft. Share the complete submission promise
// for that exact session + message so the HTTP mutation, optimistic event, and
// fallback decision all execute once. A later intentional repeat is allowed
// after the first submission settles.
export function createLiveInputSubmissionCoordinator(): LiveInputSubmissionCoordinator {
  const coalesce = createRequestCoalescer()
  return <T>(sessionKey: string, message: string, submit: () => Promise<T>) => {
    const key = JSON.stringify([sessionKey, message.trim()])
    return coalesce(key, submit)
  }
}

export const liveInputSubmissionCoordinator = createLiveInputSubmissionCoordinator()

export interface RetainedLiveInputDecision {
  requested: boolean
  fullTurnStreaming: boolean
  turnIsStreaming: boolean
  hasSession: boolean
  hasOneShotContext: boolean
}

// Product surfaces still use native tmux live input to steer a turn that is
// currently running. Once that turn is idle, however, the next user message
// starts a normal server-owned turn so the shared AgentWorks pipeline can emit
// structured progress and a definitive completion event.
export function shouldUseRetainedLiveInput({
  requested,
  fullTurnStreaming,
  turnIsStreaming,
  hasSession,
  hasOneShotContext,
}: RetainedLiveInputDecision): boolean {
  if (!requested || !hasSession || hasOneShotContext) return false
  return !fullTurnStreaming || turnIsStreaming
}

// Product conversations deliberately reattach their SSE stream at the start of
// every accepted message. A retained tmux process can outlive the EventSource
// that observed its previous turn (for example after HMR, sleep, or a completed
// turn). Merely finding an old connection object in Zustand is therefore not
// enough to guarantee that the next completion reaches the clean transcript.
// AgentWorks' default surface keeps its existing connect-only-when-missing
// behavior; the stricter rule is scoped to full-turn product surfaces.
export function shouldRefreshSessionEventStream(
  fullTurnStreaming: boolean,
  hasConnection: boolean,
): boolean {
  return fullTurnStreaming || !hasConnection
}

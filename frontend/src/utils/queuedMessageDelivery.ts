/**
 * How a queued message should reach an agent whose turn is still running.
 *
 * There are two live-delivery mechanisms and they are not interchangeable:
 *
 * - `live-query` — POST /api/query with preferLiveInput. Single-entry routing
 *   for tmux-transport coding CLIs and for workflow chats: the backend tries the
 *   minimal live-input path first and falls back to a full resume/new turn when
 *   the retained CLI is gone. This is what ChatInput does with typed text.
 * - `steer` — agentApi.sendLiveInput, injecting into an in-flight API-provider
 *   turn. This is what the steer button on a queued chip does. Deliberately not
 *   used for coding CLIs, which route through /api/query instead.
 * - `wait` — no live path; the queue drain sends it once the turn ends.
 *
 * The mapping mirrors ChatInput's own `routeLiveInputToCLI` and `canShowSteer`,
 * and exists so the two cannot drift: a message that arrives in the queue from
 * somewhere other than the input box ("ask in chat" on a pending decision, for
 * one) must be delivered the same way typing it would have been.
 *
 * Auto-notifications are never delivered mid-turn. Interrupting a running agent
 * with step-completion noise is not worth it, and they lose nothing by waiting.
 */
export type QueuedMessageRoute = 'live-query' | 'steer' | 'wait'

export function routeForQueuedMessage(params: {
  isStreaming: boolean
  hasSession: boolean
  isWorkflowMode: boolean
  isTmuxCLIProvider: boolean
  canSteer: boolean
}): QueuedMessageRoute {
  const { isStreaming, hasSession, isWorkflowMode, isTmuxCLIProvider, canSteer } = params
  // Only the mid-turn case is decided here. An idle chat is the queue drain's job.
  if (!isStreaming || !hasSession) return 'wait'
  if (isTmuxCLIProvider || isWorkflowMode) return 'live-query'
  if (canSteer) return 'steer'
  return 'wait'
}

/** Auto-notifications wait for idle; everything else is a person expecting an answer. */
export function splitQueuedMessages(messages: string[], autoNotificationPrefix: string): {
  human: string[]
  auto: string[]
} {
  return {
    human: messages.filter(message => !message.startsWith(autoNotificationPrefix)),
    auto: messages.filter(message => message.startsWith(autoNotificationPrefix)),
  }
}

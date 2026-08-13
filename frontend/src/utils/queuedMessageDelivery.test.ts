import { describe, expect, it } from 'vitest'
import { routeForQueuedMessage, splitQueuedMessages } from './queuedMessageDelivery'

const AUTO = '[AUTO-NOTIFICATION]'

const running = {
  isStreaming: true,
  hasSession: true,
  isWorkflowMode: false,
  isTmuxCLIProvider: false,
  canSteer: false,
}

describe('routeForQueuedMessage', () => {
  it('sends a workflow chat through /api/query, the way typing does', () => {
    // The reported bug: "ask in chat" on a pending decision parked as a chip on
    // a running claudecode workflow session and never arrived.
    expect(routeForQueuedMessage({ ...running, isWorkflowMode: true })).toBe('live-query')
  })

  it('sends a coding CLI through /api/query even outside workflow mode', () => {
    // ChatInput's single-entry rule: tmux transport always goes to /api/query,
    // never to the steer endpoint.
    expect(routeForQueuedMessage({ ...running, isTmuxCLIProvider: true, canSteer: true })).toBe('live-query')
  })

  it('steers an API provider that has a live turn', () => {
    expect(routeForQueuedMessage({ ...running, canSteer: true })).toBe('steer')
  })

  it('waits when an API provider has no live turn to steer', () => {
    expect(routeForQueuedMessage(running)).toBe('wait')
  })

  it('waits when idle — that is the drain\'s job, not a live path', () => {
    expect(routeForQueuedMessage({ ...running, isStreaming: false, isWorkflowMode: true })).toBe('wait')
  })

  it('waits with no session to inject into', () => {
    expect(routeForQueuedMessage({ ...running, hasSession: false, isWorkflowMode: true })).toBe('wait')
  })
})

describe('splitQueuedMessages', () => {
  it('separates people from step-completion noise', () => {
    const { human, auto } = splitQueuedMessages(
      [`${AUTO} step 3 finished`, 'I want to discuss a pending decision', `${AUTO} step 4 finished`],
      AUTO,
    )
    expect(human).toEqual(['I want to discuss a pending decision'])
    expect(auto).toHaveLength(2)
  })

  it('handles a queue with nothing in it', () => {
    expect(splitQueuedMessages([], AUTO)).toEqual({ human: [], auto: [] })
  })
})

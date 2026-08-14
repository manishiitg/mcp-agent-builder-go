import { describe, expect, it } from 'vitest'

// The rule both ChatArea and ChatInput now apply, kept as a plain function so
// the intent is testable without mounting either component.
function isTurnInFlight(tab: { isStreaming?: boolean; hasRunningBgAgents?: boolean }): boolean {
  return !!tab.isStreaming || !!tab.hasRunningBgAgents
}

describe('busy indicator gating', () => {
  // isStreaming follows the server's can_steer, which falls through to a tmux
  // busy-content heuristic and flips as the pane's output starts and stalls.
  // Anything mounted on it alone unmounts and remounts during a normal run —
  // measured at 13 mount/unmount pairs in 6 seconds.
  it('stays true across a can_steer flap while background agents run', () => {
    const steerable = { isStreaming: true, hasRunningBgAgents: true }
    const notSteerable = { isStreaming: false, hasRunningBgAgents: true }
    expect(isTurnInFlight(steerable)).toBe(true)
    expect(isTurnInFlight(notSteerable)).toBe(true)
  })

  it('is true for an ordinary foreground turn with no background agents', () => {
    expect(isTurnInFlight({ isStreaming: true, hasRunningBgAgents: false })).toBe(true)
  })

  // Once the run genuinely ends both inputs go false, so the indicator must
  // disappear — a busy indicator that outlives the turn is its own bug.
  it('is false once nothing is running', () => {
    expect(isTurnInFlight({ isStreaming: false, hasRunningBgAgents: false })).toBe(false)
    expect(isTurnInFlight({})).toBe(false)
  })
})

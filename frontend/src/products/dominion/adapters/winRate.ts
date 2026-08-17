import type { TradeOutcome, WinRateSummary } from '../types'

// Denominator excludes no_fill/open/retired -- those never resolved to a
// win/loss/flat outcome, so folding them into the rate would understate it.
// Kept in its own module with no agentApi import (unlike outcomes.ts) so it
// stays trivially unit-testable, mirroring finance/synthesize.ts.
export function computeWinRate(outcomes: TradeOutcome[]): WinRateSummary {
  let wins = 0
  let losses = 0
  let flat = 0
  let sumR = 0
  for (const outcome of outcomes) {
    if (outcome.result === 'win') wins += 1
    else if (outcome.result === 'loss') losses += 1
    else if (outcome.result === 'flat') flat += 1
    if (outcome.rMultiple != null) sumR += outcome.rMultiple
  }
  const decided = wins + losses + flat
  return {
    wins,
    losses,
    flat,
    decided,
    winRatePct: decided > 0 ? (wins / decided) * 100 : null,
    sumR,
  }
}

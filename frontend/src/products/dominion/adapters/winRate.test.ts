import { describe, expect, it } from 'vitest'
import { computeWinRate } from './winRate'
import type { TradeOutcome } from '../types'

function outcome(overrides: Partial<TradeOutcome>): TradeOutcome {
  return {
    symbol: 'NVDA',
    runDate: '2026-08-15',
    direction: 'long',
    result: 'win',
    rMultiple: 1,
    entry: 100,
    exitPrice: 101,
    exitTime: '2026-08-15T14:00:00Z',
    note: '',
    ...overrides,
  }
}

describe('computeWinRate', () => {
  it('returns null win rate and zero sums for an empty list', () => {
    const summary = computeWinRate([])
    expect(summary.decided).toBe(0)
    expect(summary.winRatePct).toBeNull()
    expect(summary.sumR).toBe(0)
  })

  it('excludes no_fill, open, and retired from the decided denominator', () => {
    const summary = computeWinRate([
      outcome({ result: 'win', rMultiple: 1 }),
      outcome({ result: 'no_fill', rMultiple: null }),
      outcome({ result: 'open', rMultiple: null }),
      outcome({ result: 'retired', rMultiple: null }),
    ])
    expect(summary.decided).toBe(1)
    expect(summary.winRatePct).toBe(100)
  })

  it('computes win rate from wins/losses/flat only', () => {
    const summary = computeWinRate([
      outcome({ result: 'win', rMultiple: 1.5 }),
      outcome({ result: 'win', rMultiple: 0.8 }),
      outcome({ result: 'loss', rMultiple: -1 }),
      outcome({ result: 'flat', rMultiple: 0 }),
    ])
    expect(summary.wins).toBe(2)
    expect(summary.losses).toBe(1)
    expect(summary.flat).toBe(1)
    expect(summary.decided).toBe(4)
    expect(summary.winRatePct).toBe(50)
    expect(summary.sumR).toBeCloseTo(1.3)
  })
})

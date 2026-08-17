import { describe, expect, it } from 'vitest'
import { groupBySymbol } from './stockGroups'
import type { OpenPosition, TradeIdea, TradeOutcome, WatchlistItem } from './types'

function idea(overrides: Partial<TradeIdea>): TradeIdea {
  return {
    symbol: 'NVDA', runDate: '2026-08-15', createdAt: '2026-08-15T14:00:00Z', conviction: 70,
    direction: 'long', entry: 100, stop: 95, target: 110, rr: 2, rationale: 'test', ...overrides,
  }
}

function outcome(overrides: Partial<TradeOutcome>): TradeOutcome {
  return {
    symbol: 'NVDA', runDate: '2026-08-15', direction: 'long', result: 'win', rMultiple: 1,
    entry: 100, exitPrice: 101, exitTime: '2026-08-15T15:00:00Z', note: '', ...overrides,
  }
}

describe('groupBySymbol', () => {
  it('orders watchlist symbols first, in watchlist order', () => {
    const watchlist: WatchlistItem[] = [{ symbol: 'TSLA', tier: 'large' }, { symbol: 'NVDA', tier: 'large' }]
    const groups = groupBySymbol(watchlist, [], [], [])
    expect(groups.map((g) => g.symbol)).toEqual(['TSLA', 'NVDA'])
  })

  it('appends symbols absent from the watchlist, sorted alphabetically, after watchlist symbols', () => {
    const watchlist: WatchlistItem[] = [{ symbol: 'NVDA', tier: 'large' }]
    const positions: OpenPosition[] = [{ symbol: 'ZOOM', qty: 1, avgEntryPrice: 10, unrealizedPl: 0 }]
    const outcomes: TradeOutcome[] = [outcome({ symbol: 'AAPL' })]
    const groups = groupBySymbol(watchlist, positions, [], outcomes)
    expect(groups.map((g) => g.symbol)).toEqual(['NVDA', 'AAPL', 'ZOOM'])
  })

  it('attaches tier, position, idea, and outcomes to the matching symbol only', () => {
    const watchlist: WatchlistItem[] = [{ symbol: 'NVDA', tier: 'large' }, { symbol: 'TSLA', tier: 'large' }]
    const positions: OpenPosition[] = [{ symbol: 'NVDA', qty: 10, avgEntryPrice: 100, unrealizedPl: 50 }]
    const ideas: TradeIdea[] = [idea({ symbol: 'NVDA' })]
    const outcomes: TradeOutcome[] = [outcome({ symbol: 'NVDA' })]
    const groups = groupBySymbol(watchlist, positions, ideas, outcomes)

    const nvda = groups.find((g) => g.symbol === 'NVDA')!
    expect(nvda.tier).toBe('large')
    expect(nvda.position?.qty).toBe(10)
    expect(nvda.idea?.conviction).toBe(70)
    expect(nvda.recentOutcomes).toHaveLength(1)

    const tsla = groups.find((g) => g.symbol === 'TSLA')!
    expect(tsla.position).toBeNull()
    expect(tsla.idea).toBeNull()
    expect(tsla.recentOutcomes).toHaveLength(0)
  })

  it('caps recentOutcomes per symbol at 3, keeping the first N in input order', () => {
    const watchlist: WatchlistItem[] = [{ symbol: 'NVDA', tier: 'large' }]
    const outcomes: TradeOutcome[] = [
      outcome({ runDate: '2026-08-15' }),
      outcome({ runDate: '2026-08-14' }),
      outcome({ runDate: '2026-08-13' }),
      outcome({ runDate: '2026-08-12' }),
    ]
    const groups = groupBySymbol(watchlist, [], [], outcomes)
    expect(groups[0].recentOutcomes).toHaveLength(3)
    expect(groups[0].recentOutcomes.map((o) => o.runDate)).toEqual(['2026-08-15', '2026-08-14', '2026-08-13'])
  })
})

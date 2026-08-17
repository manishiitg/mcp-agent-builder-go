import { describe, expect, it } from 'vitest'
import { combineAttentionItems, computeNetWorth, findStaleSources, mergeRecentActivity } from './synthesize'
import type { AttentionItem, BankAccountCard, InvestmentAccountCard, RecentTransaction } from './types'

describe('computeNetWorth', () => {
  it('sums liquid, locked, and invested across sources into one total', () => {
    const banks: BankAccountCard[] = [
      { source: 'HDFC', accountLabel: 'A', currentBalance: 100, fixedDeposit: 50, lastUpdated: '2026-01-01' },
      { source: 'ICICI', accountLabel: 'B', currentBalance: 200, fixedDeposit: 0, lastUpdated: '2026-01-01' },
    ]
    const investments: InvestmentAccountCard[] = [
      { source: 'Mutual Fund', accountLabel: 'C', totalCurrentValue: 500, totalInvested: 400, holdings: [], lastUpdated: '2026-01-01' },
    ]
    expect(computeNetWorth(banks, investments)).toEqual({
      liquid: 300, locked: 50, invested: 500, total: 850,
    })
  })

  it('returns all zeros for no data rather than throwing', () => {
    expect(computeNetWorth([], [])).toEqual({ liquid: 0, locked: 0, invested: 0, total: 0 })
  })
})

describe('mergeRecentActivity', () => {
  it('merges multiple sources into one chronological (newest-first) list', () => {
    const icici: RecentTransaction[] = [
      { source: 'ICICI', date: '2026-01-05', description: 'a', amount: 10 },
    ]
    const mf: RecentTransaction[] = [
      { source: 'Mutual Fund', date: '2026-01-10', description: 'b', amount: 20 },
      { source: 'Mutual Fund', date: '2026-01-01', description: 'c', amount: 30 },
    ]
    const merged = mergeRecentActivity(icici, mf)
    expect(merged.map((t) => t.date)).toEqual(['2026-01-10', '2026-01-05', '2026-01-01'])
  })

  it('drops entries with no date rather than sorting them arbitrarily', () => {
    const withMissingDate: RecentTransaction[] = [{ source: 'ICICI', date: '', description: 'x', amount: 1 }]
    expect(mergeRecentActivity(withMissingDate)).toEqual([])
  })
})

describe('combineAttentionItems', () => {
  it('sorts overdue items ahead of merely pending ones', () => {
    const items: AttentionItem[] = [
      { source: 'Tax', severity: 'pending', label: 'pending notice' },
      { source: 'GST', severity: 'overdue', label: 'overdue return' },
    ]
    const combined = combineAttentionItems(items)
    expect(combined[0].severity).toBe('overdue')
    expect(combined[1].severity).toBe('pending')
  })
})

describe('findStaleSources', () => {
  it('flags a source far older than the freshest one', () => {
    const stale = findStaleSources([
      { group: 'Bank', source: 'Fresh', lastUpdated: '2026-08-16T00:00:00Z' },
      { group: 'Bank', source: 'Stale', lastUpdated: '2026-07-01T00:00:00Z' },
    ])
    expect(stale.map((s) => s.source)).toEqual(['Stale'])
  })

  it('flags nothing when all sources are close together', () => {
    const stale = findStaleSources([
      { group: 'Bank', source: 'A', lastUpdated: '2026-08-16T00:00:00Z' },
      { group: 'Bank', source: 'B', lastUpdated: '2026-08-15T00:00:00Z' },
    ])
    expect(stale).toEqual([])
  })

  it('compares staleness within a group only, not across unrelated domains', () => {
    // A bank account synced today should not make a tax check from two
    // weeks ago look "stale" -- they have inherently different cadences.
    const stale = findStaleSources([
      { group: 'Bank', source: 'Fresh Bank', lastUpdated: '2026-08-16T00:00:00Z' },
      { group: 'Tax', source: 'Normal Tax Check', lastUpdated: '2026-08-02T00:00:00Z' },
    ])
    expect(stale).toEqual([])
  })

  it('ignores unparseable timestamps rather than crashing', () => {
    expect(() => findStaleSources([{ group: 'Bank', source: 'Bad', lastUpdated: 'not a date' }])).not.toThrow()
  })
})

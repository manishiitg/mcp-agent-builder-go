import type {
  AttentionItem,
  BankAccountCard,
  GstCard,
  InvestmentAccountCard,
  NetWorthSummary,
  RecentTransaction,
  TaxCard,
} from './types'

export function computeNetWorth(
  bankAccounts: BankAccountCard[],
  investmentAccounts: InvestmentAccountCard[]
): NetWorthSummary {
  const liquid = bankAccounts.reduce((sum, a) => sum + a.currentBalance, 0)
  const locked = bankAccounts.reduce((sum, a) => sum + a.fixedDeposit, 0)
  const invested = investmentAccounts.reduce((sum, a) => sum + a.totalCurrentValue, 0)
  return { liquid, locked, invested, total: liquid + locked + invested }
}

// A single, chronologically merged view across every source that can
// produce real transaction-level rows. HDFC never appears here -- it has
// no transaction rows to merge, only a count (see BankAccountCard).
export function mergeRecentActivity(...sources: RecentTransaction[][]): RecentTransaction[] {
  return sources
    .flat()
    .filter((t) => t.date)
    .sort((a, b) => (a.date < b.date ? 1 : a.date > b.date ? -1 : 0))
}

export function combineAttentionItems(...sources: AttentionItem[][]): AttentionItem[] {
  // Defensive dedup: the same obligation can legitimately appear in a
  // source more than once (confirmed real -- GST's own snapshot history
  // re-observes the same unfiled return at each check-in; the adapter now
  // reads only the latest snapshot, but this is a second layer against any
  // other source doing the same thing unnoticed).
  const seen = new Set<string>()
  const items: AttentionItem[] = []
  for (const item of sources.flat()) {
    const key = `${item.source}|${item.label}|${item.dueDate ?? ''}`
    if (seen.has(key)) continue
    seen.add(key)
    items.push(item)
  }
  const severityRank: Record<AttentionItem['severity'], number> = { overdue: 0, pending: 1 }
  return items.sort((a, b) => severityRank[a.severity] - severityRank[b.severity])
}

// Each source's own freshness field, named differently everywhere. A
// timestamp meaningfully older than the freshest one WITHIN THE SAME
// DOMAIN is worth flagging. Comparing across domains is deliberately not
// done: a bank sync and a tax-notice check have inherently different
// cadences (confirmed real -- comparing all sources on one scale flagged
// nearly every source as "stale" simply because they don't sync at the
// same frequency, which is not the same thing as any one of them being
// overdue for a refresh).
export type FreshnessEntry = { group: string; source: string; lastUpdated: string }

export function findStaleSources(entries: FreshnessEntry[], staleAfterDays = 14): FreshnessEntry[] {
  const byGroup = new Map<string, Array<FreshnessEntry & { time: number }>>()
  for (const entry of entries) {
    const time = Date.parse(entry.lastUpdated)
    if (!Number.isFinite(time)) continue
    const bucket = byGroup.get(entry.group) ?? []
    bucket.push({ ...entry, time })
    byGroup.set(entry.group, bucket)
  }
  const staleMs = staleAfterDays * 24 * 60 * 60 * 1000
  const stale: FreshnessEntry[] = []
  for (const bucket of byGroup.values()) {
    const freshest = Math.max(...bucket.map((e) => e.time))
    for (const entry of bucket) {
      if (freshest - entry.time > staleMs) stale.push({ group: entry.group, source: entry.source, lastUpdated: entry.lastUpdated })
    }
  }
  return stale
}

export function buildFreshnessEntries(
  bankAccounts: BankAccountCard[],
  investmentAccounts: InvestmentAccountCard[],
  tax: TaxCard[],
  gst: GstCard[]
): FreshnessEntry[] {
  return [
    ...bankAccounts.map((a) => ({ group: a.source, source: a.accountLabel, lastUpdated: a.lastUpdated })),
    ...investmentAccounts.map((a) => ({ group: 'Mutual Fund', source: a.accountLabel, lastUpdated: a.lastUpdated })),
    ...tax.map((t) => ({ group: 'Tax', source: t.pan, lastUpdated: t.lastChecked })),
    ...gst.map((g) => ({ group: 'GST', source: g.gstin, lastUpdated: g.lastUpdated })),
  ].filter((e) => e.lastUpdated)
}

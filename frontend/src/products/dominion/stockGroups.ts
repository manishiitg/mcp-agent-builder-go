import type { OpenPosition, TradeIdea, TradeOutcome, WatchlistItem, WatchlistTier } from './types'

export type StockGroup = {
  symbol: string
  tier: WatchlistTier | null
  position: OpenPosition | null
  idea: TradeIdea | null
  recentOutcomes: TradeOutcome[]
}

const MAX_OUTCOMES_PER_SYMBOL = 3

// Unifies four independently-loaded lists into one card per symbol, so the
// dashboard reads "here's everything about NVDA" instead of forcing the
// user to cross-reference four separate tables by eye. Symbol order follows
// the watchlist first (the user's own ordering), then any symbol that only
// appears in trade history (e.g. a since-removed watchlist symbol with an
// open position or recent grade) appended after, sorted alphabetically so
// that tail stays stable rather than jumping around by table-scan order.
export function groupBySymbol(
  watchlist: WatchlistItem[],
  positions: OpenPosition[],
  ideas: TradeIdea[],
  outcomes: TradeOutcome[],
): StockGroup[] {
  const tierBySymbol = new Map(watchlist.map((item) => [item.symbol, item.tier]))
  const positionBySymbol = new Map(positions.map((p) => [p.symbol, p]))
  const ideaBySymbol = new Map(ideas.map((idea) => [idea.symbol, idea]))
  const outcomesBySymbol = new Map<string, TradeOutcome[]>()
  for (const outcome of outcomes) {
    const list = outcomesBySymbol.get(outcome.symbol) ?? []
    if (list.length < MAX_OUTCOMES_PER_SYMBOL) list.push(outcome)
    outcomesBySymbol.set(outcome.symbol, list)
  }

  const orderedSymbols = watchlist.map((item) => item.symbol)
  const seen = new Set(orderedSymbols)
  const extraSymbols = [...new Set([...positions.map((p) => p.symbol), ...ideas.map((i) => i.symbol), ...outcomes.map((o) => o.symbol)])]
    .filter((symbol) => !seen.has(symbol))
    .sort()

  return [...orderedSymbols, ...extraSymbols].map((symbol) => ({
    symbol,
    tier: tierBySymbol.get(symbol) ?? null,
    position: positionBySymbol.get(symbol) ?? null,
    idea: ideaBySymbol.get(symbol) ?? null,
    recentOutcomes: outcomesBySymbol.get(symbol) ?? [],
  }))
}

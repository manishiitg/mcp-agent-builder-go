export type WatchlistTier = 'large' | 'mid' | 'small'

export type WatchlistItem = {
  symbol: string
  tier: WatchlistTier
}

export type PortfolioSnapshot = {
  snapshotAt: string
  equity: number
  lastEquity: number
  cash: number
  buyingPower: number
  longMarketValue: number
  openPositions: number
}

export type EquityPoint = {
  snapshotAt: string
  equity: number
}

export type OpenPosition = {
  symbol: string
  qty: number
  avgEntryPrice: number
  unrealizedPl: number
  // From the matching open paper_trades row, when one exists -- a position
  // can outlive its originating order row being queryable, so these are
  // optional rather than assumed present.
  stop?: number
  target?: number
}

export type TradeIdea = {
  symbol: string
  runDate: string
  createdAt: string
  conviction: number
  direction: 'long' | 'short' | 'stand_aside'
  entry: number
  stop: number
  target: number
  rr: number
  rationale: string
}

export type TradeOutcomeResult = 'win' | 'loss' | 'flat' | 'no_fill' | 'open' | 'retired'

export type TradeOutcome = {
  symbol: string
  runDate: string
  direction: string
  result: TradeOutcomeResult
  rMultiple: number | null
  entry: number
  exitPrice: number | null
  exitTime: string | null
  note: string
}

export type WinRateSummary = {
  wins: number
  losses: number
  flat: number
  // Denominator excludes no_fill/open/retired -- those never had a
  // resolvable outcome, so folding them in would understate the win rate.
  decided: number
  winRatePct: number | null
  sumR: number
}

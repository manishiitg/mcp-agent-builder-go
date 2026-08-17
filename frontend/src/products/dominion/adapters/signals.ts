import { agentApi } from '../../../services/api'
import type { TradeIdea } from '../types'
import { DOMINION_DB_PATH } from './portfolio'

// Latest trade_ideas row per symbol -- horizon filter is mandatory (see
// db/README.md in the workflow's own workspace: unfiltered reads mix in
// retired swing_long rows sharing the same (run_date, symbol)).
export async function loadRecentTradeIdeas(): Promise<TradeIdea[]> {
  const response = await agentApi.queryWorkflowDB(
    DOMINION_DB_PATH,
    `SELECT symbol, run_date, created_at, conviction, direction, entry, stop, target, rr, rationale
     FROM trade_ideas t
     WHERE horizon = 'intraday_60m'
       AND created_at = (
         SELECT MAX(t2.created_at) FROM trade_ideas t2
         WHERE t2.symbol = t.symbol AND t2.horizon = 'intraday_60m'
       )
     ORDER BY run_date DESC, conviction DESC
     LIMIT 50`
  )
  if (!response.success || !response.data) {
    throw new Error(response.error || 'Dominion: failed to load trade ideas.')
  }
  return response.data.rows.map((row) => ({
    symbol: String(row.symbol ?? ''),
    runDate: String(row.run_date ?? ''),
    createdAt: String(row.created_at ?? ''),
    conviction: Number(row.conviction ?? 0),
    direction: (row.direction as TradeIdea['direction']) ?? 'stand_aside',
    entry: Number(row.entry ?? 0),
    stop: Number(row.stop ?? 0),
    target: Number(row.target ?? 0),
    rr: Number(row.rr ?? 0),
    rationale: String(row.rationale ?? ''),
  }))
}

import { agentApi } from '../../../services/api'
import type { TradeOutcome } from '../types'
import { DOMINION_DB_PATH } from './portfolio'

// Despite the name, this is the full graded history (bounded well above the
// live table's actual row count), not a display-limited "recent" slice --
// computeWinRate() and the P&L KPI both need the same all-time scope, or
// the two numbers silently disagree (a capped win rate can read very
// differently from a lifetime-since-$100K P&L figure). The per-symbol
// "recent" dots in the Stocks table cap independently, client-side, in
// groupBySymbol -- this list only needs to be complete, not short.
export async function loadRecentTradeOutcomes(): Promise<TradeOutcome[]> {
  const response = await agentApi.queryWorkflowDB(
    DOMINION_DB_PATH,
    `SELECT symbol, run_date, direction, result, r_multiple, entry, exit_price, exit_time, note
     FROM trade_outcomes
     WHERE horizon = 'intraday_60m' AND source = 'live'
     ORDER BY graded_at DESC
     LIMIT 2000`
  )
  if (!response.success || !response.data) {
    throw new Error(response.error || 'Dominion: failed to load trade outcomes.')
  }
  return response.data.rows.map((row) => ({
    symbol: String(row.symbol ?? ''),
    runDate: String(row.run_date ?? ''),
    direction: String(row.direction ?? ''),
    result: (row.result as TradeOutcome['result']) ?? 'open',
    rMultiple: row.r_multiple == null ? null : Number(row.r_multiple),
    entry: Number(row.entry ?? 0),
    exitPrice: row.exit_price == null ? null : Number(row.exit_price),
    exitTime: row.exit_time == null ? null : String(row.exit_time),
    note: String(row.note ?? ''),
  }))
}

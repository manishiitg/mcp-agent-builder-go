import { agentApi } from '../../../services/api'
import type { TradeIdea, TradeOutcome } from '../types'
import { DOMINION_DB_PATH } from './portfolio'

// Every other adapter query in this product is a static string -- this is
// the first one to interpolate a caller-supplied value. queryWorkflowDB has
// no bind-parameter support (see portfolio.ts's own comment on the API
// shape), so escape the SQL string-literal form (doubled single quotes) to
// keep a symbol containing a stray quote from producing a malformed or
// scope-widening query, the same way a raw string would misbehave in any
// hand-built SQL.
function escapeSqlString(value: string): string {
  return value.replace(/'/g, "''")
}

const HISTORY_LIMIT = 200

export async function loadSymbolHistory(symbol: string): Promise<{ ideas: TradeIdea[]; outcomes: TradeOutcome[] }> {
  const escaped = escapeSqlString(symbol)
  const [ideasResponse, outcomesResponse] = await Promise.all([
    agentApi.queryWorkflowDB(
      DOMINION_DB_PATH,
      `SELECT symbol, run_date, created_at, conviction, direction, entry, stop, target, rr, rationale
       FROM trade_ideas
       WHERE horizon = 'intraday_60m' AND symbol = '${escaped}'
       ORDER BY created_at DESC
       LIMIT ${HISTORY_LIMIT}`
    ),
    agentApi.queryWorkflowDB(
      DOMINION_DB_PATH,
      `SELECT symbol, run_date, direction, result, r_multiple, entry, exit_price, exit_time, note
       FROM trade_outcomes
       WHERE horizon = 'intraday_60m' AND source = 'live' AND symbol = '${escaped}'
       ORDER BY graded_at DESC
       LIMIT ${HISTORY_LIMIT}`
    ),
  ])
  if (!ideasResponse.success || !ideasResponse.data) {
    throw new Error(ideasResponse.error || `Dominion: failed to load signal history for ${symbol}.`)
  }
  if (!outcomesResponse.success || !outcomesResponse.data) {
    throw new Error(outcomesResponse.error || `Dominion: failed to load trade history for ${symbol}.`)
  }
  return {
    ideas: ideasResponse.data.rows.map((row) => ({
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
    })),
    outcomes: outcomesResponse.data.rows.map((row) => ({
      symbol: String(row.symbol ?? ''),
      runDate: String(row.run_date ?? ''),
      direction: String(row.direction ?? ''),
      result: (row.result as TradeOutcome['result']) ?? 'open',
      rMultiple: row.r_multiple == null ? null : Number(row.r_multiple),
      entry: Number(row.entry ?? 0),
      exitPrice: row.exit_price == null ? null : Number(row.exit_price),
      exitTime: row.exit_time == null ? null : String(row.exit_time),
      note: String(row.note ?? ''),
    })),
  }
}

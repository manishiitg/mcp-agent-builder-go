import { agentApi } from '../../../services/api'
import type { OpenPosition } from '../types'
import { DOMINION_DB_PATH } from './portfolio'

type RawPosition = {
  symbol?: string
  qty?: number
  avg_entry_price?: number
  unrealized_pl?: number
}

// positions_json on the latest snapshot is the source of truth for what's
// actually open at the broker; paper_trades only supplies the stop/target
// context for symbols that still have one (a manually-closed or
// broker-reconciled position can outlive its order row).
export async function loadOpenPositions(): Promise<OpenPosition[]> {
  const [snapshotResponse, tradesResponse] = await Promise.all([
    agentApi.queryWorkflowDB(
      DOMINION_DB_PATH,
      `SELECT positions_json FROM paper_account_snapshots ORDER BY snapshot_at DESC LIMIT 1`
    ),
    agentApi.queryWorkflowDB(
      DOMINION_DB_PATH,
      `SELECT symbol, stop, target FROM paper_trades
       WHERE horizon = 'intraday_60m' AND managed_action IS NULL
       ORDER BY submitted_at DESC`
    ),
  ])
  if (!snapshotResponse.success || !snapshotResponse.data) {
    throw new Error(snapshotResponse.error || 'Dominion: failed to load open positions.')
  }
  if (!tradesResponse.success || !tradesResponse.data) {
    throw new Error(tradesResponse.error || 'Dominion: failed to load open orders.')
  }

  const stopTargetBySymbol = new Map<string, { stop: number; target: number }>()
  for (const row of tradesResponse.data.rows) {
    const symbol = String(row.symbol ?? '')
    if (!symbol || stopTargetBySymbol.has(symbol)) continue
    stopTargetBySymbol.set(symbol, { stop: Number(row.stop ?? 0), target: Number(row.target ?? 0) })
  }

  const raw = snapshotResponse.data.rows[0]?.positions_json
  if (typeof raw !== 'string' || raw.trim() === '') return []

  let parsed: RawPosition[]
  try {
    parsed = JSON.parse(raw)
  } catch {
    return []
  }

  return parsed
    .filter((p) => p.symbol)
    .map((p) => {
      const symbol = String(p.symbol)
      const stopTarget = stopTargetBySymbol.get(symbol)
      return {
        symbol,
        qty: Number(p.qty ?? 0),
        avgEntryPrice: Number(p.avg_entry_price ?? 0),
        unrealizedPl: Number(p.unrealized_pl ?? 0),
        stop: stopTarget?.stop,
        target: stopTarget?.target,
      }
    })
}

import { agentApi } from '../../../services/api'
import type { EquityPoint, PortfolioSnapshot } from '../types'

export const DOMINION_DB_PATH = 'Workflow/tectonicusadaytrading/db/db.sqlite'

export async function loadLatestSnapshot(): Promise<PortfolioSnapshot | null> {
  const response = await agentApi.queryWorkflowDB(
    DOMINION_DB_PATH,
    `SELECT snapshot_at, equity, last_equity, cash, buying_power, long_market_value, open_positions
     FROM paper_account_snapshots
     ORDER BY snapshot_at DESC
     LIMIT 1`
  )
  if (!response.success || !response.data) {
    throw new Error(response.error || 'Dominion: failed to load account snapshot.')
  }
  const row = response.data.rows[0]
  if (!row) return null
  return {
    snapshotAt: String(row.snapshot_at ?? ''),
    equity: Number(row.equity ?? 0),
    lastEquity: Number(row.last_equity ?? 0),
    cash: Number(row.cash ?? 0),
    buyingPower: Number(row.buying_power ?? 0),
    longMarketValue: Number(row.long_market_value ?? 0),
    openPositions: Number(row.open_positions ?? 0),
  }
}

export async function loadEquityCurve(): Promise<EquityPoint[]> {
  const response = await agentApi.queryWorkflowDB(
    DOMINION_DB_PATH,
    `SELECT snapshot_at, equity FROM paper_account_snapshots ORDER BY snapshot_at ASC`
  )
  if (!response.success || !response.data) {
    throw new Error(response.error || 'Dominion: failed to load equity curve.')
  }
  return response.data.rows.map((row) => ({
    snapshotAt: String(row.snapshot_at ?? ''),
    equity: Number(row.equity ?? 0),
  }))
}

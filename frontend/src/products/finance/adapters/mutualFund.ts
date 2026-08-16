import { agentApi } from '../../../services/api'
import { mutualFundTransactionDirection } from '../mutualFundTransactionDirection'
import type { InvestmentAccountCard, InvestmentHolding, RecentTransaction } from '../types'

const DB_PATH = 'Workflow/Mututal-Fund/db/db.sqlite'

export async function loadMutualFundAccounts(): Promise<InvestmentAccountCard[]> {
  const [holdingsResponse, xirrResponse, overviewResponse] = await Promise.all([
    agentApi.queryWorkflowDB(
      DB_PATH,
      `SELECT group_name, scheme_name, folio_number, units, current_value, invested_value, profit_loss
       FROM portfolio_holdings`
    ),
    agentApi.queryWorkflowDB(DB_PATH, `SELECT group_name, xirr_pct FROM account_xirr`),
    agentApi.queryWorkflowDB(DB_PATH, `SELECT generated_at FROM portfolio_overview LIMIT 1`),
  ])
  if (!holdingsResponse.success || !holdingsResponse.data) {
    throw new Error(holdingsResponse.error || 'Mutual Fund: failed to load holdings.')
  }
  if (!xirrResponse.success || !xirrResponse.data) {
    throw new Error(xirrResponse.error || 'Mutual Fund: failed to load XIRR.')
  }
  // portfolio_overview is workflow-wide, not per-account -- one sync
  // timestamp applied to every group's card below.
  const generatedAt = overviewResponse.success
    ? String(overviewResponse.data?.rows[0]?.generated_at ?? '')
    : ''

  const xirrByGroup = new Map<string, number>()
  for (const row of xirrResponse.data.rows) {
    if (row.xirr_pct != null) xirrByGroup.set(String(row.group_name), Number(row.xirr_pct))
  }

  const byGroup = new Map<string, InvestmentHolding[]>()
  for (const row of holdingsResponse.data.rows) {
    const group = String(row.group_name ?? 'Mutual Fund')
    const holding: InvestmentHolding = {
      schemeName: String(row.scheme_name ?? ''),
      folioNumber: String(row.folio_number ?? ''),
      units: Number(row.units ?? 0),
      currentValue: Number(row.current_value ?? 0),
      investedValue: Number(row.invested_value ?? 0),
      profitLoss: Number(row.profit_loss ?? 0),
    }
    const existing = byGroup.get(group)
    if (existing) existing.push(holding)
    else byGroup.set(group, [holding])
  }

  return Array.from(byGroup.entries()).map(([group, holdings]) => ({
    source: 'Mutual Fund',
    accountLabel: group,
    totalCurrentValue: holdings.reduce((sum, h) => sum + h.currentValue, 0),
    totalInvested: holdings.reduce((sum, h) => sum + h.investedValue, 0),
    xirrPct: xirrByGroup.get(group),
    holdings,
    lastUpdated: generatedAt,
  }))
}

export async function loadMutualFundTransactions(): Promise<RecentTransaction[]> {
  const response = await agentApi.queryWorkflowDB(
    DB_PATH,
    `SELECT date, scheme_name, transaction_type, amount
     FROM portfolio_transactions
     ORDER BY date DESC
     LIMIT 100`
  )
  if (!response.success || !response.data) {
    throw new Error(response.error || 'Mutual Fund: failed to load transactions.')
  }
  return response.data.rows.map((row) => {
    const type = String(row.transaction_type ?? '')
    return {
      source: 'Mutual Fund' as const,
      date: String(row.date ?? ''),
      description: `${type} — ${String(row.scheme_name ?? '')}`,
      amount: Number(row.amount ?? 0),
      direction: mutualFundTransactionDirection(type),
    }
  })
}

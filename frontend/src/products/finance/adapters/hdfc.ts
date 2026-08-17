import { agentApi } from '../../../services/api'
import { extractDateFromLatestTransaction } from '../extractDateFromLatestTransaction'
import type { BankAccountCard } from '../types'

const DB_PATH = 'Workflow/HDFC-Personal-Accounts/db/db.sqlite'

// HDFC has no per-transaction rows -- transaction_summary carries only a
// count and a latest date, so this adapter never produces a
// RecentTransaction; the count/date are carried on BankAccountCard instead.
export async function loadHdfcAccounts(): Promise<BankAccountCard[]> {
  const response = await agentApi.queryWorkflowDB(
    DB_PATH,
    `SELECT
       b.group_name AS group_name,
       b.current_balance AS current_balance,
       b.total_fixed_deposit AS fixed_deposit,
       b.updated_at_iso AS updated_at,
       t.total_transactions AS transaction_count,
       t.latest_transaction AS last_transaction_date
     FROM balance_history b
     LEFT JOIN transaction_summary t ON t.group_name = b.group_name`
  )
  if (!response.success || !response.data) {
    throw new Error(response.error || 'HDFC: failed to load account data.')
  }
  return response.data.rows.map((row) => ({
    source: 'HDFC',
    accountLabel: String(row.group_name ?? 'HDFC'),
    currentBalance: Number(row.current_balance ?? 0),
    fixedDeposit: Number(row.fixed_deposit ?? 0),
    lastUpdated: String(row.updated_at ?? ''),
    transactionCount: row.transaction_count == null ? undefined : Number(row.transaction_count),
    lastTransactionDate: extractDateFromLatestTransaction(row.last_transaction_date),
  }))
}

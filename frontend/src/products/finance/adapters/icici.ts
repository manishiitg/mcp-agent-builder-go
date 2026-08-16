import { agentApi } from '../../../services/api'
import type { BankAccountCard, RecentTransaction } from '../types'
import { parseIciciAmount } from '../parseIciciAmount'

const DB_PATH = 'Workflow/ICICI-BANK-PARSING/db/db.sqlite'

export async function loadIciciAccounts(): Promise<BankAccountCard[]> {
  const response = await agentApi.queryWorkflowDB(
    DB_PATH,
    `SELECT group_name, account_name, total_balance_inr, fd_balance, run_date
     FROM current_balances`
  )
  if (!response.success || !response.data) {
    throw new Error(response.error || 'ICICI: failed to load account data.')
  }
  return response.data.rows.map((row) => ({
    source: 'ICICI',
    accountLabel: String(row.account_name ?? row.group_name ?? 'ICICI'),
    currentBalance: parseIciciAmount(row.total_balance_inr as string | null) ?? 0,
    fixedDeposit: parseIciciAmount(row.fd_balance as string | null) ?? 0,
    lastUpdated: String(row.run_date ?? ''),
  }))
}

export async function loadIciciTransactions(): Promise<RecentTransaction[]> {
  const response = await agentApi.queryWorkflowDB(
    DB_PATH,
    `SELECT txn_date, description, amount_inr, cr_dr
     FROM recent_transactions
     ORDER BY txn_date DESC
     LIMIT 100`
  )
  if (!response.success || !response.data) {
    throw new Error(response.error || 'ICICI: failed to load transactions.')
  }
  return response.data.rows.map((row) => ({
    source: 'ICICI',
    date: String(row.txn_date ?? ''),
    description: String(row.description ?? ''),
    amount: Number(row.amount_inr ?? 0),
    direction: String(row.cr_dr ?? '').toUpperCase() === 'CR' ? 'credit' : 'debit',
  }))
}

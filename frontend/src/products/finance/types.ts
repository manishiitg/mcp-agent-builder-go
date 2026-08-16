export type BankAccountCard = {
  source: 'HDFC' | 'ICICI'
  accountLabel: string
  currentBalance: number
  fixedDeposit: number
  lastUpdated: string
  // HDFC has no transaction rows, only a count -- carried here rather than
  // faked into a RecentTransaction list.
  transactionCount?: number
  lastTransactionDate?: string
}

export type RecentTransaction = {
  source: 'ICICI' | 'Mutual Fund'
  date: string
  description: string
  amount: number
  // Undefined when the source's own transaction type doesn't unambiguously
  // mean cash in or cash out (e.g. a mutual-fund "Switch" moves money
  // between two funds, not in or out of the portfolio) -- left unlabeled
  // rather than guessed, so the UI must render it neutrally instead of
  // showing a confidently wrong credit/debit color.
  direction?: 'credit' | 'debit'
}

export type InvestmentHolding = {
  schemeName: string
  folioNumber: string
  units: number
  currentValue: number
  investedValue: number
  profitLoss: number
}

export type InvestmentAccountCard = {
  source: 'Mutual Fund'
  accountLabel: string
  totalCurrentValue: number
  totalInvested: number
  xirrPct?: number
  holdings: InvestmentHolding[]
  // From portfolio_overview.generated_at -- a single workflow-wide sync
  // timestamp (not per-account), so the same value on every card.
  lastUpdated: string
}

export type TaxCard = {
  pan: string
  tdsThisYear: number
  pendingNotices: number
  refundAmount: number
  lastChecked: string
}

export type GstCard = {
  gstin: string
  legalName: string
  turnoverAggregate: number
  ledgerBalance: { igst: number; cgst: number; sgst: number; cess: number }
  nextReturnDue?: string
  filingStatus?: string
  // From gst_snapshot.collected_at -- when this data was last synced. Not
  // to be confused with nextReturnDue, which is a future filing deadline.
  lastUpdated: string
}

export type FinanceSources = {
  bankAccounts: BankAccountCard[]
  transactions: RecentTransaction[]
  investmentAccounts: InvestmentAccountCard[]
  tax: TaxCard[]
  gst: GstCard[]
}

// Computed by combining the sources above -- never queried from any one
// source directly. See docs/design/finance_dashboard_product.md's
// "Synthesis, not just parallel cards".
export type NetWorthSummary = {
  liquid: number
  locked: number
  invested: number
  total: number
}

export type AttentionItem = {
  source: 'GST' | 'Tax'
  severity: 'overdue' | 'pending'
  label: string
  dueDate?: string
  // When this item was last reconfirmed against the actual source (a tax
  // portal, a GST return filing) -- not when it was first issued. An old
  // notice can genuinely still be open for years; what matters for trust is
  // how recently it was checked, not guessing whether it's "resolved" from
  // data with no resolved/closed status to read.
  lastConfirmed?: string
}

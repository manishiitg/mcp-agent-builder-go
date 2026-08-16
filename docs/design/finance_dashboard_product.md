# Personal Finance Dashboard: a consolidated view across finance workflows

**Status:** Concept (2026-08-16). Not started.

## The idea

A number of existing workflows each track one slice of personal finance.
Each already has its own `db/db.sqlite` with real, populated data. The idea
is a single dashboard that consolidates them into one view, plus a chat
panel (later, not v1) to ask questions across all of them.

**Scope: bank accounts, investments (mutual funds), and tax.** Trading
(`globalinvestment`, `trading`) is explicitly out — a different domain
(positions, P&L, strategies), not part of this dashboard.

**Single-user, own data, dashboard-only v1.** Not a client-facing product —
no permission model, no untrusted content, no write barrier to design, and
(per the 2026-08-16 decisions) no chat in the first slice either. That makes
it the simplest of the product concepts explored this session, and a good
candidate for the first one actually built.

## Architecture: no new backend, no Go files

**The whole thing is frontend-only.** `agentApi.queryWorkflowDB(dbPath,
sql)` (`frontend/src/services/api.ts:1487`) already exists, already works
from any React component, and already takes an explicit `dbPath` — it is a
plain `POST /api/query` call, nothing iframe- or report-specific. Proof: the
workflow report system's own `window.report.query` is *implemented as* a
thin wrapper around this exact function
(`frontend/src/components/workflow/ReportViewer.tsx:88-89`:
`query: async (sql) => agentApi.queryWorkflowDB(\`${workspacePath}/db/db.sqlite\`, sql)`).
`window.report` exists only to hand that function into an
**iframe-sandboxed** HTML document — a constraint this product doesn't have,
since it's a real React surface, not agent-authored HTML (see
`workflow_custom_ui_product.md` for where that distinction matters). So:
call `agentApi.queryWorkflowDB` directly, no bridge, no iframe, no new
backend route, no Go package, no product.yaml (nothing here needs its own
agent identity while there's no chat).

```
frontend/src/products/finance/
  adapters/
    hdfc.ts        → agentApi.queryWorkflowDB('Workflow/HDFC-Personal-Accounts/db/db.sqlite', sql)
    icici.ts        → same, ICICI's path + SQL
    mutualFund.ts   → same, Mututal-Fund's path + SQL
    tax.ts          → same, check-form-26as-xspaces's path + SQL
    gst.ts          → same, gstdatacollection's path + SQL
  types.ts          → the card types below
  FinanceSurface.tsx → calls all adapters in parallel, renders card sections
```

Every adapter is a small `.ts` file: a `dbPath` constant, one or two
`SELECT`s, and a mapper into a typed card. Buildable and testable entirely
against real data with no server restart — `agentApi.queryWorkflowDB` talks
to the already-running workspace server.

## The card types — designed against all five real schemas at once

There is no single unifying struct. Bank/investment data and tax/GST data
don't share a "balance" concept, and even within banking, HDFC only stores
a transaction *count* while ICICI stores real transaction rows — a
transaction list can't be a shared concept either, only some sources
support it. Real schemas, confirmed 2026-08-16
(`workspace-docs/Workflow/*/db/db.sqlite`):

| Workflow | Domain | Real columns (non-platform tables) |
|---|---|---|
| `HDFC-Personal-Accounts` | bank | `balance_history(group_name, current_balance, total_fixed_deposit, updated_at_iso)`; `transaction_summary(group_name, total_transactions, latest_month, latest_transaction)` — **count only, no transaction rows** |
| `ICICI-BANK-PARSING` | bank | `current_balances(group_name, account_name, account_number, total_balance_inr, fd_balance)`; `recent_transactions(group_name, txn_date, description, amount_inr, cr_dr, closing_balance)` — **real per-transaction rows** |
| `Mututal-Fund` | investments | `portfolio_holdings(group_name, scheme_name, folio_number, units, current_value, invested_value, nav, profit_loss)`; `account_xirr(group_name, xirr_pct, as_of_date)` |
| `check-form-26as-xspaces` | tax | `tax_summary(pan, total_tds_current_ay, pending_notice_count, total_refund_amount, last_checked)`; `notices(pan, din, title, status, issue_date, action_required)` |
| `gstdatacollection` | tax | `gst_snapshot(gstin, legal_name, turnover_aggregate)`; `gst_ledger_balance(igst, cgst, sgst, cess)`; `gst_return_status(fin_year, period, due_date, status, filed_date)` |

```ts
type BankAccountCard = {
  source: 'HDFC' | 'ICICI'
  accountLabel: string        // group_name / account_name
  currentBalance: number
  fixedDeposit: number
  lastUpdated: string
  // HDFC has no transaction rows, only a count -- carried here, not faked
  // into a transaction list:
  transactionCount?: number
  lastTransactionDate?: string
}

type RecentTransaction = {   // ICICI only -- HDFC's adapter emits none
  source: string
  date: string
  description: string
  amountInr: number
  direction: 'credit' | 'debit'
}

type InvestmentHolding = {   // mutual funds
  schemeName: string
  folioNumber: string
  units: number
  currentValue: number
  investedValue: number
  profitLoss: number
}

type InvestmentAccountCard = {
  source: 'Mutual Fund'
  accountLabel: string        // group_name
  totalCurrentValue: number
  totalInvested: number
  xirrPct?: number
  holdings: InvestmentHolding[]
}

type TaxCard = {
  pan: string
  tdsThisYear: number
  pendingNotices: number
  refundAmount: number
  lastChecked: string
}

type GstCard = {
  gstin: string
  legalName: string
  turnoverAggregate: number
  ledgerBalance: { igst: number; cgst: number; sgst: number; cess: number }
  nextReturnDue?: string
  filingStatus?: string
}
```

Not every workflow in the workspace is a finance source despite the
name/location — `ICICI-BANK-PARSING-v2` and `confida-login` turned out to be
unrelated QA/verification workflows on inspection. **The source list is
curated by hand, never auto-discovered** — no naming convention is reliable
enough to infer it, and silently pulling in the wrong data is a real risk.

## Decisions (2026-08-16)

| Question | Decision |
|---|---|
| Scope | Bank accounts, investments (mutual funds), and tax — trading is a different domain, out |
| Which sources feed it | A curated list named by hand, one adapter per source — not auto-discovery |
| Where it lives | A new product surface, own tile — a real dashboard needs layout a chat+aside shape doesn't fit |
| Backend | None — frontend-only, `agentApi.queryWorkflowDB` directly, no Go, no new routes |
| Chat | Not in v1 — dashboard first, chat is a later, separate slice |
| Branch | Continues on `feature/chief-of-staff-product` (not split to its own branch) |

## Build order

The reuse rule from `reusable_vertical_product_platform.md` ("extract after
a second consumer demonstrates the common behavior") does not counsel
staging schema design here — it exists to avoid *guessing* about an unknown
future consumer. All five sources are already fixed and already inspected
(table above), so there is nothing left to discover by building one adapter
first that isn't already visible in the schema dump. Building only HDFC's
adapter first would mean designing the card types around one source and
finding out later they don't fit tax or investment data — the exact mistake
sequencing exists to prevent, self-inflicted instead of avoided.

1. **The card types** (above) — already designed against all five schemas
   together, not one at a time.
2. **Write all five adapters together** — each a small `.ts` file, a
   `dbPath` constant, a couple of `SELECT`s, a mapper.
3. **One dashboard card, wired end to end, first** — pick one source (a
   bank account is the simplest) and render it before wiring the rest, so
   the `agentApi.queryWorkflowDB` path and the product surface itself are
   proven before building on top of them. This is the only place staging
   still earns its keep: it isolates *integration* risk, not *schema* risk,
   which step 1 already resolved.
4. **The full dashboard grid** over all five adapters' output — bank
   section, investments section, tax section.
5. **(Later, separate slice) Chat.** When it's wanted: read-only tool
   surface reusing these same adapters' queries, narrower than workflow
   `run` mode since nothing here should ever execute a workflow. See
   `workflow_custom_ui_product.md`'s Gap 1 finding if execution is ever
   added later — the write-barrier concern there doesn't apply to
   read-only queries, but would apply the moment "run/refresh" is added.

## Open questions

- Refresh cadence: does the dashboard query live on every load, or does it
  need caching once five sources' worth of queries per page load is
  noticeable? Five is small enough this is probably premature — revisit if
  it's actually slow.
- Where do the card types/adapters live if a second, unrelated consolidation
  surface is ever wanted — still `frontend/src/products/finance/`, or
  extracted? Per the reuse rule: wait for a second real consumer.

## Related

- `docs/design/workflow_custom_ui_product.md` — a harder, different concept
  (client-facing custom UI *per workflow*, with a real write barrier and
  permission model to design, and a genuine agent-authored-HTML sandboxing
  question). This dashboard avoids all of that: one first-party UI spanning
  several trusted sources, read-only, no backend at all in v1.
- `docs/design/reusable_vertical_product_platform.md` — the reuse rule
  invoked (and explained why it doesn't apply to schema design here) above.

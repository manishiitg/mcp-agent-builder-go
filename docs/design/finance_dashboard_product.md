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
  parseIciciAmount.ts → the one piece of real logic; own unit test (see below)
  types.ts          → the per-source card types + the aggregate types
  synthesize.ts      → pure function: five adapters' output -> NetWorthSummary,
                        AttentionItem[], ActivityEvent[] -- the actual point,
                        see "Synthesis, not just parallel cards" below
  FinanceSurface.tsx → calls all adapters in parallel, synthesizes, renders
                        synthesis views first, per-source sections below
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

// Computed by combining the cards above -- not queried from any one
// source. See "Synthesis, not just parallel cards" below.
type NetWorthSummary = {
  liquid: number       // Σ bank current balances
  locked: number        // Σ fixed deposits
  invested: number      // Σ mutual-fund current value
  total: number
}

type AttentionItem = {
  source: 'GST' | 'Tax'
  severity: 'overdue' | 'pending'
  label: string          // e.g. "GSTR-3B, June 2026 -- not filed"
  dueDate?: string
}

type ActivityEvent = {
  source: 'ICICI' | 'Mutual Fund'
  date: string
  description: string
  amount: number
}
```

Not every workflow in the workspace is a finance source despite the
name/location — `ICICI-BANK-PARSING-v2` and `confida-login` turned out to be
unrelated QA/verification workflows on inspection. **The source list is
curated by hand, never auto-discovered** — no naming convention is reliable
enough to infer it, and silently pulling in the wrong data is a real risk.

## Synthesis, not just parallel cards

Five card sections on one page is "on one page," not "consolidated." The
actual point of combining sources is the views that only exist *because*
they're combined. Grounded in real query results, 2026-08-16:

**1. Net Worth — one number.** HDFC and the mutual-fund source already
self-aggregate across their own multiple accounts
(`portfolio_summary.total_portfolio`, `portfolio_overview.total_portfolio_value`
— both are already rollup rows, not per-account data). ICICI has no such
rollup; sum `current_balances.total_balance_inr` per account after parsing
it (see the currency gotcha below). Net worth = HDFC's total + Σ ICICI
accounts + the mutual-fund total.

**2. Liquid vs. Locked vs. Invested.** A real allocation view that exists
only by combining sources: bank current balances (liquid) vs. fixed
deposits (locked) vs. mutual-fund current value (invested). No single
source has this shape.

**3. Needs Attention — one prioritized list, not buried per-card.** Pulled
from a real query against `gst_return_status` and `notices`, 2026-08-16: a
`GSTR-3B` return for June 2026 is still `Not Filed`, past its due date, and
at least one tax notice has `status: "Response Due"` with
`action_required: 1`. These belong at the top of the dashboard as one
merged list, not inside separate GST and tax cards where they're easy to
miss.

**4. A cross-source staleness check.** Each source has its own freshness
timestamp (`updated_at_iso` / `generated_at` / `sync_date`, named
differently everywhere as usual). If one source is much older than the
others, say so — a number silently shown next to fresher ones is a subtle
way to make a bad decision.

**5. A unified activity feed.** Merge ICICI's real `recent_transactions`
rows with the mutual fund's `portfolio_transactions` into one chronological
list. HDFC has no transaction rows — only `transaction_summary`'s count and
latest date — so it contributes one summary line rather than being silently
dropped from the feed.

**Deliberately not attempted: a tax-liability-vs-available-cash
calculation.** `tax_summary.total_tds_current_ay` and the refund fields
don't cleanly net out to "amount you still owe" without real tax-domain
logic this doc has no grounding in — guessing at that for a page that
touches actual money decisions is worse than not showing it.

**A real parsing gotcha, not hypothetical:** ICICI's `total_balance_inr` is
formatted text, e.g. `"INR 12,34,567.89CR"` — a currency prefix, Indian
comma grouping, and a credit/debit suffix glued directly onto the number
with no space. Comma-stripping is safe for the grouping (JS `parseFloat`
doesn't care where commas fall), but the `CR`/`DR` suffix must be stripped
before parsing or it silently produces `NaN`. This needs a real parser with
a test, not an inline `parseFloat`.

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

1. **The card types and the aggregate types** (above) — already designed
   against all five schemas together, not one at a time.
2. **The currency parser, with a test, before anything sums it.** ICICI's
   `total_balance_inr` is dirty text (see the gotcha above); Net Worth is
   wrong the moment this is wrong, silently. This is the one piece of real
   logic in the whole build and it deserves its own unit test with the
   actual observed format as a fixture, not a synthetic clean string.
3. **Write all five adapters together** — each a small `.ts` file, a
   `dbPath` constant, a couple of `SELECT`s, a mapper.
4. **One dashboard card, wired end to end, first** — pick one source (a
   bank account is the simplest) and render it before wiring the rest, so
   the `agentApi.queryWorkflowDB` path and the product surface itself are
   proven before building on top of them. This is the only place staging
   still earns its keep: it isolates *integration* risk, not *schema* risk,
   which step 1 already resolved.
5. **The synthesis layer** (Net Worth, allocation split, Needs Attention,
   staleness check, unified activity feed) — computed from the five
   adapters' combined output, not queried fresh. This is the actual point
   of the dashboard; do not treat it as a stretch goal after step 6.
6. **The full page** — synthesis views first/top, per-source card sections
   below them for anyone who wants to drill into one source.
7. **(Later, separate slice) Chat.** When it's wanted: read-only tool
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

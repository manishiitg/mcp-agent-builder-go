# Personal Finance Dashboard: a consolidated view across finance workflows

**Status:** Concept (2026-08-16). Not started.

## The idea

A number of existing workflows each track one slice of personal finance.
Each already has its own `db/db.sqlite` with real, populated data. The idea
is a single dashboard that consolidates them into one view, plus a chat
panel to ask questions across all of them.

**Scope: bank accounts and tax only.** Trading (`globalinvestment`,
`trading`) is explicitly out — a different domain with its own richer
concerns (P&L, strategies, positions), not part of this dashboard.

**Single-user, own data, read-only chat.** This is not a client-facing
product — no permission model, no untrusted content, no write barrier to
design. That makes it a much simpler build than the other two product
concepts explored the same day (see "Related" below), and a good candidate
for the first real product built cleanly on the platform pattern.

## What already exists to build on

**Real, populated sources in scope**, confirmed 2026-08-16
(`workspace-docs/Workflow/*/db/db.sqlite`):

| Workflow | Domain | Own tables (non-platform) |
|---|---|---|
| `HDFC-Personal-Accounts` | bank | `balance_history`, `transaction_summary`, `portfolio_summary` |
| `ICICI-BANK-PARSING` | bank | `current_balances`, `bank_balance_history`, `recent_transactions` |
| `check-form-26as-xspaces` | tax | `tax_summary`, `notices` |
| `gstdatacollection` | GST/tax | `gst_snapshot`, `gst_ledger_balance`, `gst_return_status` |

Out of scope by explicit decision: `Mututal-Fund`, `globalinvestment`,
`trading` — a different domain (positions, P&L, strategies) with its own
concerns, not part of this dashboard.

Not every workflow in the workspace is a finance source despite the
name/location — `ICICI-BANK-PARSING-v2` and `confida-login` turned out to be
unrelated QA/verification workflows on inspection. **The source list must be
curated, not auto-discovered** — there is no naming convention reliable
enough to infer it, and the risk of silently pulling in the wrong data is
real.

**No shared schema across sources — confirmed, not assumed.** Every source
above invented its own table and column names independently (e.g. "recent
transactions" is `transaction_summary` in HDFC but `recent_transactions` in
ICICI, with different columns). A dashboard cannot run one generic query
across N databases; it needs one small adapter per source mapping that
source's actual schema into a handful of shared concepts (balance,
recent-transactions, portfolio-value, allocation — whichever the dashboard
actually wants to show).

**The query primitive already supports cross-workflow reads.**
`workspace.Client.QueryWorkflowDB` / `QueryAuthorizedWorkflowDB`
(`agent_go/pkg/workspace/query_workflow_db.go:9-37`) takes an explicit,
caller-supplied `DBPath` — "the workspace-relative path to the SQLite file,
e.g. `Workflow/<name>/db/db.sqlite`" — and enforces read-only. It is not
scoped to one workflow at the API level; the *agent tool*
`query_workflow_db` just happens to default `DBPath` to whichever workflow
the current session is in
(`cmd/server/virtual-tools/workflow_db_tools.go:112`,
`resolveCurrentWorkflowDBPath`). A new aggregator calling the same client
once per configured source, with an explicit path each time, needs no new
primitive — just a loop and per-source SQL.

**The product-surface pattern applies directly** — see
`chief_of_staff_as_product.md` / `video_studio_inside_agentworks.md` for the
worked examples: a lazy-imported React surface, a product.yaml-declared
profile if chat needs its own identity, three hand-edits to mount it.

## Architecture

```
per-source adapter (Go)          shared shape                  UI
  hdfc.go    → query HDFC db  ─┐
  icici.go   → query ICICI db ─┼─→  []AccountSummary  ──→  Dashboard cards
  tax.go     → query tax db   ─┤    []Transaction     ──→  Transaction list
  gst.go     → query GST db   ─┘

                                                          Chat (read-only,
                                                          same adapters,
                                                          answers questions)
```

Each adapter is small: one file, one source, a handful of `SELECT`s against
that source's actual tables, mapped into a couple of shared Go structs. New
source added later = new adapter file, not a schema migration.

Chat reuses the *same* adapters rather than a separate path, so "what did
HDFC show me" and "what does the dashboard show for HDFC" can never
disagree.

## Decisions (2026-08-16)

| Question | Decision |
|---|---|
| Scope | Bank accounts and tax only — trading and mutual funds are a different domain, out |
| Which sources feed it | A curated list you name, one adapter per source — not auto-discovery |
| Where it lives | A new product surface, own tile — a real dashboard needs layout a chat+aside shape doesn't fit |
| What chat can do | Read-only for v1 — answers questions over already-synced data, no triggering a refresh run |

Read-only chat means this needs none of the run-mode / `tool_policy`
narrowing work from `workflow_custom_ui_product.md` — that question is
deferred until "refresh from chat" is actually wanted.

## Build order

1. **One adapter, one card.** Pick one bank source (HDFC or ICICI), write
   its adapter, render one dashboard card from it. Prove the shape end to
   end before adding the second source.
2. **Add the remaining three sources incrementally** (the other bank, then
   the two tax sources), one adapter each, growing the shared structs only
   as far as real data requires — do not pre-design a generic
   `FinancialAccount` schema before two sources have proven what's actually
   common between a bank account and a tax summary.
3. **The product surface** — dashboard grid over the adapters' combined
   output.
4. **Read-only chat** — same adapters, a narrow read-only tool surface (list
   sources, query a source's summary, no `execute_shell_command`, no write
   tools at all — narrower than `run` mode, since nothing here should ever
   execute a workflow).

## Open questions

- Refresh cadence: do adapters read live from `db.sqlite` on every dashboard
  load, or is a periodic snapshot/cache needed once there are enough sources
  that N live queries per page load gets slow?
- Where do shared structs live — inside the new product package, or a
  small shared package if a future second consolidation dashboard (e.g.
  something non-financial) would reuse the pattern? Reuse rule from
  `reusable_vertical_product_platform.md` applies: wait for a second real
  consumer before extracting.

## Related

- `docs/design/workflow_custom_ui_product.md` — a different, harder concept
  (client-facing custom UI *per workflow*, with a real write barrier and
  permission model to design). This dashboard is simpler: single user, one
  UI spanning many workflows, read-only.
- `docs/design/reusable_vertical_product_platform.md` — the reuse rule this
  build order follows (adapters grow from real sources, not designed ahead
  of them).

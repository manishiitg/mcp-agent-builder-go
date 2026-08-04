[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-008 — phase costs omit input and can use the wrong rate card

| Coordination | Value |
|---|---|
| Assigned agent | `Unassigned` |
| Ticket state | `runtime_verified` |
| Last synchronized | `2026-08-04` |

> Claim this ticket in this file before implementation. During active work,
> update this fragment rather than the shared index; synchronize the index
> once at handoff, review, or completion.


- **Priority:** P1
- **Owner:** cost observer/phase ledger pricing
- **Source finding:** `HARNESS-PHASE-COST-PRICING-2026-08-03`
- **Source database:** `Workflow/build-in-public/db/db.sqlite`
- **Problem:** phase rows omitted input cost and one Opus row used Sonnet output
  and cached-input rates.
- **Impact:** historical and daily spend is materially understated and changes
  without a workload change.
- **Implementation (2026-08-03):** Claude transcript usage now reports total
  prompt input (fresh + cache create + cache read), while retaining the raw
  cache components. Phase persistence carries cache read/write separately,
  recalculates every component from the effective model, and stamps
  `pricing_model_id` plus `pricing_version`. Claude Opus 5 and Sonnet 5 golden
  cases prove distinct input/output/cache-read/cache-write rates and totals.
- **Acceptance:** one immutable model identity selects one versioned rate card;
  input, output, reasoning, cache-read, and cache-write components reconcile to
  the total. Golden tests cover both Opus and Sonnet on adjacent dates.


[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-090 — no surface reports Pulse time/cost against workflow time/cost, so "is Pulse worth it?" cannot be answered

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — designed, not built |
| Last synchronized | `2026-08-11` |

- **Priority:** P2 — no incorrect behavior; the platform simply cannot report
  what its own self-improvement loop costs relative to the work it improves.
- **Owner:** Pulse measurement surface (`pulse_agent_metrics` read path,
  `pkg/costledger`, Pulse popup)
- **Requested by:** the operator, 2026-08-11 — *"we should measure pulse time,
  cost, goal progress, workflow time, cost"*
- **Depends on:** [PLAT-088](plat-088.md) (cost attribution — landed
  2026-08-11; without it every number here is wrong)
- **Related:** [PLAT-069](plat-069.md) (trend measurement: cost/time down and
  accuracy up over runs — deferred partly because this data was not trustworthy)

## Why this is open rather than merely unbuilt

Three of the four pieces already exist and are wired end to end. The reason
nothing is visible is a single missing writer, plus one absent dimension:

| Piece | State |
|---|---|
| Per-call cost with scope (`pulse` / `workflow_execution` / `builder` / `evaluation` / `chat`) | **Exists and correct** — `cost_events` in `workspace-docs/_system/costs.sqlite`, accurate as of PLAT-088 |
| `pulse_agent_metrics` table, `LoadPulseAgentMetrics`, `/api/workflow/pulse-agent-metrics`, `PulseWorkspace.tsx` panel | **Exists, reads an empty table** |
| The writer that filled that table (`RecordPulseAgentMetric`) | **Has zero callers** since `aad50dfb0` (2026-08-08) |
| Pulse/workflow stage durations | **Logged only** — never persisted anywhere queryable |

## Why the writer has no callers — and why simply restoring it is wrong

`RecordPulseAgentMetric` is not broken. It is well built: it refuses
agent-self-reported counters, derives usage only from the cost ledger via
`SummarizeExecution(executionID)`, prices any unpriced token slice through the
same immutable pricing contract the execution ledgers use, and upserts by
`execution_id`.

It became unreachable because the architecture beneath it changed. It was
written when Pulse ran **one Go-spawned child execution per reviewer module**,
so every module had its own `execution_id` to summarize. `aad50dfb0`
("stabilize pulse orchestration and scheduled sessions", the PLAT-050 change)
replaced that with **one continuing main-agent conversation** driving Gate →
Review+Fix → Finalize as scheduler-sent turns. The surrounding machinery it
depended on — `isPulseStage`, `reviewExecID`, `pulseReviewerPersistenceContext`
— was removed with it.

So the per-module `execution_id` the function is keyed on no longer exists for
the main Pulse stages. Reviving the old call site would mean reviving the old
architecture. **The table's key is the thing that went stale, not its columns.**

Note the asymmetry the current data shows: Pulse's *delegated background
children* still do get real execution ids (`bg-pulse-review+fix:-engineering…`)
and are correctly ledger-attributed. It is the main-session stages that have
no per-stage execution identity.

## Design

**1. Read from the ledger, not the dead table.** Repoint
`LoadPulseAgentMetrics` (or add a sibling) at `cost_events`, aggregating by
`workflow_id` + `scope` + time window rather than by a per-module
`execution_id` that no longer exists. This makes the existing endpoint and the
existing `PulseWorkspace.tsx` panel work again with no frontend change, and it
answers the actual question — Pulse spend versus workflow spend — which the
old table never could, since it only ever held Pulse rows and had nothing to
compare them against.

**2. Persist stage durations.** The scheduler already knows each Pulse stage's
start and end (it logs `step "gate" done`, `step "review-fix" done`,
`step "finalize" done`). Record start/end per stage against `pulse_run_id` so
time is queryable, not just greppable. Workflow-side timing already exists in
`run_metadata.json` and per-step `*-timing.json`.

**3. Report the comparison per run and as a trend.** Pulse time and cost
against workflow time and cost, plus goal progress from eval scores — which is
PLAT-069's ask, now on a foundation that can support it.

## Deliberately not in scope

- **Back-filling historical rows.** Pre-PLAT-088 data attributes scheduled
  Pulse and workflow turns to `chat`, and the 15,235 pre-2026-07-12
  `unknown`-scope rows predate attribution entirely. Any trend must either
  start at 2026-08-11 or state its own floor. Silently charting across that
  boundary would manufacture a fake improvement exactly where the accounting
  changed.
- **Reviving the per-module `execution_id` model.** See above.

## Acceptance

- The Pulse popup shows, for a given run: Pulse cost and duration beside
  workflow cost and duration, rather than an empty panel.
- The figures reconcile with `cost_events` for the same run and window.
- Any trend view states the date from which its data is trustworthy instead of
  charting across the PLAT-088 attribution change as if it were a real trend.

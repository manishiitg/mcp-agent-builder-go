[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-081 — workflow-builder chat cost writer inflated its own ledger every turn; two other cost findings were scope/documentation gaps, not bugs

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `implemented` — the one real bug fixed and tested; two findings resolved by documentation; one left as an explicit design-only gap |
| Last synchronized | `2026-08-10` |

- **Priority:** P2 — a real, silently-compounding cost-ledger inflation bug,
  plus two findings that were misreading correct-but-undocumented behavior
- **Owner:** phase token-usage cost ledger (`pkg/orchestrator/cost_storage.go`,
  `cmd/server/server.go` workflow-builder chat cost writer)
- **Found on:** PLAT-073 cluster B — `e6be98dfd6f4d639` (build-in-public),
  `43b988fe2ef952f3` (tectonicusadaytrading), `a8ab091308579946` +
  `e717a5e1a962a81f` (upwork)

## 1. `e6be98dfd6f4d639` (build-in-public) — REAL bug: `ApplyModelUsageToPhaseTokenUsageFile` merged a cumulative snapshot onto itself every turn

Filed claim: a daily cost file priced `claude-opus-5` at `sonnet-5` rates.
Confirmed in the live file
(`workspace-docs/Workflow/build-in-public/costs/phase/daily/2026-08-02.json`):
an entry keyed `claude-opus-5` whose output/cache rates back-compute to
exactly Sonnet‑5's $15/M and $0.30/M, not Opus‑5's $25/M and $0.50/M.

**Root cause**: the workflow-builder chat cost writer
(`cmd/server/server.go:6115-6206`) reads `mcpagent.ReadAgentDiagnostics(underlying).Usage`
on every chat turn — this is the coding agent's **session-cumulative**
running total (`a.cumulativePromptTokens` etc., `mcpagent/agent/agent.go:2599-2611`),
not a per-turn delta, because `underlying` is the same long-lived agent
object reused across turns via `api.sessionAgents[sessionID]`. That cumulative
snapshot was then passed to `ApplyModelUsageToPhaseTokenUsageFile`
(`cost_storage.go:725`), which used `MergeModelTokenUsage` — an **additive**
merge — to fold it into the already-persisted bucket. Every turn therefore
added the *entire, growing* cumulative total on top of what earlier turns had
already written, compounding turn over turn. If the session's active model
changed mid-session (auto-tier routing, a `/model` switch), the *entire*
blended cumulative total — including tokens spent under the previous model —
got priced at whichever model was current at write time and folded into that
model's bucket, producing exactly the observed opus-keyed-at-sonnet-rate
entry.

The code's own comment directly above the call site already stated the
intended behavior: *"One file per session — overwrites on each follow-up with
the full cumulative history."* The implementation just didn't match it.

## Fix

`ApplyModelUsageToPhaseTokenUsageFile` (`cost_storage.go`) now **replaces**
(`CloneModelTokenUsage`) the stored `ByModel`/`ByPhaseAndModel` bucket instead
of merging onto it, matching the caller's own documented intent. This
function has exactly one call site (the two calls at `server.go:6174` and
`:6199` both come from the same workflow-builder chat block, both fed the
same cumulative snapshot) — safe to change unconditionally, no other caller
depends on additive-merge semantics.

**Known residual limitation, not fixed**: `mcpagent.ReadAgentDiagnostics`
carries no per-model breakdown, only one blended cumulative total. If a
session's model changes mid-session, the new model's bucket is overwritten
with a total that already includes the prior model's tokens, while the prior
model's own bucket is left stale (not cleared). This is a data-source gap in
mcpagent's usage tracking, not something the ledger side can correct — it
would need mcpagent to track cumulative usage per-model. Documented in the
function's own comment rather than silently left unstated.

Tests (`pkg/orchestrator/plat073_phase_token_usage_overwrite_test.go`):
`TestApplyModelUsageToPhaseTokenUsageFileOverwritesRatherThanAccumulates`
(a two-turn scenario where the naive sum would be 3200 tokens but the correct
overwritten value is 2200 — the fresh cumulative snapshot) and
`TestApplyModelUsageToPhaseTokenUsageFileClonesRatherThanAliases` (the stored
bucket must not alias the caller's pointer).

## 2. `e6be98dfd6f4d639`'s second claim — NOT a bug, documented

*"every phase ledger zeroes/omits `input_cost_usd` (charging $0.000003 for
2.8M tokens)"* — verified against `2026-08-07.json`: `input_tokens: 2846936`
with `cache_tokens: 2846870` — virtually all of those 2.8M tokens are
cache-served. `calculatePricingFromModelData` correctly subtracts cache
tokens before pricing genuinely-fresh input (`base_orchestrator_tokens.go:217`),
so the real charge for that 2.8M-token turn is captured correctly under
`cache_cost_usd` ($1.79), not `input_cost_usd`. Same shape as the
already-closed `context_usage_percent` finding from this cluster: a
reviewer reading one field in isolation without noticing the cache
attribution. Documented in `file-layout.md` (see below) rather than treated
as a defect.

## 3. `43b988fe2ef952f3` (tectonicusadaytrading) — NOT a math bug, documented

*"`costs/phase/daily` reports $7.08/6 calls for a date where `costs/execution`
records $21.98/22 calls... no file declaring whether the two ledgers
overlap."* Confirmed: they don't overlap by design. `costs/phase/daily` is
written only by `PersistPhaseTokenUsage` (gated to the `planning` phase via
`IsPhaseOnlyAgent`, `base_orchestrator_tokens.go:714-724`) and the
workflow-builder chat writer above — it structurally never sees step-execution
calls. `costs/execution/{group}/{date}.json` is written on every
step-execution `TokenUsageEvent` and is the authoritative per-day total. No
code reconciles the two, and `file-layout.md` never mentioned
`costs/phase/daily/` at all, so a reviewer comparing the two totals had no way
to know the comparison was invalid. Fixed by documenting the scope
distinction explicitly (see below), not by changing behavior — unifying them
into one true daily rollup would require feeding `PersistPhaseTokenUsage`
from every step's `TokenUsageEvent`, a real design change with a much wider
blast radius than this ticket's scope.

## Documentation fix (2 and 3)

`cmd/server/guidance/templates/system/file-layout.md` — the
`costs/phase/token_usage.json` row now states its true scope (`planning`
phase + workflow-builder chat only, not workflow-wide) and the cache-cost
attribution rule; added a `costs/phase/daily/{YYYY-MM-DD}.json` row (was
entirely undocumented) with an explicit "do not compare against
`costs/execution/` and infer over/under-counting" note.

## 4. `a8ab091308579946` + `e717a5e1a962a81f` (upwork) — REAL gap, design-only, not implemented

*"date-wide overhead ledgers can't isolate current orchestrator/builder/Pulse
cost from historical, or diagnose the cached-input workload item-by-item."*
Confirmed real and **not** explained by the earlier `context_usage_percent`
removal (`e0e0494bf`) — that commit only removed a display-only field, unrelated
to scoping or per-item breakdown. `PhaseTokenUsageFile` has no
`execution_id`/`session_id` field at all — genuinely a pure date-wide sum with
no way to isolate one run's contribution.

The data model to solve this already exists but isn't exposed:
`pkg/costledger` + `pkg/costobserver` persist one row per LLM call to a
`cost_events` SQLite table with `execution_id`, `session_id`, and `scope`
(builder/pulse/workflow-execution) columns, and `Ledger.SummarizeExecution(executionID)`
already provides exactly the "isolate current vs historical, item by item"
query the finding wants — but it has exactly one caller in the whole
codebase (`pulse_agent_metrics.go:110`, used only to populate one internal
metrics row), not exposed via any tool or endpoint a reviewer could call.

**Not implemented here** — this needs a genuine design decision (what tool
surface, what access scope, whether raw per-event dumps are exposed or only
summaries) rather than a mechanical fix, consistent with how PLAT-069 (trend
measurement) was left design-only. Suggested next step: expose
`costledger.Ledger.SummarizeExecution` through a bridge tool or admin
endpoint the Pulse review agent can call directly.

## Verification

- `go build ./...` clean.
- New tests pass:
  `go test ./pkg/orchestrator/... -run TestApplyModelUsageToPhaseTokenUsageFile`.
- Full baseline (`go test ./cmd/server/... ./pkg/orchestrator/...`) still
  shows exactly 22 pre-existing failures — no new failures.
- **Not yet reverified live** — requires a restart and a multi-turn
  workflow-builder chat session before `e6be98dfd6f4d639` can be closed;
  `43b988fe2ef952f3` and the `input_cost_usd` half of `e6be98dfd6f4d639`
  can be closed once the documentation change is confirmed to prevent
  re-filing (or immediately, as "not a platform defect" via
  `resolve_run_concern(status="rejected")`, per PLAT-077's cluster H
  mechanism note).

## Acceptance

- A multi-turn workflow-builder chat session's persisted `costs/phase/`
  totals equal the session's actual final cumulative usage, not a multiple
  of it.
- `file-layout.md` states the true scope of `costs/phase/*` and warns against
  comparing it to `costs/execution/*` as if they were the same total.
- `a8ab091308579946`/`e717a5e1a962a81f` remain open, correctly, pending a
  design decision on exposing `costledger`'s existing per-execution query
  surface.

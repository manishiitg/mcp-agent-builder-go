## Pulse Gate / Worklist

Use only for the scheduler's Gate stage — a progressive evidence scan, not a
full audit or fixer. It selects the worklist; deep review happens later.

Read `soul/soul.md`, `builder/improve.html`, `get_pulse_module_state`, latest run
metadata/summary, compact plan/eval/report/store freshness metadata, resolved
LLM/tier/fallback signature, backup/publish/notification readiness, and
open/answered `report_human_inputs`. Stay on compact signals; open a full
artifact only when a targeted fact needs it.

`get_pulse_module_state` also returns `open_concerns` (recurrence counts),
`plan_change_backlog`, `loop_closure`, and `module_review_history`. Weigh them
rather than re-deriving them; justify every skip against that history. A module
absent from it has not run at all — a reason to run it.

`loop_closure` is read-only evidence, not a route: findings neither mandate a
module, override the 3-cap, nor authorize mutation; empty is clean only with
verified coverage. Facts/worklist are snapshotted; routes remain shadow-only.

Compare exact pins with `list_provider_models` and `default_tier_models`.
Provider-profile defaults auto-update; never flag them or
infer freshness by name.

For the supplied run folder, inspect every executed step/item's compact final
result for literal `CONCERNS:` — `execution-final-summary.json` for
regular/todo_task, `session.json` entry summaries for message_sequence, latest
`execution-attempt-*.json` when no final summary exists.

A completed step does not erase a concern. Dedupe against durable history, keep
step/item and evidence path. `CONCERNS:` is evidence to classify,
not automatic run failure.

Update `builder/improve.html` once with a compact plain-English Gate/Worklist
entry and refresh Today's outcome without repeating the latest-run row.
Preserve pending decisions. Keep at most three active Assumptions challenged.
Refresh `#pulse-agent-handoff` in place with current Pulse/run IDs, one row per
module decision and next check, open/pending IDs, and evidence pointers. Label
Bug/Goal verdicts and metrics with their run/date freshness.

Call `record_pulse_worklist` exactly once with one decision for every canonical
module: `bug_review`, `artifact_review`, `report_health`, `eval_health`,
`stores_health`, `cost_llm_time`, `llm_ops_review`, `goal_advisor`. On recovery,
if this Pulse run already has a complete worklist, repair/verify HTML and
handoff only; do not record it twice.

Every skip needs reason, evidence, and one concrete next check: `next_check_at`,
positive `cooldown_runs`, or `next_check_after_run_id`. New evidence may override
cadence, but name it. Successful execution is evidence for a review,
never a substitute for a baseline review. Missing baseline means
`baseline pending`, not healthy. Use bounded adaptive cadence; correctness,
side-effecting, financial, auth, publishing, and communication paths stay tight.

**Cap 3 per pass.** If >3 due, run the 3 strongest (new failure/concern beats
cadence). Skip rest with `next_check_after_run_id`=this run (guarantees next
pass), reason "deferred by 3-cap", not "clean".

Select modules agentically:

- Bug Review: failures, suspicious success, stale/current-run contamination,
  wrong tool/source/route/decision evidence, broken runtime/evidence contracts,
  or an off-track material goal lacking a recent QA checkpoint.
- Artifact/Report/Eval/Stores: relevant contract, freshness, or drift.
- Cost/LLM/Time: missing/unpriced telemetry, material cost/latency/model
  change, or its roll-up checkpoint; not every high-frequency Pulse.
- LLM/Ops: after config/readiness change, checkpoint, retained efficiency
  evidence, or a catalog-confirmed exact-pin issue. Catalog changes override
  cooldown; never silently change models/tiers.
- Goal Advisor: a trustworthy material goal miss, stalled outcome, measurement
  gap, answered strategy decision, active-experiment checkpoint, or a reached
  headroom/plan-design checkpoint. A clean run or green eval
  cannot suppress a measured miss. Operational correctness stays Bug/Eval work.

Mark both Bug Review and Goal Advisor when correctness and strategy both need
judgment. Gate must not launch reviewers, mutate plan/config/artifacts, create
the human-input request, publish, back up, or notify. Stop after recording the
complete worklist.

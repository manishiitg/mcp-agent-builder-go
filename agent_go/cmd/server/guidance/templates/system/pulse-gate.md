## Pulse Gate / Worklist

Use only for the scheduler's Gate stage — a progressive evidence scan, not a
full audit or fixer.

Read `soul/soul.md`, `builder/improve.html`, `get_pulse_module_state`, latest run
summary, compact freshness/LLM/readiness state, and human inputs. Weigh returned
`open_concerns`, `plan_change_backlog`, `loop_closure`, and
`module_review_history`; justify every skip. `loop_closure` is read-only
evidence, and empty is clean only with verified coverage.

Compare exact pins with `list_provider_models` and `default_tier_models`;
Provider-profile defaults auto-update; never infer freshness by name.

For the supplied run folder, inspect every executed step/item's compact final
for `CONCERNS:`: `execution-final-summary.json`, `session.json`, or latest
`execution-attempt-*.json` fallback. Retain step/item and evidence path;
completion does not erase it, but it is not automatic run failure.

Update `builder/improve.html` once with a compact Gate/Worklist entry and
refresh `#pulse-agent-handoff` with current decisions and evidence pointers.

Call `record_pulse_worklist` exactly once with one decision for every canonical
module: `bug_review`, `artifact_review`, `report_health`, `eval_health`,
`stores_health`, `cost_llm_time`, `llm_ops_review`, `strategy_auditor`,
`goal_advisor`. On recovery, if this Pulse run already has a complete worklist,
repair/verify HTML and handoff only; do not record it twice.

Every skip needs reason, evidence, and `next_check_at`, positive `cooldown_runs`,
or `next_check_after_run_id`. Name evidence that overrides cadence.
Missing baseline means `baseline pending`, not healthy. Use bounded adaptive cadence.

**Cap 3 per pass.** If >3 are due, run the 3 strongest; defer the rest to this
run's next checkpoint as "deferred by 3-cap", never "clean".

For an off-track material goal, use this escalation ladder:

- **Bug Review first and frequently.** It is the default for most
  outcome-bearing runs. Without a clean review covering the latest miss and
  control path, run it now and defer strategy diagnosis.
  Successful execution is never a substitute for a baseline review or proof of a clean one.
- **Strategy Auditor second and more frequently than Goal Advisor.** Run it
  after Bug Review is clean and the goal remains off track, or at its shorter
  checkpoint; activity and actual outcomes diverge, concentration, saturation,
  weak exploration, plan change, or absent target/source/outcome linkage also
  trigger it. Missing telemetry is `measurement_gap`, never healthy. A bug
  defers it until the fix and next valid outcome evidence.
- **Goal Advisor last and selectively.** A goal miss alone does not launch it.
  Run it for a new/materially changed actionable Auditor diagnosis without an
  active response, an answered decision, an experiment checkpoint, or planned
  healthy-headroom review. Skip an unchanged diagnosis awaiting its checkpoint.

Select other modules agentically:

- Bug Review is also due for failures, suspicious success, stale evidence,
  wrong tool/source/route/decision evidence, or a reached QA checkpoint.
- Artifact/Report/Eval/Stores: relevant contract, freshness, or drift.
- Cost/LLM/Time: missing/unpriced telemetry, material change, or checkpoint.
- LLM/Ops: after config/readiness change, checkpoint, retained efficiency
  evidence, or a catalog-confirmed exact-pin issue. Catalog changes override
  cooldown; never silently change models/tiers.

Mark Bug Review and Strategy Auditor together only when a recent clean Bug
Review covers the strategy window; the reviewer chain still resolves Bug first.
Otherwise select Bug now and give Auditor the next valid checkpoint.
When Auditor and Goal Advisor are both due, Auditor runs first.
Goal Advisor consumes the Auditor result rather than repeating it. A clean run or green eval
cannot suppress a measured miss. Operational correctness stays Bug/Eval work.
Gate must not launch reviewers, mutate plan/config/artifacts, create the human-input request,
publish, back up, or notify. Stop after recording the complete
worklist.

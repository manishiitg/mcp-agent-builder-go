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

Gate owns the durable worklist and the cheap per-run goal observation checkpoint.
Do not write HTML, a recovery ledger, or any workflow artifact; the dedicated
Dashboard projects recorded state later.

Call `record_pulse_worklist` exactly once with one decision for every canonical
module: `workflow_review`, `strategy_auditor`, `goal_advisor`.
`workflow_review` is one continuous read-only agent with ordered correctness,
artifact, report/eval, stores, and LLM/tool-operations lenses. Do not emit the
retired focused operational modules or a separate cost/time module.
On recovery, if this Pulse run already has a complete worklist,
verify and stop; do not record it twice.

After recording the worklist, preserve longitudinal evidence without launching
another agent. If the just-completed producing run contains one or more
trustworthy comparable success-criterion measurements, call
`record_pulse_impact` exactly once with `observations` only. Use the stable
criterion id, metric, producing run id, route/environment, numeric value or an
honest qualitative status, timestamp, unit, and exact evidence provenance.
The observation identity is idempotent, so recovery may safely retry the same
run/criterion/metric/route. If no trustworthy observation exists, do not call
the tool and do not fabricate one. Gate never creates interventions or impact
assessments; the single Fixer links verified work and judges matured windows.

Every skip needs reason, evidence, and `next_check_at`, positive `cooldown_runs`,
or `next_check_after_run_id`. Name evidence that overrides cadence.
Missing baseline means `baseline pending`, not healthy. Use bounded adaptive cadence.

Backlog drainage outranks broad discovery. When active findings, answered
decisions, unfinished fix attempts, or awaiting-verification work exist, select
the owning modules to drain that work and require them to use retained evidence
before launching another reviewer. Still select Workflow Review for a new failed or
suspicious run so a new P0 cannot hide behind the backlog. Defer discovery-only
health/advisory reviews whose artifacts and evidence did not materially change;
give each an exact backlog-drained, artifact-change, or next-valid-run check.
Do not repeatedly spend a pass rediscovering unchanged findings.

Select every module with actionable repair/verification work or genuinely new
trigger evidence. Module stages are independent and run in bounded parallel
batches: one failed or hung reviewer must not block saved evidence or another
reviewer. The scheduler waits for all selected reviewer outcomes before the
single consolidated Fixer starts.
When work must wait for a real evidence/user/external boundary, record that
boundary instead of inventing a capacity cooldown.

For an off-track material goal, select independent lenses according to their
own evidence and cadence:

- **Workflow Review frequently.** It owns execution correctness, safe
  exploratory QA, artifact/report/eval/store truth, and LLM/tool operations in
  one continuous context. Successful execution is never proof that behavior
  was correct.
- **Strategy Auditor more frequently than Goal Advisor.** Select it when the
  current strategy needs an independent completeness/effectiveness audit:
  activity and outcomes diverge, concentration, saturation, weak exploration,
  plan change, or absent target/source/outcome linkage. It improves the current
  strategic shape. Missing telemetry is `measurement_gap`, never healthy.
- **Goal Advisor selectively.** Select it for an independent blank-sheet
  opportunity review, an answered strategic decision, an experiment checkpoint,
  or planned healthy-headroom review. It explores materially different approaches
  outside the current plan; it is not a downstream handler for Auditor findings.

Workflow Review is also due for failures, suspicious success, stale evidence,
wrong tool/source/route/decision evidence, a reached QA checkpoint, relevant
artifact/report/eval/store drift, missing or unpriced telemetry, material
cost/latency change, retained tool/runtime evidence, config/readiness change, or
a catalog-confirmed exact-pin issue. Catalog changes override cooldown; never
silently change models or tiers.

Never make one reviewer due, skipped, or delayed because another reviewer has
or has not run. When evidence is unreliable, select every independently due
lens and let that reviewer return `execution_problem` or `insufficient_evidence`
within its own result. Strategy Auditor and Goal Advisor must not consume each
other's conclusions; agreement is corroboration discovered only during later
consolidation. A clean run or green eval cannot suppress a measured miss.
Operational correctness stays Workflow Review work.
Gate must not launch reviewers, mutate plan/config/artifacts, create the human-input request,
publish, back up, notify, or write HTML. Stop after recording the complete
worklist and any honest current-run goal observations.

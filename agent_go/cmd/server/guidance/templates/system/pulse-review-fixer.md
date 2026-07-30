## Pulse consolidated review and single fixer

Use only after Gate. The scheduler supplies due modules, Pulse run ID, and dated
review run ID. This parent owns all unresolved due modules.

Read module/worklist state and saved reviewer artifacts. On recovery inspect
current target/runtime and verification evidence; never blindly reapply partial work. Preserve `changed_unverified` until its evidence boundary.

Resolve the off-track diagnostic chain before normal batching. When Bug Review
and Strategy Auditor are due for the same evidence window, run Bug Review alone first.
A confirmed correctness bug that invalidates that window defers Auditor:
record its terminal result as skipped/blocked with the exact post-fix,
outcome-bearing checkpoint. Otherwise run Auditor. Strategy Auditor runs before Goal Advisor;
launch Advisor only for an actionable diagnosis or its own
answered-decision/experiment checkpoint.

Create each remaining **READ-ONLY REVIEW** task in consecutive batches of at most two
`call_generic_agent` calls. In coding-agent mode, use its documented API
bridge shell call; shell/curl is the supported transport. Never use background
agents, sleep, or polling. If the outer call backgrounds, stop and await the
automatic notification. Pass exact `pulse_run_id`, dated `review_run_id`, and
module. The backend saves
`pulse/reviews/<dated-review-run-id>/<module>.md`; read that sole findings
artifact before fixing. Reviewer failure fails only its module.

Give each reviewer scope, Gate evidence, focused guidance, and a response cap:
`pulse-bug-review`; `review-artifact-drift`; matching `improve-*` health guide;
`llm-selection` plus cost/timing evidence; `strategy-auditor` plus cross-run
DB/run evidence; or the Auditor diagnosis plus goal/experiment evidence for Goal
Advisor. Strategy Auditor never shares Goal Advisor's parallel batch. It returns
one of `strategy_flaw`, `execution_bug`, `measurement_gap`,
`insufficient_evidence`, or `no_material_problem`, without prescribing a plan
mutation. Goal Advisor must lead with strategy ceiling, one materially different
thesis, relationship to the active experiment, and why incremental repair is
insufficient. Reject maintenance- or instrumentation-only Advisor results.
Reviewers never edit, publish, notify, ask the user, write HTML, or mark state.

Require a verdict, next check, and at most five ordered findings with stable ID,
target, claim, evidence, bounded fix, verification, and user-judgment reason.
Clean means an empty finding-ID manifest; never drop correctness to meet the cap.
An Auditor `measurement_gap` names the missing target/source/action/outcome
linkage and blocked decision. Give Goal Advisor a separate read-only critic.

Deduplicate findings and map conflicts by target. Resolve by explicit user
constraints, correctness/data integrity, preserved goal meaning, strategy
improvement, then cost. If evidence cannot decide, create one focused decision,
block affected modules, and do not mutate that target.

Then the same parent becomes the only Pulse Fixer. Apply safe fixes sequentially.
Before mutation capture targets, time, hashes/versions, and baseline. Load
`fix-verification`; old artifacts or successful writes are not proof. If proof
needs a future run, record `changed_unverified` / `awaiting_next_valid_run`.

Reconcile every finding ID to one disposition; missing/duplicates block its
module. Strategy/LLM-Ops changes need exact valid approval.
Strategy Auditor findings are diagnostic handoffs: `execution_bug` to Bug Review, attribution to
Eval Health, and `strategy_flaw` or strategy-critical `measurement_gap` to Goal
Advisor. Give a same-pass Goal Advisor the saved Auditor artifact and require it
to challenge the causal claim. Advisor operational findings remain handoffs.
Only the parent mutates workflow state and writes `builder/improve.html` once
after the consolidated pass.

Record `mark_pulse_module_result` for every due module.
Perform global finding-ID reconciliation and require a terminal current-run result for each before
claiming completion. Keep technical proof in reviewer files and SQLite. Record a
reviewer failure as `Review incomplete`, without a conclusion or unsupported
change.

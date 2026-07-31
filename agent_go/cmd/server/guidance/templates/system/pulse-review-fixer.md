## Pulse consolidated review and single fixer

Use only after Gate. The scheduler supplies due modules, Pulse run ID, and dated
review run ID. This parent owns all unresolved due modules.

Read module/worklist state and saved SQLite reviewer results. On recovery inspect
current target/runtime and verification evidence; never trust HTML or blindly
reapply partial work. Preserve `changed_unverified` until its evidence boundary.
For each due module, load and revalidate the complete active retained backlog;
do not merely emit fresh findings. Also load `suppressed_concerns`: an unchanged
externally owned fingerprint is not a new finding, while materially changed
evidence/target identity is a reopen candidate. Record every evidence-backed
new finding. Attempt every safe bounded fix in the selected module. Leave a
finding active only for a concrete blocker, decision, failed check, or future
evidence checkpoint.

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
module. The backend stores its complete Markdown directly in SQLite; call
`get_pulse_review_result` with that review run and module before fixing.
Reviewer failure fails only its module.

Give each reviewer scope, Gate evidence, focused guidance, and this response contract:
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

Require a verdict, next check, and every evidence-backed ordered finding with
stable ID, target, claim, evidence, bounded fix, verification, and judgment
reason. Classify retained findings rather than omitting them. Clean means an
empty finding-ID manifest.
An Auditor `measurement_gap` names the missing target/source/action/outcome
linkage and blocked decision. Give Goal Advisor a separate read-only critic.

Use `external_action_required` only when the finding is real but no workflow
change can address it. Record `external_owner`, a stable `reason_code`, and an
evidence/capability/user `reopen_condition`. This removes it from Pulse's active
queue and suppresses unchanged rediscovery while keeping it visible on the
external-action board. `blocked` remains for retryable/current blockers;
`awaiting_user` remains in the decision queue.

Deduplicate findings and map conflicts by target. Resolve by explicit user
constraints, correctness/data integrity, preserved goal meaning, strategy
improvement, then cost. If evidence cannot decide, create one focused decision,
block affected modules, and do not mutate that target.

Then the same parent becomes the only Pulse Fixer. Apply safe fixes sequentially.
Before mutation capture targets, time, hashes/versions, and baseline. Load
`fix-verification`; old artifacts or successful writes are not proof. If proof
needs a future run, record `changed_unverified` / `awaiting_next_valid_run`.
Re-read `get_pulse_module_state`, map each actionable finding to the fingerprint
created from its `CONCERNS:` line, and call `start_pulse_fix_attempt` before
mutation. Keep its `attempt_id`. If a finding lacks a fingerprint, block it as a
reviewer-contract failure instead of making an untracked change.

Reconcile every finding ID to one disposition; missing/duplicates block its
module. Strategy/LLM-Ops changes need exact valid approval.
Strategy Auditor findings are diagnostic handoffs: `execution_bug` to Bug Review, attribution to
Eval Health, and `strategy_flaw` or strategy-critical `measurement_gap` to Goal
Advisor. Give a same-pass Goal Advisor the saved Auditor artifact and require it
to challenge the causal claim. Advisor operational findings remain handoffs.
Only the parent mutates workflow state. Neither reviewer nor Fixer writes Pulse
HTML; the dedicated Dashboard owns it.

Record `mark_pulse_module_result` for every due module. For every finding pass a
structured `finding_dispositions` row with fingerprint, finding id, disposition,
summary, attempt id when changed, changed files, and exact verification objects.
Verification verdict is exactly `passed`, `failed`, or `inconclusive`.
`fixed_verified` closes only with passed post-change proof;
`changed_unverified` remains awaiting verification; failed proof reopens it.
Perform global finding-ID reconciliation and require a terminal current-run result for each before
claiming completion. Keep technical proof in SQLite review and lifecycle rows. Record a
reviewer failure as `Review incomplete`, without a conclusion or unsupported
change.

## Pulse independent backlog review and module fixer

Use only after Gate. The scheduler supplies one due module, Pulse run ID, and
dated review run ID. This parent owns that module until it has one terminal
result.

Read module/worklist state, `get_pulse_finding_backlog`, and saved SQLite reviewer results. On recovery inspect
current target/runtime and verification evidence; never trust HTML or blindly
reapply partial work. Preserve `changed_unverified` until its evidence boundary.
For the due module, load and revalidate the complete active retained backlog;
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

The scheduler invokes this contract once per due module, in module order. This
stage owns only the supplied module. First inspect its current-run result,
active retained backlog, answered decisions, awaiting-verification work, and
any already-saved reviewer result. Drain actionable retained work before doing
more discovery. Do not launch a reviewer merely because the module is due: if
the saved review and lifecycle evidence are sufficient to apply or verify a
bounded fix, do that now.

When fresh evidence or an evidence gap genuinely requires a **READ-ONLY REVIEW**,
make exactly one `call_generic_agent` call for this module. Never combine
reviewers in one shell command, run curl in the background, use `&`/`wait`, or
wait for another module. In coding-agent mode, use the documented API bridge
shell transport without imposing a short shell timeout on the reviewer. If the
outer bridge call backgrounds, stop and await its automatic notification.
Pass exact `pulse_run_id`, dated `review_run_id`, and module. The backend stores
its complete Markdown directly in SQLite; call `get_pulse_review_result` with
that review run and module before fixing. Immediately process a successful
result; do not wait for any other reviewer. Reviewer failure is a terminal
`Review incomplete` result for this module only and cannot block later modules.

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

A tool refusal is not evidence that a finding is unfixable. Check the target's
actual type before concluding anything: rtslatency recorded two collectors as
"not editable" after `update_scripted_step` was refused, when they were
message_sequence steps and `update_message_sequence_step` was in the same tool
surface. Before `blocked` or `external_action_required` on a rejected edit, name
the tool you used, the target's real type, and the tool that type requires.

Use `external_action_required` only when the finding is real but no workflow
change can address it. Record `external_owner`, a stable `reason_code`, and an
evidence/capability/user `reopen_condition`. This removes it from Pulse's active
queue and suppresses unchanged rediscovery while keeping it visible on the
external-action board. `awaiting_run` is a real finding waiting only on a scheduled run to produce its
evidence: no fix was applied and nobody is stuck. It requires `next_check`
naming that run. Use it rather than `blocked` whenever the answer is "the data
does not exist yet" — calling that blocked points the operator at a decision
that does not exist and hides the ones that do.
`blocked` remains for retryable/current blockers;
Escalate to the operator only for a real decision. Before choosing
`awaiting_user`, state the cost of acting, the alternative, and why `soul.md`
does not already settle it. If the goal implies the answer and the cost is
negligible, decide, act, and record the reasoning — asking anyway spends the
operator's attention and buries the decisions that genuinely need them. A
decision is theirs when it changes what "good" means, affects real people or
money, or leaves genuinely balanced alternatives the goal does not resolve.
rtslatency asked whether to retain per-turn latency rows: ~30MB a year against a
2MB database, required by its own success criterion for reproducible
percentiles. Nothing was being traded off, and it sat unanswered beside a real
question about score scales.

`awaiting_user` remains in the decision queue and requires a still-pending
`create_human_input_request`, passed as `human_input_id`. Create the decision
first: a finding marked awaiting_user with no question leaves the operator told
that something needs them and given nothing to answer.

Deduplicate findings before filing: match stable target/component, behavioral
claim, and evidence boundary against active and suppressed findings. Reuse the
existing fingerprint and exact `CONCERNS:` payload for unchanged or newly
evidenced instances, and record the new evidence in the immutable review body.
Create a new finding only for a genuinely different target/claim, or reopen one
whose recorded reopen condition is now satisfied. Then map conflicts by target. Resolve by explicit user
constraints, correctness/data integrity, preserved goal meaning, strategy
improvement, then cost. If evidence cannot decide, create one focused decision,
block affected modules, and do not mutate that target.

Then start the only Pulse Fixer for this module as one `call_generic_agent` with
`role="fixer"`, passing this run's `pulse_run_id`, a fresh `review_run_id`, and
`module`. Never run two at once: it is the single writer. Do not apply fixes
inline in this turn — a scheduled turn's model is pinned for its whole life, so
an inline fixer mutates on a weaker tier than the reviewers that only read. The
stage runs on the maintenance tier and is lent write authority for this run
alone. It applies safe fixes sequentially.
Before mutation capture targets, time, hashes/versions, and baseline. Load
`fix-verification`; old artifacts or successful writes are not proof. If proof
needs a future run, record `changed_unverified` / `awaiting_next_valid_run`.
Re-read `get_pulse_module_state`, map each actionable finding to the fingerprint
created from its `CONCERNS:` line, and call `start_pulse_fix_attempt` before
mutation. From `get_pulse_finding_backlog`, pass `issue.id` as `finding_id` and
the fingerprint from that same item; keep its `attempt_id`. IDs address records
but never decide semantic sameness. If a finding lacks either value, block it as
a reviewer-contract failure instead of making an untracked change.

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
Perform module finding-ID reconciliation and require a terminal current-run result before
claiming completion. Keep technical proof in SQLite review and lifecycle rows. Record a
reviewer failure as `Review incomplete`, without a conclusion or unsupported
change.

## Pulse selected-lane review and sequenced fixing

Use only after Gate. The scheduler supplies the due modules, Pulse run ID, and
dated review run ID. Gate decides separately whether `workflow_review`
(Engineering Review), `llm_ops_review`, `strategy_auditor`, and `goal_advisor`
are due. Engineering and LLM/Ops share one context with separate perspective
turns, a persisted consolidation checkpoint, and then one bounded Fixer turn in
that same agent; skipped perspectives are not sent. Strategy Auditor and Goal
Advisor remain independent read-only agents. A residual Fixer runs afterward
only when those independent modules or a failed operational sequence still lack
a terminal result.

Read module/worklist state, `get_pulse_state(view="backlog")`, and saved SQLite reviewer results. On recovery inspect
current target/runtime and verification evidence; never trust HTML or blindly
reapply partial work. Preserve `changed_unverified` until its evidence boundary.
For the due module, load and revalidate the complete active retained backlog;
do not merely emit fresh findings. Also load `suppressed_concerns`: an unchanged
externally owned fingerprint is not a new finding, while materially changed
evidence/target identity is a reopen candidate. Record every evidence-backed
new finding. Attempt every safe bounded fix in the selected module. Leave a
finding active only for a concrete blocker, decision, failed check, or future
evidence checkpoint.

Strategy Auditor and Goal Advisor are independent and may run in one bounded
parallel batch. Complete that read-only batch before the shared operational
review-and-fix sequence starts, so no reviewer observes artifacts changing under
it. The operational lanes remain independent in meaning but intentionally share
evidence and conversation context with their own Fixer turn.
An unreliable evidence window is classified inside the affected review as an
execution problem or insufficient evidence; it does not cancel another review.
Goal Advisor is selected only for its own blank-sheet opportunity, answered
decision, healthy-headroom, or experiment-checkpoint trigger—not as a handler
for a Strategy Auditor result.

The scheduler invokes at most one shared operational reviewer and at most one
agent for each selected strategic module. The operational reviewer receives an
exact `review_lanes` list in canonical order and owns only those lanes. Each
strategic stage owns only its supplied module. First inspect current-run result,
active retained backlog, answered decisions, awaiting-verification work, and
any already-saved reviewer result. Do not launch a reviewer merely because the
module is due: if saved review and lifecycle evidence already answer the review
question, stop and leave that evidence for the applicable final Fixer turn. Reviewer
independent Strategy/Goal stages never mutate, start fix attempts, or mark
module state. The operational sequence remains read-only through its lane and
consolidation turns; only its final Fixer turn mutates and marks its selected
lanes.

When fresh evidence or an evidence gap genuinely requires a **READ-ONLY REVIEW**,
make exactly one `call_generic_agent` call for the independent module.
`workflow_review` instead uses one `role="fixer"` agent for only the Gate-selected
ordered operational lanes followed by its bounded Fixer turn; pass the exact
non-empty `review_lanes` list and do not spawn one child per lane.
Never combine the independent agents in one shell command, run curl
in the background, use `&`/`wait`, or wait for another module. In coding-agent mode, use the documented API bridge
shell transport. The call returns an `execution_id` immediately; record it,
end the current turn, and resume only from its automatic notification of completion.
Pass exact `pulse_run_id`, dated `review_run_id`, and module. The backend stores
its complete Markdown and structured verification results directly in SQLite;
call `get_pulse_state(view="review")` with that review run and module before the review
stage ends. Reviewer failure is retained as `Review incomplete` evidence for
this module only and cannot block later reviewers. The operational agent records
its selected lane results; the residual Fixer records only still-unresolved
independent module results.

### Compact response contract

SQLite lifecycle rows, saved review records, run artifacts, and tool history are
the proof store. A reviewer response is an evidence index for people and the
next turn, **not** an investigation transcript. Do not paste raw logs, SQL
rows, full tool results, source excerpts, or reasoning already captured in a
prior lane. Use exact paths, query names, IDs, and short observed values as
evidence pointers instead.

For an operational consolidated review, return a 2–3 sentence executive verdict
followed by one compact entry per valid finding: claim, impact, next action, and
evidence pointers. Merge the same root cause but retain every distinct valid
finding. Keep it brief; compact wording must never cause a finding to disappear.
The final Fixer turn is only a short change/verification/blocker outcome. It
must not repeat or reconsolidate the review.

If a saved review has status `contract_failed`, the backend retained its raw
Markdown but quarantined its invalid structured verification markers. Do not
copy, repair, or route those markers. Mark that module `failed` with the exact
contract error, continue processing every other due module, and leave its
findings unchanged for a clean reviewer retry on the next pass.

Give each reviewer scope, Gate evidence, focused guidance, and this response
contract. Engineering Review conditionally loads execution, artifact-drift,
report/eval implementation, and store-integrity evidence packs; those are not
separate reviewer identities. LLM/Ops evaluates correct execution for cost,
latency, model, tool, retry, and runtime fitness. The shared operational reviewer
executes only selected perspectives in canonical order, reuses shared evidence,
and consolidates the same root cause before returning. Strategy and Goal remain
fresh independent contexts. Load docs with
`read_skill(skills=[{"name":"builder-reference","path":"references/<name>.md"}])`:
`pulse-bug-review`; `review-artifact-drift`; matching `improve-*` health guide;
`llm-selection` plus cost/timing evidence; `strategy-auditor` plus cross-run
DB/run evidence; or goal/constraint/outcome and experiment evidence for Goal
Advisor. Strategy Auditor and Goal Advisor may share a parallel batch and must
reason independently. Strategy Auditor reasons from a product/business
perspective and returns a user-facing in-strategy improvement brief. A report or
evaluation that is technically correct but does not measure useful goal progress
is its `measurement_gap`; broken implementation belongs to Engineering Review.
Goal Advisor must lead with strategy ceiling, one materially different
thesis, relationship to the active experiment, and why incremental repair is
insufficient. Reject maintenance- or instrumentation-only Advisor results.
Reviewers never edit, publish, notify, ask the user, write HTML, or mark state.

The Engineering store-integrity evidence pack must load `improve-learnings`,
`improve-knowledge`, and `improve-database` when selected by Gate evidence. Its
learning review covers the complete skill package, not
only the root index or a sample of references, and audits every effective
read-write `learning_objective`. References are part of the skill: only
reusable execution HOW may remain anywhere under `learnings/_global/`.
Relocating business facts, run state, owner values, strategy, incidents,
decisions, provenance, or architecture history from `SKILL.md` into a reference
does not fix the finding.

The store-integrity pack returns one reconciled `ownership_manifest`, not three
disconnected lists. Each misplaced or duplicated item names its current
location, semantic type, authoritative owner, duplicate locations, bounded
migration/removal action, and verification. Enforce one semantic item, one
authoritative owner across Soul, Plan/step config, Validation, Learnings,
Knowledgebase, DB, and Pulse. The KB lens inventories every content-bearing
note in `kb_purity_manifest`; the DB lens maps every relevant table and
content-bearing TEXT/JSON column in `db_ownership_manifest`. Stable references
to canonical records are valid; copied content is not. Lock recommendations are
valid only after the complete relevant manifest is clean, with learning locks
proven per step from its own objective, description hash, successful runs, and
`.learning_metadata.json` rather than the shared global skill alone.

**Verify before discovering.** The reviewer is the independent check on fixes it
did not make. Before looking for anything new, take every `changed_unverified`
finding this module owns whose `next_check` evidence has since arrived, and
judge it against the post-change evidence: `passed`, `failed`, or
`inconclusive`. Emit the required `PULSE_VERIFICATION_JSON` marker with
finding ID, fingerprint, attempt ID, verdict, expected, observed, evidence, and
next-check boundary when inconclusive. The backend validates and transports
these records separately from prose. Return those verdicts as a verification
list, separate from new findings. The Fixer routes them — passed closes the finding as `fixed_verified`,
failed reopens it, inconclusive leaves it awaiting the boundary it still names.

Verification does not count against the finding cap. It is not discovery, and
charging it to the same budget makes a reviewer choose between confirming past
work and finding new problems.

Without this, only failure is detectable. A fix that worked is never confirmed,
so it is re-attempted every pass: rtslatency carried a finding at seen_count 4,
still awaiting_verification, repaired again on each cycle because nothing ever
checked the run that had since produced its evidence.

Require a verdict, next check, and every evidence-backed ordered finding with
stable ID, target, claim, evidence, bounded fix, verification, and judgment
reason. Classify retained findings rather than omitting them. Clean means an
empty finding-ID manifest.
An Auditor `measurement_gap` names the missing target/source/action/outcome
linkage and blocked decision. Goal Advisor uses its own separate read-only critic.

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

Route outputs by professional perspective. Engineering Review normally creates
an actionable issue and repair/verification lifecycle, never a product proposal.
LLM/Ops normally creates a safe optimization issue; ask only for a real
quality-versus-cost, spend, or reliability tradeoff. Strategy Auditor creates a
user-facing in-strategy improvement artifact. Enforce its declared route:
`decision_required` must create and link one `awaiting_user` decision;
`evidence_wait` becomes `proposal_only` only with the exact non-empty
`next_check`; `fixer_handoff` must be attempted through the normal safe repair
lifecycle rather than parked as a proposal. Use
`create_human_input_request(source="strategy_auditor", input_id="strategy-proposal-...")`
for a Strategy decision so the UI preserves who asked. Goal Advisor creates a
materially different opportunity artifact. Its actionable thesis must create an
approve/reject/defer decision with `source="goal_advisor"` and
`input_id="plan-proposal-..."`; only a thesis explicitly waiting on named future
evidence may use `proposal_only`, with that boundary in `next_check`. Route any
technical prerequisite to the Fixer and never apply a materially different
approach before that exact approval. Engineering may reach
`awaiting_user` only for an exceptional repair that changes business meaning,
affects real users or money, or leaves a genuinely balanced choice not settled
by `soul.md`.

Reconcile answered advisor-specialization decisions separately from finding
dispositions. For a `report_human_inputs` row with source `pulse` and id prefix
`advisor-specialization-`: on `activate`, call
`update_workflow_config(advisor_specialization_approval_input_id="<id>")` so the
tool resolves and activates the exact approved pair; never copy or rewrite the
texts yourself. On `reject`, preserve the current specialization and consume the
decision with that outcome. On `revise`, call
`get_workflow_command_guidance(kind="specialize-advisors", focus="<the owner's revision note>")`,
create the replacement proposal it specifies, then consume the old decision only
after the replacement exists. These decisions are configuration work, not Pulse
findings, and must not be converted into duplicate issues.

`awaiting_user` remains in the decision queue and requires a still-pending
`create_human_input_request`, passed as `human_input_id`. Create the decision
first: a finding marked awaiting_user with no question leaves the operator told
that something needs them and given nothing to answer.
For `strategy_auditor`, the question source must be `strategy_auditor` and its
id must start `strategy-proposal-`. For `goal_advisor`, the source must be
`goal_advisor` and its id must start `plan-proposal-`. The backend rejects an
advisor `proposal_only` disposition with no `next_check`, so every accepted
advisor recommendation has a concrete route to action or evidence.

Deduplicate findings before filing: match stable target/component, behavioral
claim, and evidence boundary against active and suppressed findings. Reuse the
existing fingerprint and exact `CONCERNS:` payload for unchanged or newly
evidenced instances, and record the new evidence in the immutable review body.
Create a new finding only for a genuinely different target/claim, or reopen one
whose recorded reopen condition is now satisfied. Then map conflicts by target. Resolve by explicit user
constraints, correctness/data integrity, preserved goal meaning, strategy
improvement, then cost. If evidence cannot decide, create one focused decision,
block affected modules, and do not mutate that target.

For selected Engineering/LLM-Ops lanes, call one generic agent with
`role="fixer"`, `module="workflow_review"`, the exact `review_lanes`, and this
run's identities. The backend constructs the review turns, persists the
consolidated review and filed concerns, then sends the bounded Fixer turn to the
same conversation. It runs on the maintenance tier with write authority limited
to this Pulse run. Never launch a second operational Fixer.

After that sequence, start `module="pulse_fixer"` only when a due module is
still non-terminal—normally an independent Strategy/Goal result needing
lifecycle or decision handling, or recovery from a failed operational sequence.
It preserves terminal operational results and processes only unresolved due
modules. If every due module is terminal, skip the residual Fixer entirely.

The Fixer first calls `get_pulse_state(view="backlog")` without a module filter
and freezes the complete active starting manifest. Due reviewer modules decide
which reviews ran; they do not narrow the consolidated Fixer's retained
backlog. Include every owning module represented by that manifest in the Fixer
instructions and lifecycle close-out. It then builds one compact,
priority-ordered Fix queue. Each queue item is a coherent repair bundle. Group findings only when
they share the same root cause, require compatible changes to the same target,
and have one verification condition. Cross-reviewer grouping is allowed;
conflicts remain separate. Waiting-on-run, waiting-on-user, proposal-only, and
externally owned findings stay visible but do not enter the actionable queue.
There is no arbitrary queue cap and no finding may disappear. Process bundles
sequentially and checkpoint/disposition one bundle before beginning the next; the
backend opens the durable fix-attempt record from the disposition you write, so
there is nothing to declare before mutating.
Before mutation capture targets, time, hashes/versions, and baseline. Load
`read_skill(skills=[{"name":"builder-reference","path":"references/pulse-fixer-practices.md"},{"name":"builder-reference","path":"references/fix-verification.md"}])`;
follow the engineering-practices reference to diagnose and bundle the root cause,
including its **Full-backlog drain contract**, then the verification reference
to establish proof. Maintain the exact remaining finding-ID/fingerprint list
throughout the pass and reconcile it with a final unfiltered
`get_pulse_state(view="backlog")` read;
old artifacts or successful writes are not proof. If proof
needs a future run, record `changed_unverified` / `awaiting_next_valid_run`.

For learning-content repairs, follow the practices reference's **Learning and
skill purity repair** contract. Re-read the whole content-bearing skill package
and changed step configs after mutation. A smaller `SKILL.md`, valid Markdown,
or content moved into `references/` is not proof; semantic purity and correct
objective/access pairing are the postcondition.
Re-read `get_pulse_state(view="module")` and map each actionable finding to the
fingerprint created from its `CONCERNS:` line. From `get_pulse_state(view="backlog")`,
pass `issue.id` as `finding_id` and the fingerprint from that same item. IDs
address records but never decide semantic sameness. If a finding lacks either value, block it as
a reviewer-contract failure instead of making an untracked change.

Reconcile every starting finding ID/fingerprint pair to one disposition or one
checked, still-unmet waiting/external boundary; missing/duplicates block its
module and the Fixer must not claim completion. Strategy or model-routing changes need exact valid approval. Preserve each
reviewer's conclusion under its owning module. Strategy Auditor findings contain
bounded improvements within the current strategic shape; Goal Advisor findings
contain materially different proposal-only approaches. Operational observations
may be cross-referenced to the matching module during consolidation, but never
rewritten as a dependency or used to suppress that module's independent result.
Advisor operational findings remain out-of-scope observations.
Only a Fixer turn mutates workflow state. Neither reviewer turns nor Fixer turns
write Pulse HTML; the dedicated Dashboard owns it.

Before finishing, call `record_pulse_impact` once when there is an intervention,
a matured assessment, or a trustworthy observation Gate did not already store.
Load the retained impact ledger first; do not duplicate Gate's current-run
observations. Create or advance one intervention per coherent verified repair
bundle or approved strategy experiment, and append an assessment only when its
comparable evidence window matured. Classify operational repairs as `reliability`, instrumentation
as `measurement`, and dashboard-only work as `presentation_maintenance`; none
may claim `direct_goal` impact. Use `inconclusive` or `confounded` rather than
inventing attribution when observations are missing or interventions overlap.

Record `record_pulse_result` exactly once for every due module. For every finding pass a
structured `finding_dispositions` row with fingerprint, finding id, disposition,
summary, changed files, and exact verification objects.
Verification verdict is exactly `passed`, `failed`, or `inconclusive`.
`fixed_verified` closes only with passed post-change proof;
`changed_unverified` remains awaiting verification; failed proof reopens it.
Perform module finding-ID reconciliation and require a terminal current-run result before
claiming completion. Keep technical proof in SQLite review and lifecycle rows. Record a
reviewer failure as `Review incomplete`, without a conclusion or unsupported
change.

## Pulse agent-owned review and fixing

Use only after Gate. This is the Review+Fix turn in the same main-agent
conversation that received Gate and will later receive Finalize.

Read `get_pulse_state(view="module", pulse_run_id=<current Pulse run>)` before
dispatching any child. Its `gate_mode` is the contract for this pass:

- In `backlog_drain`, first verify every due `changed_unverified` issue against
  newly available producing-run evidence, then repair retained active roots.
  Do not run broad discovery. Launch the Strategic Review child only when
  retained strategic work has matured verification/disposition evidence, and
  omit its opportunity phase. A new finding is
  allowed only when the repair/verification work uncovers a genuinely different
  root cause.
- In `discovery`, perform the selected evidence-driven reviews, still updating
  existing roots before filing a distinct new one.
- In `strategy`, run only the selected Strategic Review work; do
  not turn it into an Engineering discovery pass.
- In `observe`, do not launch a reviewer or fixer; record the required skipped
  receipts and wait for the named evidence boundary.
Gate decides separately whether `workflow_review` (Engineering Review),
`llm_ops_review`, and `strategic_review` are due. Read that
durable worklist yourself. Go does not choose reviewers or automatically
launch a residual Fixer/recovery agent.

## Dispatch model

The first Review+Fix turn is a launcher, not a long-running parent review.
Use `run_in_background` for every selected child. When Engineering and/or
Operations are due, use one `agent_type="executor"` child with one ordered
`message_sequence`. Give it the exact run-scoped checkpoint path from the
dispatch message. Every turn reads that file first and updates it before
ending, so context compaction cannot erase evidence or decisions. SQLite is
still authoritative; the checkpoint is compact working memory, not a second
issue database. Pass an opening `instruction`, then explicit follow-up items
with a non-empty `message`; IDs and labels are optional diagnostics.

1. **Prior verification + Engineering Review** — verify every retained repair
   whose evidence boundary arrived, then inspect runtime correctness,
   plan/artifact drift, and report/evaluation implementation selected by Gate.
   Closure requires the real runtime path, a post-change producing run that
   exercised it, expected behavior, and no regression. Persist the truthful
   verdict: `fixed_verified`, `changed_unverified` with
   `awaiting_next_valid_run`, `reopened`, `still_active`, `blocked`, or
   `inconclusive`.
2. **Stores Health** — only when the `workflow_review` Gate evidence names
   store integrity, or the first turn finds a likely learnings/knowledgebase/DB
   ownership or contract issue. Load the learnings, knowledgebase, and database
   evidence packs as needed. This is a distinct second turn in the same
   conversation, not a `stores_health` module or separate receipt.
3. **Operations Review** — only when `llm_ops_review` is due.
4. **Fix and verify** — apply safe owned repairs and run proportional proof.
5. **Persist and close** — write typed findings, dispositions, verification,
   attempts, and exactly one terminal receipt for each due module.

Do not add a consolidation turn. Every later message continuously updates the
checkpoint's canonical root causes and merges semantic duplicates while it
works.

When `strategic_review` is due, launch one separate executor sequence:

1. **Prior verification + hidden-mechanism audit** — verify retained strategic
   work, then test for feedback loops, selection bias, observation
   contamination, proxy optimization, concentration, saturation, and local
   optima across comparable runs.
2. **Independent opportunity discovery** — only when Gate evidence warrants
   it; search outside the current plan for materially different strategies.
   Omit this message in audit-only and backlog-drain passes.
3. **Critic and conclusion** — choose exactly one of `keep`, `improve`,
   `propose_alternative`, `experiment`, or `evidence_wait`.
4. **Persist** — write typed strategic findings/decisions/impact records and
   the terminal `strategic_review` receipt.

The Strategic sequence also receives one run-scoped checkpoint. An experiment
is optional. Multiple running/measuring experiments may coexist only when
their declared interference domains do not overlap; proposed or approved but
not started experiments do not consume an active slot.

Give each child the Pulse run ID, selected lens, Gate evidence, checkpoint
path, and clear normal builder authority. The final sequence message owns the
typed SQLite writes and receipts because it has the complete sequence context.
Launch all selected children, then end the parent turn without polling. The
runtime waits for registered children. The next parent turn only validates
receipts and resolves genuine cross-module ownership conflicts; it must not
reconstruct findings from truncated automatic-notification prose. If a child
dies before persistence, record that module as incomplete instead of inventing
its findings.

Read module/worklist state, `get_pulse_state(view="backlog")`, and saved SQLite reviewer results. On recovery inspect
current target/runtime and verification evidence; never trust HTML or blindly
reapply partial work. Preserve `changed_unverified` until its evidence boundary.
For the due module, work the complete active retained backlog **before** new
discovery. For each relevant issue, verify the next-check boundary if it has
arrived, repair it when a safe bounded repair exists, or record its concrete
blocker/decision/evidence boundary. Also load `suppressed_concerns`: an
unchanged externally owned issue is not a new finding, while materially changed
evidence/target identity is a reopen candidate. A new finding is justified only
by a genuinely distinct root cause with a different repair, owner, or
verification boundary—not different wording, a second symptom, or another
evidence path. When new evidence belongs to an existing root cause, call
`record_pulse_finding` with that visible `issue_id`. After a semantic backlog
review, call `merge_pulse_issues` to retire symptom-level duplicates into their
one canonical root cause while preserving their history. There is no numeric
finding limit: causal distinctness is the limit.

### Cross-run evidence rule

Start from the current run. For a recurring finding, a prior repair awaiting
verification, or a claimed cost/quality regression, compare it with up to the
last three **comparable** retained runs: same route/group and materially
equivalent configuration. Read compact summaries, typed findings, metrics, and
receipts first. Open raw conversation/tool logs only for the precise
step/attempt that differs or is suspicious; never bulk-read every conversation
from every run. If fewer comparable retained runs exist, record that as an
evidence limitation rather than inferring a trend.

Strategic Review is one product/business sequence. Its child may run in
parallel with the Engineering/Ops child.
Do not mutate the same artifacts in competing children; the Engineering/Ops
sequence owns its technical repair phase.
Engineering and Operations remain independent in meaning but intentionally
share evidence.
An unreliable evidence window is classified inside the affected review as an
execution problem or insufficient evidence; it does not cancel another review.
`context_usage_percent`/`context_window_usage`/`model_context_window` are
absent from the persisted cost ledger for every session, not only coding-CLI
ones (PLAT-072). Never file the absence as a telemetry regression or use it —
or a stale value in an old record — to support a context-pressure finding.
Cumulative token and cost evidence is unaffected; use that instead.

First inspect current-run results, the complete active retained backlog,
answered decisions, awaiting-verification work, and saved typed review receipts.

**Drain every answered decision in this pass (PLAT-092).** Inspecting them is
not enough: an answered decision that is only re-read leaves the operator's
answer doing nothing, and 26 of them accumulated across six workflows this way,
the oldest for 31 days. Each one you see must reach a terminal state before
this turn ends — either apply it and call `mark_human_input_consumed` with an
`outcome_summary` stating what actually changed, or re-park it with a concrete
reason and a next-check boundary. Do not leave one silently `answered`.

Two rules this depends on. Consume only what you really applied: if the change
could not be made, re-park it with that reason rather than consuming it to tidy
the list. And drain it yourself regardless of which module asked — a question's
source module may be suppressed while its answer is still valid, so an answered
decision must never wait on `strategic_review` being selectable.
Do not launch a reviewer merely because a module is due: if durable evidence
already answers the question, send that module directly to the Engineering/Ops
consolidation-and-repair sequence. Never combine agents in one shell command,
run background curl, or use `&`/`wait`. Specialist output is evidence for the
parent consolidation, not permission to omit its final receipt. Reviewer
failure is truthful evidence for that lens and cannot erase or block other due
work.

Continuously maintain canonical roots in the checkpoint, then persist findings
and prior-fix verification through typed Pulse tools in the final sequence
turn. Apply safe approved repairs through normal Workflow Builder tools and
record exact proof or the future producing-run boundary. Finish by calling
`record_pulse_result` exactly once for every due module. If no module is due,
record the required terminal skip receipts and stop. Do not render HTML, back
up, publish, or notify in this turn.

### Compact response contract

SQLite lifecycle rows, saved review records, run artifacts, and tool history are
the proof store. A reviewer response is an evidence index for people and the
next turn, **not** an investigation transcript. Do not paste raw logs, SQL
rows, full tool results, source excerpts, or reasoning already captured in a
prior lane. Use exact paths, query names, IDs, and short observed values as
evidence pointers instead.

Return only a compact executive verdict and short change/verification/blocker
outcome. Include counts for existing issues updated, genuinely new root causes,
duplicates merged, repairs attempted, verified closures, and the resulting
active backlog. Every valid finding, check, attempt, and evidence pointer
belongs in the typed SQLite lifecycle tools, not in a Markdown report or a
large final response. Merge the same root cause and retain every duplicate's
history. Use only the visible `PUL-…` `issue_id` from the backlog in Pulse
tools; fingerprints and attempt IDs are internal plumbing.
If a typed write is rejected, correct that write or record the exact module
failure; never replace missing structured state with prose.

Give each reviewer scope, Gate evidence, focused guidance, and this response
contract. Engineering Review conditionally loads execution, artifact-drift,
report/eval implementation, and store-integrity evidence packs; those are not
separate reviewer identities. LLM/Ops evaluates correct execution for cost,
latency, model, tool, retry, and runtime fitness. The shared operational reviewer
executes only selected perspectives in canonical order, reuses shared evidence,
and consolidates the same root cause before returning. Strategic Review remains
a separate ordered sequence with fresh phase contexts joined by one checkpoint.
Load docs with
`read_skill(skills=[{"name":"builder-reference","path":"references/<name>.md"}])`:
`pulse-bug-review`; `review-artifact-drift`; matching `improve-*` health guide;
`llm-selection` plus cost/timing evidence; and `strategy-auditor` plus
goal/constraint/outcome, cross-run DB/run, and experiment evidence for Strategic
Review. Its first phase audits the current strategy without inheriting the
opportunity phase's conclusion. A report or
evaluation that is technically correct but does not measure useful goal progress
is its `measurement_gap`; broken implementation belongs to Engineering Review.
Only when the audit shows a material ceiling, unexplained opportunity gap, or a
reached strategic checkpoint should the next phase explore materially different
theses. Reject maintenance- or instrumentation-only strategic results.
Reviewers never edit, publish, notify, ask the user, write HTML, or mark state.

The **Stores Health** turn loads `improve-learnings`, `improve-knowledge`, and
`improve-database` when selected by Gate evidence. It is an Engineering Review
turn, not a separate module. Its learning review covers the complete skill package, not
only the root index or a sample of references, and audits every effective
read-write `learning_objective`. References are part of the skill: only
reusable target-system execution HOW may remain anywhere under `learnings/_global/`.
Relocating business facts, run state, owner values, strategy, incidents,
decisions, provenance, or architecture history from `SKILL.md` into a reference
does not fix the finding.

The same evidence pack enforces shared-platform purity across Plan, Learnings,
and KB. AgentWorks bridge/auth variables and envelopes, api-bridge routing,
Folder Guard internals, managed workflow-DB tool syntax, `get_api_spec`
workarounds, and coding-agent tmux/native-session plumbing are platform-owned,
not reusable workflow HOW. Consolidate all occurrences into one workflow-level
finding and one repair attempt. Preserve the workflow's semantic action,
inputs/outputs, side effects, safety constraints, persistence, and verification.

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
`inconclusive`. Call `record_pulse_verification` with the visible `issue_id`,
verdict, expected, observed, evidence, and the next-check boundary when
inconclusive. The backend resolves and validates the internal attempt, and stores these records separately
from prose. Return those verdicts as a verification
list, separate from new findings. The Engineering/Ops executor records them — passed closes the finding as `fixed_verified`,
failed reopens it, inconclusive leaves it awaiting the boundary it still names.

Verification is not discovery. It must be done before new discovery so a
reviewer never chooses between confirming past repairs and describing fresh
symptoms.

Without this, only failure is detectable. A fix that worked is never confirmed,
so it is re-attempted every pass: rtslatency carried a finding at seen_count 4,
still awaiting_verification, repaired again on each cycle because nothing ever
checked the run that had since produced its evidence.

Require a verdict, next check, and every evidence-backed root cause with its
visible issue ID, claim, evidence, bounded fix, verification, and judgment
reason. Classify retained findings rather than omitting them. Clean means an
empty active issue manifest.
A Strategic Review `measurement_gap` names the missing
target/source/action/outcome linkage and the decision it prevents. Its
opportunity phase is a separate message/context, not a separate Pulse module.

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
`queued_for_engineering` means a safe workflow repair exists but was not chosen
for this pass. It remains on Gate's active queue and requires `next_check`
naming the next Engineering/Pulse pass. Never call deferred, deprioritized, or
unattempted work `blocked`. `blocked` is only for a genuine current condition
with no safe action at all; `awaiting_run` is for a future run/evidence boundary.

Dashboard rendering, backup, publishing, and notification are owned by the
ordered Pulse finalizer. A reviewer or Engineering/Ops executor being forbidden to perform those
later stages is normal stage separation—not a defect, blocker, or external
action. Never file a finding merely because one of those commands is still
`waiting` before its turn. Judge it only from `pulse_final_command_state` after
the command becomes terminal; a real terminal `failed` or `blocked` status may
be reported, while `waiting`, `running`, or later `done` must not create a
platform finding.

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
quality-versus-cost, spend, or reliability tradeoff. Strategic Review creates a
user-facing strategic conclusion. Enforce its declared route:
`decision_required` must create and link one `awaiting_user` decision;
`evidence_wait` becomes `proposal_only` only with the exact non-empty
`next_check`; `engineering_handoff` must be attempted through the normal safe repair
lifecycle rather than parked as a proposal. Use
`create_human_input_request(source="strategic_review", input_id="strategy-proposal-...")`
for a strategic decision so the UI preserves who asked. An actionable alternative
must create an approve/reject/defer decision using that same source and prefix;
only a thesis explicitly waiting on named future
evidence may use `proposal_only`, with that boundary in `next_check`. Route any
technical prerequisite to the Engineering/Ops executor and never apply a materially different
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
For `strategic_review`, the question source must be `strategic_review` and its
id must start `strategy-proposal-`. The backend rejects a strategic
`proposal_only` disposition with no `next_check`, so every accepted
recommendation has a concrete route to action or evidence.

Deduplicate findings before filing: match stable target/component, behavioral
claim, and evidence boundary against active and suppressed findings. Reuse the
existing issue for unchanged or newly evidenced instances, and record the new
evidence through the typed finding tool.
Create a new finding only for a genuinely different target/claim, or reopen one
whose recorded reopen condition is now satisfied. Then map conflicts by target. Resolve by explicit user
constraints, correctness/data integrity, preserved goal meaning, strategy
improvement, then cost. If evidence cannot decide, create one focused decision,
block affected modules, and do not mutate that target.

The Engineering/Ops executor first calls `get_pulse_state(view="backlog")` without a module filter
and freezes the complete active starting manifest. Due reviewer modules decide
which reviews ran; they do not narrow the executor's retained
backlog. Include every owning module represented by that manifest in the executor
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
to establish proof. Maintain the exact remaining visible issue-ID list
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
visible `issue.id` from `get_pulse_state(view="backlog")`. Send that one ID back
as `issue_id`; the backend resolves its fingerprint and current attempt. IDs
address records but never decide semantic sameness. If a finding lacks an issue
ID, block it as a reviewer-contract failure instead of making an untracked change.

Reconcile every starting issue ID to one disposition or one
checked, still-unmet waiting/external boundary; missing/duplicates block its
module and the executor must not claim completion. Strategy or model-routing changes need exact valid approval. Preserve each
reviewer's conclusion under its owning module. Strategic Review may conclude
keep, improve, propose an alternative, run an approved bounded experiment, or
wait for named evidence. Operational observations
may be cross-referenced to the matching module during consolidation, but never
rewritten as a dependency or used to suppress that module's result.
Strategic operational findings remain out-of-scope observations.
Only the Engineering/Ops executor mutates workflow state. It should normally
review, consolidate, repair, and verify in its one ordered message sequence,
so it does not autonomously create a duplicate residual Fixer/recovery agent.
It may deliberately use a separately scoped repair agent when that is the
right workflow decision. The parent continuation only records typed receipts
from completed children and never mutates workflow artifacts. Neither reviewer
nor executor writes a separate Pulse presentation artifact; the Pulse popup reads
the typed records directly.

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
structured `finding_dispositions` row with `issue_id`, disposition, summary,
changed files, and exact verification objects. Fingerprints and attempt IDs are
backend-only identifiers.
Verification verdict is exactly `passed`, `failed`, or `inconclusive`.
`fixed_verified` closes only with passed post-change proof;
`changed_unverified` remains awaiting verification; failed proof reopens it.
Perform module finding-ID reconciliation and require a terminal current-run result before
claiming completion. Keep technical proof in SQLite review and lifecycle rows. Record a
reviewer failure as `Review incomplete`, without a conclusion or unsupported
change.

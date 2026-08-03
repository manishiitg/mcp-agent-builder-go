# STANDALONE PULSE FIXER

Apply and verify bounded fixes from findings that already exist in the
workflow's SQLite Pulse backlog or are named in the user's focus. This command
is the single writer; it does not rerun Pulse Gate, launch review agents, or
write the Dashboard HTML.

Work the **whole active backlog across every module**, not a slice of it. Do not
scope by module, and do not treat a module's Pulse cadence as a reason to skip
its findings — a finding whose owning module has not been reviewed for a week is
usually the one most in need of attention, not least.{{if .Focus}}

The user additionally asked you to prioritise: {{.Focus}}. Order that work first;
it does not narrow the pass.{{end}}

## Size the work before choosing an order

`get_pulse_state(view="backlog")` returns a flat list, which hides shape. Before
selecting anything, query the backlog with `query_workflow_db` and decide how to
work from what you find:

- **How much is there** — counts by status, module, and age.
- **What clusters** — group by `step_id`. Many findings on one step are usually
  one defect filed once per field it names; repair the cause once rather than
  triaging the symptoms separately.
- **What recurs** — `seen_count >= 3` and still open means earlier passes did not
  actually fix it. Recurrence outranks recency.
- **What is already owed to the operator** — `report_human_inputs` rows with
  status `answered` are decisions the user already made that nothing has acted
  on. These need no reviewer and no fresh evidence; the answer *is* the evidence.
  Drain them first.
- **What is waiting on evidence that does not exist yet** — anything whose
  `next_check` names a run that has not happened. Leave it, and say so.

Then state the plan before acting: how many findings, how many coherent repair
bundles, what you are doing first and why. A pass that repairs five symptoms of
one cause, or that spends its budget on fresh trivia while a six-day-old answered
decision sits unconsumed, has chosen badly even if every individual fix is
correct.

Read the schema before writing SQL against it, in a separate call — the lifecycle
tables do not use the column names an agent would guess.

## Select work

1. Load `read_skill(skills=[{"name":"builder-reference","path":"references/pulse-review-fixer.md"},{"name":"builder-reference","path":"references/pulse-fixer-practices.md"},{"name":"builder-reference","path":"references/fix-verification.md"}])`.
   Use the practices reference to consolidate symptoms and choose a complete
   repair before applying the lifecycle and verification contracts. Follow its
   **Full-backlog drain contract** literally: freeze the starting manifest,
   maintain the remaining finding-ID list, and reconcile it before completion.
2. Call `get_pulse_state(view="module")`, then `get_pulse_state(view="backlog")` with no module
   filter so it returns the complete active backlog. Treat active concerns, finding lifecycles, attempts,
   verification history, decisions, and saved review identities as the source of
   truth. The Dashboard is a projection, never the backlog. Select only existing
   findings with precise evidence and a bounded recommended fix. If the user
   named a finding, prioritize it. Do not infer a fix from a vague historical
   note, and do not launch discovery to replace missing evidence.
3. Recheck the cited evidence before mutation. If the evidence is stale,
   contradictory, unsafe to verify, or no longer reproducible, preserve or
   disposition the existing finding instead of forcing a change.
4. Collect the owning Pulse modules and call
   `begin_pulse_fixer_run(workspace_path, modules)` exactly once. Use its returned
   `pulse_run_id` for every lifecycle write. If it reports that a module belongs
   to an unresolved automatic Pulse run, stop rather than taking it over.

Before each mutation, establish a **post-change evidence boundary** per
`read_skill(skills=[{"name":"builder-reference","path":"references/fix-verification.md"}])`: record the mutation start time,
canonical target identity, pre-change hash or version, and the latest relevant
pre-change run/artifact ids. Old artifacts are baseline only, never proof.

## Apply safely

- Apply operational correctness, stale-path, validation, current-run binding,
  artifact-wiring, report/eval truthfulness, and other semantics-preserving
  repairs directly when evidence is strong.
- Strategy, goal meaning, thresholds, rubric semantics, LLM/provider choices,
  recipients, destinations, credentials, and broad plan changes require the
  existing exact approved human-input request. A free-form or unrelated answer
  is not approval.
- Run the entire repair pass as exactly one `call_generic_agent` with
  `role="fixer"`, `module="pulse_fixer"`, the selected module list in its
  instructions, and the `pulse_run_id` from `begin_pulse_fixer_run`; the backend
  derives the stage identity. This is the same single-writer stage scheduled Pulse uses, so a
  manual run exercises the real path rather than a parallel one, and it runs on
  the maintenance model tier instead of this chat turn's model. Do not apply
  fixes inline here. Record the returned `execution_id`, end the current turn,
  and resume only from its automatic completion notification. Have it
  semantically consolidate the selected findings into
  a short priority-ordered list of coherent repair bundles. Group only the same
  root cause with compatible target changes and one verification condition;
  preserve every finding link and keep conflicts or waiting items separate. Do
  not run an externally side-effecting workflow merely
  to verify a repair.
- Run targeted side-effect-free validation after every change and accept it only
  under the `fix-verification` contract: verify the real runtime consumer reads
  the changed canonical store; a successful write alone is not proof.
- If verification requires an externally side-effecting run or the next
  scheduled producing run, do not trigger it merely to verify. Record
  `changed_unverified` with reason `awaiting_next_valid_run`, the exact next
  evidence boundary, and do not claim the finding is fixed.

Every disposition carries the exact existing pair from
`get_pulse_state(view="backlog")`: pass `issue.id` as `finding_id` and the
`fingerprint` from that same item, plus the files it changed. The backend opens
the fix-attempt record from that disposition. The issue ID is an address, not
duplicate-detection evidence. Do not mutate a finding that cannot be linked to
both values.

## Close out

Before any final answer, re-read `get_pulse_state(view="backlog")` and reconcile
every exact pair from the starting manifest. Do not equate "made useful fixes"
with "drained the backlog." Any starting open/acknowledged item without a
current-pass disposition means the Fixer is still running; continue. A retained
waiting/external item is accounted for only when you checked and named its still
unmet evidence, decision, version, or ownership boundary.

Call `record_pulse_result` exactly once for every selected module, with one
structured disposition for every selected finding. Use `fixed_verified`, `changed_unverified`, `verified_no_change`,
`blocked`, `awaiting_user`, `proposal_only`, `external_action_required`,
`failed`, or `rejected` honestly under the shared lifecycle contract.

Do not call `record_pulse_worklist` or final-command status tools. Do not update
`builder/improve.html` or `builder/card.health.html`; the next Dashboard render
projects SQLite review, finding, fix, verification, decision, and module state.

Only after the Fixer's completion notification, finish with changes applied,
verification performed, findings not changed and why, approvals still needed,
and the next real-run evidence required.

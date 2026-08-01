# STANDALONE PULSE FIXER

Apply and verify bounded fixes from findings that already exist in the
workflow's SQLite Pulse backlog or are named in the user's focus. This command
is the single writer; it does not rerun Pulse Gate, launch review agents, or
write the Dashboard HTML.{{if .Focus}}

Fix focus: {{.Focus}}.{{end}}

## Select work

1. Load `get_reference_doc(kind="pulse-review-fixer")` and
   `get_reference_doc(kind="fix-verification")`.
2. Call `get_pulse_module_state`, then `get_pulse_finding_backlog` for the
   selected owning modules. Treat active concerns, finding lifecycles, attempts,
   verification history, decisions, and saved review identities as the source of
   truth. The Dashboard is a projection, never the backlog. Select only existing
   findings with precise evidence and a bounded recommended fix. If the user
   named a finding, prioritize it. Do not infer a fix from a vague historical
   note, and do not launch discovery to replace missing evidence.
3. Recheck the cited evidence before mutation. If the evidence is stale,
   contradictory, unsafe to verify, or no longer reproducible, preserve or
   disposition the existing finding instead of forcing a change.
4. Group selected findings by owning Pulse module and call
   `begin_pulse_fixer_run(workspace_path, modules)` exactly once. Use its returned
   `pulse_run_id` for every lifecycle write. If it reports that a module belongs
   to an unresolved automatic Pulse run, stop rather than taking it over.

Before each mutation, establish a **post-change evidence boundary** per
`get_reference_doc(kind="fix-verification")`: record the mutation start time,
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
- Run the repair itself as one `call_generic_agent` with `role="fixer"` per
  selected module, passing the `pulse_run_id` from `begin_pulse_fixer_run` and
  that `module`; the backend derives the review identity. Start one at a time —
  it is the single writer. This is the same stage scheduled Pulse uses, so a
  manual run exercises the real path rather than a parallel one, and it runs on
  the maintenance model tier instead of this chat turn's model. Do not apply
  fixes inline here, and do not run an externally side-effecting workflow merely
  to verify a repair.
- Run targeted side-effect-free validation after every change and accept it only
  under the `fix-verification` contract: verify the real runtime consumer reads
  the changed canonical store; a successful write alone is not proof.
- If verification requires an externally side-effecting run or the next
  scheduled producing run, do not trigger it merely to verify. Record
  `changed_unverified` with reason `awaiting_next_valid_run`, the exact next
  evidence boundary, and do not claim the finding is fixed.

Before each mutation call `start_pulse_fix_attempt` with the exact existing
fingerprint/finding ID, intended files, and before references. Do not mutate a
finding that cannot be linked to its durable identity.

## Close out

Call `mark_pulse_module_result` exactly once for every selected module, with one
structured disposition for every selected finding and the returned attempt ID
where required. Use `fixed_verified`, `changed_unverified`, `verified_no_change`,
`blocked`, `awaiting_user`, `proposal_only`, `external_action_required`,
`failed`, or `rejected` honestly under the shared lifecycle contract.

Do not call `record_pulse_worklist` or final-command status tools. Do not update
`builder/improve.html` or `builder/card.health.html`; the next Dashboard render
projects SQLite review, finding, fix, verification, decision, and module state.

Finish with changes applied, verification performed, findings not changed and
why, approvals still needed, and the next real-run evidence required.

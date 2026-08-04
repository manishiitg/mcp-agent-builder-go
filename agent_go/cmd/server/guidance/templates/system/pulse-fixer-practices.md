# Pulse Fixer engineering practices

Use this reference for every Pulse Fixer mutation pass. It defines how to turn
review findings into durable repairs. The lifecycle and proof rules remain in
`references/fix-verification.md`; this reference governs diagnosis and repair
quality.

## Core method

For each actionable finding:

1. **Reproduce or re-read the cited failure.** Treat reviewer prose as a lead,
   not proof. Resolve the exact run, target, inputs, tool result, and consumer.
2. **Separate symptom from root cause.** Group failures only when they share the
   same cause, compatible target changes, and one verification condition. Keep
   every finding link on the bundle.
3. **Map the contract boundary.** Identify the producer, authoritative contract,
   validators, durable stores, downstream consumers, evals, reports, learnings,
   and knowledge notes affected by the target.
4. **Choose the smallest complete repair.** Change every surface required for
   consistency, but do not broaden into strategy, policy, or unrelated cleanup.
5. **Preserve meaning and safety.** Never weaken a check, invent missing data,
   lower a threshold, change a destination, or reinterpret an operator decision
   merely to obtain a pass.
6. **Exercise the real consumer path.** Prefer the actual parser, validator,
   query, scheduler transition, or tool boundary over a look-alike check.
7. **Record the honest lifecycle result.** Use `fixed_verified` only for passed
   post-change proof. Use `changed_unverified` when a producing run or external
   event is still required.

## Full-backlog drain contract

A useful handful of repairs is not completion. Every Fixer pass must account for
the complete active backlog that existed when the pass began, using the existing
Pulse state and lifecycle tools rather than an arbitrary top-N queue.

1. **Freeze a starting manifest.** Call `get_pulse_state(view="backlog")` with no
   module filter and retain every exact `finding_id` + `fingerprint` pair. Use
   `query_workflow_db` to count and inspect status, owning module, step, age,
   recurrence, attempts, and next-check boundaries before choosing an order.
2. **Classify every manifest item.** Put each finding in exactly one working
   lane: actionable now, awaiting evidence/run, awaiting an unanswered user
   decision, externally owned, or no longer reproducible. This classification
   is agentic judgment based on current evidence; status text alone is not a
   verdict.
3. **Bundle semantically.** Group actionable items only by shared root cause,
   compatible targets, and one verification condition. One repair may carry
   many finding links, but no finding may disappear inside a bundle.
4. **Maintain an explicit remaining list.** After each repair bundle, prepare a
   lifecycle disposition for every linked manifest item and remove only those
   exact pairs from the working list. Continue until no actionable starting
   item remains. Do not stop because several valuable fixes succeeded or
   because the backlog is large.
5. **Check waiting boundaries rather than re-mutating.** An existing waiting
   state is accounted for only after checking whether its named run, answer,
   version, or evidence has arrived. If it has arrived, verify or resume the
   repair. If it has not, preserve the waiting state without manufacturing a
   redundant fix attempt.
6. **Reconcile before completion.** Re-read
   `get_pulse_state(view="backlog")` before the final response and compare it
   with the starting manifest and the dispositions prepared in this pass. Every
   starting open or acknowledged finding must have a current-pass disposition;
   every retained waiting/external finding must have its concrete unmet boundary
   named. Submit the complete per-module disposition sets through the existing
   `record_pulse_result` calls.

If any starting item is still unaccounted for, continue the pass. If a tool,
evidence, approval, or runtime failure makes continuation impossible, record
that exact blocker for the affected finding and report the pass as incomplete;
never silently omit the item or claim the Fixer completed. Findings first
created while the frozen pass is running belong to the next pass unless they
are inseparable consequences of the repair currently being recorded.

## Evidence hierarchy

Resolve conflicts in this order:

1. Explicit user decisions, safety constraints, and `soul.md` goal meaning.
2. Current platform and tool contracts proven by the running implementation.
3. Current plan and step configuration at the canonical saved path.
4. Current durable store schema and real consumer behavior.
5. Current validation and evaluation contracts.
6. Examples, learnings, knowledge notes, and old run artifacts.

Lower-ranked evidence may reveal drift but does not override higher-ranked
evidence. An old successful artifact proves only the old boundary.

## Repair-bundle contract

Before editing, state one compact bundle:

- root cause;
- linked finding IDs and fingerprints;
- canonical target and producer;
- affected consumers;
- intended files;
- baseline evidence;
- one post-change verification condition.

Do not file or repair one issue per field, stack frame, retry, table row, or
downstream symptom when they come from one cause. Do not merge findings that
need different policy decisions or different proof.

## Schema and artifact contract repair

Apply this playbook when JSON, database rows, tool envelopes, or other produced
artifacts fail shape, type, required-field, enum, or consistency checks.

### Diagnose

1. Group every prevalidation error from one producing step into one root
   finding. Artifact paths, fields, run identity, and contract version are
   evidence under that step finding, never separate bugs.
2. Read one real failing artifact. Do not infer its shape from error strings.
3. Read the producer instructions or code and the exact validation schema.
4. Search every downstream consumer, eval, report query, learning example, and
   schema manifest for the affected file and fields.
5. Determine which case applies:
   - **producer stale:** consumers and validator agree; repair the producer;
   - **validator stale:** producer and consumers agree; repair the validator;
   - **contract split:** producer, validator, and consumers disagree; establish
     one canonical shape from higher-ranked evidence and update all dependents;
   - **missing semantics:** the required value cannot be derived; keep the step
     failed or explicitly unavailable rather than fabricating a default.

### Repair

- Prefer a deterministic serializer or shared output builder when the step can
  compute the fields mechanically.
- For agentic producers, make the output contract concise and unambiguous, give
  one canonical example, and require a mechanical self-check before first write.
- Update producer instructions/code, validation, consumer assumptions, evals,
  report queries, and active learning/schema references together when they are
  in the blast radius.
- Preserve unknown, unavailable, skipped, and empty as distinct semantic states.
- Do not add placeholder zeros, empty arrays, or success values merely because
  the schema requires a field.
- Do not remove required checks until real consumers prove the field obsolete.
- Avoid compatibility aliases unless a live consumer still requires them; if
  needed, name the owner and removal condition.

### Verify

1. Validate a representative post-change artifact with the real prevalidator.
2. Exercise at least one real downstream consumer or parser against it.
3. Check a negative fixture so the validator still rejects the original defect.
4. Confirm every linked file- and field-level check is covered by the step-level
   root repair.
5. If an agentic or externally producing step has not run after the mutation,
   record `changed_unverified` with the exact next run/artifact boundary.

Prevalidation is a guard, not the durable repair. A repair turn may correct one
artifact, but recurring drift requires aligning the saved producer and every
consumer contract.

## Database repair

- Read the declared schema and real table shape before querying or mutating.
- Distinguish schema defects, invalid rows, stale derived data, and inaccessible
  storage; they require different repairs.
- Preserve keys, provenance, and idempotency. Use a transaction for related
  mutations and verify through the canonical query tool or consumer.
- Never perform a speculative migration. When row meaning is ambiguous, retain
  the evidence and request the missing decision.

## Learning and skill purity repair

Apply this playbook whenever a finding concerns `learnings/_global/`, a
`learning_objective`, or `learnings_access`.

1. **Inventory the complete skill package.** Read the root `SKILL.md` and every
   content-bearing Markdown file under `references/`. References are part of
   the skill, not an archive for content removed from the index.
2. **Classify before editing.** Keep only reusable execution HOW: procedures,
   selector strategy, auth/API/CLI/tool quirks, parsing/retry/recovery rules,
   stable failure signatures, and concise operational constraints needed to
   perform the task. Route everything else by meaning:
   - business/domain facts → knowledgebase;
   - run results/current values/status → DB or run evidence;
   - owner goals/preferences/thresholds → `soul.md`;
   - strategy/cadence/routing/current step behavior → plan/config;
   - incidents, provenance, action/run IDs, decisions, and fix history → durable
     Pulse review/finding/decision evidence;
   - platform architecture/schema history → authoritative documentation or DB
     contract, retaining only the short runner-facing instruction in the skill.
3. **Do not launder content through references.** Move detailed reusable HOW
   from `SKILL.md` to a focused reference. Remove non-skill content from the
   whole package; putting it in `references/` is not a repair.
4. **Preserve evidence before removal.** Cite the exact source block in the
   durable Pulse result. If an authoritative destination already contains the
   material, remove the duplicate. If a bounded authorized KB/config repair is
   safe, apply it through the canonical tool. Never invent a fact, rewrite
   user-owned context, mutate run data speculatively, or discard the only copy
   of an unresolved user decision.
5. **Repair contribution configuration.** For every effective
   `learnings_access="read-write"` step, require a concrete objective that asks
   only for reusable HOW. Clear a misplaced/legacy objective and set access to
   `read` when the step should consume but not contribute. Preserve
   `read-write` only when the objective and actual skill coverage agree.
6. **Keep the root as an index.** `SKILL.md` contains valid frontmatter, a short
   scope, core cross-cutting instructions, and links to focused references.
   Detailed HOW belongs in references without duplication.

Verification is an immediate semantic re-read, not merely a successful patch:

- re-inventory and re-read every content-bearing Markdown file in the skill;
- confirm every remaining block is reusable execution HOW;
- confirm no incident/provenance, decision ledger, run result/current status,
  business fact, strategy, owner-value duplicate, or architecture history was
  retained or hidden in a reference;
- validate all index links and confirm references are reachable without
  duplicate homes;
- re-read every changed step config and confirm effective access/objective
  pairing;
- name separately routed work that could not be safely applied. Content cleanup
  may be `fixed_verified` when this complete post-change re-read passes; routed
  KB/DB/plan changes still follow their own proof boundary.

## Tool, path, and permission repair

- Interpret nested bridge envelopes and semantic error markers, not only process
  exit codes.
- Confirm the actual tool surface and target type before declaring a capability
  missing.
- Resolve canonical absolute workspace and run paths; do not make a stale
  relative-path workaround permanent.
- Repair the harness when the same failure crosses workflows. Do not copy a
  platform workaround into every workflow.

## Scheduler and lifecycle repair

- Trace live execution state, child completion, durable run metadata, and UI
  projection separately.
- Define the state transition invariant before editing. A UI badge change is not
  a scheduler repair.
- Test ordering, retries, cancellation, recovery, and durable terminal state.

## Evaluation and report repair

- Missing evidence is unavailable, never a perfect score or zero by default.
- Preserve raw precision and provenance until presentation formatting.
- Verify that report and evaluation queries resolve the intended run, group,
  time window, and canonical store.
- Presentation-only changes cannot claim direct goal impact.

## Stop conditions

Stop and disposition honestly when:

- the change alters goal meaning, policy, money, recipients, credentials, or
  external side effects without exact approval;
- evidence cannot establish the canonical semantics;
- only a future producing run can prove behavior;
- the root cause is an externally owned platform or vendor defect;
- the proposed repair would merely silence detection.

Never describe these states as verified fixes.

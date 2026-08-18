[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-138 — Engineering Review buries prior-fix verification inside one oversized contract and has no compaction-safe sequence memory

| Coordination | Value |
|---|---|
| Assigned agent | Codex |
| Ticket state | `implemented` — automated contract verification passed; live post-restart/compaction verification pending |
| Last synchronized | `2026-08-18` |

- **Priority:** P1 — Pulse already instructs Engineering Review to verify its
  retained backlog before new discovery, but verification debt remains high and
  findings can accumulate faster than repairs are proven. The behavior exists
  in prose without a sequence and persistence boundary that reliably preserves
  it through long reviews and context compaction.
- **Owner:** Pulse Review+Fix dispatch and guidance, background
  message-sequence construction, run-scoped review checkpoints, typed Pulse
  persistence, recovery reconciliation, and focused contract tests.

## Existing behavior that must be preserved

The current Review+Fix contract already requires the executor to:

- load the complete active SQLite backlog;
- freeze the starting issue manifest;
- verify matured `changed_unverified` and next-run-gated fixes;
- repair retained active roots before broad discovery;
- compare up to three comparable runs where needed;
- reconcile every starting issue; and
- close only with passed post-change proof.

Stores Health is not a separate Pulse module. When selected by Gate evidence,
it is a distinct message in the same Engineering executor session. Operations
Review may follow in that session when `llm_ops_review` is due, and a later turn
applies and verifies safe technical repairs with the accumulated context.

The defect is not the absence of verification guidance. It is that prior
verification, new discovery, store/operations analysis, deduplication, repair,
verification and persistence are interleaved inside a very large contract with
no durable current-sequence working state and conflicting child-versus-parent
persistence instructions.

## Intended sequence

Build only the messages required by the Gate worklist. The same retained
background executor owns the complete sequence.

### Discovery mode

1. **Previous verification + Engineering Review**
   - Load the complete relevant backlog and prior attempts from typed SQLite
     state.
   - Verify every matured prior Engineering boundary before new discovery.
   - Inspect selected execution, plan/artifact, report and evaluation evidence.
   - Create or update the checkpoint's canonical finding list.
2. **Stores Health (conditional)**
   - Verify prior learnings/knowledgebase/workflow-DB repairs.
   - Add evidence to matching roots before creating a genuinely distinct
     store-integrity finding.
3. **Operations Review (conditional)**
   - Verify prior model, cost, latency, retry and reliability repairs.
   - Update matching roots before creating a genuinely distinct operational
     finding.
4. **Fix and verify**
   - Read the already-deduplicated checkpoint.
   - Repair retained active/reopened issues first, then safe new findings.
   - Capture exact changed targets, checks, expected results and observations.
5. **Persist and close**
   - Perform no new investigation or mutation.
   - Persist final findings, evidence updates, dispositions, verification and
     exactly one terminal receipt per owned due module through typed tools.

There is no default standalone consolidation message. Messages 2 and 3 read the
existing canonical roots and continuously merge evidence. Message 4 performs a
short conflict/safety check before mutation.

### Backlog-drain mode

1. Verify matured previous fixes.
2. Repair retained active/reopened issues and verify them.
3. Persist and close.

Do not perform broad strategic discovery in backlog-drain mode. A Strategic
Review child may run only when retained strategic findings have matured
verification or disposition work; it skips opportunity discovery. `observe`
launches no executor. `strategy` is owned by PLAT-137's Strategic Review
sequence.

## Compaction-safe checkpoint

Inject one exact run-scoped path, for example:

```text
runs/<run>/pulse/<pulse-run-id>/engineering-ops-checkpoint.md
```

Every sequence message reads it first and updates it last. It contains compact
working state only:

- Gate scope and complete starting issue manifest;
- a previous-issue verification matrix;
- canonical semantic root causes and owning modules;
- exact evidence references and remaining proof gaps;
- duplicate/evidence mapping;
- repair plan, affected targets and rollback boundary;
- changes made and verification observations; and
- remaining work plus the next message's exact task.

It must not contain copied logs, full tool outputs, HTML, or a long-form review
report. It is retained with the run and is not lifecycle authority; typed
SQLite state remains authoritative after final persistence and checkpoint
pruning.

## Continuous deduplication

Treat observations as one root when they share:

```text
affected target/component
+ behavioral root cause
+ required repair or decision boundary
```

Different wording, another symptom, another reviewer lens, or a second evidence
path updates the root instead of creating a new issue. A distinct issue requires
a different repair, owner, or verification boundary.

## Previous-issue verification

For every starting issue, record in the checkpoint:

```text
issue | previous change | required boundary | current evidence | verdict
```

Allowed outcomes include `fixed_verified`, `changed_unverified`,
`awaiting_next_valid_run`, `reopened`, `still_active`, `blocked`, and
`inconclusive`. Closure requires proof that the change is on the real runtime
path, a valid post-change execution exercised it, the expected behavior
occurred, and no material adjacent regression appeared. File existence, a
green static test, or absence of the old error alone is not producing-run proof.

Every starting issue receives a terminal disposition or an exact still-unmet
evidence/decision/external boundary. No item disappears when context compacts.

## Persistence ownership

- Intermediate messages update only the run-scoped checkpoint and workflow
  artifacts legitimately owned by their phase.
- The final sequence message owns normal typed findings, verification,
  dispositions and module receipts because it has the full review/fix context.
- The main Pulse continuation validates those receipts and handles only genuine
  cross-sequence merges/conflicts. It does not reconstruct records from an
  automatic-notification summary.
- If the sequence dies before persistence, the main continuation records a
  truthful incomplete result; it does not infer clean, fixed, or verified work
  from partial prose.
- Go validates and stores typed calls; it does not select reviewers, semantic
  duplicates, repairs or verdicts.

## Out of scope

- Merging Strategy Auditor and Goal Advisor is PLAT-137.
- Changing domain workflow plans or their success criteria.
- Turning Stores Health back into a separate scheduled module or agent.
- Adding a Go-selected residual Fixer or recovery reviewer.

## Acceptance

1. Discovery, backlog-drain, observe and strategy Gate modes construct only the
   required sequence messages.
2. Stores Health and Operations remain conditional messages in the same
   Engineering executor session, not independent module agents.
3. A forced context-compaction test proves the starting manifest, canonical
   findings, repair evidence and verification matrix survive to final
   persistence through the run-scoped checkpoint.
4. Every matured previous issue is verified before broad new discovery; every
   starting issue receives a disposition or exact unmet boundary.
5. Evidence from later messages updates a matching semantic root and does not
   create a duplicate finding.
6. Backlog-drain mode produces no broad discovery findings; a Strategic Review
   child is allowed only to verify or disposition retained strategic work and
   must omit opportunity discovery.
7. The Fix-and-Verify turn works retained active/reopened issues before safe new
   findings and captures exact changed-target plus observed verification proof.
8. Exactly one final sequence message writes normal lifecycle records and
   exactly one terminal receipt per owned due module.
9. The main continuation reads the typed receipts rather than recreating child
   findings from auto-notification prose.
10. A pre-persistence crash records an incomplete review and leaves prior
    issues open; it never claims a clean or verified result.

## Implementation status — 2026-08-18

- Review+Fix dispatch now builds an explicit retained Engineering sequence:
  prior verification plus Engineering Review, conditional Stores Health,
  conditional Operations Review, Fix-and-Verify, then final typed persistence.
- Discovery and backlog-drain construct different worklists; backlog drain
  prioritizes retained roots and suppresses broad discovery. A due Strategic
  child in that mode may only verify or disposition retained strategic work.
- Every message uses one run-scoped compact checkpoint, continuously updates
  matching semantic roots and leaves the final child turn as the sole normal
  lifecycle writer. The parent validates typed receipts rather than rebuilding
  findings from notification prose.
- Scheduler/guidance contract tests and the repository Go build pass. A live
  Pulse run after server restart, including forced context compaction and a
  pre-persistence interruption exercise, remains required before closure.

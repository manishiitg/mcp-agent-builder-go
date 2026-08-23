[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-163 — Pulse technical and strategic reviews need durable focus rotation, one canonical technical identity, and visible coverage history

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `implemented` — canonical `technical_review` migration, technical/strategic focus catalogs, durable review counts, slash/scheduled parity, and visible focus coverage shipped; live Pulse verification remains |
| Last synchronized | `2026-08-23` |

- **Priority:** P1 — technical and strategic reviews are expensive, yet the
  platform cannot currently explain which deep theme a pass selected, why it
  took priority, what evidence it inspected, which themes it deferred, or when
  each theme was last reviewed deeply.
- **Owners:** Pulse review dispatch/guidance, typed Pulse persistence, review
  agenda/query tools, and the Pulse review-history UI.
- **Related:** [PLAT-138](plat-138.md), [PLAT-155](plat-155.md),
  [PLAT-090](plat-090.md), [PLAT-114](plat-114.md),
  [PLAT-156](plat-156.md), [PLAT-158](plat-158.md), and
  [PLAT-137](plat-137.md).

## Problem

Technical Review and Strategic Review cannot inspect every meaningful
concern deeply on every pass without becoming slow, expensive, repetitive, and
compaction-prone. PLAT-138 correctly bounded a pass to one agent-chosen
coherent objective, but the platform still has no durable coverage model behind
that choice.

Today a reviewer can rediscover the same familiar issue while another area is
never examined. A later reviewer cannot answer basic questions such as:

- Which engineering or operations theme was reviewed deeply last time?
- Why was it selected ahead of the other themes?
- Was the pass verifying an earlier repair, applying an answered decision, or
  doing new discovery?
- Which evidence selectors and canonical issue IDs were considered?
- What was deliberately deferred, and when should it become due?
- Has a changed plan/runtime surface invalidated an older review?

The existing typed state is useful but only module-level:
`pulse_module_state` stores the latest module result and next-check condition,
while `pulse_review_log` stores a terminal module receipt. Neither records
coverage within a module. Reusing either row as an overloaded focus ledger
would destroy its current meaning.

## 2026-08-21 architecture decision

Engineering Review and Operations Review were separate durable module names
even though PLAT-138 already ran them as one retained sequence. That split
created duplicate Gate decisions, receipts, focus histories, and slash-command
behavior for one technical lifecycle. The canonical model is now:

- `technical_review`: one module, one retained reviewer sequence, one focus,
  and one terminal receipt. Engineering correctness, Stores Health, operations,
  tool/runtime reliability, orchestration, models, cost, schedules, and
  efficiency are lenses inside it.
- `strategic_review`: a separate module because goal/measurement validity,
  feedback loops, alternative headroom, and experiments have a different
  evidence and decision lifecycle.

Legacy `workflow_review` and `llm_ops_review` data is migrated into
`technical_review` across module state/audit, review receipts, focus
state/history, findings/attempts, and metrics. New prompts and tools must not
dual-write the old names. Historical engineering/ops human-input sources remain
readable but new technical decisions use `source=technical_review` and a
`technical-decision-...` identity.

Gate may skip technical review, strategic review, or both when no useful
evidence has matured, but every skip records the next evidence/time/run
boundary. Every selected reviewer performs a lightweight urgent-evidence scan
and agentically chooses the smallest sufficient route-aware deep-focus set.
Review completion belongs to the reviewer; the
independent Fixer owns mutations and repair outcome and never rewrites the
review receipt.

## Desired review model

Each technical or strategic pass has two deliberately different depths:

1. **Lightweight safety scan.** Inspect only the compact lifecycle agenda for
   new critical regressions, matured verification work, and answered but
   unapplied human decisions. This prevents rotation from hiding urgent work.
2. **Agentic deep-focus set.** The reviewer maps relevant evidence to stable
   route/group/sub-workflow scopes, then chooses the smallest set of coherent
   themes worth investigating. A small route often needs one focus; a large or
   multi-route workflow may justify several when each has distinct evidence,
   risk, decision, or repair value. It records why each theme won and which
   other themes were deferred. There is no fixed numerical quota.

The technical catalog is: `execution_health`,
`plan_orchestration_integrity`, `store_integrity`, `report_quality_truth`,
`evaluation_quality_truth`, and `model_cost_fitness`. The strategic catalog is:
`goal_measurement_validity`, `strategy_effectiveness`,
`feedback_loops_bias`, `concentration_saturation`, `alternative_headroom`, and
`experiment_impact`. Go stores these stable coverage identities but does not
choose their semantic priority; the agent does.

The current operator decision deliberately removes Safety & Permissions as a
standalone rotating focus. Safety invariants remain enforced by the platform
and reviewer contracts, but the popup does not offer a separate safety review.

### 2026-08-23 — Catalog consolidation and one-off focused review

- Combined execution correctness, efficiency, tool/runtime reliability, and
  schedule recovery into `execution_health`.
- Combined plan contracts and orchestration fitness into
  `plan_orchestration_integrity`, explicitly including scripted-step fitness.
- Split report and evaluation truth so each can enforce its own best practices.
- Combined model/tier fitness and cost attribution into `model_cost_fitness`.
- Added a focused-review action to the Pulse popup. It reuses or opens the
  normal interactive automation chat, queues behind an active foreground turn,
  and sends a curated `/pulse-review` or `/strategy-auditor` request containing
  the exact canonical focus key. It does not create or modify a schedule.
- Historical merged focus rows are retained under their canonical successor.
  The old combined report/evaluation history is kept as legacy history rather
  than copied into both new focuses and falsely counted twice.

## Priority and rotation semantics

The platform supplies a compact typed agenda. The agent reasons within the
highest applicable lifecycle class:

1. new critical regression, data loss, security issue, or widespread runtime
   failure;
2. an earlier fix whose verification condition has matured;
3. an answered decision that is still unapplied;
4. a never-reviewed or materially changed focus;
5. an overdue focus;
6. the oldest remaining focus.

This is not rigid round-robin and is not a numeric relevance score. Go may
enforce lifecycle invariants and require at least one focus for a completed due
review, but it must not choose the semantic subjects or their count. The
reviewer owns that judgment, must persist each reason and route scope, and must
stop when another focus would repeat evidence or could not change a decision,
repair, or next check.

## Durable model

Add focus-level state without changing the authority of existing stores. Exact
names may change during implementation, but the responsibilities must remain
separate:

### `pulse_review_focus_state`

One current row per `(workspace_path, module, focus_key)` containing:

- last deep-review time, review run ID, and `pulse_run_id`;
- last verdict and targeted evidence selectors;
- last reviewed route scope plus route-specific coverage counts in the agenda;
- related canonical issue IDs and verification IDs;
- next-check time/run/evidence condition and reason;
- a source-change fingerprint or equivalent invalidation marker;
- update timestamps.

### `pulse_review_focus_history`

Append-only entries containing:

- module, focus key, review run ID, and `pulse_run_id`;
- stable route/group/sub-workflow scope (empty only for workflow-wide evidence);
- lifecycle class and agent-authored selection reason;
- compact scope and evidence references, not copied evidence bodies;
- verdict, canonical finding IDs, and verification IDs;
- deferred focus keys and their reason;
- terminal status and recorded time.

SQLite remains lifecycle authority. A run-scoped Markdown checkpoint may remain
the reviewer's compaction-safe working memory, as established by PLAT-138, but
it must not become a second backlog or history database.

## Review flow

1. Build a compact agenda from focus state, canonical issues, matured
   verification conditions, answered decisions, and relevant source changes.
2. Let the reviewer perform the lightweight safety scan and select the smallest
   sufficient route-aware deep-focus set.
3. Load only targeted authoritative evidence through existing files/tools.
4. Persist one focus-history entry per investigated focus and update focus state.
5. Write the existing `pulse_review_log` terminal receipt for the module.
6. Hand canonical repair work to the independent Fixer defined by PLAT-155;
   the reviewer does not repair its own findings.

## Compatibility audit and adverse-impact guardrails

### PLAT-138 — bounded, agent-chosen review objectives

This ticket complements PLAT-138; it does not reopen the unbounded backlog.
One pass still receives a bounded coherent review objective, but its agent may
cover multiple route-scoped focuses when each adds distinct decision value.
Rotation gives the agent a compact durable agenda and records its choices; it
must never instruct a
single review to process every due focus.

### PLAT-155 — observations are not the canonical repair queue

Focus coverage must operate on review themes and canonical lifecycle state. It
must not flatten raw observations back into an apparent issue count, duplicate
canonical issues, or let Review absorb Fixer's responsibility.

### PLAT-090 — cost authority

Focus history may reference `pulse_run_id` for cost/timing correlation, but it
must not create another cost ledger or revive per-module synthetic execution
IDs. Cost remains owned by the authoritative phase/execution ledger.

### PLAT-114 — background-agent audit

Focus history records why a review focus was chosen and its verdict. It must
not copy raw agent output or duplicate `background_agent_log`. A query surface
may correlate the records later.

### PLAT-156 — selective agentic evidence reading

Do not inject the full plan, full review history, or Go-authored plan
interpretation into the prompt. The compact agenda contains selectors and
reasons; the reviewer reads authoritative files and tools selectively.

### PLAT-158 — dedicated Pulse scheduling

Rotation runs only inside explicit manual review commands or the dedicated
Pulse review schedule. It must not add a hidden recurring job, revive
`post_run_monitor`, or cause ordinary workflow runs to launch Pulse.

### PLAT-137 — strategic review

Strategic Review remains separate, but now uses the same focus-state/history
contract. This does not undo PLAT-137's merged Strategy+Goal sequence or fold
strategic work into technical themes.

## Implementation slices

1. **Shipped:** focus-state/history schema, migrations, and focused repository tests.
2. **Shipped:** compact agenda read/write tools with strict workspace/module scoping.
3. **Shipped:** Technical and Strategic review guidance to run the safety scan,
   choose one focus agentically, read targeted evidence, and persist the
   selection/defer reasons.
4. **Shipped:** the terminal-write invariant: a completed review cannot report success
   without its focus-history entry and existing review receipt.
5. **Shipped:** work-area cards show all focuses selected in the latest review,
   their route scope, why selected,
   last-reviewed time, review count, and next candidates. Before the first
   recorded review they show the next eligible focus candidates rather than an
   empty state. A richer
   related-issues/next-due/recent-focus timeline
   remains presentation work; it does not block the rotation lifecycle.

## Acceptance

- Two consecutive unchanged review passes cannot silently select the same deep
  focus while an eligible never-reviewed or overdue focus exists; doing so
  requires a persisted urgent/verification reason.
- A critical regression, matured verification, or answered unapplied decision
  can preempt normal rotation and the preemption is visible in history.
- Each completed Technical/Strategic review has one or more route-aware
  deep-focus history entries, chosen agentically, plus exactly one existing
  `pulse_review_log` receipt.
- Review prompts receive a compact agenda and selectors, not full plan/history
  payloads.
- Raw observations remain distinct from canonical issues, and review remains
  distinct from Fixer.
- Ordinary workflow runs do not trigger the rotation mechanism.
- The UI can explain, without reading logs, what was reviewed, why, what was
  found, what was deferred, and what becomes due next.

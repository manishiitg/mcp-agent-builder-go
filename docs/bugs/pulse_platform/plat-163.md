[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-163 — Pulse reviews have no durable focus rotation or coverage history, so expensive passes can repeat familiar analysis while important themes remain unreviewed

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `partially implemented` — SQLite focus state/history, compact agenda/read-write tools, reviewer guidance, terminal-write enforcement, and current-focus UI shipped; richer timeline UI and live Pulse verification remain |
| Last synchronized | `2026-08-20` |

- **Priority:** P1 — Engineering and Operations reviews are expensive, yet the
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

Engineering Review and Operations Review cannot inspect every meaningful
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

## Desired review model

Each Engineering or Operations pass has two deliberately different depths:

1. **Lightweight safety scan.** Inspect only the compact lifecycle agenda for
   new critical regressions, matured verification work, and answered but
   unapplied human decisions. This prevents rotation from hiding urgent work.
2. **One deep focus.** The reviewer chooses one coherent theme and investigates
   it thoroughly with targeted evidence. It records why that theme won and
   which other themes were deferred.

Example Engineering themes include state correctness, tools/API contracts,
database and artifact lifecycle, reports, validation/tests, and scheduler
lifecycle. Example Operations themes include model/tier choice, step versus
orchestrator shape, tool/payload efficiency, timeout/completion behavior, cost
attribution, schedule/runtime design, and reflection yield. These are an
extensible vocabulary, not a hardcoded Go decision tree.

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
enforce lifecycle invariants and the one-focus boundary, but it must not choose
the semantic review subject. The reviewer owns that judgment and must persist
its reason.

## Durable model

Add focus-level state without changing the authority of existing stores. Exact
names may change during implementation, but the responsibilities must remain
separate:

### `pulse_review_focus_state`

One current row per `(workspace_path, module, focus_key)` containing:

- last deep-review time, review run ID, and `pulse_run_id`;
- last verdict and targeted evidence selectors;
- related canonical issue IDs and verification IDs;
- next-check time/run/evidence condition and reason;
- a source-change fingerprint or equivalent invalidation marker;
- update timestamps.

### `pulse_review_focus_history`

Append-only entries containing:

- module, focus key, review run ID, and `pulse_run_id`;
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
2. Let the reviewer perform the lightweight safety scan and select one deep
   focus.
3. Load only targeted authoritative evidence through existing files/tools.
4. Persist one focus-history entry and update focus state.
5. Write the existing `pulse_review_log` terminal receipt for the module.
6. Hand canonical repair work to the independent Fixer defined by PLAT-155;
   the reviewer does not repair its own findings.

## Compatibility audit and adverse-impact guardrails

### PLAT-138 — bounded, agent-chosen review objectives

This ticket complements PLAT-138; it does not reopen the unbounded backlog.
One pass still receives at most one coherent deep objective. Rotation gives the
agent a compact durable agenda and records its choice; it must never instruct a
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

The first implementation covers Engineering and Operations review. Strategic
Review already has its own cross-workflow lifecycle and experiment semantics;
it must not be silently folded into these engineering/operations themes.

## Implementation slices

1. **Shipped:** focus-state/history schema, migrations, and focused repository tests.
2. **Shipped:** compact agenda read/write tools with strict workspace/module scoping.
3. **Shipped:** Engineering and Operations review guidance to run the safety scan,
   choose one focus agentically, read targeted evidence, and persist the
   selection/defer reasons.
4. **Shipped:** the terminal-write invariant: a completed review cannot report success
   without its focus-history entry and existing review receipt.
5. **Partially shipped:** work-area cards show the current focus, why selected,
   and last-reviewed time. A richer related-issues/next-due/recent-focus timeline
   remains presentation work; it does not block the rotation lifecycle.

## Acceptance

- Two consecutive unchanged review passes cannot silently select the same deep
  focus while an eligible never-reviewed or overdue focus exists; doing so
  requires a persisted urgent/verification reason.
- A critical regression, matured verification, or answered unapplied decision
  can preempt normal rotation and the preemption is visible in history.
- Each completed Engineering/Operations review has exactly one deep-focus
  history entry plus its existing `pulse_review_log` receipt.
- Review prompts receive a compact agenda and selectors, not full plan/history
  payloads.
- Raw observations remain distinct from canonical issues, and review remains
  distinct from Fixer.
- Ordinary workflow runs do not trigger the rotation mechanism.
- The UI can explain, without reading logs, what was reviewed, why, what was
  found, what was deferred, and what becomes due next.

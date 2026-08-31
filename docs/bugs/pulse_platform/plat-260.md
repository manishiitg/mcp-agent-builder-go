[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-260 — Reflection write locks have no dependable Pulse lifecycle owner

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `proposed` |
| Last synchronized | `2026-08-30` |

- **Priority:** P2
- **Owner:** Pulse post-run governance → Pulse Fixer
- **Related:** PLAT-055 (merged reflection), PLAT-059 (reasoned learnings locks), PLAT-258 (plan-drift review)

## Problem

Reflections now write both reusable execution learnings and durable KB notes in
one post-completion pass. The platform has two controls for stopping those
writes: per-step `lock_learnings` and workflow-wide `lock_knowledgebase`.

Both controls work, but neither has a dependable lifecycle owner. Runtime
never changes them. The user is not expected to discover or operate internal
configuration flags. The Workshop Builder and Pulse Fixer *can* set them, but
no Pulse module is responsible for deciding when to lock, retain, or reopen
them. Consequently a stable workflow keeps paying for low-yield reflection
writes indefinitely, while existing locks can remain stale after behavior
changes.

PLAT-059 deliberately made a learnings lock a considered, reasoned action; it
did not supply the evidence-gathering process that should initiate that action.
PLAT-258 reviews whether locks are appropriate during plan-drift work, but its
current policy is recommendation-only and is not a reflection-lock lifecycle.

## Required outcome

Pulse owns the lock lifecycle. It must evaluate reflection yield from durable
evidence, apply safe freezes through the existing configuration tools, and
reopen freezes when their evidence becomes stale. This is governance over
reflection writes, not a new third `lock_reflections` flag.

### Learnings — per-step lifecycle

For each step with `learnings_access="read-write"` and a learning objective,
Pulse must inspect its current description hash, recent successful runs, and
`.learning_metadata.json`. It may lock only when the current behavior is stable
and recent reflection output is repetitive, empty, or otherwise no longer
improving the shared skill. It records this evidence through
`update_step_config(lock_learnings=true, lock_learnings_reason=...)`.

Pulse must reopen a learnings lock when a material description,
learning-objective, access-mode, or ownership change makes the evidence stale.
It must never lock before the shared skill has bootstrapped, and must prefer
`learnings_access="read"` when a step simply has no reusable HOW to contribute.

### KB — workflow lifecycle

Pulse decides the KB lock only from workflow-wide evidence: the current
ownership/notes manifest is clean, active KB contributors are stable, and
automatic note writes have become low-yield. It then uses
`update_workflow_config(lock_knowledgebase=true)`. A material plan, ownership,
or KB-contribution change makes that lock due for review and allows Pulse to
reopen it.

## Design constraints

- Retain the existing two flags; do not introduce `lock_reflections`.
- Every action records reviewed runs/metadata/manifest, expected cost benefit,
  and risk of freezing new knowledge.
- An inconclusive review leaves state unchanged and records a decision-required
  finding rather than inventing a rationale.
- One destination's lock does not silence the other destination's reflection
  write.
- The lifecycle must work for unattended/scheduled Pulse, not only an
  interactive Builder turn.
- Plan-drift review may supply evidence, but stable unchanged workflows also
  need periodic reflection-yield review.

## Acceptance criteria

1. Manual and scheduled Pulse can evidence-lock eligible learnings per step.
2. Pulse locks KB only after the complete relevant workflow-wide manifest is
   clean.
3. Material changes make a locked learning/KB surface due for reassessment;
   Pulse can reopen it with an audit record.
4. Tests cover bootstrap protection, reason persistence, inconclusive no-op,
   stale-lock reopening, cross-store independence, and scheduled reachability.

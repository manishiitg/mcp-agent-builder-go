[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-199 — Pulse paid for a fresh Fixer context after Technical Review and stretched one repair across too many cycles

| Coordination | Value |
|---|---|
| Assigned agent | Codex |
| Ticket state | `implemented` — sequenced dispatch, child-session receipt identity, and backend review-phase tool gate pass focused scheduler/runtime tests; live Pulse reverify pending |
| Last synchronized | `2026-08-28` |

- **Priority:** P0 — review, approval, Fixer selection, and later verification
  repeatedly turned one small repair into several expensive Pulse passes.
- **Owner:** scheduled Pulse dispatch, background message-sequence runtime,
  reviewer receipt identity, tool authorization, and Review→Fix guidance.
- **Related:** [PLAT-138](plat-138.md), [PLAT-155](plat-155.md),
  [PLAT-163](plat-163.md), and [PLAT-198](plat-198.md).

## Evidence and decision

The previous scheduler launched a retained Technical Review sequence, returned
to the parent, then launched a fresh independent Fixer. That clean-room boundary
prevented self-approval, but it also discarded the reviewer's selected evidence,
route context, and repair reasoning. The Fixer paid to reload them and could
select a different queue item; approved prompt cleanup therefore remained open
across repeated Review/Fixer passes.

Pulse now keeps the useful separation as a phase boundary inside one retained
technical executor:

```text
review messages (read-only)
  → typed findings/focus + completed child receipt
  → backend receipt barrier
  → later repair message (mutation enabled)
  → immediate proportional proof + module result
```

This supersedes only the “fresh independent Fixer conversation” decision in
PLAT-138/155/163. Their still-valid boundaries remain:

- raw workflow observations are not repairable issues until Technical Review
  classifies/promotes them;
- the review receipt is durable and separate from repair outcome;
- Strategic Review remains a separate read-only sequence;
- high-risk, ambiguous, public-action, or operator-decision work is not silently
  repaired merely because the technical child retained context.

## Backend enforcement

1. `run_in_background` accepts
   `pulse_phase_contract="technical_review_then_fix"` only with a message
   sequence, exact parent `pulse_run_id`, and required
   `technical_review` receipt.
2. The child tool session starts with read-only evidence access, its exact
   `runs/pulse/<pulse_run_id>/` checkpoint, and typed review-persistence tools.
   Plan/config/DB/workflow mutation and `record_pulse_result` are rejected.
3. The receipt turn must return before repair. The runtime loads a completed
   `technical_review` receipt for that exact child session; recording a receipt
   does not unlock tools in the middle of the same model turn.
4. At least one later sequence turn must remain. Only then are the normal
   Workflow Builder write paths, DB mutation, repair tools, and module-result
   persistence restored.
5. Session-scoped MCP bridge routes consult the same phase gate, so a child
   cannot bypass it by invoking a tool endpoint from shell code.
6. Review rows now keep two identities: `pulse_run_id` correlates the parent
   Gate pass, while `review_run_id` is the exact child tool session used by the
   receipt barrier and review lookup.

## Acceptance

1. A due Technical Review and its bounded repair use one background executor,
   one conversation history, and one MCP session.
2. No workflow mutation tool can execute before a completed receipt for the
   same child session, including through session-scoped bridge URLs.
3. A receipt recorded during a turn unlocks only the next turn.
4. A sequence that ends on the receipt turn fails rather than claiming repair
   completion.
5. The post-receipt turn either applies/verifies a bounded canonical repair or
   records a truthful no-safe-repair technical result.
6. The scheduler no longer launches a separate Fixer stage or fresh Fixer
   background agent.
7. Strategic Review remains independently receipted and cannot enter the
   technical repair phase.
8. The parent scheduler advances to finalization only after due module results
   and reviewer receipts are both durable.
9. Manual `/pulse-review`, `/plan-prompt-bloat`, and every focused
   `/pulse-review-*` alias use the same retained child and receipt barrier;
   store aliases select `store_integrity` plus their knowledgebase, learnings,
   or database lens. The explicit `/pulse-fixer` command remains available for
   repair-only recovery against an already reviewed queue.

## Decision history

- **2026-08-28:** Replace the fresh Fixer child with one retained Technical
  Maintenance message sequence. This supersedes the conversation-isolation
  mechanism, not the review/issue/repair authority boundaries. A backend
  receipt-gated tool transition is required; prompt wording alone is rejected
  as insufficient.
- **2026-08-28:** Focused scheduler, background-sequence, typed-receipt, and
  phase-authorization tests pass. Full package suites still contain unrelated
  pre-existing failures tracked outside this ticket; live scheduled-Pulse
  re-verification remains.
- **2026-08-28:** Extended the same retained receipt-gated Review+Fix contract
  to all manual Pulse Review slash aliases. A manual child uses its own exact
  tool-session identity as `pulse_run_id`, keeping worklist, receipt, findings,
  and repair outcome in one correlation scope.

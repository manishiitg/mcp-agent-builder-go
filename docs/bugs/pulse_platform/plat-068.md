[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-068 — the step-type checklist names an automated owner that never loads it

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — confirmed by direct grep of the guidance templates |
| Last synchronized | `2026-08-10` |

- **Priority:** P2 — no incorrect behaviour; an entire review dimension simply never runs
- **Owner:** Pulse guidance templates (`review/ops-review.md`, `builder/design-plan.md`)
- **Found on:** cross-workflow, while asking why no step-type findings exist anywhere

## What happened

The plan-design guidance is not missing. It is thorough, and it explicitly claims an owner.

`builder/design-plan.md` line 3:

> `llm_ops_review` is this checklist's automated owner (engineering correctness — **step-type**/prevalidation/schema fitness, not tactic quality)

and it carries a full **PART 3 — STEP-TYPE FITNESS** section, including a well-reasoned mode rule (line 101):

> deterministic API/CLI/SDK/data-fetch/parse/transform steps start **scripted** … Judgment, synthesis, adaptive discovery, and browser/UI work stay **agentic**. The 10+-run evidence bar is only for *freezing* a proven script with `lock_code`, not for declaring an obviously deterministic step scripted. When `llm_ops_review` has recorded tool-call trace evidence that a step's actual behavior is deterministic and identical in shape across runs, ground the scripted-candidate finding in that real evidence rather than the description alone — a description that reads as agentic can still be masking a step whose real tool-call sequence never varies.

**The ownership claim is one-directional.** `review/ops-review.md` — the guidance `llm_ops_review` actually runs — contains **zero** references to `design-plan`, `review_plan`, "checklist", "structural", or "scripted". Confirmed by grep. No Pulse lens template invokes `review_plan` either.

So the checklist declares an owner; the owner's own instructions never load it. Nothing in a Pulse pass ever asks:

- should this step be scripted rather than agentic (or the reverse)?
- should these adjacent steps be one `message_sequence` sharing context?
- should this oversized sequence be split at a real context boundary?

`pulse-fixer-practices.md` has an "Applying an Ops config recommendation" section covering `declared_execution_mode`, but it is explicit that the Fixer is only a writer: *"Never apply one without an owning Ops finding. … If no finding recommends the change, the change is not justified."* With no lens producing those findings, that playbook is unreachable by construction.

## Evidence this is real, not theoretical

Upwork's own numbers make the missing dimension concrete. `search-find-and-shortlist` is a 6-turn `message_sequence` costing **$29.26 and 32.4 minutes in one run**, with **197 tool calls** — 70 of them inside a single turn. Whether that is the right shape is exactly the PART 3 question, and no reviewer is asked it. Meanwhile the *builder* is told to apply these rules when authoring a plan, so a plan is held to the standard once at creation and never again as it drifts.

## Compounding factor

`llm_ops_review` is currently **disabled at the Gate** (`system/pulse-gate.md` core-system verification block permits only `workflow_review`). So even the half that exists does not run today. Re-enabling the module is necessary but **not sufficient** — without the linkage below it would come back still not reading the checklist.

## Suggested fix

1. In `review/ops-review.md`, load the checklist explicitly for the structural pass and state that step-type/shape fitness is in scope, attributing findings as `llm_ops_review` per the existing rule in `design-plan.md` line 3.
2. Ground it in the evidence Ops uniquely holds: it already sees per-step cost, tool-call counts and tool-call traces, which is precisely the "real evidence rather than the description alone" that line 101 asks for. A step whose trace never varies in shape is a scripted candidate; a turn with 70 tool calls is a shape question.
3. Keep the Fixer contract unchanged — it stays the only writer, and still refuses changes with no owning finding.

## Acceptance

- A Pulse pass with `llm_ops_review` due produces step-type/shape findings where warranted, citing trace evidence, not just tier/model findings.
- The reachable-but-unused `declared_execution_mode` path in `pulse-fixer-practices.md` becomes reachable.
- Verification is blocked until `llm_ops_review` is re-enabled at the Gate; record that rather than claiming verification from a pass that never invoked the lens.

[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-287 — Retire `declared_execution_mode`: a step's plan type alone decides its execution model

| Coordination | Value |
|---|---|
| Assigned agent | Claude Fable 5.1 |
| Ticket state | `implemented` (half 1: make every plan type explicit, contract v1.0.38); half 2 (runtime flip + field removal) pending |
| Last synchronized | `2026-09-05` |

- **Priority:** P3 — no live failure; a model-cleanup the user asked for
  ("can we just remove this from code base") once PLAT-286 made the field
  redundant.
- **Owner:** `agent_go/pkg/orchestrator/agents/workflow/step_based_workflow/declared_execution_mode_migration.go`
  (trusted tool), `agent_go/cmd/server/workflow_manifest.go` +
  `workflow_version_upgrades.go` (contract v1.0.38), registration/allowlists
  alongside `migrate_orchestrator_step_type`.
- **Related:** PLAT-286 (`change_step_type`, which made the type change *be*
  the mode change), PLAT-280 (the drift a stray declaration caused),
  v1.0.35 (`migrate_orchestrator_step_type`, the pattern copied).

## What was found

`declared_execution_mode` is a leftover of the model in which a step's plan
type and its execution mode were independent. Today it is implied on one
type and forbidden on the other: a `regular` step is scripted by definition
(`add_scripted_step` sets the declaration automatically), a `regular` step
*without* it is the legacy agentic shape that the runtime silently runs as a
`message_sequence` through a compatibility shim
(`normalizeRegularStepToMessageSequence`, chosen by
`shouldNormalizeRegularStepToMessageSequence` in
`controller_message_sequence.go`), and `update_step_config` rejects the
declaration on a `message_sequence` outright. Deleting the field outright
would flip every legacy agentic regular step on disk to scripted with no
`main.py`, so the removal has to be staged.

**Review finding that reshaped the staging (2026-09-05, before anything
ran):** the first cut of half 1 also stripped the field from
`step_config.json`. That was wrong: the *current* runtime still reads it to
tell a scripted regular step from a legacy agentic one, so stripping it
would have turned every real scripted step into a message_sequence whose
`main.py` never runs — and a re-run would have converted every remaining
regular step. The field can only go in the same release as the runtime
rule that makes it redundant. Half 1 is now convert-only.

## What shipped (half 1)

- **`migrate_declared_execution_mode`**, a trusted, idempotent,
  behavior-preserving tool that touches only `planning/plan.json`: every
  legacy agentic `regular` step becomes the `message_sequence` it already
  ran as (same id, description, dependencies, validation, `next_step_id`,
  position; nested and orphan steps included); any `message_sequence` still
  declared scripted (PLAT-280 drift) becomes `regular`. After it, every
  `regular` step in the plan is a declared scripted step — which is exactly
  the precondition half 2 needs. `step_config.json` is read, never written.
  The changelog entry carries the old/new step JSON of every type change.
  Two passes, so the one case it refuses to guess about — a step declared
  scripted with no `learnings/<id>/main.py`, already broken today — leaves
  the plan untouched and names the step. Idempotent by construction: a
  second run finds every regular step declared scripted and does nothing.
- **Contract v1.0.38** (`workflowContractDeclaredExecutionModeRetiredVersion`,
  rank 37, `WorkflowContractCurrentVersion` bumped): the upgrade prompt tells
  the Builder to call the tool once, not hand-edit, not run the workflow,
  report a refusal without stamping, otherwise stamp. It rolls out per
  workflow on each Builder's next preflight, exactly like v1.0.35/1.0.37.
- Registered in the Workshop allowlist, `IsPlanModificationTool`, and the
  toolset invariant.
- Known leftovers half 1 does not touch (by design — they are inert until
  half 2): `use_code_execution_mode`/`lock_code` values that
  `syncDeclaredExecutionModeConfig` set on converted steps; steps of other
  types whose config carries the field.

## Half 2 (not started — one release, gated as below)

In one release: the runtime rule becomes "`regular` ⇒ scripted" (delete
`shouldNormalizeRegularStepToMessageSequence` and the shim,
`isScriptedExecutionModeConfig` collapses to the type check); every writer
stops writing the field (`add_scripted_step`, `change_step_type`,
`update_step_config`, `migrate_message_sequence_code_items`,
`update_scripted_step`'s PLAT-280 downgrade); a v1.0.39 migration strips
`declared_execution_mode`/`_reason` (and stale `lock_code`) from every
`step_config.json` entry; the field is deleted from `AgentConfigs`; guidance
and docs updated (`step-config.md`, `optimize-playbook.md`,
`learn_code_flow.md`, `instructions.go`, Pulse templates) and ~8 test files.
One guard so a straggler fails loudly: plan validation flags a `regular`
step with no `main.py`.

**Gate:** every live workflow on both boxes is on 1.0.38 — i.e. no legacy
agentic regular step remains (every `regular` step is declared scripted and
has a `main.py`). Not "the field is absent": writers keep producing it until
half 2, so that gate could never be met.

## Verification

- `declared_execution_mode_migration_test.go`: the mixed plan (legacy
  agentic regular → explicit sequence with fields kept; true scripted stays
  regular; plain sequence untouched; `step_config.json` byte-for-byte
  untouched; changelog with the type change and old/new JSON; a second run
  is a no-op), the PLAT-280 drift repair with the declaration preserved, the
  refusal that changes nothing (including the step it would otherwise have
  converted), the clean-workflow no-op with a real scripted step present,
  `IsPlanModificationTool`.
- The contract-ladder tests extended by the new rung (every ladder gains
  one); `TestToolSetInvariants`.
- Not yet done: running the upgrade on a real workflow (Dominion's
  `tectonicusadaytrading` is the first candidate, after the pending
  `dominion-agent` restart).

[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-287 — Retire `declared_execution_mode`: a step's plan type alone decides its execution model

| Coordination | Value |
|---|---|
| Assigned agent | Claude Fable 5.1 |
| Ticket state | `implemented` (half 1: migration + contract v1.0.38); half 2 (delete the field) pending until every live workflow is on 1.0.38 |
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
(`normalizeRegularStepToMessageSequence`), and `update_step_config` rejects
the declaration on a `message_sequence` outright. 41 files reference it.
Deleting the field outright would flip every legacy agentic regular step on
disk to scripted with no `main.py`, so the removal has to be a migration
first and a code deletion second.

## What shipped (half 1)

- **`migrate_declared_execution_mode`**, a trusted, idempotent,
  behavior-preserving tool: every legacy agentic `regular` step becomes the
  `message_sequence` it already ran as (same id, description, dependencies,
  validation, `next_step_id`, position; nested and orphan steps included);
  any `message_sequence` still declared scripted becomes `regular`; then
  `declared_execution_mode` and `declared_execution_mode_reason` are
  stripped from every `step_config.json` entry, with each removed mode and
  reason preserved as per-field changes in the changelog entry alongside
  the old/new step JSON of every type change. Two passes, so the one case
  it refuses to guess about — a step declared scripted with no
  `learnings/<id>/main.py`, already broken today — leaves plan and config
  untouched and names the step. No-op on a clean workflow.
- **Contract v1.0.38** (`workflowContractDeclaredExecutionModeRetiredVersion`,
  rank 37, `WorkflowContractCurrentVersion` bumped): the upgrade prompt tells
  the Builder to call the tool once, not hand-edit, not run the workflow,
  report a refusal without stamping, otherwise stamp. It rolls out per
  workflow on each Builder's next preflight, exactly like v1.0.35/1.0.37.
- Registered in the Workshop allowlist, `IsPlanModificationTool`, and the
  toolset invariant.

## Half 2 (not started — gated on the boxes being clean)

Delete `DeclaredExecutionMode`/`Reason` from `AgentConfigs`; collapse
`isScriptedExecutionModeConfig` to "is the type regular"; remove
`syncDeclaredExecutionModeConfig`, the runtime shim, and
`update_scripted_step`'s PLAT-280 downgrade; drop the field from
`update_step_config` and `change_step_type`; update guidance/docs
(`step-config.md`, `optimize-playbook.md`, `learn_code_flow.md`,
`instructions.go`, Pulse templates) and ~8 test files. One guard so a
straggler fails loudly: plan validation flags a `regular` step with no
`main.py`. Done-criteria before shipping it: `grep declared_execution_mode`
over every `step_config.json` on Dominion and RTS returns nothing, and every
`regular` step has a `main.py`.

## Verification

- `declared_execution_mode_migration_test.go`: the mixed plan (legacy
  agentic regular → explicit sequence with fields kept; true scripted stays
  regular; plain sequence untouched; field stripped everywhere, other config
  fields kept; changelog preserves the reasons and carries the type change
  with old/new JSON), the PLAT-280 drift repair, the refusal that changes
  nothing (including the step it would otherwise have converted), the
  clean-workflow no-op, `IsPlanModificationTool`.
- The contract-ladder tests extended by the new rung (every ladder gains
  one); `TestToolSetInvariants`.
- Not yet done: running the upgrade on a real workflow (Dominion's
  `tectonicusadaytrading` is the first candidate, after the pending
  `dominion-agent` restart), then the grep-clean check on both boxes that
  gates half 2.

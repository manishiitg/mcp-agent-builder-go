[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-259 — Split routing's overloaded semantics: `routing` becomes the "route" (major fork) concept, new `branch` step type for small in-flow decisions

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `design complete; phase A (the branch step type) implemented; guidance best-practices reuse of plan_drift_review and frontend per-route reporting tabs not yet built` |
| Last synchronized | `2026-08-30` |

- **Type:** platform feature (design only, no code changed). Filed at the
  user's explicit request immediately after the design converged, to record
  the full negotiated shape before implementation starts.
- **Origin:** the user opened a design discussion, not a bug report: today's
  single `routing` step is used interchangeably for two genuinely different
  needs — a small in-flow "if this then step A else step B" decision, and
  what should really be a major, self-contained sub-workflow fork. The user
  named this directly: *"route is like a major sub workflow... branch is a
  small decision to choose next step... right now we use route vs branch
  interchangeably, but both should be different."*

## Current state (confirmed by code reading, not assumption)

`RoutingPlanStep` (`agent_go/pkg/orchestrator/agents/workflow/step_based_workflow/planning_agent.go:393`,
`step.Type == "routing"`) is a deterministic N-way switch:

- Never runs an agent/LLM itself. `description` must be empty — the
  executor (`controller_routing.go:44-46`) hard-errors if it isn't, forcing
  any judgment/probe logic into a *prior* step that writes
  `route_selection.json`.
- Route selection (`controller_routing_deterministic.go`,
  `resolveDeterministicRoutingSelection`) checks, in order: a pre-seeded
  `route_selection.json` in the step's own execution folder (settable via
  `run_full_workflow`'s `route_selections` param before the run starts),
  `route_source_file`, a `context_dependencies` entry literally named
  `route_selection.json`, then `default_route_id`; otherwise a hard error.
- No sub-workflow concept exists anywhere in the plan schema — every step
  type lives in one flat `steps[]` array regardless of which route led to
  it. Nothing stops two routes' downstream paths from converging back onto
  a shared step. No per-route eval breakdown, no per-route reporting
  grouping.

## Design (agreed with the user across this session)

**Keep `routing`/`RoutingPlanStep` completely unchanged in code.** Zero
migration, zero backward-compat risk to any existing stored `plan.json` —
nothing about the struct, the JSON type tag, or the executor changes.
Reinterpret it *conceptually*, going forward, as the **route** concept
(major fork): "routing" already means choosing a route linguistically, so
no new type is needed for this half. Existing plans that used `routing` for
small decisions keep executing exactly as before — nothing here is a
retroactive runtime requirement.

**Add one new step type, `branch`**, for the small in-flow next-step
decision — same shape/executor as `routing`'s current behavior, just a
distinct type tag so guidance, reporting, and eval tooling can tell the two
apart going forward.

**Route-specific properties are agentic/guidance-level, not Go-enforced.**
No shared downstream steps between sibling routes, and always getting an
eval breakdown when the workflow has one, are deliberately *not* hard
runtime validators — the user was explicit about this ("not enforceable in
golang code"), wanting a planning-agent self-check (the same pattern as the
`step-description.md` guidance skill from PLAT-255) rather than a rigid
validator that could block a legitimate exception. The user also confirmed
the "no shared steps" rule already conceptually exists today and simply
isn't consistently followed — this ticket's guidance work is what's
supposed to close that gap, not a new mechanism.

**Reporting is the one real, buildable feature, not a best practice.**
Once `routing` is reliably treated as "route" going forward, execution-log
reporting can mechanically group by `step.Type == "routing"` into a
top-level tab per route — this is deterministic UI work, not something to
nudge the planning agent toward.

### Alternatives considered and rejected

- **New `route` type + rename existing `routing` → `branch`.** Rejected:
  forces a choice between a one-time migration of every stored `plan.json`
  or a permanent `routing`-as-alias-of-`branch` shim, for no benefit —
  `routing`'s existing name already fits the "route" concept better than
  it fits "branch."
- **New `route` type added alongside unchanged `routing`.** Rejected:
  `routing` and `route` are lexically almost identical (one is a substring
  of the other), reintroducing the exact route/branch mixup that started
  this discussion — a planning agent or a human skimming `plan.json` could
  easily conflate them.

## Phase A — implemented: the `branch` step type

`branch` is a real, working step type now, functionally identical to
`routing` (unchanged, per the design above).

**Backend** (`agent_go/pkg/orchestrator/agents/workflow/step_based_workflow/`):
- `StepTypeBranch` constant + `BranchPlanStep` struct (`planning_agent.go`)
  — a **distinct Go struct**, not `RoutingPlanStep` reused with a `Type`
  differentiator (resolves the first open question below): the codebase's
  dispatch pattern is pervasive type-assertion (`step.(*RoutingPlanStep)`),
  which a single reused struct with two type tags would have fought against
  everywhere. One deliberate field-name difference: `branch_question`, not
  `routing_question` — `RoutingPlanStep` itself stays untouched.
- Extracted a new `routeSwitchStep` interface (`GetRoutes`,
  `GetDefaultRouteID`, `GetRouteSourceFile`, `GetRoutingQuestionText`,
  `SetSelectedRouteID`, `SetRoutingResponse`, plus `PlanStepInterface`)
  implemented by both `RoutingPlanStep` and `BranchPlanStep`. The entire
  executor (`executeRoutingStep` in `controller_routing.go`,
  `resolveDeterministicRoutingSelection` and its helpers in
  `controller_routing_deterministic.go`, ~540 lines total) now operates on
  this interface instead of the concrete `*RoutingPlanStep` type — one
  execution code path for both step types, nothing duplicated.
- Wired into every polymorphic step-type switch (JSON parse/unmarshal ×3,
  `updateToolForStepType`, `createSingleStepAdder`'s validation switch,
  `isRoutingStep` in `controller_execution.go` — broadened to recognize
  both types since every existing caller wanted identical treatment for
  branch: no learnings, routes through the same executor — and the
  legacy-description pre-flight guard in `planning_exports.go`).
- New `add_branch_step`/`update_branch_step` tools (**distinct tools**,
  resolves the second open question below — matches the codebase's
  established one-tool-pair-per-step-type convention rather than a shared
  tool with a `kind` parameter): own JSON schema
  (`getAddBranchStepSchema`/`getUpdateBranchStepSchema`), own executors
  (`createAddBranchStepExecutor` delegates to the same generic
  `createSingleStepAdder("branch", ...)` routing already uses;
  `createUpdateBranchStepExecutor` mirrors `createUpdateRoutingStepExecutor`
  exactly), registered next to the routing tools. Added to both allow-lists
  (`interactive_workshop_manager.go`'s Workshop-mode tool list,
  `planning_management.go`'s tool-name group checks) and the toolset
  invariant test's tracking list (`cmd/server/toolset_invariant_test.go`) —
  the exact registered-but-unreachable gap PLAT-258 phase 3 caught for
  `record_plan_drift_review` doesn't recur here.
- New `cmd/server/guidance/templates/system/branch.md` (mirrors
  `routing.md`'s structure: selection contract, single mode, structure,
  convergence, anti-patterns) plus a short reinterpretation note at the top
  of `routing.md` and updated coverage in `plan-design.md`'s step-type
  decision guide (Step 6 + the type enumeration) so the planning agent
  actually learns branch exists at the point it chooses a step type, not
  only inside the reference doc. New `branch` entry in `guidance.go`'s
  `referenceKinds` registry.

**Frontend** (`frontend/src`): no manual step-type picker exists anywhere —
steps are authored by the planning agent — so this is purely
recognize/render/exclude, no new UI. `stepConfigMatching.ts`'s `PlanStep`
union gained `BranchPlanStep` + `isBranchStep`/`isRouteSwitchStep` guards;
`usePlanToFlow.ts`'s node/edge building, route-target collection, and
layout sizing now handle branch alongside routing; `nodes/index.ts`/
`edges/index.ts` register `branch: RoutingStepNode`/`branch: RoutingEdge`
(reused as-is, not forked, since branch renders identically to routing);
`WorkflowCanvas.tsx`'s node inspector and `LearningsPopup.tsx`'s
learnings-eligible-step filter both extended; `ExecutionLogsPopup.tsx`'s
three step-type label/description/badge helpers gained a `branch` case
with its own cyan badge, distinct from routing's indigo, so the two are
visually distinguishable in Execution Logs (the one surface where telling
them apart actually matters for this phase — reporting them into separate
top-level tabs is still future work, see below).

## Explicitly not done (still open)

- **Frontend per-route top-level reporting tabs.** Not built this phase —
  Execution Logs currently show routing and branch steps with distinct
  badges/labels, but neither gets a dedicated tab. This is real, buildable
  UI work once `routing` reliably means "route" going forward; scoping it
  is a separate pass.
- **Route best-practices guidance/self-check** (no shared downstream steps
  between sibling routes, always pair a route with an eval). Per the design
  above, this should extend PLAT-258's `plan_drift_review` infrastructure
  (durable evidence-required record + due-detection + the privileged
  `record_plan_drift_review` tool) with route-specific check types, rather
  than inventing a second, parallel self-check mechanism — not started.

## Open questions for the implementation phase

The two structural questions from the design phase are now resolved by
Phase A above (distinct `BranchPlanStep` struct; distinct
`add_branch_step`/`update_branch_step` tools). Remaining, for the two
not-yet-done items above:

- Exact shape of the route-specific `plan_drift_review` check types (new
  `check_id`s? a distinct reviewer-turn trigger for route steps
  specifically, or folded into the existing due-scan?).
- Frontend: where exactly the per-route top-level tab lives in the
  Execution Logs / reporting surfaces, and how it interacts with the
  existing step-summary view for plans with zero `routing` steps.

## Verification

- `go build ./agent_go/... ./workspace/...` clean, `gofmt -l` clean.
- `go test ./pkg/orchestrator/agents/workflow/step_based_workflow/...`,
  `./cmd/server/guidance/...`, and `./cmd/server/ -run TestToolSetInvariants`
  all pass — including 7 new tests in `controller_branch_test.go`
  (validation accept/reject parity with routing, `isRoutingStep` recognizing
  both types, JSON marshal always setting `type: "branch"`, `parseStepFromJSON`
  round-tripping a real branch payload) and one existing test updated for the
  new "regular, human_input, todo_task, routing, branch, or message_sequence"
  error text (`TestPlanningResponseRejectsLegacyConditionalStep`). Full
  `cmd/server` suite has one pre-existing, unrelated failure
  (`TestEveryPulsePlatformTicketIsLinkedFromTheRegister`, missing
  `plat-248.md`/`plat-249.md` from a concurrent session — confirmed
  reproducible with this ticket's changes fully stashed out).
- `cd frontend && npx tsc --noEmit -p . && npm run build` clean;
  `npx vitest run` has 2 pre-existing failures unrelated to this change
  (`sessionRestore.productFallback.test.ts`, video-studio mock-argument
  drift — confirmed reproducible with this ticket's changes fully stashed
  out).

## Reverify

Ask the planning agent to add a `branch` step to a real workflow plan via
`add_branch_step`, confirm it appears correctly on the canvas (indigo
routing / cyan branch badges distinguishable in Execution Logs), executes
via `run_full_workflow` exactly like a routing step would, and that
`read_skill(skills=[{"name":"builder-reference","path":"references/branch.md"}])`
returns the new guidance. The two "explicitly not done" items above
(reporting tabs, guidance/self-check reuse of `plan_drift_review`) remain
open follow-up work, not covered by this reverify.

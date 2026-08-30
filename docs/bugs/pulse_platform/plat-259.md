[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-259 — Split routing's overloaded semantics: `routing` becomes the "route" (major fork) concept, new `branch` step type for small in-flow decisions

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `open` — design agreed, **not implemented, not scheduled** |
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

## Explicitly not done in this ticket

This is a design record only — no code was written, no Go type added, no
guidance skill authored, no frontend reporting change made. It exists to
capture the agreed shape before implementation work is scoped and started.

## Open questions for the implementation phase

- Exact `branch` Go type: a distinct struct, or `RoutingPlanStep` reused
  verbatim with the `Type` field as the only differentiator?
- New planning tool surface: a distinct `add_branch_step` alongside the
  existing routing-step tooling, or one tool with a `kind` parameter?
- Content of the new "route" best-practices guidance skill (no shared
  steps, always pair with an eval when the workflow has one) — needs the
  same treatment `step-description.md` got in PLAT-255, including hints
  wired into the relevant add/update step tool responses.
- Frontend: where exactly the per-route top-level tab lives in the
  Execution Logs / reporting surfaces, and how it interacts with the
  existing step-summary view for plans with zero `routing` steps.

## Reverify

N/A — no implementation exists yet to verify. Reverify once a follow-up
ticket implements the `branch` step type and the route reporting/guidance
work described above.

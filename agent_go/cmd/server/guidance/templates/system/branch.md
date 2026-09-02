## BRANCH STEP DESIGN

A branch step is a deterministic switch — a small in-flow next-step
decision. It reads `route_selection.json`, resolves the selected value to
one of its `routes[]`, and jumps to that route's `next_step_id`. Mechanics
are identical to a routing step (`references/routing.md`); the only
difference is intent. **Routing is now the "route" concept: a major,
self-contained sub-workflow fork.** Use branch instead when the decision is
small — a lightweight fork inside an otherwise normal plan, not a whole
alternate path through the workflow. See PLAT-259.

Use branch when the workflow must run **exactly one** of a few existing
downstream steps, and the fork itself is minor — not a candidate for its
own eval breakdown or a top-level tab in reporting. If the decision is
actually a major sub-workflow fork (each option leads to a substantially
different, self-contained continuation of the plan), use a routing step
instead.

### When to use branch

- The path forward is conditional on a known signal (e.g., "logged in",
  "MFA required", "document type is invoice")
- The fork is small: a couple of steps differ, then the plan converges back
  to a shared continuation
- There are 2-N mutually exclusive paths and only one should run
- The selected path can be represented as a stable `route_id` or a unique
  `next_step_id`

### Route selection contract

Identical to routing. The branch step reads this file shape:

```json
{
  "select_route": "route_id_here"
}
```

Compatibility aliases are accepted: `route_id` and `selected_route_id`.

The value may be:

- a `routes[].route_id`
- a unique `routes[].next_step_id`

If the file exists but is invalid, the branch fails. If no file exists,
`default_route_id` is used when set; otherwise the branch fails. Branch
never silently chooses the first route.

### Single mode

Branch steps never execute agents. Leave `description` and `context_output`
empty. The step reads a caller-preseeded `route_selection.json`, an explicit
`route_source_file`, a `context_dependencies` entry named
`route_selection.json`, or `default_route_id`.

When an agent/probe/classifier must decide the route, model it as:

- prior `message_sequence` step: performs the probe/classification and
  writes `route_selection.json`
- branch step: declares `route_source_file` or
  `context_dependencies: ["route_selection.json"]` and branches from that
  file

### Branch structure

A branch step has:

- `branch_question` — **REQUIRED (non-empty)**: the runtime errors if it is
  missing. It is not evaluated by an LLM; it is kept for plan
  readability/compatibility, but you must still set it.
- `routes[]` — minimum 2 entries (required)
- `default_route_id` — optional fallback `route_id` used when no route file
  exists
- `route_source_file` — optional explicit route file source produced by a
  prior step

Each entry in `routes[]` has:

- `route_id` / `route_name` — stable identifier the route file selects
- `condition` — short prose explaining when this route should be selected
- `next_step_id` — the ID of an existing step in the plan that this route
  jumps to

Branch routes do **not** define inline sub-agents. Every `next_step_id`
must reference a step that already exists in the plan. Add those
downstream steps separately (as regular, message_sequence, orchestrator, or
human_input steps), then point the routes at their IDs.

### Convergence — options MUST rejoin via `next_step_id`

A branch step jumps execution **into** the selected option, but it does not
stop the engine from continuing in list order afterwards. So each option
must explicitly route **out** to the shared continuation (or `end`), or
execution will fall through into the *next* option in the list and run a
non-selected path. Same rule as routing (see `references/routing.md`'s
"Convergence" section for the full explanation) — give the terminal step of
each option a `next_step_id` pointing to the shared downstream step, or
`"end"` to finish.

### Branch vs. other primitives

- **Branch vs. routing**: routing is now the "route" concept — a major,
  self-contained sub-workflow fork; use it when the alternatives lead to
  substantially different continuations of the plan. Branch is the small
  in-flow decision.
- **Branch vs. orchestrator**: orchestrator can run multiple sub-tasks. Branch
  runs exactly one alternative.
- **Branch vs. message_sequence**: message_sequence is one ordered
  conversation with no branching.
- **Branch vs. human_input**: do not ask the user again when the builder
  already knows the requested option. Use `route_selections`. Use
  `human_input` only when the workflow must pause mid-run for information
  that was not available at launch.

### Anti-patterns

- Branch with only one route, or with a generic catch-all route that should
  be normal step logic.
- Asking the branch step to infer the route from prose without writing
  `route_selection.json`.
- Setting `description` on a branch step. Use a prior message sequence for
  probe/judgment work.
- `next_step_id` pointing to a step that does not exist yet.
- Branch with no caller `route_selections`, no `route_source_file`, no
  `route_selection.json` dependency, and no `default_route_id`.
- Using branch for what is actually a major sub-workflow fork — use routing
  instead.

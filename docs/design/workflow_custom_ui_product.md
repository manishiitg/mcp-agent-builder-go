# A Workflow as a Product: Custom UI Frontend, AgentWorks as Backend

**Status:** Concept + feasibility research (2026-08-16). Not started. Two of
the three gaps below need design work before anyone estimates them.

## The idea

Today a workflow is operated from the Workshop — a builder's surface. The
idea here is to let a workflow ship its own **custom-built UI**, one per
workflow, aimed at non-technical users:

> "a custom UI interface for a workflow… this is like the frontend and
> AgentWorks is the backend agent… this makes it very easy for users to do
> stuff via UI and also view via UI, and we have chat but chat can access
> data, or run steps or run workflow — not update the plan or update
> learnings"

Explicitly **not** one generic operator console over all workflows. Each
workflow gets its own bespoke product UI, built for its own domain.

**The UI is hand-written React that we build** — a product surface in
`frontend/src/products/<name>/`, the same shape as `video-studio` and
`chief-of-staff`. It is *not* agent-authored HTML, and *not*
`db/reports/index.html`. That distinction is load-bearing: it is trusted
first-party code, so it calls the existing REST APIs directly with the
user's session like any other component, and needs no sandboxed bridge.

### Decisions taken (2026-08-16)

| Question | Decision |
|---|---|
| Who uses it | **Different people** — clients / ops staff, who should not see the builder |
| Which workflows appear | **Only ones explicitly published** to it, not all of `Workflow/` |
| What chat may do | **Answer + run.** No config changes; those go through UI forms |

The third decision matters more than it looks: variables and groups should
be **platform-rendered forms**, not chat and not agent-authored HTML. The
data is already a clean shape —
`variables/variables.json` → `{variables:[{name,value,group}], groups:[{id,name,enabled}]}`
(`cmd/server/instructions.go:240`) — so a form writing it over REST is
deterministic, costs no tokens, can't be talked into something else, and
keeps the untrusted-HTML surface smaller.

## Why this fits: most of it already exists

Verified in-repo, 2026-08-16:

| Piece | Status | Evidence |
|---|---|---|
| **Product surface pattern** | exists | A hand-written React surface is a lazy-imported branch in `App.tsx` plus three edits (store union, switcher array, ternary). `video-studio` and `chief-of-staff` are the two worked examples. This is the pattern the custom UI follows. |
| *(adjacent, not the mechanism used here)* | exists | `db/reports/index.html` is a workflow-owned **agent-authored** HTML report reading `db/db.sqlite` via `window.report` (`cmd/server/instructions.go:255`, `cmd/server/server.go:2284-2285`). Useful precedent for "a workflow owning a view", but this product uses first-party React instead — see above. |
| **"Can run, can't restructure" runtime** | exists | `GetToolsForWorkshopMode("run")` (`step_based_workflow/interactive_workshop_manager.go:1437`, run case at `:1598`) grants execution + `run_full_workflow` + read-only review, and withholds plan-mod, step-config, variable-config, schedule, skills, LLM-config, eval, report-authoring, and Pulse-state tools. Its own comment: *"No plan changes, no optimization, and no config changes — those belong to Workshop."* |
| **Profile → workflow execution bridge** | exists | A profile declaring `capabilities.workflow_execution` gets exactly 7 tools (`cmd/server/agent_profile_workflow.go:21-30`), is forced to `SetWorkshopModeOverride("run")` (`:184`, `:204`), and has `disable_eval: true` forced. |
| **Declarative tool narrowing** | exists | `tool_policy: {mode: allowlist, enabled: [...]}` in a product manifest, enforced at agent construction by `productToolGate` (`cmd/server/product_tool_gate.go:47`) and mirrored onto the coding-agent bridge catalog. |
| **Multi-user auth + access tiers** | exists | Three auth modes + JWT/OAuth; `cmd/server/workflow_permissions.go` defines `read\|write\|owner`, and every mutating workflow route is wrapped in `requireWorkflowWriteAccess(...)` (`cmd/server/server.go:2280-2283`, and throughout). |

So "a workflow with its own frontend" is not a new concept to introduce —
it is `db/reports/index.html` grown from a **report** into a **product**.

### One correction to the spec

The stated envelope was "no plan updates, no learnings updates." Run mode
currently **does** include `capture_context` — the "remember this while
running" tool, gated on explicit confirmation
(`interactive_workshop_manager.go:1614`, rationale at `:1600-1605`). The
desired envelope is therefore *run mode minus `capture_context`*, which is
precisely what a `tool_policy` allowlist expresses. Nothing about run mode
itself needs changing: declare a narrower allowlist in the product manifest
and both layers apply — the profile gate on the chat agent, run mode on the
execution session.

## The three gaps

### Gap 1 — The write barrier is soft, not real (highest priority)

**This is the one that blocks everything else for external users.**

`execute_shell_command` and `diff_patch_workspace_file` are in the `system`
list, documented as *"always available regardless of mode"*
(`interactive_workshop_manager.go:1438-1443`). Run mode therefore removes
the *typed* plan-editing tools but leaves a writable shell pointed at the
workflow folder. Nothing stops `echo … > plan.json` or edits under
`learnings/`. The remaining enforcement is prompt text — e.g. *"do not edit
`soul/soul.md` in Run mode"* inline in the template
(`interactive_workshop_manager.go:2445`).

For the builder's own use that is a reasonable guardrail against accident.
For clients and ops staff it is a convention, not a boundary.

**What's needed:** `plan.json`, `learnings/`, and `soul/` genuinely
read-only at the folder guard for this profile, with writes confined to run
outputs. The mechanism exists — `wrapExecutorsWithPlanFolderGuard(executors,
root, readOnlyFolders, …)` already takes a read-only list — but is currently
wired only for delegation-extracted `#workflow` paths
(`cmd/server/delegation.go:518-543`), not for protecting a workflow's own
plan in run mode. Needs a test that proves a shell write to `plan.json`
actually fails, not that a tool is absent.

### Gap 2 — API coverage for what the UI needs to do (mostly assembly)

Because the UI is first-party React inside the app, there is **no sandboxed
bridge to design** — it calls REST directly with the user's session, and
`requireWorkflowWriteAccess(...)` already guards the mutating routes. The
earlier concern about agent-authored HTML holding an action bridge does not
apply and has been dropped.

What remains is ordinary API work: confirm the endpoints a non-technical UI
needs (run workflow, run step, read run status/outputs, read + write
`variables/variables.json`) exist, are permission-checked, and return shapes
a UI can render without re-deriving workflow internals. Most exist; this
needs an inventory pass, not a design.

One thing to decide rather than assume: whether the *client-facing* UI
writes variables through the same routes the builder uses, or through a
narrower endpoint that can only touch values and group enablement — never
variable *definitions*. The narrower option is better aligned with "no
config changes," and the data shape already separates the two.

### Gap 3 — Permissions are global tiers, not per-workflow

`workflowPermissionConfig.entries` is `map[userKey]WorkflowAccessLevel`
(`cmd/server/workflow_permissions.go:30-33`) — one level per user across
**all** workflows. A `read` user is blocked from editing everything, but
there is no "user X sees workflows A and B only."

"Only workflows explicitly published to it," for a specific audience,
requires per-workflow visibility. Note also that `Workflow/` is **shared**
across users on disk — `perUserPrefixes` is only
`{Chats, Downloads, chat_history, memories}` (`workspace/utils/path.go:135`)
— so this is an access-control layer, not a storage change.

## Build order

Barrier-first. With the UI being first-party React, only Gap 1 carries real
design risk; the rest is assembly.

1. **The write barrier (Gap 1).** Wire read-only folder guarding for
   `plan.json` / `learnings/` / `soul/` under this profile, with a test
   asserting a *shell* write fails — not merely that a typed tool is absent.
   Everything else is unsafe for external users until this holds.
2. **Per-workflow `published` flag + visibility list (Gap 3).** Smallest
   real schema change; decide whether it lives in `workflow.json` or
   alongside the permission config.
3. **API inventory (Gap 2).** Confirm run/step/status/variables endpoints
   exist, are permission-checked, and return UI-renderable shapes. Decide
   whether client variable writes go through a narrowed values-and-groups
   endpoint rather than the builder's routes.
4. **The first custom React surface**, against one real workflow, with a
   run-mode chat panel using the narrowed `tool_policy` allowlist. Build one
   properly before generalizing — the second one is what reveals which parts
   were actually reusable.

Step 1 is where the actual thinking is. Steps 2–4 are known work.

## Open questions

- **How does one bespoke UI per client scale in a single frontend?**
  Product surfaces are lazy-imported (`App.tsx:46-47`), so the eager bundle
  budget (`EAGER_GZIP_BUDGET_BYTES = 1_030_000`, already raised once after
  main hit 1010.38 kB) is not the immediate constraint. But every client's
  UI still lives in one repo and ships in one deployment: all clients
  redeploy together, build time grows with N, and one client's chunk is
  fetchable by anyone who knows the URL. At what N does that stop being
  acceptable, and what is the answer then — a build-time per-client bundle,
  or something else?
- **Adding a product currently means three hand-edits** (store union,
  switcher array, `App.tsx` ternary) plus per-product boilerplate. That is
  fine for three products and annoying at fifteen. Worth a registry before
  this pattern is used per client, not after.
- **Where does the published/visibility list live** — `workflow.json`,
  the permission config, or a new store?
- **Does a client user need a login at all**, or a shared link with a
  scoped token? Changes the auth story substantially.
- **What happens to a published workflow whose plan changes underneath it?**
  The custom UI may reference steps or variables that no longer exist.
- **Is the chat panel per-workflow or global** across the published set?

## Non-goals

- One generic operator console for all workflows. Each UI is bespoke.
- Letting chat edit plan, learnings, config, or schedules.
- Changing run mode itself — the narrowing belongs in a product manifest's
  `tool_policy`.
- A separate binary or frontend app. This is a product surface inside
  AgentWorks (see `reusable_vertical_product_platform.md` for when a
  separate application *is* justified — this is not that case).

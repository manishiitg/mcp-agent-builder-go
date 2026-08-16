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
| **Custom UI per workflow** | exists | `db/reports/index.html` is "the complete workflow-owned reporting experience… owns any tabs, sidebar, sections, or scrolling layout; the platform adds no report navigation" (`cmd/server/instructions.go:255`). Rendered by `frontend/src/components/workflow/ReportViewer.tsx`; reads `db/db.sqlite` through `window.report` (`cmd/server/server.go:2284-2285`). Agent-authorable via the `design-reporting-ui` guidance skill (`guidance/guidance.go:85`) and validated by `validate_report_html`. |
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

### Gap 2 — `window.report` is read-only; there is no action bridge

The page can read durable data. Nothing lets it run the workflow, run a
step, or set a variable.

**Design risk worth naming:** the custom UI is **agent-authored HTML**. A
read bridge is safe. An action bridge means LLM-written JavaScript can
trigger runs and mutate state — and if that page was generated by an agent
that had read untrusted workflow data, it is a privilege path.

**Proposed shape (not yet designed in detail):** expose *declarative
intents, not arbitrary calls*. The page posts
`window.report.requestRun({workflow, group})`; the **platform** validates it
against the caller's permissions and the published-workflow list and
executes. The page never holds the user's token and never names an arbitrary
endpoint. This mirrors the existing rule that `RuntimeContext` holds
"trusted, server-resolved state" whose values are "never accepted from tool
arguments" (`pkg/agentprofiles/types.go:287-290`).

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

Deliberately barrier-first: gaps 1 and 2 carry the design risk, and steps 3–4
are mostly assembly on top of them.

1. **Per-workflow `published` flag + visibility list.** Smallest real schema
   change; unblocks the rest. Decide whether it lives in `workflow.json` or
   alongside the permission config.
2. **The write barrier (Gap 1).** Wire read-only folder guarding for
   `plan.json` / `learnings/` / `soul/`, with a test asserting a shell write
   fails.
3. **The action bridge (Gap 2), run-only.** `requestRun` / `requestStep`,
   platform-validated. No config mutation through this path.
4. **Variables/groups as platform-rendered forms.** Deterministic, no agent
   involvement.
5. **The product surface.** Lists published workflows, renders each one's
   own UI, plus a run-mode chat panel with the narrowed `tool_policy`
   allowlist.

Steps 2 and 3 are where the actual thinking is.

## Open questions

- **Does each workflow's UI live at `db/reports/index.html`, or a new
  sibling?** Reusing it means one artifact serves both "report" and
  "product"; splitting means two artifacts to keep coherent. Reuse looks
  right, but the naming would then be wrong everywhere.
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

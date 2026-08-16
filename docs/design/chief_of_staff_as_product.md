# Chief of Staff as a standalone product

Status: **requirements draft** — architecture prerequisites investigated and
largely settled; UI scope intentionally still open, captured below as
questions rather than decisions.

## Why

Chief of Staff — the user's operations hub, the multi-agent chat that runs
automations, remembers what matters across them, and surfaces what needs
attention — is not a product today. It is a sub-mode of AgentWorks, defined
purely as an *absence*: multi-agent mode with no Agent Profile attached
(`isChiefOfStaffChat := isToolBackedChat && resolvedProfile == nil`,
`agent_go/cmd/server/server.go:5285`). No directory, no `product.yaml`, no
frontend surface. Video Studio, by contrast, is a real product: a declarative
manifest, a registered Agent Profile, its own frontend surface, its own tile
in the product switcher.

The goal is to give Chief of Staff that same standing — a third top-level
product, alongside AgentWorks (which becomes Automations-only) and Video
Studio — with its own genuinely new UI, not a re-parented chat window.

## Core purpose (the MVP, everything else is an enhancement on top)

Chief of Staff's main purpose going forward: **a chat interface with
read-only access to every workflow.** Not primarily a dashboard product —
the chat itself, backed by visibility across all of `Workflow/`, is the
thing that has to work first. The custom automations-oversight dashboard
(below) is a later enhancement on top of a working chat, not a blocker for
it.

This isn't new backend behavior to invent — it matches what's already true
today (the delegation prompt already says *"Read workflow files with shell
tools, but do not modify workflow internals from Chief of Staff chat"*,
backed by the existing `workflowReadOnlyFolders` grant). The global-scoped
profile (see Backend architecture) needs to preserve exactly this: read
access across all workflows, write access confined to `pulse/`/org-owned
artifacts and its own chat history — the same shape the no-profile path
already grants today, not a new permission model.

## Decided

- **Three top-level products.** Chief of Staff gets its own tile in the
  product switcher (`frontend/src/components/ProductSurfaceSwitcher.tsx`), a
  peer to AgentWorks and Video Studio, not nested under AgentWorks.
- **Explicit, not a fallback.** Selecting Chief of Staff is deliberate, the
  same way opening Video Studio is. It's no longer the silent default for a
  profile-less multi-agent chat — though existing chats/tabs created before
  this change must keep working unmodified (see Backend architecture below).
- **AgentWorks means Automations.** After the split, AgentWorks is the
  automation/workflow product and lands there by default, not on a chat.
- **Org goals feature dropped for now.** Chief of Staff's current
  goal-tracking/goal-alignment behavior (`pulse/goals.html`, the "Org Goals"
  view, workflow-vs-goal alignment reporting) is removed from scope, not
  carried into the new product. This resolves the goals backend question
  below — there is no new `OrgGoal` typed model or JSON API to build if the
  feature itself isn't shipping. Can be revisited as a later addition once
  the underlying Pulse-goals data migration (see Data-source findings) is
  worth doing on its own merits. Practically: `OrgGoalsPanel`
  (`OrgHtmlPanels.tsx`) is removed, not ported; the org-goals guidance
  references in the delegation system prompt
  (`delegation_tools.go:697,717,728`) and `org-goals.md`/parts of
  `org-html.md` become dead prompt content to prune, not functionality to
  preserve.
- **Single pinned LLM, like Video Studio — no more High/Medium/Low tiers.**
  Chief of Staff drops its whole tier-routing concept and pins one model via
  `profile.runtime.provider`/`model_id`, the same field Video Studio already
  uses. This has real reach, more than it first looks like:
  - `DelegationTierConfigModal.tsx` (already on the "delete entirely" list
    above) is now doubly confirmed — not just relocated out of AgentWorks,
    genuinely unnecessary, since there's no tier concept left to configure.
  - The `ChiefOfStaffLLM` field and `ResolveProviderProfileChiefOfStaffConfig`
    cascade (`pkg/workflowtypes/types.go`) become dead code specific to
    Chief of Staff — remove alongside the tier UI, not leave unused.
  - `llmConfigSourceScheduledChiefOfStaff` (`server.go:1201-1209`) — the
    scheduled-run LLM-config source — now confirmed dead code, not just
    "likely": the built-in Org Pulse job is being removed fully (see
    "Scheduled tasks" above), so there's no scheduled Chief-of-Staff run left
    that needs it at all.
  - **Resolved: drop `reasoning_level` for now, keep it simple.**
    `delegate(reasoning_level)` currently selects *which tier's model*
    handles a sub-agent — with tiers gone and one pinned model, there's
    nothing left for it to select. Not building a reasoning-effort-on-one-model
    variant as part of this migration; can be added later if it turns out to
    matter.
- **`multi-agent` mode is untouched.** This is not a new mode. It is the
  shared execution substrate every product already runs on — Chief of Staff,
  the workflow-builder chat, and Video Studio all execute through
  `agent_mode: "multi-agent"` today (Agent Profiles are hard-rejected unless
  `agent_mode == "multi-agent"`, `agent_profile_runtime.go:156`). Chief of
  Staff becomes a *profile within* multi-agent mode — `agentProfileId:
  'chief-of-staff'` alongside the unchanged `mode: 'multi-agent'` — the exact
  relationship Video Studio already has. The name refers to the delegation
  capability (`delegate`/`query_agent`/`terminate_agent`/`list_agents`), not
  to Chief of Staff specifically; a fossil proves its age —
  `normalizeAgentMode` still maps the legacy value `"simple"` →
  `"multi-agent"` at the request boundary.
- **A genuinely new UI, like Video Studio's.** Not a thin wrapper around the
  existing `ChatArea`. Video Studio's surface
  (`frontend/src/products/video-studio/VideoStudioSurface.tsx`, ~1,100 lines)
  has real structure worth using as the template:
  - A **home screen** listing projects as cards (`ProjectCard`,
    `CreateProjectDialog`) — the landing view before you're inside any one
    project.
  - A **project workspace** (`ProjectWorkspace`) combining a persistent chat
    (`VideoStudioConversation`) with a **production panel**
    (`ProductionPanel`) of collapsible sections (`VideosSection`,
    `CharactersSection`, `DocumentsSection`), plus `FilesPanel` and
    `WorkflowPanel`.
  - Custom presentation rendering for domain content inline in chat
    (`MediaVideoPresentation`).

  Chief of Staff's equivalent structure is not yet decided (see Open
  questions) but should follow this same shape: a home view plus a workspace
  view, not just a chat.

## Backend architecture (investigated, holds regardless of UI scope)

This part of the plan is independent of how much new frontend gets built, and
was verified against the live code:

- **Agent Profiles are project-shaped; Chief of Staff is global.** A resolved
  profile today collapses folder-guard grants to one project root and
  **drops the `pulse/` write grant** the profile-less path has
  (`server.go:4787-4810`, confirmed by reading the actual branch — its own
  comment says *"Do not inherit the Chief-of-Staff chat-wide grants"*). Naively
  giving Chief of Staff a Video-Studio-shaped profile would silently break its
  ability to write `pulse/goals.html`/`pulse/org-pulse.html`.
  **Resolution:** add a declared `scope: global | project` property to
  `agentprofiles.Profile` (default `project`, so Video Studio is untouched),
  and make the ~7 narrowing branches in `agent_profile_runtime.go`/`server.go`
  skip project-only requirements (folder, project title, secrets scoping) for
  global-scoped profiles.
- **Safe inversion via widening, not migration.** Legacy Chief-of-Staff tabs
  (no `agent_profile_id` sent) keep resolving `resolvedProfile == nil` exactly
  as today — unchanged code path. New explicit tabs resolve the real
  `chief-of-staff` profile and take the *same* branch via
  `isChiefOfStaffChat := isToolBackedChat && (resolvedProfile == nil ||
  isGlobalScopedProfile(resolvedProfile))`. No forced tab-metadata migration,
  no window where an in-flight chat breaks.
- New backend package `agent_go/internal/chiefofstaffproduct/`, mirroring
  `videoproduct/` structurally but much smaller: no per-project provisioning,
  no embedded skills.
- **The three CoS-only tools move to `profile.tools[]`/`ToolFactory`** —
  corrected from an earlier draft, which recommended leaving them manually
  registered in `server.go` since `newProductToolGate` gates them correctly
  regardless of registration path. True but beside the point: the actual
  requirement (see "Old Chief-of-Staff code must be removed") is getting
  CoS-specific code out of `server.go`, not just correct gating. Checked and
  confirmed feasible: `ToolRuntimeContext` (`agentprofiles/types.go:216-225`)
  already carries `UserID`/`SessionID` — everything
  `registerActivityStatusTool(agent, currentUserID)`,
  `registerMultiAgentNotificationTool(agent, userID)`, and
  `registerWorkflowCreatorTool(agent)` need. Convert each to a `ToolFactory`
  (`func(ToolRuntimeContext, json.RawMessage) (ToolSpec, error)`), same
  pattern as `video.show-video`'s `showCharacterFactory`/
  `showDocumentFactory` in `internal/videoproduct/presentation_tools.go`,
  declared in `chiefofstaffproduct`'s `profile.tools[]`.
- **Delegation (`delegate`/`query_agent`/`terminate_agent`/`list_agents`)
  stays exactly where it is — it was never CoS-specific.** Verified:
  registered for ALL multi-agent chat (`if !isWorkflowPhase`,
  `server.go:4971`), not gated by `isChiefOfStaffChat`. Video Studio already
  gets this today. Nothing to move — already correctly platform-owned,
  generic infrastructure. Distinct from the *scheduled* background runs (Org
  Pulse firing as an async session), which genuinely is CoS-specific — see
  "Scheduled tasks" below.
- Keep the *dynamic* system-prompt assembly
  (`GetMultiAgentDelegationInstructionsWithUser` +
  per-request spawn capabilities + delegation-tier config +
  `AttachReferenceSurface`) rather than porting it into a static
  `product.yaml` prompt template — it depends on runtime parameters the
  static-template model can't express. Manifest's `prompt.file` is a short
  placeholder satisfying validation only.

Full technical detail (exact file:line changes, the widened condition at every
call site, test plan) lives in the implementation plan at
`~/.claude/plans/iridescent-snuggling-starfish.md`; that plan's frontend
section is now superseded by the UI scope decision above and needs a rewrite
once the questions below are answered.

## Scheduled tasks: product.yaml needs a schedule concept it doesn't have

Chief of Staff keeps its own scheduled tasks — this isn't new. But today's
mechanism doesn't fit the product model at all: `DefaultBuiltinSchedules()`
(`agent_go/cmd/server/builtin_schedules.go`) is a hardcoded Go function
returning `[]WorkflowSchedule`, entirely separate from `agentprofiles`/
`product.yaml`. Video Studio's manifest has no schedule concept either —
nothing in the product system today lets a product *declare* a built-in
schedule; it's Chief-of-Staff-specific special-casing.

**Decided: add this as a real product.yaml capability**, not keep it as a
one-off. A `schedules:` section (mirroring how `workflows:` already declares
fixed pipelines) — cron, name, description, default-enabled, and a
prompt/message-sequence — with generic registration replacing the hardcoded
function, the same way `videoproduct.BuiltinAgentProfiles()` is a generic
registration point today. This makes scheduling available to any future
product, not just Chief of Staff.

**Resolved: remove the built-in Org Pulse job fully**, not rewrite or defer
it. `builtinOrgPulseQuery`/`builtinOrgPulseMessages`
(`agent_go/cmd/server/builtin_schedules.go`) is goal-alignment-centric end to
end — every step reads/writes `pulse/goals.html`, compares workflows against
org goals. With goals dropped, there's nothing left to salvage by rewriting
it; it goes away entirely, consistent with the goals decision above. The
`schedules:` product.yaml capability itself stays as a real, generally
useful addition — this just means Chief of Staff ships with zero declared
schedules initially, not that the capability was pointless to build.

## Not a reassembly of existing AgentWorks UI

Rejected direction: relocating the existing scattered `Org*` components
(`OrgDashboard`, `OrgHtmlPanels`/`OrgGoalsPanel`/`OrgPulsePanel`/
`ChiefTasksPanel`, `OrgPulseControl`, `OrgBackupPublishControls`,
`WorkflowNotificationPopup`, `MultiAgentSchedulesPopup`,
`DelegationTierConfigModal`) into a new host component. That's still
AgentWorks UI, just re-parented — not a custom product.

The actual requirement: Chief of Staff moves toward the same model Video
Studio established — a `product.yaml`-defined product with a genuinely
custom UI, purpose-built around its own domain concepts, the way Video
Studio's UI is built around *projects → videos/characters/documents* rather
than reusing some generic AgentWorks file browser.

**What can still be reused** is the data/API layer underneath those
components (`agentApi.getBuilderDoc`, `schedulerApi.*`, the `pulse/*.html`
read paths) — that's plumbing, not UI. The presentation layer is what needs
to be designed fresh, grounded in what Chief of Staff actually does (per its
own system prompt: track org goals, oversee automations, delegate to
sub-agents, notify proactively, run on schedules) rather than in what
AgentWorks' generic chat/org chrome happens to already render.

### Domain concepts a custom UI could be built around

Video Studio's structure exists because video production has real, distinct
objects (a project, its videos, its characters, its documents). Chief of
Staff's domain has its own candidate objects, none of which have a
purpose-built visual today — they're either invisible (delegation) or
presented as raw agent-authored HTML (goals/pulse/tasks):

- **Delegations** — `delegate`/`query_agent`/`terminate_agent`/`list_agents`
  currently happen entirely inside chat text. A live roster of
  running/completed sub-agents, as first-class visual objects, doesn't exist
  anywhere today.
- **Goals** — currently an iframe of agent-authored `pulse/goals.html`.
  Could become a structured, purpose-designed goal-tracking view instead of
  rendered HTML.
- **Automations oversight** — health/goal/cost status per workflow (today:
  `OrgDashboard`'s HTML-card-scraping triage board) as a custom-designed
  status view.
- **Scheduled work / history** — recurring jobs and their run history (today:
  `MultiAgentSchedulesPopup`'s CRUD table) as a purpose-built timeline.

This list is a starting point for discussion, not a decision — see Open
questions.

### Data-source findings for the two chosen focus areas

Investigated whether real structured data exists to build custom UI on, or
whether new backend work is needed first (research only, no edits made):

**Automations oversight — real data already exists, no new backend work
needed.** Pulse has typed, SQLite-backed state (`PulseModuleState`,
`PulseFindingLifecycle`, `PulseGoalObservation`, `PulseImpactAssessment`,
`PulseAgentMetricRecord` — `agent_go/cmd/server/pulse_worklist.go`), already
served as JSON via real REST routes (`/workflow/pulse-module-state`,
`-findings`, `-reviews`, `-agent-metrics`, `-impact`, `-context`,
`server.go:2024-2029`), already has frontend TS types
(`services/api-types.ts:414-752`) and client calls
(`services/api.ts:1506-1556`), and already has a live precedent UI built on
exactly this data: `frontend/src/components/workflow/PulseWorkspace.tsx`
(972 lines), scoped to one workflow today. A custom automations-oversight
dashboard is "call the same per-workflow endpoints across all workflows and
aggregate" — proven pattern, not new plumbing.

**Bug found in passing, unrelated to this migration:** `OrgDashboard.tsx`
does not use any of the above. It reads `builder/card.health.html` /
`card.progress.html` / `card.cost.html` via `getBuilderDoc` — a pathway
nothing currently writes. The Pulse finalizer prompt explicitly forbids
writing them (*"Do not write a separate presentation artifact in this
turn"*, `guidance/templates/system/pulse-finalizer.md:5-6`), the only
remaining Go reference is a skipped test
(`t.Skip("Pulse no longer snapshots dashboard artifacts")`), and
`docs/workflow/org_dashboard_design.md` is marked historical. Today's org
dashboard is silently stale, independent of whether this product split
happens. Worth a separate fix regardless.

**Goals — no structured data exists; new backend work is required first.**
`pulse/goals.html` is freeform prose-in-HTML. A schema is *documented*
(`org-html.md`/`org-goals.md`: status, KPI targets, contributing workflows,
evidence — `org-html.md:240-337`) but unenforced — no Go struct, no DB table,
no JSON serialization, nothing validates an agent actually followed it on any
given write. A real (non-iframe) goals UI needs, at minimum: a typed
`OrgGoal`/`GoalTarget` struct, persistence, and new JSON read/write
endpoints — built by **extending** Pulse's existing
`PulseGoalObservation`/`PulseIntervention` types (already carry
`metric`/`value`/`status`), not a separate parallel model. This is not a new
pattern: module state, findings, reviews, agent metrics, and impact all
already made the same HTML-card → SQLite-backed-typed-state move. Goals is
the one piece of org-level state that never got that migration — this
finishes it rather than inventing a fifth approach.

## Old Chief-of-Staff code must be removed from AgentWorks, not orphaned

Explicit requirement, not incidental cleanup: once Chief of Staff has its own
product surface, the AgentWorks-side code that used to serve it gets
**deleted**, not left present-but-unused. This splits into two buckets:

**Chief-of-Staff-specific — delete entirely:**
- `frontend/src/components/org/OrgDashboard.tsx`
- `frontend/src/components/org/OrgHtmlPanels.tsx` (`OrgGoalsPanel`,
  `OrgPulsePanel`, `ChiefTasksPanel`, the shared `OrgHtmlPanel` primitive —
  all Chief-of-Staff-only)
- `frontend/src/components/OrgPulseControl.tsx`
- `frontend/src/components/org/OrgBackupPublishControls.tsx`
- `frontend/src/components/scheduler/MultiAgentSchedulesPopup.tsx`
- `frontend/src/components/DelegationTierConfigModal.tsx`
- The "Organization" sidebar section inside `EmployeeDashboard.tsx`
- The "Chief of Staff" and "Org" pills in `ModePresetBar.tsx`
- The multi-agent right-panel split-view logic in `App.tsx` (`files`/
  `org-goals`/`tasks`/`org-pulse` tab cycling beside the old chat lane)
- The Chief-of-Staff-specific toolbar wiring in `ChatTabs.tsx` — it stops
  being reached for Chief of Staff at all once the product branches before it
  (`App.tsx:1243`'s `productSurface === 'video-studio'` split is the proven
  shape: Chief of Staff gets the same kind of branch, bypassing `ChatTabs`/
  `ModePresetBar` entirely, same as Video Studio does today)

**Generic/shared — keep the component, remove only the Chief-of-Staff call
site:**
- `frontend/src/components/WorkflowNotificationPopup.tsx` — also used by
  `WorkflowToolbar.tsx` with `scopeKind="workflow"`; stays, loses its
  `scopeKind="chief-of-staff"` usage
- Backend Pulse/scheduler/notification machinery Chief of Staff uses but
  doesn't own (`scheduler.go`, `multiagent_notifications.go`,
  `report_human_inputs.go`) — unaffected, still serves other callers

## Resolved questions (kept for history, not currently open)

1. ~~What does the home screen show?~~ **Resolved:** chat is the core
   surface (see "Core purpose"); the automations-oversight dashboard is an
   enhancement on top, not the primary screen. Goals dropped, so it's no
   longer a candidate home-screen section.
2. ~~Is chat primary or secondary?~~ **Resolved: primary.** "Mainly purpose
   of chief of staff can be a chat interface... like video studio, not
   agentworks" — `ChatArea`-direct, no right-panel split.
3. ~~Which scattered org components get absorbed?~~ **Resolved: none.** Not
   a reassembly of existing AgentWorks UI — see that section above. They get
   removed, not relocated (see "Old Chief-of-Staff code must be removed").
4. ~~Multi-tab or single view?~~ **Resolved: single, like Video Studio.**
   `ChatTabs.tsx` is bypassed entirely for Chief of Staff, same
   `productSurface`-level branch shape as Video Studio
   (`App.tsx:1243`) — confirmed Video Studio never imports `ChatTabs`.
5. ~~How does chat relate structurally to the dashboard?~~ **Resolved: the
   dashboard is the home screen** — the same two-screen shape as Video
   Studio (home → workspace), just with no "list" step in between, since
   there's one singleton Chief-of-Staff chat to enter rather than a list of
   projects to choose from. Doesn't contradict "chat is primary" above —
   that was about build priority/core value, not which screen renders
   first. Video Studio's own home (project list) isn't the product's core
   value either; the workspace is. Same relationship here: dashboard is the
   front door, chat is what's behind it.

## Deferred (deliberately, not forgotten — revisit later)

- **MCP servers via product.yaml.** No working precedent exists anywhere in
  the codebase (Video Studio's `dependencies.mcp_servers: []` is an unused
  placeholder; `resolveAgentProfileForQuery` never touches
  `EnabledServers`/`SelectedServers`). Genuinely new design needed, not
  urgent for the first version — `delegate(servers)` already covers
  per-sub-agent MCP scoping today, which is enough to ship without this.
- **The stale `OrgDashboard` bug** (reads dead `builder/card.*.html` files
  nothing writes anymore — see Data-source findings). Real, independent of
  this migration, but not blocking it. Revisit separately.

## Status

All open design questions are resolved except the two intentionally
deferred items above (MCP servers, the stale `OrgDashboard` bug). This
requirements pass is complete — ready to move into a proper implementation
plan (rewriting/superseding the frontend section of
`~/.claude/plans/iridescent-snuggling-starfish.md`, whose backend section
still holds).

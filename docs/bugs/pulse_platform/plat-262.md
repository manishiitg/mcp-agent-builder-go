[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-262 — Read-only workflow user: restore the read permission tier with real enforcement (not the removed dead gate)

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `implemented, verified by build/vet/gofmt/tests, live reverify not yet run` — mechanism keyed on `WorkflowAccessLevel` (access-level axis) instead of `WorkshopMode` (focus axis), after finding a dedicated design doc that deliberately removed mode-based tool filtering; confirmed the corrected mechanism matches an already-shipped precedent and closes cleanly, not a workaround |
| Last synchronized | `2026-08-31` |

- **Type:** platform feature (design only at filing time, no code changed
  yet). Filed at the user's explicit request immediately after the design
  converged, to record the full negotiated shape before implementation
  starts.
- **Origin:** the user asked directly whether AgentWorks has an admin-vs-
  read-only user model. It does not: authentication supports multiple users
  (`AUTH_USERS=user:pass,...`, or full multi-user mode via Cognito/Supabase),
  but there is no authorization boundary once someone is logged in — every
  authenticated identity can do everything today. The user wants a second,
  read-only user: same chat/UI experience, but everything is read-only.

## Design constraints discovered during the conversation

1. **A read-only permission tier already existed once and was deliberately
   removed** (`cmd/server/workflow_permissions.go` lines 14-24, removed
   2026-08-17): it was never actually configured in any deployment (every
   caller resolved to `owner`, since `WORKFLOW_READ_USERS` /
   `WORKFLOW_WRITE_USERS` / `WORKFLOW_OWNER_USERS` /
   `WORKFLOW_USER_PERMISSIONS` were never set), and its removal also cleaned
   up "a query-access gate full of unreachable checks against workshop modes
   that no longer exist," which cost real debugging time (PLAT-125). This
   plan restores the tier but wires it to concrete, currently-real
   enforcement points instead of the old dead workshop-mode gate — the
   mistake to avoid repeating is adding a permission check that nothing
   actually reads.
2. **"Run mode" is currently prompt-only, not enforced.**
   `WorkflowInteractiveWorkshopAgent.Execute`
   (`pkg/orchestrator/agents/workflow/step_based_workflow/interactive_workshop_manager.go`)
   calls `registerFullWorkshopAgentTools` unconditionally regardless of the
   `WorkshopMode` template variable — a Run-mode session today has every
   mutating tool available; the system prompt just asks the agent not to use
   them. The read-only user's whole safety property depends on Run mode
   becoming a real, enforced boundary, so this ticket makes that real as
   part of building the role, not as a side effect.
3. **Shell (`execute_shell_command`) is already folder-guarded, not
   unrestricted** — `createInteractiveWorkshopAgent` already scopes it via
   `SetWorkspacePathForFolderGuard`/`WrapWorkspaceToolsWithFolderGuard`.
   Today's write paths (`workshopWritePaths`) include `variables/`,
   `evaluation/`, `db/`, which already lets shell bypass several
   typed-tool-level restrictions (`update_variable`, `update_evaluation_plan`,
   `mutate_workflow_db`) by writing the underlying files/DB directly — so
   gating the typed tools alone would not actually close the loop. Run mode
   must also get its own narrower (here: empty) shell write-path list.
4. **`request_workflow_folder_access` never grants anything itself** — it
   only creates a pending request a human must explicitly approve via the
   desktop UI toolbar. Confirmed inert without that approval step, so it
   stays available in Run mode.

## Agreed design

**Superseded in one respect by the RCA below — kept here for the historical
record of what was first agreed, corrected there, not deleted.** The
corrected version: tool/shell restriction is keyed on `WorkflowAccessLevel`
(read vs write/owner), not on `WorkshopMode` (workshop vs run stays
prompt-only focus, exactly as today, freely available to every user
regardless of access level). Read that section for why.

- Read-only user **can chat** (not locked out of the chat surface — chosen
  explicitly over blocking chat entirely).
- ~~Read-only user is server-side forced into Run mode~~ — superseded; see
  RCA. `WorkshopMode` is not touched by this ticket at all.
- The mutating tool set becomes **genuinely restricted for a read-access
  identity**: everything that mutates plan structure, step config,
  variables, schedules, secrets, LLM config, or skills is excluded.
  Execution monitoring/triggering, read-only inspection, and human
  communication stay available regardless of access level. Full
  tool-by-tool classification recorded in the plan file used to build this
  (see Implementation below).
- Shell gets **zero write access for a read-access identity** (reads stay
  full — same broad read set as everyone else). Actual step execution runs
  through its own separately folder-guarded process regardless of the chat
  session's own shell grant.
- `request_workflow_folder_access` stays available regardless of access
  level (inert without human approval).
- `get_workflow_command_guidance` stays available; its existing per-`kind`
  `Modes` gating in `cmd/server/guidance/guidance.go`'s `allKinds` governs
  the focus axis only (workshop vs run) and is unrelated to this ticket's
  access-level axis — left as-is, not audited as part of this ticket.
- **No new user-creation mechanism needed.** Creating a read-only user is:
  add them via `AUTH_USERS` (or their normal login provider), then an owner
  grants them `read` access via the existing `WorkflowAccessPopup.tsx` UI
  (gets a third `'read'` option) or `WORKFLOW_USER_PERMISSIONS` /
  `PUT /workflow/user-permissions` — infrastructure that already exists and
  is already wired end to end for `write`/`owner`.

## Implementation (planned, not yet built) — corrected per RCA below

- `cmd/server/workflow_permissions.go`: restore `WorkflowAccessRead`, stop
  collapsing `read`/`reader`/`run`/`runner`/`view`/`viewer` aliases into
  `write`. (Done — landed ahead of this ticket's filing.)
- `cmd/server/server.go` / `cmd/server/workflow_phase_tools.go`: inside
  `installWorkflowPhaseTools` (already re-run every turn from
  `handleQuery`'s live request), resolve `workflowAccessForClaims(GetUserFromContext(ctx))`
  and pass the resulting access level down into tool registration.
  `WorkshopMode`/`SetWorkshopModeOverride` are untouched.
- `pkg/orchestrator/agents/workflow/step_based_workflow/interactive_workshop_manager.go`:
  gate the mutating subset of `registerFullWorkshopAgentTools`/
  `registerInteractiveWorkshopTools` (plan-mod tools + the ~41-tool
  Workshop-only subset) on access level, modeled on the existing
  `prepareReadOnlyBackgroundAgentTools`/`FilterCustomToolsByCategory`
  pattern rather than a new mode-branch; pass empty shell write paths to
  `SetWorkspacePathForFolderGuard` for a read-access identity, independent
  of `WorkshopMode`.
- `cmd/server/secrets_tools.go`: skip the four secret-mutation tools for a
  read-access identity; `list_secrets` stays.
- `frontend/src/components/workflow/WorkflowAccessPopup.tsx`: add the
  `'read'` option to the access-level dropdown.
- `cmd/server/guidance/guidance.go`: out of scope for this ticket (governs
  the focus axis, not access level — see RCA).

Full tool-by-tool RUN-safe/WORKSHOP-only classification, exact line numbers,
and the verification plan are recorded in the approved implementation plan
this ticket was filed alongside (not duplicated here — see the commits that
follow this filing for the executed version).

## RCA: why tool registration is gated by access level, not by WorkshopMode

Mid-implementation, re-examination found `docs/design/agent_tool_surface_single_source.md`
— a dedicated design doc recording a deliberate, largely-executed decision to
remove mode-based tool filtering from AgentWorks entirely, with three
documented production incidents it fixed (Video Studio secrets tools,
`list_llm_capabilities`, a Builder background-child mismatch). Its core
argument: a coding CLI caches its tool catalog once at launch; *removing* a
tool the agent already knows about degrades gracefully (it calls, gets an
error, adapts), but *adding* one after launch is invisible (the agent can't
ask for something it was never told exists, so it silently shells out
instead). `GetToolsForWorkshopMode` filtered the registered-tool catalog
itself by the `WorkshopMode` request parameter — a value the *same* live,
continuously-cached chat session could freely toggle turn to turn — and that
mismatch between what the CLI's cache believed existed and what was actually
registered on a later turn was the recurring bug. `installWorkflowPhaseTools`'s
own doc comment already states the fix landed: *"registers one complete tool
surface and applies no workshop-mode narrowing... Mode is a focus rule now
and lives in the Builder prompt."* Confirmed live: `GetToolsForWorkshopMode`
has zero callers today — dead code, not wired to anything.

This looked, at first read, like it invalidated the whole "restrict Run
mode's tool set" mechanism this ticket depends on. Two follow-up questions
resolved it instead of scrapping it:

1. **Does the doc's objection apply to a role-based split, not just a
   mode-toggle split?** No — traced to `prepareReadOnlyBackgroundAgentTools`
   / `runReviewPlanAgent` (`interactive_workshop_manager.go`), an
   already-shipped precedent that creates a *separate, dedicated* agent
   instance (its own `sessionID`, its own `configureWorkshopToolAgentSession`
   call) with a permanently reduced tool set decided once at that agent's
   creation (`orchestrator.FilterCustomToolsByCategory`, only
   `execute_shell_command` read-only-folder-guarded + `human_tools:*` +
   `query_workflow_db`). The design doc's own table names exactly this
   pattern as the one to **keep**: *"tool-agent registration...paired with
   writePaths — authority — Reader vs Writer — keep"*, distinct from the
   deleted *"focus — BUILD vs DEBUG"* gate. A read-only user's chat session
   and an admin's chat session never share a `sessionID` — the specific bug
   (one session's catalog drifting across its own turns) cannot occur
   between two people who never share a session in the first place.
2. **Could a user's access level change mid-session and leave a stale,
   over-privileged registration in place?** Traced the actual call path:
   `handleQuery` (`server.go:3045`, the `/api/query` HTTP handler, invoked
   once per message) derives `isWorkflowPhase := req.AgentMode ==
   "workflow_phase"` from *that request's own payload*
   (`server.go:3460`), and the tool-registration block containing
   `installWorkflowPhaseTools` runs inside that same per-request condition —
   confirmed by reading the handler top to bottom, not inferred from a
   comment. So `installWorkflowPhaseTools` re-runs, and re-resolves the
   caller's access level fresh from that turn's own JWT claims via
   `GetUserFromContext(ctx)`, on **every conversational turn**, not once at
   session creation. A demotion takes effect on the person's very next
   message; if the agent still believes an excluded tool exists from an
   earlier turn and tries to call it, the MCP bridge returns a normal
   "unknown tool" error — exactly the degrade-gracefully path the design doc
   accepts as safe, not the silent-shell-fallback failure mode it warns
   against. This closed the one gap that would have justified an extra
   per-executor live-recheck layer; that layer was dropped as unnecessary
   once this was confirmed, not merely simplified away.

**Net design correction from the original filing above:** tool-registration
exclusion is keyed on `WorkflowAccessLevel` (the authority axis — read vs
write vs owner), resolved fresh per turn from the live request's own claims —
**not** on `WorkshopMode` (the focus axis — workshop vs run), which stays
exactly as it is today: prompt-only, freely togglable by any user regardless
of role, per the design doc's own rule that conflating focus and authority
in code is "the same one-decision-two-places failure" the doc exists to
remove. Forcing `WorkshopMode="run"` server-side for read-only users — part
of the original design above — is dropped for the same reason: it would
smuggle an authority decision through a variable the codebase has already
deliberately committed to treating as advisory-only. The shell folder-guard
write-path restriction is gated the same way: on access level, not on
`WorkshopMode`.

## Verification

Implementation complete. Results:

- `go build ./...`: clean (only the pre-existing, unrelated
  `libonnxruntime.dylib` linker version warning).
- `gofmt -l` on every touched Go file: clean.
- `go vet ./cmd/server/ ./pkg/orchestrator/agents/workflow/step_based_workflow/...`:
  clean except two pre-existing, unrelated findings
  (`message_sequence_stop_test.go` context-leak lint, `scheduler_test.go`
  unreachable code) — neither touched by this ticket.
- `go test ./cmd/server/ ./pkg/orchestrator/agents/workflow/step_based_workflow/...`:
  all pass. `cmd/server/virtual-tools` fails at `go vet`/`go test` setup on a
  pre-existing missing-module import (`mcpagent/agentreview`), unrelated to
  this ticket, present before this work started.
- New Go tests:
  - `cmd/server/workflow_permissions_test.go`: `read`/`reader`/`run`/
    `runner`/`view`/`viewer` aliases resolve to `WorkflowAccessRead` (not
    `WorkflowAccessWrite`); `write`/`owner` aliases unaffected;
    `workflowPermissionInfo(WorkflowAccessRead)` yields
    `CanRunWorkflows=true`, `CanWriteWorkflows=false`,
    `CanManageWorkflowAccess=false`.
  - `pkg/orchestrator/agents/workflow/step_based_workflow/workshop_readonly_gate_test.go`:
    an AST-based test (matching this file's existing
    `workshop_tool_allowlist_test.go` pattern, since
    `registerInteractiveWorkshopTools` has too many construction-time
    dependencies to invoke with fakes) proving all 18 mutating tools
    (`debug_step`, `update_step_config`, `review_plan`, `review_step_code`,
    `update_variable`, `add_group`, `update_group`, `delete_group`,
    `update_workflow_config`, `set_workflow_contract_version`,
    `create_schedule`, `create_calendar_schedule`, `update_schedule`,
    `delete_schedule`, `import_skill`, `uninstall_skill`, `install_skill`,
    `set_workflow_llm_config`) are registered only inside an
    `if iwm.readOnlyAccess {...} else if err := mcpAgent.RegisterCustomTool(...)`
    guard, and that `execute_step`/`query_step`/`run_full_workflow`/
    `list_secrets` are never gated; plus a direct test proving
    `runBackgroundTaskAgentSequence`'s write-path narrowing zeroes shell
    write grants under `readOnlyAccess` while `workshopWritePaths` itself
    still returns a non-empty list for a normal caller (guards against the
    test passing vacuously).
- Frontend: `npx tsc --noEmit -p .` clean; `npm run build` clean (pre-existing
  bundle-size warning only, not a failure).

Enforcement points implemented, all keyed on `WorkflowAccessLevel` resolved
fresh every turn, `WorkshopMode` untouched per the RCA above:
- `RegisterPlanModificationTools` (both call sites in
  `installWorkflowPhaseTools`, plus inside `registerFullWorkshopAgentTools`
  for background sub-agents) — skipped entirely for read-only access.
- The 18 Workshop-only tools inside `registerInteractiveWorkshopTools`, plus
  `mark_changelog_artifact_reviewed` — skipped for read-only access.
- `registerSecretManagementTools` — `set_user_secret`, `delete_user_secret`,
  `set_workflow_secret`, `delete_workflow_secret` skipped for read-only
  access; `list_secrets` always registered.
- `reorganize_knowledgebase`/`consolidate_knowledgebase` — skipped for
  read-only access.
- Shell/file write paths: `server.go`'s main chat-session folder guard grant,
  and `runBackgroundTaskAgentSequence`'s background-agent folder guard, both
  pass empty write paths for read-only access; read paths stay full and
  unchanged in both cases.
- `WorkshopChatSession.readOnlyAccess` / `InteractiveWorkshopManager.readOnlyAccess`
  are re-set from the live caller's claims on every turn (both the
  session-reuse and fresh-session-creation paths in
  `installWorkflowPhaseTools`), matching the per-turn re-registration
  behavior the RCA relies on.
- `agent_profile_workflow.go`'s separate agent-profile-driven workflow
  runtime (which never registered plan-modification tools and already
  hardcoded `WorkshopMode="run"`) now also threads through
  `SetReadOnlyAccess` for defense in depth on its Workshop-only tool subset.

Also removed as dead code at the user's explicit request, found while tracing
this ticket's live tool-registration paths: `InteractiveWorkshopOnly`
(zero callers anywhere in the codebase), and everything reachable only from
it — `createInteractiveWorkshopAgent`, `enableWorkflowMainCodingAgentKeepAlive`,
`newWorkflowInteractiveWorkshopAgent`, the `WorkflowInteractiveWorkshopAgent`
struct and its `Execute` method, and the two now-orphaned unit tests that
covered `enableWorkflowMainCodingAgentKeepAlive` directly
(`controller_agent_factory_test.go`). `interactiveWorkshopSystemTemplate`/
`interactiveWorkshopUserTemplate` were kept — confirmed still live via
`planning_exports.go`'s `RegisterWorkshopChatTools`, the real path that
builds this same system prompt today. Initially misclassified
`registerFullWorkshopAgentTools`/`workshopWritePaths`/
`prepareBackgroundWorkshopToolDefinitions` as part of the same dead chain;
corrected before deleting anything after tracing them to the live
`run_in_background` background-sub-agent path
(`runBackgroundTaskAgentSequence`).

Not done in this pass (no live reverify environment available in this
session): creating a real read-only user end-to-end and confirming in a
running desktop app that chat works, a mutating tool call is rejected as
unregistered, shell reads but doesn't write, and a live demotion takes
effect on the very next turn of an already-open session. The static/unit
verification above exercises the same code paths a live check would, but
this is flagged as the one remaining manual-verification gap.

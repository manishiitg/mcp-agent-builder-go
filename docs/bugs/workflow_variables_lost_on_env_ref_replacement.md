# Bug: A workflow's variables vanish from the shell because the env map is replaced

## Status: Fixed 2026-08-05 (code merged, **needs a server restart to take effect**).

## Symptom

An agent working in `confida-qa-testing`, verifying a staging API bug, stopped
and said:

```text
Variables aren't injected in this shell — reading them from the group config instead.
```

It then worked around the platform by hand:

```bash
export VAR_SITE_URL=$(jq -r '.groups[]|select(.name=="confida-staging").values.SITE_URL' variables/variables.json)
```

The diagnosis was correct and the workaround was sound. `$VAR_*` genuinely was
not in the shell. The cost is invisible: every agent that hits this either
re-derives values from `variables/variables.json`, or hardcodes them — which is
exactly what the builder prompt tells it never to do.

## Evidence

The variables WERE loaded. From `server_debug.log`:

```text
12:16:15  [VARIABLES] Synced 28 variable values as VAR_* env vars:
          [VAR_SITE_URL VAR_LOGIN_EMAIL VAR_ADMIN_PASSWORD …]
12:19:29  🔗 Stored workspace env ref (keys: [MCP_SESSION_ID MCP_MCP MCP_CUSTOM …])
```

The second line has no `VAR_*` at all. Across the whole day's log:

| check | result |
|---|---|
| `[VARIABLES] Synced` fired | 17 times, 3 of them with the full 28-variable confida set |
| `VAR_SITE_URL` present in any `Stored workspace env ref` | **0 times, ever** |
| `SECRET_*` present in those same env refs | yes, consistently |

So variables were computed and written, and then disappeared, while secrets
sitting in the same map survived.

## Root cause

`BaseOrchestrator.SetWorkspaceEnvRef` does not merge into the existing env map.
It **replaces** it, and then restores what must survive the swap:

```go
bo.workspaceEnvRef = env                                   // whole map swapped
for _, secret := range bo.secrets {
    env["SECRET_"+secret.Name] = secret.Value              // secrets restored
}                                                          // VAR_* were not
```

`SyncVariablesToWorkspaceEnv` wrote `VAR_*` into whichever map was current at
the time. Any later `SetWorkspaceEnvRef` — and initialization performs several —
dropped them.

**The tell was already in the file.** The doc comment immediately above that
function describes this exact failure, for secrets:

> *"Backfill them here so initialization order cannot leave the builder shell
> without `SECRET_*` variables that are already attached to the workflow."*

Someone hit this, fixed it for secrets, and variables were never added to the
same backfill. One mechanism, two kinds of state, only one of them protected.

## Why confida-login showed it and others did not

Severity depends on how much a workflow loads and how early.

`controller_workshop.go:130` auto-loads the full variable set when a workflow has
exactly one group — confida-login has exactly one (`confida-staging`), so all 28
values were resolved at workshop start, giving the longest possible window for a
later env-ref store to wipe them.

Workflows that sync only a couple of framework variables late (the sessions
logging `[VAR_USER_ID VAR_CREDENTIALS_SHEET_ID]`) lose less, and nobody notices.
The bug was equally present in both.

## Fix

Give the orchestrator the variables to restore, exactly as it already holds
secrets:

- `BaseOrchestrator.workspaceVars`, guarded by the existing `workspaceEnvMu`.
- `SetWorkspaceVariables(values)` records them. It **merges** rather than
  replaces, so a later group-scoped load adds to what is known instead of
  dropping what an earlier one resolved. Values are copied, so a caller mutating
  its own map afterwards cannot change what gets backfilled.
- `SetWorkspaceEnvRef` backfills `VAR_*` in the same place it backfills
  `SECRET_*`.
- `SyncVariablesToWorkspaceEnv` records on the orchestrator **before** touching
  the live map.

That last ordering change fixes a second case that was never reported:
variables resolved before any env map exists were previously discarded outright.
They are now applied to the first map that appears.

## Tests

`pkg/orchestrator/workspace_vars_backfill_test.go` — four cases: survives a map
replacement, applies when recorded before any map exists, merges across repeated
loads, and secrets still backfill alongside.

Confirmed the tests catch the defect rather than merely describing it: removing
only the backfill loop makes two of them fail with
`VAR_SITE_URL after first store = ""`.

## Notes

- **Requires a server restart.** The running process keeps the old behavior, so
  a session started before the restart still has no variables.
- Two tests in `step_based_workflow` (`TestWorkshopModeIsMergedSuperset`,
  `TestWorkshopCLIPromptUsesProjectedWorkspaceToolReference`) fail both with and
  without this change — verified by stashing. They arrived with unrelated
  upstream commits; `run_goal_advisor_review` is missing from the workshop
  prompt. Not addressed here.
- The shape is this archive's recurring one: **one fact, two sources, and
  nothing checking they agree.** Here the two sources are the live env map and
  the orchestrator state that can rebuild it, and only half the state was
  registered for rebuilding.

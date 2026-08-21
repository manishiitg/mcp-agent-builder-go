[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-169 — Folder Guard authorization was registered before platform-owned workspace paths existed

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `deployed; live workflow reverify pending` — shared lifecycle policy and fresh-workspace tests pass; release `7d7d4080d-20260821155314` is healthy on the Linux host |
| Last synchronized | `2026-08-21` |

- **Priority:** P1 — on a fresh project, one missing platform-owned directory
  makes every guarded shell and browser call fail before a workflow step can
  read its inputs or create outputs.
- **Owner:** `agent_go/pkg/workspacepathpolicy` and the shared workflow agent
  bootstrap in `step_based_workflow`.
- **Related:** [PLAT-078](plat-078.md), [PLAT-118](plat-118.md), and
  [PLAT-124](plat-124.md).

## Live evidence

On 2026-08-21, Video Studio's `shortform-script` message-sequence step failed
while its required `shortform-brief.md` input was present. Both
`execute_shell_command` and `agent_browser` reported:

```
SANDBOX_UNAVAILABLE (missing tool_output_folder)
```

All three workflow Folder Guard builders granted the project-level
`tool_output_folder`, but the directory was absent. The bridge previously
created that spill directory lazily, after a large tool result occurred.
Landlock canonicalizes every granted path before starting the command, so the
first command could never run and therefore could never trigger lazy creation.

The immediate PLAT-124 repair creates this one directory in `mcpagent`. That is
necessary defense-in-depth at the component that owns the spill, but it does
not solve the generic platform defect: every newly granted platform-owned
folder could repeat the same failure.

## Root cause

The system had two separate contracts with no common lifecycle boundary:

1. workflow code assembled strings representing authorized read/write paths;
2. unrelated code sometimes created the corresponding directories.

The three principal workflow guard builders — execution, message sequence, and
KB maintenance — also assembled their policies independently. Tests checked
that `tool_output_folder` appeared in each list, but no test started from an
empty workspace and proved every platform-owned granted directory existed
before Linux compiled the sandbox policy.

Authorization does not imply existence. Treating the two as equivalent made
old projects work by history while fresh projects failed deterministically.

## Platform contract

Workspace paths granted to a sandbox have an explicit lifecycle:

- **required** — user or upstream input; missing is a clear startup error;
- **platform-managed** — directory is safely materialized before guard
  registration;
- **optional** — missing is valid and the inactive grant is omitted.

All declared paths resolve below the workspace docs root. Traversal and symlink
escapes fail closed. Platform-managed paths are directories only and default to
mode `0700`; the policy cannot manufacture an empty file that masquerades as a
required input.

## Implementation

- Added reusable `agent_go/pkg/workspacepathpolicy` with lifecycle and kind
  declarations, safe root resolution, symlink-escape checks, managed-directory
  creation, required-path validation, and optional-path omission.
- Added one shared `materializeWorkflowGuardPaths` boundary. It declares all
  workflow write grants and the bridge's read-only `tool_output_folder` as
  platform-managed directories.
- Applied that boundary before registering guards for regular execution,
  message-sequence execution, KB consolidate/reorganize agents, and direct
  scripted execution. The scripted path is the route that reproduced the live
  Video Studio failure.
- Kept `mcpagent`'s spill-directory creation from PLAT-124 as
  defense-in-depth. Either component now fails with a contextual setup error
  instead of allowing a later opaque sandbox failure.

The policy package is product-neutral. Video Studio, Domain AI, and future
products using the shared workflow runtime inherit it without product-specific
installation code. A future runtime-owned read-only directory must be declared
`PlatformManaged` at this boundary; it must not be added as a bare guard string.

## Tests

- managed directories are created from an empty root;
- required missing paths fail and optional missing paths are omitted;
- `..` traversal and symlink escapes are rejected;
- platform-managed files are rejected;
- fresh workspaces materialize the managed paths from all three workflow guard
  builders;
- an escaping workflow write grant fails closed.

## Acceptance

- A fresh workspace can execute its first guarded shell/browser command without
  relying on a previous run to create directories.
- All platform-owned write grants exist before the session Folder Guard is
  registered.
- `tool_output_folder` exists for execution, message-sequence, KB maintenance,
  and direct scripted execution paths.
- Adding a new platform-owned directory requires a lifecycle declaration and is
  covered by the shared materializer, not product-specific setup.
- Traversal, symlink escape, and failed directory creation stop agent startup
  with a contextual error.
- The smallest real Linux workflow producer passes from a newly created
  project; unit tests alone do not close the ticket.

## Deployment evidence

Deployed rootlessly on 2026-08-21 as release
`7d7d4080d-20260821155314`. Agent, workspace, and gateway services were active
with zero restarts. The agent and workspace health probes passed, the workspace
reported Landlock filesystem ABI 8 with launcher preflight successful, and the
deployed agent binary contained the shared materialization boundary. The public
login returned HTTP 200 and the unauthenticated application root correctly
returned HTTP 303 to `/login`. Retry the smallest failed Video Studio step to
complete live acceptance.

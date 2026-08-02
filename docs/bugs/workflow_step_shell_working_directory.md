# Bug Report: A Workflow Step's Shell Runs in the Workspace Root, Not Its Run Folder

## Status

Fixed 2026-08-02. Root cause confirmed from the parent, group, and dedicated
step session identities in the server logs. Dedicated workflow-step sessions
now receive their run execution cwd directly, and the shell bridge refuses a
workflow-step request with no cwd instead of falling back to workspace root.

## Symptom

A CDP-test message-sequence step ran this and got the workspace root:

```text
$ pwd
<workspace-docs>
```

It then spent four tool calls orienting, and hit folder-guard denials:

```text
find: Workflow/social-media: Operation not permitted
ls:   .../Workflow/social-media/runs/iteration-0/default: Operation not permitted
ls:   .../runs/iteration-0/default/inputs: No such file or directory
sed:  .../learnings/_global/references/browser-usage.md: No such file or directory
```

Some returned `exit_code: 0` because pipelines such as `find ... 2>/dev/null |
sort` report the last command's status and may suppress the failing command's
stderr. `find` and `ls` themselves normally return non-zero on permission
errors. The UI stderr improvement remains useful when stderr is not redirected.

## The two-directory architecture (verified, and not itself the bug)

An agent in a workflow step has **two unrelated working directories**.

**1. The coding CLI process** runs in an isolated temp dir:

```text
/var/folders/.../T/mlp-cli-session-<sha256(sessionID)[:8]>
```

Set by `IsolateCodingAgentWorkspace` (`controller_agent_factory.go:770`, `:1540`;
`base_orchestrator_agent.go:152` — *"Run coding-CLI in a fresh tmp dir (workflow
steps only)"*). The rationale is documented at
`interactive_workshop_manager.go:10316`: without it, a background CLI session
collides with the workshop chat's session on the same directory with a different
MCP config, and agy-cli fails with *"does not support concurrent sessions"*.

**2. Every command the agent runs** executes server-side over HTTP:

```text
workspace/server.go:120        api.POST("/execute", …, handlers.ExecuteShellCommand)
workspace/handlers/shell.go:25   func ExecuteShellCommand(c *gin.Context)
workspace/handlers/shell.go:159  cmd.Dir = workingDir
```

`execute_shell_command` is not a local shell. It is an HTTP POST to the workspace
server, which runs the command in *its own* chosen directory. And the CLI has no
local shell at all — codex is launched with `--disable shell_tool
--disable unified_exec`, so **every** command crosses the bridge.

The consequence, which is easy to miss: bridge shell commands cannot execute in
or report the isolated CLI temp dir. Native CLI skill loading can still consume
skills projected there. `read_skill` (a bridge tool, see
`mcpagent/agent/skill.go:90`) is the transport-neutral path and is required for
reliably loading the same skill bundle through the bridge; a bridge shell cannot
navigate to the projected temp files.

None of the above is the bug. It is the context the bug hides in.

## The bug

The server-side working directory was supposed to be the run's execution folder.
`controller.go:791`:

```go
if hcpo.httpSessionID != "" && hcpo.GetWorkspacePath() != "" {
    common.SetSessionWorkingDir(hcpo.httpSessionID,
        fmt.Sprintf("%s/runs/%s/execution", hcpo.GetWorkspacePath(), selectedRunFolder))
}
```

Resolution order, `pkg/workspace/execute_shell_command.go:277`:

```text
param > session config > client field > ExtraEnv > empty (workspace root)
```

The observed `pwd` is the **last** entry. So the session-config lookup missed and
resolution fell through to the workspace root.

That single fact explains the rest of the session. From
`runs/iteration-0/default/execution`, the step's own outputs are `./step-0-cdp-test/`
and it never needs to walk the workflow tree. From `workspace-docs`, the only way
to locate anything is to traverse down from the workflow root — which the folder
guard correctly denies, because a step is granted `runs/<run>/execution` and a few
named siblings, never the workflow root.

So the denials are a *symptom*: the agent was orienting from the wrong root
because `pwd` told it the wrong root.

## Confirmed root cause

The runtime logs show the complete identity break:

1. The correct execution cwd was stored on parent HTTP session
   `4ceacd72-…`.
2. Group session `session-group-default-…` copied the parent's folder guard but
   not its working directory.
3. The message-sequence shell request used dedicated session
   `msgseq-iteration-0-default-step-1-step-0-cdp-test`.
4. That session received its guard and environment but no cwd.
5. The workspace request therefore arrived with `working_dir=""` and executed
   with `WorkDir=<workspace-docs>`.

This was not a race or premature cleanup. The cwd was recorded under one
identity while the shell lookup used another, and the intermediate group did
not carry the value.

## Why it matters

Every failure in the observed session is downstream of this:

- four tool calls spent orienting, all denied;
- a reference doc searched for on the filesystem under a path derived from the
  wrong root;
- the appearance, in the UI, of a clean run.

It also silently widens what a step touches. Workspace-root-relative commands
from a step that believes it is at the workspace root will resolve differently
from the same commands issued from the execution folder, so a step's behaviour
depends on which fallback it landed on.

## Related changes already made (do not re-derive)

- `controller_agent_factory.go:432` — read grant widened from
  `runs/<run>/execution` to include `runs/<run>`, so a step can list its own run
  folder. This was made before the `pwd` finding and is **treating a symptom**;
  it is defensible on its own (a step reading its own `logs/` and
  `run_metadata.json` is ordinary) but should be re-examined if the working
  directory is fixed.
- `frontend/src/utils/toolCallFormatting.ts` — stderr permission denials now
  render as errors despite `exit_code: 0`. Independent of this bug and worth
  keeping regardless.
- `pkg/skills/builtin_browser_skills.go` — the `agent-browser` skill description
  now names `read_skill(...)` instead of leaving the agent to guess a filesystem
  path.

## Fix implemented

- `configureSubAgentSessionGuard` derives
  `Workflow/<name>/runs/<run>/execution` from the orchestrator's own execution
  context and writes it directly onto the dedicated child session. It does not
  depend on the parent HTTP/group session identity or creation order.
- The same path covers regular execution, message-sequence, and todo-task
  sessions.
- `execute_shell_command` now fails closed when a workflow-step request carries
  step ownership (`STEP_OUTPUT_DIR` or `RUNLOOP_STEP_ID`) but no cwd resolves.
  Interactive non-workflow callers retain the workspace-root default.
- Regression coverage verifies all three child-session kinds, the
  session-aware MCP bridge request, the missing-cwd refusal, and explicit-cwd
  compatibility.

## Suggested acceptance

- A workflow step's bridge request carries its run execution folder as
  `working_directory`, so `pwd` reports that folder when run by the workspace
  server.
- A step can locate its own output directory without traversing from the
  workflow root.
- The workspace-root fallback is rejected for workflow steps with a structured
  configuration error, while remaining available for non-workflow callers.

## Verification completed

Verified 2026-08-02 against the implemented code:

- dedicated regular-execution, message-sequence, and todo sessions all store
  `Workflow/<name>/runs/<run>/execution` as their own session cwd;
- a session-aware workspace bridge request forwards that value as
  `working_directory` rather than relying on the parent session;
- a workflow-step shell request with no resolved cwd is rejected before an HTTP
  execution request can fall back to the workspace root;
- a workflow-step request with an explicit cwd continues through the normal
  execution path;
- focused tests pass under the race detector;
- `go vet` passes for the workspace and step-based workflow packages; and
- the complete `go test ./... -count=1` suite passes, including the concurrently
  updated `read_skill` coverage.

The regression tests live in:

- `agent_go/pkg/orchestrator/agents/workflow/step_based_workflow/workflow_step_working_directory_test.go`
- `agent_go/pkg/workspace/execute_shell_working_directory_test.go`

## Related

`docs/bugs/custom_tool_category_as_agent_addressing.md` — same family: the agent
is told one thing and the enforced reality is another, and the discrepancy is
only discoverable by burning tool calls.

# `diff_patch_workspace_file` waited forever on a hidden `/dev/tty` prompt

**Status:** Fixed, regression-tested, and deployed to the local development
stack on 2026-08-02.
**Repositories:** `mcp-agent-builder-go` (`workspace`, `agent_go/pkg/workspace`)
**Original evidence:** `agent_go/logs/server_debug.log`,
`agent_go/logs/workspace_debug.log`, run of 2026-08-02 14:33–14:41
(`upwork` / `job-search` / `check-cdp`)

## Symptom

Three `diff_patch_workspace_file` calls against one file ran for minutes and
eventually returned this error to the agent:

```text
failed to call workspace API: Patch "http://127.0.0.1:18744/api/documents/
Workflow/upwork/runs/iteration-0/job-search/execution/check-cdp/
cdp_status.json/diff": context canceled
```

| tool call | started | ended | duration |
|---|---|---|---|
| 1 | 14:33:53 | 14:36:28 | 2m34.96s |
| 2 | 14:39:00 | 14:41:31 | 2m30.96s |
| 3 | 14:37:12 | 14:41:31 | 4m18.90s |

A healthy `PATCH .../diff` normally completes in milliseconds. Calls 2 and 3
ended at the same instant because their shared parent turn was torn down. The
reported `context canceled` was therefore a teardown artifact, not the reason
the subprocess had stopped making progress.

The intended file content existed only because the agent gave up on the patch
tool and wrote it through shell redirection. The patch requests themselves never
completed.

## Confirmed root cause

The handler did two things in combination:

1. When a target did not exist, it created an empty target file first.
2. It then passed a creation diff whose old side was `--- /dev/null` to BSD
   `patch(1)` without a non-interactive flag or subprocess context.

BSD `patch` interprets that as a suspicious creation of a file that already
exists. When the workspace server owns a controlling terminal, `patch` opens
`/dev/tty` directly and asks whether the patch should be reversed. Redirecting
stdin does not help because the prompt does not use file descriptor 0.

The live process inspection confirmed this rather than inferring it:

- Nine orphaned `patch` children were still blocked beneath the workspace
  server before deployment.
- Every child had an open descriptor to `/dev/tty` while stdin was
  `/dev/null`.
- Every blocked command corresponded to a `/dev/null` creation diff.

This also explains why an ordinary local shell probe originally appeared to
disprove the prompt hypothesis: that probe had no controlling terminal, so BSD
`patch` consumed defaults instead of blocking. The production launch path did
have a controlling terminal.

The blocking call was the unbounded subprocess in
`workspace/handlers/diff_patch.go`:

```go
cmd := exec.Command("patch", "-u", "-F", "0", tempFile.Name(), patchFile.Name())
output, err := cmd.CombinedOutput()
```

The request could be canceled on the client while the server goroutine and
child process remained alive. The agent therefore saw a cancellation, retried,
and accumulated more blocked children.

## Deterministic regression reproduction

`workspace/diff_patch_tty_e2e_test.go` is a real HTTP end-to-end reproducer. It:

- builds the actual workspace server;
- launches it under a PTY so it owns a controlling terminal;
- sends a real `PATCH /api/documents/.../diff` request with a `/dev/null`
  creation diff;
- enforces a two-second client deadline; and
- kills the isolated server process group during cleanup so a failing test
  cannot leak the blocked child.

Before the fix, the test failed deterministically after two seconds:

```text
creation diff did not return within 2s (possible interactive patch hang):
context deadline exceeded
```

That is the first bounded automated reproduction of the production failure.

## Implemented fix

The handler now has four defenses:

1. **Creation diffs do not invoke `patch`.** A strict `--- /dev/null` creation
   hunk is validated and assembled in Go. The target is written only after the
   diff succeeds; the handler no longer manufactures an empty-file conflict.
2. **Existing-file patches cannot prompt.** External `patch` runs with BSD/GNU
   portable `-f` (forward, no questions), empty stdin, and zero fuzz. `-t` was
   deliberately not used because batch mode assumes suspicious patches are
   reversed; that is not the desired write-tool contract.
3. **The subprocess is bounded and cancellable.** It uses
   `exec.CommandContext` with the HTTP request context and a five-second
   subprocess timeout. `WaitDelay` adds a final bound around inherited pipe
   shutdown.
4. **Cancellation cannot commit late.** The handler checks the patch context
   again immediately before writing the real workspace file. All subprocess
   work happens against temporary files.

Creation is also idempotent when the existing text is exact. Different bytes —
including semantically equivalent JSON with different formatting — return a
fast conflict and require an update diff instead of guessing.

## Verification

### Automated

- The exact PTY E2E failed before the fix and passed after it.
- Handler tests cover new-file creation, equivalent-JSON idempotency,
  conflicting existing content, cancellation of a deliberately hanging fake
  `patch`, and the guarantee that an invalid creation leaves no empty target.
- `agent_go/pkg/workspace` tests pass.
- Workspace and handler package tests pass.
- The workspace binary builds successfully.
- `git diff --check` passes.

The broader `workspace` `go test ./...` run reaches an existing unrelated
failure in `workspace/security/TestMacOSSandboxProfile`: the generated sandbox
profile contains a duplicate working-directory allow rule. No file changed for
this fix is in that package.

### Deployed local service

After restarting the local agent and workspace services with the patched code:

- both health endpoints returned HTTP 200;
- a real `/dev/null` creation request returned HTTP 200 in **0.8 ms** and wrote
  the expected JSON;
- a real existing-file update returned HTTP 200 in **3.2 ms** and wrote the
  expected update;
- no `patch` process existed before or after either request; and
- the disposable verification file was deleted through the workspace API.

The services remained healthy after the live verification.

## Follow-up: preserve the patch result exactly

The first real workflow after the hang fix exposed a second, independent
contract defect. A creation diff requested this exact one-line JSON:

```json
{"connected": true, "browser_version": "Chrome/unknown"}
```

The diff applied successfully, but `applyDiffPatchFlexibleContext` then passed
the result through `validateAndRepairJSON`. That helper pretty-printed valid
JSON and could reorder object keys. The workflow's byte-level verification
therefore failed, and the agent had to replace the file through shell output.

This post-processing has been removed from every successful diff-patch path.
The tool now writes the content produced by the patch rather than interpreting,
repairing, or formatting it. JSON formatting is not part of a patch operation.

The observed contract also required no trailing newline. The validator and
creation handler now support the standard unified-diff marker
`\ No newline at end of file`. The correction pass preserves this metadata
without counting it as content, and creation emits no final newline when the
marker follows the last addition. Creation idempotency requires exact existing
text; semantically equal JSON with different bytes is a conflict that requires
an update diff.

Two regressions pin the contract for both creation and existing-file updates.
Before the change they failed with multi-line JSON and reordered keys; after the
change they pass with the exact requested one-line content. The complete handler
suite, workspace-client suite, and controlling-TTY E2E also pass.

After the active workflow completed, the shared local services were restarted
with this follow-up. A live creation wrote the exact 56 requested bytes and a
live update wrote the exact 57 replacement bytes, both without trailing
newlines. Both returned HTTP 200 in milliseconds, left no `patch` child behind,
and the disposable file was removed through the workspace API.

## Related

- [custom_tool_category_as_agent_addressing.md](custom_tool_category_as_agent_addressing.md)
  and [tool_failures_invisible_in_backend_logs.md](tool_failures_invisible_in_backend_logs.md) —
  this incident was found because failed bridge calls now carry visible
  `[TOOL_ERROR]` evidence.
- The retry loop has the same general failure shape as the earlier
  `exit_code: 0` masking issue: the displayed result described teardown rather
  than the actual blocked operation, so the agent retried without knowing the
  outcome.

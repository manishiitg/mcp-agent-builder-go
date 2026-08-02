# Bug Report: Tool Failures Are Not Greppable in Backend Logs

## Status

Fixed 2026-08-02 in `mcpagent/executor/handlers.go`. The UI half was fixed
2026-08-01 in `frontend/src/utils/toolCallFormatting.ts`.

## Symptom

A tool that fails behind the HTTP bridge returns its error as **stdout with
`exit_code: 0`** — the curl succeeded, the tool did not. There was no way to see
these from either side:

- **UI** rendered every one with a green check.
- **Logs** recorded `Custom tool execution failed` with only the tool name — no
  session, no arguments, no duration, and no marker that could be grepped
  without matching unrelated prose.

A scan of one day of codex rollouts (2026-08-01) found **78 matching
failure-envelope occurrences** that were invisible in both places:

```text
46  virtual_tool_handler   get_api_spec
14  custom_tool_handler    mark_pulse_module_result
 8  custom_tool_handler    get_pulse_review_result
 6  custom_tool_handler    execute_shell_command
 4  custom_tool_handler    mark_human_input_consumed
```

`get_api_spec` alone was the single largest source of failed tool calls in the
system, and nobody knew, because a failure and a success looked identical
everywhere an operator would look.

The 14 `mark_pulse_module_result` failures are the ones that should worry a
reader: that is the Pulse Fixer's terminal write. A pass could look complete
while its result was rejected.

## Root cause

Two independent blind spots, one on each side of the bridge.

**Logs.** Failures were logged as:

```go
h.logger.Error("Custom tool execution failed", err, loggerv2.String("tool", req.Tool))
```

Enough to know something failed; not enough to find it, group it, or reproduce
it. No stable marker, no session, no arguments.

**UI.** `exit_code` was the only error signal, and it is 0 for a bridge failure
because the outer shell/curl transport succeeded. Permission denials were also
observed inside results carrying `exit_code: 0`, including commands wrapped by
pipelines where the final command determines the status. `find` and `ls`
themselves normally return non-zero for direct permission failures; the UI must
judge the returned envelope, not infer the underlying command's status.

## Fix

**Backend** — one greppable marker carrying everything needed to locate the call:

```go
h.logger.Error("[TOOL_ERROR] custom tool failed", err,
    loggerv2.String("layer", "custom_tool_handler"),
    loggerv2.String("tool", req.Tool),
    loggerv2.String("session_id", req.SessionID),
    loggerv2.String("duration", toolDuration.String()),
    loggerv2.String("args", truncateForLog(string(argsJSON), 400)))
```

The same marker on the virtual-tool path, which is where `get_api_spec` lives.
`grep '\[TOOL_ERROR\]'` is now the whole debugging story.

Arguments are bounded at 400 bytes by `truncateForLog`. They are the most useful
field for reproducing a failure and also the most likely to be enormous — a
prompt, a file body, a SQL result. Logging them whole turns one failure into an
unreadable log; logging none of them means the line says a tool failed without
saying what was asked of it.

**Frontend** — two detectors, deliberately narrow:

```ts
const HARNESS_TOOL_ERROR      = /tool execution (?:failed|canceled|timed out): layer=/
const SHELL_PERMISSION_DENIED = /(?:Operation not permitted|[Pp]ermission denied)/
```

The first matches on `layer=` rather than the word "error", so tool output that
merely *discusses* errors — a findings table, a log query — is not flagged. The
second is checked on **stderr only**, because a denial quoted in stdout is
usually a log being read. `No such file or directory` is deliberately excluded:
probing for a path that may not exist is ordinary, and flagging it would train
the operator to ignore the marker.

`jsonValueIsError` also recurses, because a bridge failure nests — the shell
result wraps an MCP envelope which wraps the failing tool's payload.

## Why this mattered more than it looks

Every investigation in `docs/bugs/` from 2026-08-01 onward was slower than it
needed to be because of this. The `get_api_spec` addressing bug ran at
46 failures a day for an unknown period; it was found by scanning codex rollout
JSONL by hand, not by reading a log or noticing a red UI element.

The general rule worth keeping: **a failure that is invisible is not a smaller
problem than a loud one, it is a larger one.** It survives longer, costs more
tool calls, and is diagnosed from the agent's confused behaviour rather than
from the error itself.

## Verification

Six frontend tests covering the harness envelope in all three forms
(`failed`/`canceled`/`timed out`), nesting inside an MCP envelope, stderr
denials, and two negative controls — output that mentions errors, and a missing
path. Verified honest: reverting each detector fails the matching tests.

Backend logging is not unit-tested. The implementation has been verified in
both custom- and virtual-tool handlers, including bounded arguments, session,
tool, layer, and duration. A post-rebuild runtime acceptance check remains:
trigger one known-safe failing call and confirm
`grep '\[TOOL_ERROR\]' <log>` returns it. No such marker was present in the
currently retained logs during the 2026-08-02 documentation review, which may
mean no post-rebuild failure has occurred; absence alone is not runtime proof.

## Related

`docs/bugs/custom_tool_category_as_agent_addressing.md` — the defect this
invisibility concealed.

# Bug Report: Codex Structured Stage Agents Hang Forever After Completing Their Work

## Status: Fixed ✅ (uncommitted as of 2026-08-01)

Cross-repo: symptom in `mcp-agent-builder-go` (Pulse), defect and fix in
`multi-llm-provider-go` (`pkg/adapters/codexcli`, `pkg/adapters/internal/procshutdown`).

## Symptoms

- Pulse reviewer and Fixer stages running on `codex-cli` would emit two opening lines
  and then appear frozen indefinitely. A `/pulse-fixer` stage sat for 65+ minutes.
- The parent agent narrated it accurately: *"The durable result was recorded; only the
  HTTP response remained stuck."* Its `call_generic_agent` bridge call never returned.
- Strongly provider-skewed: `claude-code` 77 ok / 2 timeout against `codex-cli` 2 / 13.
  This read for weeks as "codex is unstable."
- The work itself was **not** lost. Findings, attempts, and dispositions were already
  committed to SQLite before the hang.

## Root Cause

Teardown was reachable from exactly one place in
`codexcli_structured_adapter.go`:

```go
case "turn.completed":
    go procshutdown.GracefulAfterNaturalExit(cmd, scannerDone, 3*time.Second, c.logger)
```

If that event never arrived on stdout there was no fallback, no deadline, and no
diagnostic. The scan loop ended (or blocked), and the bare `cmd.Wait()` below it blocked
on a live process that nothing would ever signal.

Three separate gaps compounded:

1. **Single completion signal.** stdout was the only source of truth for "the turn ended."
2. **Unbounded reap.** `cmd.Wait()` had no deadline, so the failure mode was an infinite
   hang rather than a bounded, logged error.
3. **Silent scan failures.** `scanner.Err()` was never consulted, making an oversized line
   (>1 MB) or a stream error indistinguishable from a clean EOF.

The contract was complicit. `docs/coding_sdk_structured_contract.md` §9 specified teardown
"once a terminal-state event ... is observed **on stdout**", and left reaping to "the
adapter's main goroutine" with no bound. The adapter implemented §9 faithfully; §9 was
incomplete.

### Evidence

Child stage agent for module `eval_health`, social-media workflow, 2026-08-01:

| Observation | Value |
|---|---|
| Rollout | `~/.codex/sessions/2026/08/01/rollout-2026-08-01T15-42-51-019fbccf-*.jsonl` |
| `session_meta.cwd` | `/var/folders/.../T/mlp-cli-session-749a97315667d5e2` |
| Last rollout write | `event_msg/task_complete` at **10:20:02Z**, full report attached |
| Anything written after | none, for 65 minutes |
| Process (pid 56905) | alive 1h12m at **3.43s total CPU**, 0% at sampling |
| Teardown budget once a terminal event is seen | 3 + 10 + 10 + 5 = ~28s to SIGKILL |

`task_complete` is codex's own end-of-turn marker. Combined with 3.4s of CPU across 72
minutes, this rules out "the turn was merely slow": the work had finished and the process
was idle. And had the event reached us, procshutdown would have reaped it inside a minute.

The child was reaching tools normally throughout — `custom_tool_call_output` entries show
api-bridge calls returning in 25ms.

### Two diagnoses ruled out along the way

Both were wrong, and both are worth recording because they are easy to reach again:

- **"CLI stage agents have no MCP bridge"** (because `runGoalAdvisorStageAgent` sets
  `config.UseCodeExecutionMode = false`). Wrong: `mcpagent/agent/agent.go` force-enables
  code-exec for every CLI provider in two places (~1728 "Pre-set ... for CLI provider",
  ~1981 "safety net"), so that assignment is inert. The stale comment on
  `requiresCodeExecutionForProvider` — "Phase agents normally disable code-exec mode" —
  is what misled; the adjacent `isCliProviderForPrompt` comment is correct.
- **"`ServerNames = NoServers` leaves the agent unable to act."** Wrong: `NO_SERVERS`
  suppresses *external* MCP servers only. Workflow custom tools and the api-bridge remain
  fully available.

## The Fix

`pkg/adapters/internal/procshutdown/procshutdown.go`

- New `Reap(cmd, grace, logger)`: calls `cmd.Wait()`, escalating through the existing
  SIGTERM ladder if the process outlives its output stream. Placed here rather than in the
  codex adapter because all four structured adapters share the bare-`Wait` pattern.

`pkg/adapters/codexcli/codexcli_structured_adapter.go`

- Teardown moved behind a `sync.Once` so it is reachable from more than one signal.
- **Rollout fallback**: a watcher polls `codexTurnCompletionTracker` for native
  `task_complete` and triggers the same teardown. This tracker already existed and the
  *interactive* adapter has always used it; the structured adapter simply never did.
  Requires a working dir — the rollout is matched by `session_meta.cwd`.
- `scanner.Err()` is checked and forces teardown instead of being swallowed as EOF.
- `cmd.Wait()` replaced with `procshutdown.Reap(cmd, codexReapGrace, ...)`, 45s — above the
  full 28s ladder so an in-flight teardown always wins and this stays a last resort.
- If stdout stalls before the final `agent_message`, the answer is recovered via
  `readCodexTranscriptFinalAssistantText` rather than failing a turn whose work is durable.

None of this can cut a running turn short: the watcher fires only on `task_complete`, the
scanner check only on a real error, and the reap grace only starts after the output stream
has already ended.

`docs/coding_sdk_structured_contract.md` — §9 and certification row #22 updated to require
an independent completion signal and a bounded reap.

## Verification

- `TestReapKillsProcessThatOutlivesItsOutputStream` reproduces the defect with a
  SIGTERM-ignoring process. Confirmed honest: reverting `Reap` to a bare `cmd.Wait()` makes
  it fail on timeout; restored, it passes.
- `TestReapReturnsPromptlyForExitedProcess` guards against regressing the normal path into
  a fixed delay.
- Full `./pkg/adapters/...` suite green; `agent_go` builds.

**Not yet proven end-to-end.** The confirming evidence is a codex stage agent completing and
returning through the bridge on a rebuilt server. The tmux session exited mid-investigation,
so the live reproduction was lost before the fix could be tried against it.

## Open Items

- `claudecode`, `picli`, and `cursorcli` structured adapters still use a bare `cmd.Wait()`.
  Adopting `Reap` is one line each. None has been observed hanging.
- **Foreground stage agents have no stall detection at all.** The scheduler has a 10-minute
  idle detector; the chat path used by `/pulse-fixer` does not. That is why this ran 65
  minutes instead of failing, and it is independent of codex.
- The upstream reason `turn.completed` never reached stdout is still unknown. Candidates:
  the `codex-code-mode-host` grandchild holding the inherited stdout pipe open (no EOF), or
  a JSON line above the 1 MB scanner cap. The fix does not depend on which.

## Debugging Recipe

Codex rollouts live at `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`, selected by
`session_meta.cwd`. `event_msg/task_complete` is the definitive turn-over marker — its
timestamp against the process's elapsed time, plus **elapsed vs CPU time**, distinguishes
"hung" from "slow" in one step.

[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-179 — a retained (tmux-delivered) coding-agent turn is declared complete on the first non-empty assistant text, even explicit intermediate commentary, before a queued tool call runs

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `fixed` — root cause confirmed live, fix shipped in `multi-llm-provider-go`, build/test verified; live reverify pending |
| Last synchronized | `2026-08-22` |

- **Priority:** P0 — silently returns the wrong answer and ends the turn
  early, for every tmux-backed coding-agent provider using retained/live-steer
  delivery (claude-code, codex-cli, cursor-cli, agy-cli, pi-cli), not one
  provider. A caller has no way to tell a genuinely-final reply from a
  truncated one without independently re-verifying.
- **Owner:** `multi-llm-provider-go/pkg/adapters/picli/picli_retained_turn.go`
  (fixed here); the same shape needs auditing in the other three providers'
  `ReadRetainedTurnMessages` (`claudecodeadapter`, `codexcli`, `cursorcli`) —
  see Related Work below.
- **Related:** [PLAT-174](plat-174.md), [PLAT-175](plat-175.md),
  [PLAT-176](plat-176.md) (same investigation session — started as a request
  to validate pi-cli against OpenRouter, surfaced this along the way).

## How this surfaced

Testing whether `pi-cli` works correctly with an OpenRouter model
(`stealth/ox-alpha`), via `mcp-agent test coding-agent-chat-e2e
--provider pi-cli --retained-window-p0-only`, which exercises the IC-11
cross-repository retained-session contract.

The investigation took three wrong turns before finding the real cause,
recorded here so a future reader doesn't retrace them:

1. First hypothesis: OpenRouter-specific. Disproven — identical failure with
   the catalogued Gemini baseline model.
2. Second hypothesis: pi-cli chat uses "structured" (no-pane) transport, so
   the tmux-liveness check in IC-11 doesn't apply to it. Disproven —
   `[QUERY->LIVE] ... provider=pi-cli transport=tmux` in the server log is
   computed by mcpagent's own delivery code; pi-cli chat sessions genuinely
   use tmux, same as the other three providers.
3. Third hypothesis (after #2): pi-cli's tmux session isn't registering with
   `/api/terminals`, a real gap in event-driven terminal-store population.
   Disproven — `/api/terminals` was returning a deliberate `http.NotFound`
   from `runtimeDiagnosticsHandler` because `AGENTWORKS_RUNTIME_DEBUG` wasn't
   set on the local dev server. Not a bug; a missing local env var. Setting it
   made `assertRetainedTmuxLive` pass immediately.

With that noise cleared, the real failure was one step further in:

```
Error: IC-11 retained-window P0 failed: retained turn completed before
execute_shell_command started; intermediate commentary was treated as final
```

The IC-11 prompt asks for three things in order: (1) a brief progress update,
explicitly labeled *"intermediate commentary, not your final answer"*, (2)
one `execute_shell_command` call running `sleep 2; printf <token>`, (3) only
then the real final answer. The turn reported `"status":"completed"` after
**2.2 seconds** — not enough time for even the `sleep 2` to finish — with
`final_result` set to the progress-update text, not the real answer.

## Root cause

Traced through both repositories:

- `mcpagent/agent/turn_session.go`'s `startRetainedCompletionWatch` polls
  every 100ms and calls a reader function. The moment that reader returns any
  non-empty string, it calls `completeRetainedTurn` and emits a
  `canonical_turn_completion` event — unconditionally:
  ```go
  case <-ticker.C:
      finalResult := strings.TrimSpace(reader(provider, s.agent.sessionID, startedAt))
      if finalResult == "" {
          continue
      }
      s.completeRetainedTurn(lifecycle, seq, input, finalResult, provider, transport, startedAt)
      return
  ```
  This loop's own contract — "empty means not done yet, keep polling" — is
  sound. The defect is what the reader was willing to call non-empty.

- The reader is `multi-llm-provider-go/pkg/adapters/picli/picli_retained_turn.go`'s
  `ReadRetainedTurnMessages`. Before this fix, it parsed pi's own on-disk
  transcript (`~/.pi/agent/sessions/*.jsonl`) and returned the last assistant
  message's text the moment that message had *any* content — with no check
  for whether the overall exchange was actually finished. pi's interactive
  session can legitimately write a complete, well-formed assistant message
  record for intermediate commentary before continuing on to a tool call in
  the same live turn; the transcript alone cannot distinguish "one message
  finished" from "the whole exchange is over."

- The signal that *can* distinguish them already exists and is already
  trusted elsewhere: pi's own tmux status line reports "idle" when it is
  genuinely done. The ordinary, non-retained turn flow already uses exactly
  this (`piPaneReadyForInput`, `picli_interactive_adapter.go:1250`) to decide
  when a turn has really finished. The retained-turn reader was simply never
  consulting it — an omission, not a considered design choice; nothing in
  either file argues retained turns should be exempt from the same check.

## Fix

`ReadRetainedTurnMessages` now withholds its result — returns `nil`, which
`startRetainedCompletionWatch`'s existing loop already treats as "not done,
keep polling" — until a new `piRetainedTurnPaneReady` check confirms pi's
pane is genuinely idle:

```go
readyCtx, cancel := context.WithTimeout(context.Background(), piRetainedTurnPaneReadyTimeout)
defer cancel()
if !piRetainedTurnPaneReady(readyCtx, tmuxSessionName) {
    return nil
}
return summary.Messages
```

`piRetainedTurnPaneReady` reuses `piPaneReadyForInput` — the same check the
healthy turn path already trusts — rather than inventing a new signal. A
capture failure (tmux transiently unreachable) fails closed (treated as "not
ready"): an unconfirmed pane state is not evidence the turn is done, and the
100ms poll simply tries again next tick.

Deliberately did not touch `mcpagent`'s poll loop — its "empty means not
done" contract was already correct; the bug was entirely in what the reader
was willing to report as non-empty.

## Related work not done in this pass

- **The other three tmux-backed providers.** `ReadRetainedTurnMessages` exists
  separately for `claudecodeadapter`, `codexcli`, and `cursorcli`
  (`multi-llm-provider-go/pkg/adapters/{claudecode,codexcli,cursorcli}/`).
  Whether each already gates on an equivalent idle/ready signal, or shares
  this same gap, was not audited — pi-cli is the only one confirmed and fixed
  here, because it is the only one with live reproduction. Worth a dedicated
  pass given the severity.
- **A chat-mode E2E artifact for "structured" transport was written, then
  reverted.** While investigating hypothesis #2, a new
  `--structured-resume-p0-only` test mode was added to
  `cmd/testing/coding_agent_chat_e2e.go` (mcp-agent-builder-go), asserting
  `DeliveryTransport == "structured"`. Once #2 was disproven — chat sessions
  use `"tmux"` transport, not `"structured"` — that assertion could never
  legitimately pass through this harness (structured transport is a
  workflow-step-only path, `MetadataKeyStructuredTransport`, never reachable
  via `/api/query`). Reverted rather than left in the tree as a misleading,
  permanently-failing artifact. A real equivalent, if wanted, belongs in a
  workflow-step-level harness, not this chat E2E tool.

## Verification

- `TestReadRetainedTurnMessagesWithholdsResultUntilPaneIsIdle` — new. Fakes a
  registered pi interactive session and a transcript with non-empty assistant
  text, overrides `piRetainedTurnPaneReady` to report "busy," and asserts the
  reader returns nothing; flips the override to "idle" and asserts the same
  transcript now returns the message. Confirmed failing before the fix
  (`go build`: undefined symbol, via a scoped `git stash` of only the
  production file) and passing after.
- `TestReadRetainedTurnMessagesReturnsNothingWhenPaneCheckFails` — a pane
  check that can't confirm readiness must fail closed, not be treated as done.
- Full `pkg/adapters/picli` suite passes (42.8s, includes real-tmux tests).
- `go build ./...` clean in `multi-llm-provider-go`.

Not yet reverified live: the direct signal is re-running IC-11
(`--retained-window-p0-only`) against pi-cli end-to-end and confirming the
retained turn now waits for the actual tool call and final answer instead of
completing on the progress update.

## Acceptance

- [x] A retained pi-cli turn does not complete on intermediate commentary
      alone; it waits for pi's pane to report idle.
- [x] A pane-check failure fails closed (treated as not-yet-done), not open.
- [ ] Live: IC-11 passes end-to-end for pi-cli, with the tool call and real
      final answer both landing before the turn is marked complete.
- [ ] The same gap audited (and fixed if present) in claude-code, codex-cli,
      and cursor-cli's `ReadRetainedTurnMessages`.

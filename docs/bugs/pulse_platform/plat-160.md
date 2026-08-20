[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-160 — interactive tool-call completion is reconstructed by polling a file that can lose the final event at turn-end, when an already-proven synchronous signal exists and is only used as a fallback

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `filed` — consolidates a root cause and a fix direction that were each partially found across three other tickets; no code change in this ticket |
| Last synchronized | `2026-08-20` |

- **Priority:** P1 — this is the actual reason tool calls go missing for
  interactive (tmux) coding-CLI sessions at all. PLAT-141's settle/recovery
  path and PLAT-149's shared recovery both compensate for this after the
  fact; neither stops it from happening.
- **Owner:** `multi-llm-provider-go/pkg/adapters/{claudecode,codexcli,cursorcli}`
  interactive transcript tailers (the lossy signal), `agent_go/pkg/agentwrapper/llm_agent.go`
  + `mcpagent/executor/handlers.go`'s `toolcalllog` hook (the reliable signal,
  currently recovery-only)
- **Related:** [PLAT-141](plat-141.md) (settle + transcript recovery, the
  compensating fix), [PLAT-149](plat-149.md) (found and proved the reliable
  mechanism, used it for recovery only), [PLAT-152](plat-152.md) (closed as
  not-a-defect on its own claim, but reconfirmed live in production that the
  lossy watcher below actually runs)

## Why this ticket exists

Reported live: a batch of settled tool-call chips all displayed the identical
clock time despite having different, real, recovered durations — a
[PLAT-141](plat-141.md)-class symptom pointing at the same underlying gap that
ticket compensates for but doesn't close. Explaining *why* the backend loses
these calls in the first place surfaced that the explanation, and a concrete
fix direction, already existed — split across three tickets, never connected.

## The two mechanisms, and why one is lossy

**The lossy one.** For interactive/tmux coding-CLI sessions, tool-call
completion is reconstructed by periodically polling the CLI's own transcript
file on disk — e.g. `claudecode_transcript_stream.go`'s tailer, a
`time.NewTicker`-driven loop:

```go
for {
    events, err := tailer.Read(time.Now(), turnStart, pendingToolStarts)
    if err == nil {
        for _, e := range events {
            ...
            select {
            case streamChan <- chunk:
            case <-ctx.Done():
                return   // <-- exits WITHOUT a final read
            }
        }
    }
    select {
    case <-ctx.Done():
        return           // <-- also exits WITHOUT a final read
    case <-ticker.C:
    }
}
```

If the turn's context is cancelled (normal completion, timeout, or a user
Stop) in the gap between one poll and the next, the loop returns immediately.
Anything the CLI wrote to its transcript in that gap — including a tool call
that finished just before teardown — is never read and never becomes an
event. This is confirmed current code, not a stale finding.

[PLAT-149](plat-149.md) noted this exact race as an aside while investigating
a different question, at a time when it believed this tailer was "opt-in...
and, as far as could be confirmed, only ever called by family-server, not
this platform" — i.e. not a live production exposure.
[PLAT-152](plat-152.md)'s investigation (closed as not-a-defect on its own
unrelated claim) incidentally disproved that belief: `enableStreaming`
auto-enables this exact tailer (`WithClaudeStreamTranscript` /
`WithCodexStreamTranscript` / `WithCursorStreamTranscript`) for ordinary
interactive sessions on this platform, confirmed via
`mcpagent/agent/coding_agent_integrations.go` and
`coding_agent_options.go`. So the race is live in production today, not a
theoretical opt-in path.

**The reliable one.** [PLAT-149](plat-149.md) separately proved a second,
independent mechanism exists: `agent_go/pkg/agentwrapper/llm_agent.go:1143`
registers a `toolcalllog.RegisterHook` whose `OnStart`/`OnEnd` fire from
`mcpagent/executor/handlers.go`'s `HandleCustomExecute` — the HTTP handler
every bridge tool call, on every provider, actually goes through to execute.
This is synchronous by construction: it is called at the exact moment a tool
starts and the exact moment it returns, not on a timer. Measured on one
production session: 1 of 36 calls unpaired via this mechanism, versus 59 of
605 (~10%) via the polling mechanism above.

## Why this hasn't already been fixed

Not for lack of a proof it works — [PLAT-149](plat-149.md) shipped it, but
only as a **fallback**: `cmd/server/tool_result_recovery.go` and
`tool_call_backfill.go` try `toolcalllog.Recover` after a call has already
gone unpaired, to patch up the display after the fact. It was never promoted
to be the **primary** live source the UI/Pulse evidence draws from while a
turn is still running. [PLAT-149](plat-149.md)'s own "what this does not do"
section says as much: "does not suppress the unreliable mechanism... needs
the site found first" — and the site (the polling tailer above) is now
found.

## What a real fix looks like (not implemented here)

1. Route live tool-call display through `toolcalllog`'s synchronous
   `OnStart`/`OnEnd` as the primary signal for interactive sessions, not only
   as post-hoc recovery.
2. Either stop constructing `ToolCallStartEvent`/`ToolCallEndEvent` from the
   polling transcript tailer for interactive sessions (removing the
   duplication PLAT-149 measured between the two mechanisms), or reconcile
   the two at a single point so a consumer sees one signal, not two
   competing ones.
3. Independently, the transcript tailer's own `ctx.Done()`-without-a-final-
   read shape is a real, fixable bug regardless of (1) and (2) — a `select`
   that tries one more non-blocking `tailer.Read()` before returning would
   close some of the gap even if the tailer keeps its current role.
4. `toolcalllog`'s ids (`toolu_<decimal>`, its own counter) and the
   transcript tailer's ids (the provider's real `toolu_...`/`call_...`
   opaque ids) are a disconnected id space — confirmed in
   [PLAT-149](plat-149.md). Any consolidation has to either unify identity
   or explicitly document that `toolcalllog` is keyed differently by design.

## What this ticket does not do

- No code change. This connects an already-proven root cause
  ([PLAT-149](plat-149.md)'s reliable mechanism, [PLAT-152](plat-152.md)'s
  confirmation the lossy one is live in production) to a fix direction that
  wasn't written down anywhere as an actionable plan.
- Does not re-litigate [PLAT-141](plat-141.md)'s settle/backfill mechanism,
  which stays correct and necessary as a safety net regardless of whether
  this ticket's fix ships — a synchronous primary signal reduces how often
  the fallback is needed, it doesn't make it unnecessary.
- Does not extend to Pi CLI, which [PLAT-149](plat-149.md)/[PLAT-152](plat-152.md)
  already found uses a different mechanism (an inline marker side-channel,
  not a polled transcript file) and so is not exposed to this specific race.

## Acceptance

- Live tool-call display for interactive sessions is driven by the
  synchronous `toolcalllog` signal, not the polling transcript tailer, or
  the two are reconciled to a single non-duplicating signal.
- The measured ~10% unpaired rate from the polling mechanism drops for
  whichever signal ends up primary — verified against a real session, not
  a synthetic one, per this register's standing discipline for this class
  of ticket.
- [PLAT-141](plat-141.md)'s settle/backfill path still exists as a backstop
  and its existing tests still pass, but fires measurably less often.

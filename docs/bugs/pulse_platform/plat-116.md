[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-116 — a completed CLI turn's own JSON completion signal doesn't reliably reach the platform; the blunt 10-minute idle timeout is the only real backstop, and it leaks the session

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `partially implemented` — the diagnostic + leak fix below shipped and is tested; the deeper root cause (why the bridge stalls in the first place) is still not pinned, deliberately deferred |
| Last synchronized | `2026-08-16` |

- **Priority:** P1 — silently turns real successes into false `error` schedule
  runs, and permanently leaks the session/goroutine so the UI keeps showing it
  as stuck long after the platform has already given up on it.
- **Owner:** coding-agent completion detection (`multi-llm-provider-go`
  interactive adapters) + the bridge from a provider's own turn completion to
  `textChan` closing (`agent_go/pkg/agentwrapper/llm_agent.go`,
  `cmd/server/server.go`) + the scheduler's safety net
  (`waitForConversationTurnTree`, `cmd/server/conversation_turn_lifecycle.go`)

## Why this ticket exists

A live incident on 2026-08-16: the `upgrade-periodic-pulse-review` contract
upgrade turn (the new rollout rung from [PLAT-115](plat-115.md)) ran against
social-media as schedule `5227790a-...`, session
`schedule-manual--5227790a_1786887536591212000`. The scheduler's own history
records it as `error`:

```
workshop turn 1/2 (upgrade-periodic-pulse-review) failed: workshop idle wait
timed out: execution query_1786887536605000000 made no progress for 10m0s
(running_children=0)
```

`started_at=13:38:56Z`, `completed_at=13:58:09Z` — a 19-minute run that ends
in a declared failure.

That failure is wrong. The turn actually succeeded:

- **Codex's own rollout JSONL proves it.** The candidate file resolved by
  working directory + tmux session name + timing
  (`~/.codex/sessions/2026/08/16/rollout-2026-08-16T19-09-01-...jsonl`)
  contains `event_msg`/`task_complete` at `13:40:56.495Z` — **two minutes**
  after the turn began, not ten — with `last_agent_message` reading
  *"Completed the migration and stamped workflow contract version
  1.0.26. Kept per-run monitoring..."*
- **The side effect it describes actually landed.** `workflow.json` on disk
  really does show `version: 1.0.26` — the migration genuinely ran and
  persisted, unaffected by what happened next.
- **The terminal really did go idle right after.** A UI long-poll of the tmux
  pane at `13:42:37Z→13:48:09Z` (`19:12:37→19:18:09` IST) returned 0 bytes
  changed over a 2m22s window — nothing was happening in that pane.

So the CLI-level, JSON-based completion signal this ticket's reporter asked
about by name — *"we have tmux streaming we have proper final message
response via codex json"* — exists, is real, and fired correctly. The gap is
entirely on the platform side: **something between that signal and the
platform's own turn-completion state never closes the loop**, so the only
thing that ever resolved this turn was the scheduler's blunt 10-minute
inactivity timeout (`waitForConversationTurnTree`,
`conversation_turn_lifecycle.go:301-307`) — a safety net never meant to be
the primary signal, doing exactly what a safety net does when the real
signal is silent: it waits, then declares failure, even though the work
underneath it had already succeeded 17+ minutes earlier.

## Traced call chain (confirmed by log evidence, not assumption)

```
scheduler.go: runPostRunMonitor → workshop turn (schedule-triggered chat query)
  → server.go:6272 "llmAgent.StreamWithEvents() started successfully"
  → server.go:6277 "Entering streaming loop" → for chunk := range textChan   [BLOCKS HERE]
      (agentwrapper/llm_agent.go:1069 StreamWithEvents — textChan only closes
       when the goroutine below returns)
  → mcpagent Session.Run (agent/turn_session.go:187)
  → continueAgentSessionWithHistory (agent/session_handle.go:124, native-resume path)
  → askWithHistory → GenerateContentWithRetry → llmInstance.GenerateContent
  → multi-llm-provider-go codexcli_adapter.go → generateContentInteractive
  → waitForCodexInteractiveResponse (codexcli_interactive_adapter.go:1814)
      — this loop DOES prioritize completionTracker.completed() over terminal
        text (lines 1897-1908) and, per the rollout evidence above, the event
        it is watching for really was written at 13:40:56Z.
```

No `[STREAMING_LIFECYCLE] StreamWithEvents completed` line, no
`[COMPLETION] Updating session ... completed`, no `[AGENT DEBUG] Query ...
completed successfully` — none of `server.go`'s own completion logging ever
fired for this session between the turn starting and the scheduler's timeout
19 minutes later. The blocking `for chunk := range textChan` at
`server.go:6278` genuinely never unblocked; the scheduler's 10-minute
watchdog is a completely separate mechanism (`LastProgressAt` on the tracked
execution tree) and has no way to reach into, or resolve, that stuck
goroutine — it can only give up waiting on it.

## Most likely mechanism (narrowed, not proven)

`codexTurnCompletionTracker.completed()`
(`codexcli_transcript_completion.go:65-105`) is the exact signal PLAT-108
already bound to session identity for the Codex interactive transport. Its
resolver, `codexRolloutResolverForSession`
(`codexcli_rollout_binding.go:111-132`), snapshots `threadID` /
`rolloutPath` / the set of rollouts other sessions have claimed **once**,
into a closure reused for the entire poll loop. For a turn whose rollout
file is brand new (this one was created ~5 seconds after the turn began, not
a pre-existing pinned thread), that closure falls through every single poll
to a live `findCodexRolloutByWorkingDirExcluding` directory scan.

Checked and ruled out one candidate explanation: the rollout's own
`session_meta.cwd` is `/Users/mipl/ai-work/mcp-agent-builder-go/workspace-docs/Workflow/social-media`
— it exactly matches the workflow's real working directory, so a naive
working-directory mismatch is not the cause. The remaining gap is inside
that live directory-scan/exclusion path or the thread-ID snapshot timing,
and pinning it further needs either debug instrumentation on a real
reproduction or careful review of `codexPersistentRegistry` state at the
exact moment this session's tracker snapshot was taken — deliberately not
pursued further in this pass (see "Deliberately deferred" below).

## This is not Codex-specific

The reporter's correction mid-investigation — *"also this should work for
all, not just codex"* — is the right framing and matches how PLAT-108
already treated this as a class, not a one-off. The vulnerable layer above
the provider-specific tracker is entirely provider-agnostic:

- `server.go`'s `for chunk := range textChan` has no timeout of its own; it
  blocks on whatever the provider layer eventually does.
- `mcpagent`'s `Session.Run` → `askWithHistory` → `GenerateContent` chain is
  identical for every CLI provider.
- The scheduler's `waitForConversationTurnTree` 10-minute safety net is
  provider-agnostic by construction — it has no idea which provider is
  running underneath it, which is exactly why it can only ever produce a
  generic "made no progress" failure instead of a real diagnosis.

Checked each interactive-capable adapter directly rather than assuming
parity from the provider contract table. First pass wrongly claimed Claude
Code has no JSON completion signal — corrected after review found
`waitForCompletedAssistantResponseFromTranscript`
(`claudecode_interactive_adapter.go:534`), which reads Claude's own JSONL
transcript for an assistant message committed with `stop_reason=end_turn`
and treats it as authoritative: it *overrides* whatever text the pane-scrape
found, and — stricter than Codex's version — explicitly fails the turn if
the transcript exists but shows no completed `end_turn`, rather than
silently trusting a possibly-stale pane scrape.

| Provider | Native JSON/transcript completion signal in interactive (tmux) mode? |
|---|---|
| Codex CLI | **Yes** — `task_complete` in the rollout JSONL, tracked as the primary signal inside the poll loop (`codexTurnCompletionTracker`, this ticket's reproduced incident) |
| Claude Code | **Yes** — `stop_reason=end_turn` in Claude's own JSONL transcript (`completedAssistantResponseFromTranscript`), read once the pane looks idle and treated as authoritative over the pane text; can fail the turn outright if the transcript disagrees |
| Pi CLI | **Yes** — an `agent_end` marker from Pi's own embedded JS harness (`picli_interactive_adapter.go:1473-1477`) is the *sole* completion signal for the wait loop — no terminal-text fallback at all |
| Cursor CLI | **No** in interactive/tmux mode — pure terminal-pane pattern matching (`waitForCursorInteractiveResponse`). Cursor's *default* transport is structured (one-shot `--json`), which sidesteps this whole class of problem by design; interactive/tmux mode is only reachable via an explicit per-profile transport override, so this is not a live exposure today |

So all three tmux-interactive providers actually in use (Codex, Claude Code,
Pi CLI) have a genuine JSON/transcript-based completion oracle, each bound
to session identity per PLAT-108. The exposure this ticket is about is
therefore fully shared, not Codex-specific: the shared bridge
(`textChan`/`Session.Run`) and the scheduler's blunt 10-minute safety net sit
*above* all three signals equally, provider-agnostic by construction — so
whatever the exact Codex-side miss turns out to be once traced further, the
same shared-layer gap can just as easily swallow Claude Code's `end_turn` or
Pi's `agent_end`. Cursor is the one provider genuinely unaffected today,
because its default transport never enters this code path at all.

This ticket's scope is that shared layer plus, for Codex specifically, the
one concretely reproduced tracker-miss.

## Consequence found live: the session leaks and never gets cleaned up

Checked directly while this incident was still visibly "stuck" in the UI,
after the scheduler had already recorded the terminal `error` result:

- `[ACTIVE_SESSION] Tracked active session: schedule-manual--5227790a_...`
  was logged exactly once, at turn start. No matching untrack ever appears.
- None of `CloseHTTPSession`, `removeSessionQueryID`, or
  `cleanupBrowserSessions` — all three of which server.go's own completion
  path (`server.go:4102-4110`, `~6600-6620`) runs on every real completion —
  ever ran for this session.
- The underlying tmux pane (`mlp-codex-cli-int-1786887540197594000-1f06f68e`)
  was independently confirmed gone (`tmux capture-pane` returns "can't find
  pane") several minutes after the scheduler's timeout fired, while the
  session was still being served as "active" to the UI.

This is the exact symptom the reporter flagged live: *"the schedule is still
stuck till now"*, observed several minutes after the backend had already
moved on. The scheduler's timeout only unblocks *its own* wait — it cannot
reach into and terminate the separate goroutine still parked on `for chunk
:= range textChan`, since that goroutine owns the session's active-tracking
state and is the only code path that clears it. The two problems compound:
not only is a successful turn recorded as a failure, the session it ran in
is never released, so it keeps presenting as "running" indefinitely — a
resource leak on every occurrence, not just a UI staleness artifact.

## What shipped: reuse the same reliable signals as a safety-net corroboration + leak fix

Both providers with a real completion signal reachable outside their live
session registry (Codex, Claude Code) now get a second, independent use of
that same signal, purely as a post-hoc diagnostic — never as a live
completion oracle, so this can never make a stuck turn falsely report
success:

- **`multi-llm-provider-go`** — two new exported, read-only functions, each
  a thin wrapper around the exact reader the interactive adapter already
  trusts as authoritative:
  - `codexcli.DiagnoseTurnCompletion(workingDir, since)` — scans the rollout
    for a `task_complete` event after `since` (`codexcli_turn_diagnostics.go`).
  - `claudecode.DiagnoseTurnCompletion(nativeSessionID, workingDir, since)` —
    wraps `completedAssistantResponseFromTranscript` for a committed
    `end_turn` after `since` (`claudecode_turn_diagnostics.go`).
  - Deliberately **not done for Pi CLI**: its marker file lives in a
    per-session temp directory only known to its internal live-session
    registry, with no OS-standard path an external caller can scan the way
    Codex's `~/.codex/sessions/...` or Claude Code's `~/.claude/projects/...`
    allow. Doing this for Pi needs exporting registry-level lookup first — a
    real, separate piece of work, not done here.
  - Also **not done for Cursor CLI**: confirmed not reachable in its default
    (structured) transport, so there is nothing live to corroborate today.

- **`agent_go`** — `cmd/server/conversation_turn_stall_diagnostics.go`, wired
  into `waitForConversationTurnTree`'s idle-timeout branch
  (`conversation_turn_lifecycle.go:301-308`). When the 10-minute safety net
  gives up on a turn, two things now happen instead of one:
  1. **Diagnosis**: `diagnoseStalledConversationTurn` looks up the session's
     live `*mcpagent.Agent` (already registered in `api.runningAgents` at
     turn start, so reachable even while the turn is stuck), snapshots its
     provider/working-dir/native-session-id via the already-exported
     `mcpagent.SnapshotAgentSession`, and dispatches to the right adapter's
     `DiagnoseTurnCompletion`. A hit enriches the timeout error with exactly
     what the CLI actually said and when — turning a bare "made no progress"
     into an immediately diagnosable "Codex/Claude Code completed at HH:MM:SS
     with: '...' — the platform's bridge never observed it (PLAT-116)",
     instead of requiring the manual rollout-file archaeology this ticket
     itself needed.
  2. **Cleanup, unconditionally**: `cleanupOrphanedConversationTurnSession`
     runs the same teardown the normal completion path runs —
     `updateSessionStatus(..., "error")`, `removeSessionQueryID`,
     `mcpagent.CloseHTTPSession`, `cleanupBrowserSessions` — regardless of
     whether the diagnosis found anything. This is what actually fixes the
     live "the schedule is still stuck till now" symptom: the session stops
     presenting as `"running"` in the active-session map the moment the
     safety net gives up, instead of leaking until the server process
     restarts. Every step is idempotent, so nothing breaks if the originally
     stuck goroutine ever does unblock and runs its own cleanup afterward.

## Formalized as a required P0 contract, not left as two one-off adapters

Raised directly: two adapters happening to have `DiagnoseTurnCompletion`
because I wrote them proves nothing about the *next* adapter, or about Pi
CLI staying without one indefinitely. `multi-llm-provider-go` already has a
mature capability-flag → required-certification framework
(`coding_agent_contract.go` + `coding_agent_certification.go`) — exactly the
mechanism PLAT-108's own unfinished "Layer 3 — certify it" plan called for
generally. Extended it rather than inventing something new:

- **`CodingAgentProviderContract.SupportsStalledTurnDiagnosis`** — a new
  capability flag, `true` only for `ProviderClaudeCode` and `ProviderCodexCLI`
  today (Pi CLI and Cursor's rationale for staying `false` is documented
  inline, same as the "not Codex-specific" section above).
- **`CertStalledTurnDiagnosis`** — a new certification ID, added to the
  `codingAgentCapabilityCertifications` gating table (so
  `TestAllCodingAgentCapabilityClaimsHaveRegisteredCertification` enforces
  every provider claiming the flag has a registered proof) and promoted to
  P0 wherever claimed, in both `CodingAgentCertificationPriorityForID` and
  `RequiredP0CodingAgentCertificationIDs` — the identical pattern already
  used for `CertStructuredStreaming`. A false "made no progress" timeout on
  a turn that actually succeeded is release-blocking, not a nice-to-have.
- **Real live E2E certs, not synthetic fixtures** —
  `codexcli_stalled_turn_diagnosis_live_test.go` and
  `claudecode_stalled_turn_diagnosis_live_test.go`: each runs one genuine
  CLI turn to completion (real tmux, real API), then calls
  `DiagnoseTurnCompletion` with **no turn in flight** and proves it
  independently rediscovers that exact turn's real completion text and
  timing. This is a materially different proof than the deterministic
  fixture tests already covering the parsing logic — it certifies the
  function against a genuine provider transcript, not a hand-written one.
  **Actually run live** (`-coding-cli-p0-live`) against real `codex` and
  `claude` CLIs on this machine, both passed (10.3s and 8.3s), both tmux
  sessions confirmed cleanly torn down afterward — not just written and
  assumed, per this codebase's own standing rule that this class of bug has
  repeatedly passed unit suites while broken.
- Every existing contract/certification drift test reverified passing
  after the change, including the strictest one
  (`TestActiveCodingAgentProvidersSatisfyP0Contract`, which independently
  checks `Priority == P0`, `RealE2E == true`, and the exact live-gate `Env`
  for every ID `RequiredP0CodingAgentCertificationIDs` returns).

## Deliberately still deferred, not fixed here

The deeper question — why the bridge (`textChan`/`Session.Run`) stalls in
the first place, rather than just diagnosing and cleaning up after it does —
is unchanged from the original investigation. Instrumenting
`codexRolloutResolverForSession`'s closure and `waitForCodexInteractiveResponse`'s
poll loop and reproducing live is still the next step if that's ever
pursued; this pass deliberately did not chase it further.

## Verification

- `codexcli_turn_diagnostics_test.go` / `claudecode_turn_diagnostics_test.go`
  (`multi-llm-provider-go`): finds a completion after `since`, ignores one
  before it, and reports not-found when no transcript/rollout exists at all.
- `conversation_turn_stall_diagnostics_test.go` (`agent_go`):
  `TestWaitForConversationTurnTreeIdleTimeoutRunsOrphanCleanup` — the direct
  regression guard, proving the idle-timeout path now leaves the session's
  active-tracking status as `"error"` and clears its query index, not stuck
  at `"running"` forever. **Verified failing before the wiring change and
  passing after** (temporarily reverted the one-line wiring, confirmed the
  test fails with the exact live symptom — status stays `"running"` —
  restored it, confirmed the test passes). Cleanup idempotency and the
  no-running-agent early-return are covered separately.
- Full existing suite reverified after the change: the only failures present
  (23, across `cmd/server/guidance` Pulse/prompt-content tests and one
  unrelated `schedule_execution_history_test.go` pair) are pre-existing and
  untouched by any file this change modified — zero new failures introduced.
- Not yet done: a live reverify on a real stuck schedule turn. This class of
  bug has, per PLAT-108's own history, repeatedly passed unit suites while
  broken — only a real tmux-driven run exercises the actual poll loop this
  fix reads from.

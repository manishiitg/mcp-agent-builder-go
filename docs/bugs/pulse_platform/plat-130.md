[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-130 — the schedule Stop button marks the run stopped but does not stop the work

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — root cause confirmed by tracing every layer in the cancellation chain; fix (locating and wiring the actual dispatch-site cancel func) not started |
| Last synchronized | `2026-08-17` |

- **Priority:** P1 — this is not cosmetic. A stopped schedule can continue
  executing side-effecting work (e.g. posting) after the UI, the run history,
  and the schedule's own recorded status all say it stopped. The user cannot
  trust Stop to mean stop.
- **Owner:** `cmd/server/workflow_execution_tracker.go`
  (`cancelTrackedExecutionsForSession`), `cmd/server/session_lifecycle.go`
  (`cancelBackgroundAgents`), and whatever dispatch site spawns the goroutine
  that actually drives a `run_full_workflow` execution — not yet located.

## How it surfaced

Reported live: clicking Stop on a running schedule interrupts the current
message, but the run then continues — the next queued
`message_sequence` item (e.g. a second social post) fires anyway, in the same
run, as if nothing had been stopped.

## Root cause, confirmed layer by layer

The frontend Stop button (`WorkflowScheduleRunsPanel.tsx`'s `handleStopRun`)
calls `POST /api/scheduler/jobs/{id}/stop` →
`stopScheduledJobHandler` → `SchedulerService.StopRunningJobForWorkflow` →
`stopRunningJob`, which does two things: cancels a per-run context
(`cancelScheduleRunContext`) and tears down session-level work
(`cancelSessionRuntimeWork` → `cancelBackgroundAgents` +
`cancelTrackedExecutionsForSession`, plus tmux/CLI teardown).

Each layer below this was traced and **confirmed correct** — not assumed:

1. **Scheduler → workshop-turn context.** `registerScheduleRunContext(runID)`
   creates a real `context.WithCancel`; `runJob`/`executeWorkshopJob` receive
   that exact context; `executeWorkshopJob`'s per-turn loop calls
   `waitForConversationTurnTree`, whose `select` includes
   `case <-ctx.Done(): return ctx.Err()`. Cancellation here correctly aborts
   the *outer* workshop-chat turn loop and produces the "stopped" status the
   UI and run history show.
2. **The `message_sequence` item loop** (`executeMessageSequenceStep`,
   `for _, item := range plannedItems`) stops on *any* non-nil error from
   `executeMessageSequenceItem` — including a wrapped `context.Canceled` — so
   if the context reaching this loop were canceled, it would correctly not
   proceed to the next item.
3. **The core per-turn LLM/tool-calling loop**, `askWithHistory`
   (`mcpagent/agent/conversation.go`), checks `agentCtx.Err() != nil` at the
   top of every turn and returns `fmt.Errorf("conversation cancelled: %w",
   agentCtx.Err())` — confirmed for both the plain API path and the
   coding-CLI continuation path (`continueAgentSessionWithHistory` calls the
   identical `askWithHistory`).

**The break is at the next layer down.**
`cancelTrackedExecutionsForSession` (`workflow_execution_tracker.go:359`):

```go
for _, exec := range api.trackedWorkflowExecutions {
    if exec == nil || exec.SessionID != sessionID || exec.Status != trackedExecutionStatusRunning {
        continue
    }
    exec.Status = trackedExecutionStatusCanceled
    exec.CompletedAt = &now
}
```

and `cancelBackgroundAgents` (`session_lifecycle.go:439`, `bgAgentRegistry.CancelAll(sessionID)`)
both **only mutate an in-memory status field in a registry.** Neither calls a
`context.CancelFunc`, neither signals a channel, neither touches the goroutine
that is actually driving the execution. The status flag is read by
*watchers* — `waitForConversationTurnTree`'s poller sees the flip and stops
waiting, which is why the *schedule's own* view correctly reports "stopped."

But the goroutine that actually runs a dispatched `run_full_workflow`
execution — the one driving `executeMessageSequenceStep`'s item-by-item
loop — is not one of these watchers. Nothing in its own call path reads
`trackedWorkflowExecutions[...].Status` or holds a context tied to this
cancellation. It has no way to learn it has been marked canceled, so it keeps
running exactly as if Stop had never been clicked.

**In short: marking a tracked execution canceled tells observers to stop
watching it. It does not tell the worker to stop working.**

## Why this matters more than it looks

The schedule's recorded status, the run history entry, and the UI are all
*correct and consistent with each other* — they all say "stopped." There is
no error, no crash, no visible inconsistency to notice. The only way to see
the bug is to watch whether the actual downstream side effect (a post, a
write, an outbound call) happens anyway. That makes it exactly the shape of
bug this register has repeatedly found today (PLAT-116/117 and others): the
platform is confidently and consistently reporting a state that isn't true.

## Required repair (not started)

1. Find the dispatch site that spawns the goroutine actually driving a
   `run_full_workflow` execution (most likely inside the tool handler that
   `run_full_workflow` resolves to) and confirm whether it already has access
   to a per-execution `context.Context`/`CancelFunc`, or needs one added.
2. Wire that cancel func into `cancelTrackedExecutionsForSession` (and
   `cancelBackgroundAgents`, if background agents have the same gap) so
   canceling the registry entry also cancels the actual work, not only the
   status watchers.
3. A test that starts a `message_sequence` with at least two `user_message`
   items, cancels mid-way through the first, and asserts the second item's
   `executeMessageSequenceItem`/`executeMessageSequenceUserMessage` is never
   called — fail-before/pass-after against the real execution path, not a
   mock, matching this register's standing discipline for coding-agent code.
4. Live reverify: stop a real running schedule mid-`message_sequence` and
   confirm no further queued item executes.

## Not fixed here

- The actual fix. This ticket documents a confirmed root cause; the dispatch
  site itself has not been located.
- Whether `cancelBackgroundAgents`/`bgAgentRegistry.CancelAll` has the
  identical gap was inferred from its shape (also status-only) but not traced
  with the same rigor as `cancelTrackedExecutionsForSession` — worth
  confirming in the same follow-up rather than assuming symmetry.

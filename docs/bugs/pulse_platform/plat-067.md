[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-067 — scheduled parent continues after its coding-agent terminal has disappeared

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — reproduced from a complete scheduler and server-log trace |
| Last synchronized | `2026-08-10` |

- **Priority:** P0 — a successful child step can make the rest of a scheduled producing run impossible, while consuming its full timeout budget.
- **Owner:** scheduled coding-agent session recovery, background-completion delivery, and scheduler turn dispatch.
- **Source workflow:** RTS Latency.
- **Source run/session:** cron occurrence at 05:00 IST on 2026-08-10; `schedule-cron--42eca39a_1786318255358919000`.
- **Related tickets:** PLAT-004 (correct terminal run status), PLAT-020 (same-session continuation), PLAT-035 (retained turn settlement), PLAT-052 (known scheduler turns retain one native CLI), and PLAT-054 (do not expire live child work).

## What happened

1. The scheduled parent dispatched `step-ingest-notion-feedback` as a background child at 05:48 IST.
2. The child itself completed successfully: it scanned 29 tickets, wrote its output, and its final validation completed.
3. At 05:53:11, delivery of that completion to the parent Claude Code session failed with `context deadline exceeded`; it was queued instead.
4. At 05:53:28, the parent session's tmux server was already absent (`no server running`).
5. At 05:58:38, the backend tried to prelaunch a restored Claude Opus session, waited five minutes for a prompt, and timed out. It nevertheless called `StreamWithEvents` for the next production collector turn.
6. That request produced no usable response and was cancelled at 06:26:12. Pulse then repeated the same failed restored-session pattern with Claude Sonnet.

The scheduler ultimately recorded the run as an error, which is correct under PLAT-004. However, the production voice collector and all later producing work did not run. The fault is not the Notion child or an RTS workflow artifact: the child completed before the parent transport disappeared.

## Root-cause boundary

The exact reason the original Claude tmux server disappeared is not yet isolated. The proven defect is the recovery decision after that loss: **a failed/missing parent transport is treated as usable enough to receive another scheduler turn.** The next call then waits until cancellation rather than recovering or failing decisively.

## Required behaviour

Before dispatching any next scheduled turn or queued child completion:

1. verify that the parent transport/pane exists and can accept input;
2. if it does not, attempt one same-conversation replacement/resume;
3. if that replacement does not reach a verified ready prompt, do **not** call `StreamWithEvents` for the next turn;
4. preserve the completed child's queued notification and report a specific recoverable scheduler error, with the failed transport identity and last terminal evidence;
5. retry/recover only the parent continuation, never re-run an already successful child merely to recreate its completion.

## Acceptance

A real scheduled run with a background child must either:

- deliver the child's completion and continue the same parent conversation; or
- stop promptly with an explicit `parent_transport_unavailable` / recovery-failed outcome.

It must not spend a further turn timeout on a dead session, and it must retain the child's successful result for the retry path.

## Evidence

`agent_go/logs/server_debug.log` for the source session contains:

- `05:53:11` — `Live steer delivery failed ... context deadline exceeded — falling back to queue`;
- `05:53:28` — tmux capture failed: `no server running`;
- `05:58:38` — restored Claude Opus prelaunch timed out after five minutes, followed by a new `StreamWithEvents` call;
- `06:26:12` — that turn completed only after cancellation;
- `06:31:16` and `06:58:50` — equivalent failed Pulse resume/cancellation sequence.

The durable conversation record is `workspace-docs/Workflow/rtslatency/builder/conversation/2026-08-10/session-schedule-cron--42eca39a_1786318255358919000-conversation.json`.

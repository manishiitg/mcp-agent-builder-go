# Bug: "Ask in chat" on a pending decision never reaches a running chat

## Status: Fixed 2026-08-05.

## Symptom

A pending Goal Advisor decision offers "ask in chat". Asked against a chat that
was already running, the question appeared as a chip above the input —

```text
#1: I want to discuss a pending Goal Advisor decision. Do not submit, dismiss,
    or mark the decision handled yet; answer my question first.  [expand] [×]
```

— and nothing else happened. The session was `claudecode` in workflow mode. The
toast said *"Question added to the chat and queued behind the current turn."*,
which was accurate and useless: the turn had no reason to end soon, and there
was no way to push the question through.

## Root cause

`sendReportHumanInputQuestionToChat` only appends to `queuedMessages`. The drain
in `ChatArea` refuses to run while the tab is streaming:

```ts
if (queuedTabIsStreaming || !queuedTabId || isProcessing || queuedMessages.length === 0) return
```

That is correct for an API provider that cannot be interrupted, and wrong here.
`ChatInput` already sends typed text into the same running session:

```ts
if (routeLiveInputToCLI) { onSubmit(trimmed, { preferLiveInput: true }); return }
```

So the capability existed; the queue path simply did not use it. The comment on
`routeLiveInputToCLI` says as much: *"The local queue is only useful for
non-workflow API chat providers."* A workflow message should never have been
sitting in it.

There was no manual escape either. The chip's steer action is wired
`canShowSteer && tabSessionId ? handleSteerQueuedMessage : undefined`, and
`canShowSteer = canSteer && !isCLIProvider`, so on a coding CLI there is no
steer button at all. Queued, undeliverable, no affordance.

## The part that made the first fix wrong

There are TWO live-delivery mechanisms, not one, and they are not
interchangeable:

| route | mechanism | for |
|---|---|---|
| `live-query` | `POST /api/query` with `preferLiveInput` | tmux coding CLIs and workflow chats — backend injects into the attached CLI, falling back to a full resume if it is gone |
| `steer` | `agentApi.sendLiveInput` | an in-flight API-provider turn |
| `wait` | queue drain | no live path; send when the turn ends |

The first attempt patched the drain in `ChatArea` and handled only
`preferLiveInput`. It therefore did nothing for API providers, whose whole
mechanism is steer — and it duplicated routing that already existed in
`ChatInput`, which is how two copies drift apart. It was reverted; `ChatArea` is
untouched.

## Fix

Delivery lives in `ChatInput`, where `routeLiveInputToCLI`, `canShowSteer`,
`mainAgentIsTmuxCLI` and the steer handler already are. An effect watches the
queue and, while a turn is running, delivers human messages by the same route
typed text would take. The decision is extracted to
`utils/queuedMessageDelivery.ts` so the two cannot silently diverge.

Notable: a coding CLI with `canSteer: true` still routes to `live-query`, never
to steer. That mirrors ChatInput's single-entry rule for tmux transport, and is
the case a naive "steer if you can steer" implementation gets wrong.

Auto-notifications are deliberately excluded and keep waiting for idle.
Interrupting a running agent with step-completion noise is not worth it, and
they lose nothing by waiting.

The effect replaced dead code: a `useEffect` at that spot already had guards and
an empty body — the remains of a debug `console.log` deleted in `a8f2f871`,
whose dependency array was still being maintained.

## Tests

`utils/queuedMessageDelivery.test.ts` — 8 cases: workflow chat, coding CLI
outside workflow mode, coding CLI that *could* steer (must not), API provider
with a live turn, API provider without one, idle, no session, and the
human/auto split.

## Notes

- Moving the logic into `ChatInput` removed a limitation the `ChatArea` version
  had: CLI detection needs the provider manifest, which `ChatArea` does not
  have, so a coding CLI outside workflow mode would have kept waiting. In
  `ChatInput` that signal is already derived.
- A re-entrancy guard prevents one state update from firing delivery twice.

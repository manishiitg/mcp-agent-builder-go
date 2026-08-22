[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-180 — a tool call made during a retained (tmux-delivered) turn is tagged with the launching turn's ID, not the retained turn's own ID, because two separate turn-identity systems were never connected

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — root cause traced to file:line, not fixed; downstream consequence not yet assessed |
| Last synchronized | `2026-08-22` |

- **Priority:** P2 — surfaced as a test-contract failure (IC-11's
  `assertCanonicalRetainedTurnIdentity`), not as a user-visible defect. The
  actual downstream consequence of a mistagged `turn_id` on a
  `tool_call_start`/`tool_call_end` event — wrong grouping in a trace/cost
  view, a UI element attributed to the wrong turn, or genuinely nothing
  anyone currently reads — has not been assessed. Raise if evidence of real
  impact turns up; this ticket is root-cause documentation, not an impact
  study.
- **Owner:** `agent_go/pkg/agentwrapper/llm_agent.go` (`toolcalllog.RegisterHook`,
  the `OnStart`/`OnEnd` callbacks around line 1163); `mcpagent/agent/turn_session.go`
  (`Session.Send`, the competing turn-identity system).
- **Related:** [PLAT-179](plat-179.md) — this surfaced immediately after
  PLAT-179's fix let the retained-window test progress far enough to reach
  the check this ticket is about. Not reachable before that fix landed.

## How this surfaced

Once [PLAT-179](plat-179.md) was fixed and confirmed live for pi-cli, the
`--retained-window-p0-only` E2E test progressed past the premature-completion
check and hit a new failure:

```
Error: IC-11 retained-window P0 failed: retained execute_shell_command
tool_call_start has no stable turn_id
```

Reproduced identically for cursor-cli (via a temporary, env-gated test
bypass forcing it through the tmux-retained path it does not use in real
product usage — see PLAT-179's Related Work). Not yet checked for
claude-code or codex-cli, but the mechanism below is shared code, not
provider-specific, so there is no reason to expect them to differ.

`assertCanonicalRetainedTurnIdentity` (the E2E test's own check) requires
every event belonging to one retained turn — the `unified_completion` and
every `tool_call_start`/`tool_call_end` for the tool that turn ran — to carry
the SAME, non-empty `turn_id`. The completion event has one. The tool-call
events do not.

## Root cause

There are two separate, disconnected systems that both claim to identify
"the current turn," and only one of them is wired into tool-call event
tagging:

1. **`agent_go`'s own system.** Every time `AskWithHistory`-style execution
   runs, `buildSessionTurn` (`pkg/agentwrapper/llm_agent.go:1076-1084`) mints
   a fresh ID via `newPlatformTurnID()`. A hook is registered once, per that
   call, to tag every `tool_call_start`/`tool_call_end` event with that same
   ID via closure:
   ```go
   unregisterHTTPToolHook = toolcalllog.RegisterHook(w.config.SessionID, toolcalllog.Hook{
       OnStart: func(tc toolcalllog.StartedCall) {
           ev := events.NewToolCallStartEventWithCorrelation(...)
           attachTurnID(ev, turn.ID)   // <- turn.ID from THIS setup, captured once
           w.emitEvent(ev)
       },
       ...
   })
   ```

2. **`mcpagent`'s own system.** A retained/live-steer delivery
   (`agent/turn_session.go`'s `Session.Send`) has its own, entirely separate
   turn identity — a `canonicalTurnLifecycle` with its own `id` — created
   fresh for that specific retained delivery and attached to the eventual
   `unified_completion` event via `withCanonicalTurnLifecycle`.

For an ordinary (non-retained) turn these happen to line up, because both get
created together at the start of the same call. **A retained delivery never
calls `AskWithHistory` again** — that is the entire point of "retained": the
coding agent is already running in a live pane, and the platform just types
text into it. So the `toolcalllog.RegisterHook` closure from system (1) is
never re-registered for the retained delivery; it is still the one
registered back at the *first* turn, holding onto *that* turn's `turn.ID`.
When the tool call fires later, during the retained turn, `tool_call_start`
gets tagged with the **first turn's ID** from system (1) — a real, non-empty
value, just the wrong one — while `unified_completion` gets the **retained
turn's own ID** from system (2). Two different values, from two identity
systems that were never told about each other.

(The test's own error message — "no stable turn_id" — is consistent with
this: it compares the tool-call event's `turn_id` against the completion
event's `turn_id` and finds them different/absent relative to each other,
not necessarily that the tool-call event's field is a literal empty string.
The exact string mismatch was not captured verbatim in this pass and is
worth confirming directly before implementing a fix.)

## What the correct fix looks like, not yet implemented

The two systems should not both exist for this case. The session already
tracks which turn is currently active (`Session.activeTurn`, set at the start
of `Send` and cleared on completion) — a tool-call event firing while a
retained turn is active should ask the session what turn is active *right
now*, rather than reading a value captured once at an unrelated, earlier
setup. Concretely, `toolcalllog`'s `OnStart`/`OnEnd` callbacks would need a
way to look up the session's live `activeTurn.id` at the moment the event
fires, instead of closing over `turn.ID` from `buildSessionTurn`. This
crosses the `agent_go` / `mcpagent` boundary — `toolcalllog.RegisterHook` is
`agent_go`-owned, `activeTurn`/`canonicalTurnLifecycle` are `mcpagent`-owned
— so it likely needs either a small exported accessor from `mcpagent`'s
`Session`, or a callback `mcpagent` invokes into `agent_go` at retained-turn
start with the real ID, rather than `agent_go` trying to read `mcpagent`
internals directly.

Not designed further here. This ticket is root-cause documentation from a
live investigation, not an implementation.

## Deliberately out of scope in this pass

- **Implementing the fix.** Traced far enough to state the mechanism
  precisely; the actual change crosses a repo boundary and deserves its own
  focused pass rather than being appended to an already-long investigation.
- **Assessing downstream impact.** Whether anything currently reads
  `tool_call_start`/`tool_call_end`'s `turn_id` in a way that a wrong value
  causes visible harm (versus it being an unused/lightly-used bookkeeping
  field) was not checked.
- **Confirming this for claude-code and codex-cli.** The mechanism is shared,
  provider-agnostic code (`toolcalllog`, `buildSessionTurn`), so there is no
  structural reason to expect them to be exempt, but only pi-cli and
  cursor-cli were live-reproduced.

## Acceptance

- [ ] Root cause confirmed with the exact `turn_id` values from both events
      (the first-turn ID vs. the retained-turn's own ID), not inferred from
      code reading alone.
- [ ] Downstream consumers of `tool_call_start`/`tool_call_end`'s `turn_id`
      identified, and real impact (if any) assessed.
- [ ] A fix design that resolves the cross-repo (`agent_go`/`mcpagent`)
      boundary cleanly — not a per-provider patch, since the root cause is
      shared, provider-agnostic code.
- [ ] Confirmed for claude-code and codex-cli, not only pi-cli and
      cursor-cli.

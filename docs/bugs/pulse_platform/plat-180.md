[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-180 — a tool call made during a retained (tmux-delivered) turn is tagged with the launching turn's ID, not the retained turn's own ID, because two separate turn-identity systems were never connected

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `fixed` — the two-system mismatch is resolved; downstream consequence still not assessed |
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

## Fix

Implemented exactly as sketched: agent_go's tool-call hook now asks
mcpagent's `Session` what turn is active *right now*, at the moment each
event fires, instead of trusting a turn ID captured once when the hook was
registered.

`mcpagent/agent/turn_session.go` gained a small exported accessor:

```go
// ActiveTurnID reports the ID of whichever turn owns this session's canonical
// completion right now -- a Session.Run in flight, or a retained tmux turn
// started by Send between Runs -- or "" if none is active.
func (s *Session) ActiveTurnID() string {
	if s == nil {
		return ""
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.activeTurn == nil {
		return ""
	}
	return s.activeTurn.id
}
```

`agent_go/pkg/agentwrapper/llm_agent.go` gained the resolving glue and now
calls it from both `OnStart`/`OnEnd` instead of closing over `turn.ID`:

```go
func (w *LLMAgentWrapper) currentTurnID(fallback string) string {
	w.mu.RLock()
	session := w.session
	w.mu.RUnlock()
	if session != nil {
		if id := session.ActiveTurnID(); id != "" {
			return id
		}
	}
	return fallback
}
```

```go
attachTurnID(ev, w.currentTurnID(turn.ID))
```

This needed no per-provider branching — `currentTurnID` doesn't know or care
which coding-agent provider is running, so it structurally covers
claude-code, codex-cli, cursor-cli, and pi-cli alike, the same way the
underlying `activeTurn` mechanism already does for `unified_completion`.
`w.session` (a `*mcpagent.Session`) was already a field on the wrapper —
already set by `FinalizeDefinition` before the hook is registered — so no new
plumbing was needed to reach it; the fix is the resolving call itself, not
new wiring to the session.

## Verification

- `mcpagent/agent/turn_session_retained_completion_test.go`:
  `TestActiveTurnIDReflectsTheRetainedTurnNotAnEarlierCachedTurn` drives real
  `startRetainedCompletionWatch` machinery (no LLM call needed — the lifecycle
  is set synchronously before the completion-watch goroutine starts) and
  confirms `ActiveTurnID()` reports the retained turn's own, distinct ID while
  it's in flight, then `""` once it completes. Confirmed failing before the
  fix (`session.ActiveTurnID undefined` — a compile failure, since the method
  did not exist) and passing after.
- `mcpagent/agent/public_api_golden_test.go`: `TestSessionPublicMethodSurface`
  is a deliberate migration ratchet pinning `*Session`'s exact public API —
  updated in the same commit to include `ActiveTurnID`, per its own stated
  convention.
- `agent_go/pkg/agentwrapper/llm_agent_turn_id_test.go`:
  `TestCurrentTurnIDFallsBackWhenSessionIsNil` and
  `TestCurrentTurnIDFallsBackWhenSessionHasNoActiveTurn` cover the delegation
  glue itself. Confirmed failing before the fix
  (`wrapper.currentTurnID undefined`) and passing after.
- `go build ./...` clean in both repositories. Full `agent`/`retainedturn`
  suites pass in mcpagent; `agentwrapper` suite passes in agent_go. Three
  pre-existing, unrelated failures were confirmed present identically with
  and without this change (`TestAgentReviewsApproved` — a stale local
  testdata-review gate; `TestWorkshopResolveLLMConfigExpandsCodingAgentMode`,
  `TestStandalonePulseReviewCommandsUsePersistedReviewerPipeline`,
  `TestArtifactDriftAuditsTheSchedule` — unrelated LLM-tier-default and
  guidance-drift checks), not caused by this fix.
- **Not re-verified live end-to-end in this pass.** No dev server was running
  at fix time to re-run `--retained-window-p0-only` against a real pi-cli
  session the way PLAT-179 was confirmed. The fix is exercised through real
  production code paths at the unit level (genuine `Session.Run`/
  `startRetainedCompletionWatch` machinery, not a stub), but the original P0
  contract failure this ticket was filed from has not been re-run to
  literal green. Worth doing before fully closing this out.

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
      code reading alone. (Still inferred from code reading + unit-level
      reproduction, not a captured live pair of mismatched values.)
- [ ] Downstream consumers of `tool_call_start`/`tool_call_end`'s `turn_id`
      identified, and real impact (if any) assessed.
- [x] A fix design that resolves the cross-repo (`agent_go`/`mcpagent`)
      boundary cleanly — not a per-provider patch, since the root cause is
      shared, provider-agnostic code. Shipped as `Session.ActiveTurnID()` +
      `LLMAgentWrapper.currentTurnID()`.
- [ ] Confirmed for claude-code and codex-cli, not only pi-cli and
      cursor-cli. (Fix is structurally provider-agnostic — see Fix section —
      but not individually re-run per provider.)
- [ ] Re-run `--retained-window-p0-only` live against a real coding-agent
      session to confirm `assertCanonicalRetainedTurnIdentity` now passes,
      the way PLAT-179's fix was confirmed. Not done in this pass — no dev
      server was running at fix time.

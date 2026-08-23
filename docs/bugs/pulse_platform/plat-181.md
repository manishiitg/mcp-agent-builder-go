[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-181 — CDP shared-browser tab quota can be misattributed across unrelated workflows

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `partially fixed` — defense-in-depth fix shipped (cdpOwnerID can no longer resolve to the shared connection identity), but the original hypothesis was corrected same day: that specific gap does NOT explain the message-sequence/route step path that actually hit the live symptom; true root cause of the live incident is still unproven, see "Correction" below |
| Last synchronized | `2026-08-23` |

- **Priority:** P1 — a workflow can be permanently blocked from opening its
  own browser tab by a quota it never actually used, with an error message
  that gives no hint the real cause is cross-workflow attribution rather than
  the workflow's own tab usage.
- **Owner:** CDP shared-browser tab ownership (`pkg/browser/cdp_tabs.go`,
  `pkg/browser/cdp_registry.go`, `pkg/browser/executor.go`), shared browser
  session binding (`pkg/orchestrator/agents/workflow/step_based_workflow/controller.go`,
  `controller_batch_execution.go`, `controller_agent_factory.go`).
- **Related:** none yet filed for this mechanism specifically.

## Symptom

A user's Instagram workflow (`route-generate-illustrations`, run
`test-run`/`iteration-0`, 2026-08-23 00:02:15) hit:

```
cannot create another tab in the shared CDP browser: this workflow already
has 4 labeled tab(s) (max 4). Reuse an existing tab by label, or close one
first with agent_browser(command="tab", args=["--cdp", "<endpoint>", "close", "<label>"]).
```

The user inspected the actual 4 labeled tabs at the time and found none of
them belonged to Instagram — they belonged to other, unrelated workflows
(Upwork, Apollo, WhatsApp). Instagram had used zero of its own allowance and
was still blocked.

## Root cause mechanism, confirmed by direct code read

**The quota is per-owner by design** —
`MaxCDPTabsPerOwner` (default 4, `pkg/browser/cdp_registry.go:23`) and
`guardCDPTabCreation`/`countCDPTabAliasesForOwner`
(`cdp_registry.go:228-237`, `cdp_tabs.go:263-276`) count only tabs whose
alias key starts with `cdp:<port>:<ownerID>:`. This is correct in isolation
— the bug is in what `ownerID` resolves to.

**Two identities exist and get conflated.** CDP mode connects every workflow
to one real, shared Chrome, so the *browser connection* itself is
intentionally shared: `executor.go:456-461` unconditionally overwrites the
local `session` variable to `sharedCDPSessionName(port)` = literally
`"shared-cdp-<port>"` for every workflow using that port (confirmed live in
`server_debug.log` today — `upwork_harvest`, `publish-check`, `main`, and
`review-measure` all logged `[BROWSER] CDP: remapped session ... ->
"shared-cdp-9222" for shared browser port 9222`, proving multiple distinct
workflow sessions genuinely share this one string). That sharing is correct
for "which Chrome to talk to."

But `cdpOwner = cdpOwnerID(workflowSessionID, agentSessionID, session)`
(`executor.go:743`) is computed *after* that overwrite, using the now-shared
`session` as one of its inputs — and `cdpOwnerID`
(`cdp_tabs.go:452-466`) falls through to it:

```go
func cdpOwnerID(workflowSessionID, agentSessionID, session string) string {
	for _, candidate := range []string{agentSessionID, workflowSessionID} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if resolved := common.ResolveBrowserSessionID(candidate, "default"); resolved != "" && resolved != "default" {
			return resolved
		}
	}
	for _, candidate := range []string{workflowSessionID, agentSessionID, session} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return "default"
}
```

The first loop is meant to resolve a workflow-specific identity via
`common.ResolveBrowserSessionID(candidate, "default")`
(`pkg/common/types.go:361-370`), which looks up a per-session shell config
(`SessionShellConfig.BrowserSessionID`) set by
`bindWorkshopBrowserSession` → `common.SetSessionBrowserSessionID`
(`controller.go:288-296`), itself fed by `resolveWorkshopBrowserSessionID`
(`controller.go:276-286`), which correctly hashes the workflow's own
`workspacePath` + group name into a distinct value per workflow — this path
IS correctly workflow-scoped when it fires.

The gap: `bindWorkshopBrowserSession` is called with a specific session ID —
confirmed at `controller_batch_execution.go:345-346`, it binds
`groupSessionID` (the *group-level* session). If the agent that actually
executes a given step/route (e.g. `route-generate-illustrations`) runs under
a different, more granular session ID than `groupSessionID` for its own
`ChatSessionIDKey`/`WorkflowSessionIDKey` context values
(`executor.go:429-436`), the shell-config lookup keyed on *that* session ID
finds nothing, `ResolveBrowserSessionID` falls through to returning
`"default"` (`types.go:367-369`), `cdpOwnerID`'s first loop treats that as
"not resolved" and continues, and the second loop then returns the first
non-empty raw value among `workflowSessionID`, `agentSessionID`, `session` —
where `session` is, by this point, the shared `"shared-cdp-<port>"` string
if the other two are also empty for this call. Every workflow hitting this
same gap on the same port collapses onto the identical owner key, and their
tab counts sum together.

## What is and isn't confirmed

- **Confirmed by code**: the shared-connection-identity overwrite exists
  exactly as described, the ownership-resolution fallback chain exists
  exactly as described, and the final fallback value it can reach
  (`"shared-cdp-<port>"`) is identical across workflows by construction —
  not a hypothetical, the whole point of that string is to be shared.
- **Confirmed live** (`server_debug.log`, 2026-08-23): multiple distinct
  workflow session labels remap to the identical `"shared-cdp-9222"` string,
  proving the shared-connection layer is active and in real use across
  concurrent workflows on this deployment.
- **Not confirmed**: the exact server log for the failing 00:02:15 moment
  itself — the current `server_debug.log` only starts at `00:28:53`
  (the server restarted between the incident and now, and no rotated log
  covers that window), so the specific `cdpOwner` value used for this exact
  call could not be read directly. The mechanism above is the most concrete,
  code-confirmed explanation matching the symptom, not a directly observed
  smoking gun for this one incident.

## Correction — the specific gap does NOT fire for the step that actually hit this (2026-08-23, same day)

A follow-up investigation traced the exact session ID `route-generate-illustrations`
executes under, to check whether it genuinely diverges from what
`bindWorkshopBrowserSession` binds, as hypothesized above. It does not:

1. **Live evidence of the actual session used**: the step's own execution
   log (`.../logs/route-generate-illustrations/execution/execution-attempt-1-iteration-1.json`)
   captures the real call using
   `session=msgseq-iteration-0-test-run-step-1-sub-route-generate-illustrations-todo-id-route-generate-illustrations`.
2. That exact string is not an unbound leftover — it is `execSessionID`,
   built by `messageSequenceRuntimeSessionID(stepPath, stepID)`
   (`controller_message_sequence.go:1107-1117`), assigned as
   `session.RuntimeSessionID` (`controller_message_sequence.go:1032-1036`),
   injected into the agent's context via `messageSequenceRuntimeSessionOverride`
   (`controller_message_sequence.go:1057-1059`), and consumed as
   `execSessionID` inside `createExecutionOnlyAgent`
   (`controller_agent_factory.go:1298-1300`).
3. **This exact `execSessionID` is what gets bound**, synchronously, before
   any tool call: `createExecutionOnlyAgent` unconditionally calls
   `hcpo.bindWorkshopBrowserSession(execSessionID, sharedBrowserSessionID)`
   (`controller_agent_factory.go:1312-1313`) — not only the group-level
   session as this ticket originally assumed.
4. Both delivery paths for `ChatSessionIDKey` (in-process via
   `base_orchestrator_agent.go:167`/`base_agent.go:336`, and the HTTP/mcpbridge
   path used by CLI-transport coding agents via `MCP_SESSION_ID` →
   `X-Session-ID` → `cmd/server/server.go`'s context injection) carry this
   same `execSessionID` value through to `pkg/browser/executor.go:433-436`'s
   `agentSessionID`.
5. So at `cdpOwnerID(workflowSessionID, agentSessionID, session)`
   (`executor.go:743`), `agentSessionID == execSessionID`, `cdpOwnerID`'s
   first loop's `common.ResolveBrowserSessionID(agentSessionID, "default")`
   finds the binding from step 3, resolves to the workflow+group-scoped
   `workflow-browser-<hash>-<groupName>` identity, and returns immediately —
   **never reaching the second loop's fallback to the shared `session`
   value** this ticket originally blamed.

**What this means:** the overwrite-before-ownership-computation shape
described in "Root cause mechanism" above is real in the code, but for this
specific call path it is neutralized by `createExecutionOnlyAgent`'s own
binding call, which this ticket's first pass had not checked closely enough
before concluding the gap applied here.

**What remains unproven:** the actual mechanism behind the live incident
(Upwork/Apollo/WhatsApp tabs counting against Instagram's quota) is still
unknown. `server_debug.log` and its rotated files have no entries for the
2026-08-23 00:02:15 window (server restarted; already noted above), and
`schedule.log` for that window contains only heartbeat lines, no per-tool
session detail — so there is still no direct log evidence of what `cdpOwner`
actually resolved to at the failing moment. The true cause may be a
different code path than `createExecutionOnlyAgent`'s message-sequence
branch (a todo-task or generic background-agent browser call that binds
differently, or doesn't bind at all), a race/staleness condition not visible
to a static trace, or something not yet considered. This needs fresh
investigation from a different angle rather than further scrutiny of the
mechanism already ruled out above.

## Why this matters beyond the one incident

Any two workflows sharing a CDP port are at risk whenever the step/route
executing a browser call runs under a session ID that was never itself
bound via `bindWorkshopBrowserSession` — not just Instagram. The error
message itself actively misleads whoever hits it ("this workflow already
has 4 labeled tab(s)") since it has no way to say the 4 tabs are actually
someone else's.

## Fix implemented (angle 2 of the original two proposed)

Two independent angles were proposed; only the second was implemented, as
defense-in-depth that closes the general class of bug regardless of which
call path turns out to cause the live incident:

1. ~~Bind the browser session ID at every session ID that can reach
   `cdpOwnerID`~~ — **not implemented**. The Correction above shows this
   path (`createExecutionOnlyAgent`/message-sequence steps) already does
   this correctly; whatever call path the live incident actually went
   through has not been identified, so there is nothing concrete to bind
   yet.
2. **Never let `cdpOwnerID` fall back to the shared connection identity —
   implemented.** `cdpOwnerID` (`pkg/browser/cdp_tabs.go`) now takes an
   explicit `sharedConnectionIdentity` parameter — `sharedCDPSessionName(port)`
   for CDP-mode callers, `""` for the one non-CDP caller (artifact
   ownership in `executor.go`, where `session` remains a genuine identity).
   Its fallback loop skips any candidate equal to that value. The final
   "nothing resolved at all" fallback no longer returns the fixed literal
   `"default"` either — that had the identical collision problem across two
   independently-unidentified callers — and instead generates a fresh,
   call-unique value (`cdpUnidentifiedOwnerID`), so quota tracking degrades
   to "doesn't accumulate for this one call" rather than "silently shared
   with a stranger."

## Acceptance tests

Covered by three new tests in `pkg/browser/cdp_tabs_test.go`:

1. `TestCDPOwnerIDNeverReturnsTheSharedConnectionIdentity` — given no bound
   per-workflow identity and `session` equal to the shared connection name,
   the returned owner is never that shared value.
2. `TestCDPOwnerIDUnidentifiedFallbacksDoNotCollideAcrossCalls` — two
   independent calls with nothing resolvable do not return the same owner.
3. `TestCDPOwnerIDStillUsesSessionAsFallbackOutsideCDPMode` — confirms the
   non-CDP artifact-ownership call site is unaffected (empty
   `sharedConnectionIdentity` lets `session` through exactly as before).

The existing `TestCDPOwnerIDUsesStableBrowserSessionOverride` (the correct,
bound-identity happy path) still passes unchanged.

## Not implemented / still open

- The actual live-incident mechanism (Upwork/Apollo/WhatsApp tabs counting
  against Instagram's quota) remains unidentified — see Correction above.
  This fix makes the specific failure mode it targets impossible going
  forward, but cannot be claimed to have fixed the observed incident until
  the true cause is found and matches this shape.
- `guardCDPTabCreation`'s error message still cannot name which tabs are
  actually counted against a caller — it reports a count, not identities.

## Verification

- `go build ./...` and `go test ./pkg/browser/...` clean.
- `go test ./...`: only pre-existing failures unrelated to `pkg/browser`
  (confirmed independently, matching PLAT-182's own baseline check the same
  day).
- The original mechanism was traced to file:line and looked well-evidenced,
  but a same-day follow-up trace of the actual executing session ID for the
  step that hit the symptom disproved it for that call path — corrected
  above rather than left standing as an unverified conclusion. The fix
  implemented is real and tested, but its relationship to the actual live
  incident is unconfirmed. Live reverify pending regardless.

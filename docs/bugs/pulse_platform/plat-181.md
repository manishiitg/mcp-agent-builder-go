[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-181 — CDP shared-browser tab quota can be misattributed across unrelated workflows

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — mechanism confirmed by code and partial live evidence, exact triggering call site not pinned down |
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

## Why this matters beyond the one incident

Any two workflows sharing a CDP port are at risk whenever the step/route
executing a browser call runs under a session ID that was never itself
bound via `bindWorkshopBrowserSession` — not just Instagram. The error
message itself actively misleads whoever hits it ("this workflow already
has 4 labeled tab(s)") since it has no way to say the 4 tabs are actually
someone else's.

## Suggested fix direction (not implemented)

Two independent angles, not mutually exclusive:

1. **Bind the browser session ID at every session ID that can reach
   `cdpOwnerID`, not only the group-level session.** If step/route execution
   creates its own session ID distinct from `groupSessionID`, that session
   also needs its own `bindWorkshopBrowserSession` call (or to inherit the
   parent's binding) before it can make a CDP browser call.
2. **Never let `cdpOwnerID` fall back to the shared connection identity.**
   The second loop in `cdpOwnerID` should not include `session` as a
   candidate once `session` has already been overwritten to the
   port-shared name — that fallback exists to handle a genuinely
   session-less caller, not "the per-workflow binding was missing." A
   missing per-workflow identity should be a loud, diagnosable error
   (or fall back to something still empty/unowned, never counted against
   anyone's quota) rather than silently landing on a value that happens to
   be shared by construction.

## Acceptance tests (once a fix is designed)

1. Two different workflows opening labeled tabs on the same CDP port never
   see each other's tabs counted against their own `MaxCDPTabsPerOwner`
   quota, even when the step actually issuing the browser call runs under a
   session ID distinct from its workflow's group-level session.
2. `guardCDPTabCreation`'s error message, when it does legitimately fire,
   names tabs that provably belong to the workflow hitting the limit — not
   an untraceable shared count.
3. A regression test exercising `cdpOwnerID` directly: given an
   `agentSessionID`/`workflowSessionID` with no bound shell config, the
   function does not fall back to a value shared across other workflows.

## Verification

Investigation only — no code change in this pass. Root cause traced to
file:line; live log evidence supports the shared-connection layer's
existence but not the exact incident's owner value, honestly noted above
rather than presented as fully proven.

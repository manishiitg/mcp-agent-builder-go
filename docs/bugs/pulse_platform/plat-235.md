[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-235 — Internal CDP tab-listing helper had zero retry on timeout, unlike every other read command

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `implemented; runtime reverify` |
| Last synchronized | `2026-08-29` |

- **Priority:** harness_issue, severity high across findings.
- **Findings:** Twitter/social-media `PUL-5629287B`, `PUL-0752CD98` (the
  latter itself an aggregation of 6 already-deduped observations —
  `PUL-19A355A8`, `PUL-4BE982AB`, `PUL-B8569A93`, `PUL-4071B4A8`,
  `PUL-C0FD029A`, `PUL-1234D778` — across intent queue, targeting audit,
  verify, profile health, unfollow cleanup, and remediation steps).

## The pattern

Every finding shares the identical shape: a Stage-0 CDP connection test
passes (`connected=true`, `twitter_visible=true`, `handle_match=true`), then
a later step's authenticated-tab lookup fails with the managed CDP tab
listing timing out after exactly 15 seconds, leaving only a stale
remembered tab ID available. The workflow's own safe-defer behavior — block
public actions, skip verification, report the unavailable state honestly —
is explicitly called out as correct in both findings. The gap is that a
transient 15-second listing timeout, against an endpoint proven reachable
minutes earlier, cost an entire run's worth of actions with no attempt to
recover.

## Root cause

`agent_go/pkg/browser/executor.go` has a general CDP command dispatcher that
already retries certain read-only commands once on timeout
(`shouldRetryCDPTimeout`: `skills`, `snapshot`, `get`, `is`, `screenshot`,
`console`, `errors`) — deliberately excluding commands with side effects,
like `wait`. `"tab"` is excluded from that set too, but for a different
reason: the same command name also covers `tab <id>` (switches focus,
side-effecting) and `tab new` (creates a tab, side-effecting), so it can't
simply be added to the shared allowlist without also making those
side-effecting forms retry-eligible.

The actual internal helper this repo uses everywhere it needs the tab list
for an internal decision — `listCDPTabs` — never goes through that
dispatcher at all. Every one of its 7 call sites (`reuseCDPTabForNew`,
`listCDPTabsForUser`, `selectCDPTabForCommand`, and others) calls
`e.Client.ExecuteCommand` directly with the bare `tab --cdp <url> --json`
list form and a hardcoded 15-second timeout, with no retry whatsoever. That
bare form has no id/selector argument and is always side-effect-free — the
exact class the dispatcher's retry logic exists for — but it never got that
protection.

## Fix

Added a single internal retry directly inside `listCDPTabs` (all 7 call
sites funnel through this one function): on a command-timeout error, sleep
500ms and retry once, mirroring the dispatcher's own existing
timeout-retry shape for other CDP read commands. This is scoped precisely
to the always-side-effect-free bare-list form and does not touch
`shouldRetryCDPTimeout`'s shared allowlist, so `tab <id>`/`tab new` remain
correctly excluded from blanket retry.

## Verification

Two new regression tests in `cdp_tab_list_retry_test.go`:
`TestListCDPTabsRetriesOnceAfterTimeout` (a listing that times out once
then succeeds on retry returns the recovered result, exactly 2 attempts)
and `TestListCDPTabsDoesNotRetryASecondTime` (a listing that keeps timing
out surfaces the error after exactly 2 attempts, not an unbounded loop).
`go build ./...` and `go test ./pkg/browser/...` pass.

## Reverify

No live step has exercised this retry against a real transient CDP timeout
yet. Reverify by observing whether a future authenticated-tab lookup that
would previously have hit the bare 15-second timeout now recovers on the
internal retry instead of forcing safe-skip/block behavior for the rest of
the run.

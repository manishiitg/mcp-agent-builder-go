[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-127 — the tool-error suspect scan flags two documented successful outcomes as failures

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `implemented` — both suppressions shipped with fail-before/pass-after tests; live reverify pending |
| Last synchronized | `2026-08-17` |

- **Priority:** P3 — nothing is lost or broken; the `[TOOL_ERROR_SUSPECT]`
  channel exists specifically for humans reviewing logs, and these two classes
  drowned out the genuine signal in it.
- **Owner:** `mcpagent/toolerr` (`Suspicious`, `problemReportingTools`)
- **Fixed in:** [manishiitg/mcpagent@2200bad](https://github.com/manishiitg/mcpagent/commit/2200bad)
  — a separate repo from this one, vendored into `agent_go` via
  `go.mod`'s local `replace` directive. No `agent_go` change was needed or
  made; this ticket documents and links the fix so it is discoverable from
  this register.

## How it surfaced

Auditing a day's tool-error logs for PLAT-125/126 turned up 929 error-level
lines, of which 524 were `[TOOL_ERROR_SUSPECT]` — the detector's own
"reported success but reads like a failure" heuristic. Tracing every distinct
shape found two categories that were false positives of the detector itself,
not real tool problems:

**`agent_browser` "timeout" — 142 of 524 (27%).** `agent_browser`'s `wait`
action reports `{"waited":"timeout"}` when its poll deadline elapses without
the awaited condition firing. That is success — the tool did what it was
asked and reported what happened — but the substring scan for the word
"timeout" matched the value text of a field that was never an error signal.
Every one of the 142 carried `"success":true` alongside it.

**`get_route_description` — 12 of 12.** Called with no `route_id` (a full
route-catalog dump), its result is prose describing every configured route's
behavior. One of those descriptions happened to contain a word the substring
scanner treats as a failure signal. Zero of the 12 had any actual error
field. This is the exact shape that justified `problemReportingTools` in the
first place — a documented incident on that list states *"70 of 173 suspect
hits came from these five, and not one was a tool failure."*
`get_route_description` was doing the same thing and was not on the list.

## Fix

**Field-aware suppression, not a blanket exemption.** `agent_browser` has
real failures too (`SNAPSHOT_RESULT_TOO_LARGE`, CDP tab-selection errors), so
excluding the whole tool would hide genuine problems. `benignJSONOutcomePattern`
strips only the exact `"waited":"timeout"` field/value pair before the signal
scan runs — narrow by construction, since it removes text this codebase
itself produces with a known vocabulary, not general prose. A real timeout
reported anywhere else in the same payload is untouched and still fires;
pinned by a test that puts both in the same result.

**`get_route_description` added to `problemReportingTools`**, the existing
per-tool suppression list, alongside `get_workflow_command_guidance` — the
same shape, same justification, same mechanism.

## Verification

Four new tests in `toolerr/toolerr_test.go`, against the literal payload
shape from the live log line:

- `{"success":true, ..., "waited":"timeout"}` is not flagged
- the same field with `"waited": "timeout"` spacing is not flagged
- a genuine timeout (`context deadline exceeded`) *beside* a benign
  `"waited":"timeout"` field is still flagged — the suppression does not
  overreach
- `get_route_description` returning prose that would otherwise trip the scan
  is not flagged; an unrelated tool given the same text still is

Fail-before/pass-after confirmed by stashing `toolerr.go` alone and
re-running. Full `mcpagent` repo: `go build`, `go vet` clean; one pre-existing
failure (`TestAgentReviewsApproved`, unrelated to this change), identical
before and after.

## Not fixed here

- **The general substring-list design is unchanged**, deliberately. Its own
  documentation states the accepted trade — *"a false positive costs one log
  line, a false negative costs an investigation that starts with no
  evidence"* — and both fixes here are precise, evidenced exceptions to that
  design, not a rewrite of it.
- **No other `TOOL_ERROR_SUSPECT` category was audited for false positives**
  beyond these two; they were the largest by volume (154 of 524, ~29%) found
  in one day's data, not a claim that nothing else in the list ever
  misfires.

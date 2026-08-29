[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-232 — `agent_browser click` success proves the event dispatched, not that a toggle control's state changed

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `guidance fixed; underlying click primitive is a third-party boundary, same class as PLAT-224` |
| Last synchronized | `2026-08-29` |

- **Priority:** harness_issue, medium/high severity across findings.
- **Findings:** Twitter/social-media `PUL-B6B9F1D7`, `PUL-83421488`,
  `PUL-57F0FE8D`, `PUL-A319446F`, `PUL-BEE6AC69` — five independent
  occurrences across like and follow actions, converging on the identical
  mechanism and the identical recovery recipe without coordinating with each
  other.

## The pattern

`agent_browser click` on a toggle-style control (X's like/follow buttons)
returns `success=true` — the click event genuinely dispatched — while the
live DOM's `aria-label`/`data-testid` on that exact element never changes:
the target still reads unliked / not-following after the call returns. Two
findings show a scoped DOM click (`data-testid=like`/`-follow` on the exact
article/control, not a fresh top-level click) recovering it immediately
after; one shows a concurrent HTTP 429 / X `code:88` rate-limit diagnostic
as a plausible real cause; the other two show the same unchanged-state shape
with no rate-limit evidence. All five workflow runs independently
discovered and applied the same defensive recipe: dispatch, re-read the
exact element (not a generic re-snapshot), retry once with a scoped DOM
click if unchanged, and refuse to record success without a real state
transition.

## Why this is not an in-repo click-mechanism fix

Checked first, matching the investigation already done for PLAT-224:
`agent_browser` is invoked as an installed external binary
(`agent_go/pkg/browser/executor.go`), and `"click"` has no dedicated
handling in this repo's Go wrapper at all — it is a pure passthrough, same
as `"network"` was for PLAT-224. There is no Go-side hook this repository
owns that could add post-click DOM verification to the click mechanism
itself; that would require the external CLI's own source.

## Fix

Added a dedicated, prominent section to `browser-usage.md` (the guidance
skill every browser-driving step loads) naming the exact failure mode and
codifying the four-step recipe all five findings independently converged
on, rather than leaving each workflow to rediscover it per incident. This
is a materially more specific lesson than the pre-existing generic
"re-snapshot after every interaction" bullet — that one doesn't warn that
the click *response itself* is unreliable for confirming a toggle action's
semantic effect, only that cached refs go stale.

## Deliberately not folded into this ticket

`PUL-7074FD09`/`PUL-5A977094` (X quote-compose navigating to a route with no
quote embed or enabled submit control) looked adjacent at first glance —
same workflow, same general "X UI didn't do what the click implied"
shape — but the actual symptom is different: a composer failing to render
expected content after navigation, not a toggle control silently not
flipping. Left open as a separate, still-uninvestigated item.

## Verification

`go test ./cmd/server/guidance/...` passes (template renders clean). No
Go code changed — this is a guidance-only fix, consistent with PLAT-224's
conclusion that the click mechanism itself is outside this repository's
reach.

## Reverify

No live step has loaded this guidance section through the deployed server
yet. Reverify by observing whether a future like/follow action that hits
this exact shape now applies the documented verification recipe before
recording success.

[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-215 — `agent_browser`'s `download` command loses the file if the final write path is denied; confirmed third-party, workaround already documented

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `resolved` — confirmed external, no platform fix available this pass; a workaround was already documented before this ticket |
| Last synchronized | `2026-08-29` |

- **Priority:** P3 — real data-loss failure mode, but self-classified by the
  harness as `external_action_required` with no workflow-side fix, and a
  working documented workaround already existed before this pass.
- **Owner:** N/A — no code change made or needed by this ticket. Root
  mechanism lives in the third-party `agent-browser` CLI, not this repo.
- **Related:** `harness:agent_browser.download` (`Workflow/ICICI-BANK-PARSING`,
  medium; canonical after `merge_pulse_issues` folded a duplicate,
  `PUL-8F2C747E`, into it) — the finding this closes. Same "root cause in a
  third dependency, not ours" shape as PLAT-186/187/188/197/200 this
  session.

## The finding

`agent_browser(command="download", ...)` to a destination outside the
calling session's write-path allowlist permanently deletes the downloaded
file with no recoverable copy anywhere, instead of leaving it recoverable
at whatever staged/temp location it was written to before the final copy
was rejected.

## Confirmed: platform-owned folder guard does not intercept this path

Checked `agent_go/pkg/browser/executor.go` for any code that validates or
copies a `download` command's destination path before or after invoking
`agent-browser` — none exists. The `download <sel> <path>` argument passes
straight through to the third-party `agent-browser` binary, which owns
staging the browser-downloaded file and writing it to the requested
destination itself. Any rejection of the final destination (e.g. this
platform's kernel-level write-deny sandboxing for a path outside the
session's granted write paths, per `FolderGuardConfig`'s own documented
mechanism) happens *inside* `agent-browser`'s own write attempt, and its
error-handling around that failure — not this repo's code — is what
discards the staged copy instead of leaving it recoverable.

## Why this stays a closure record, not a fix

- The harness finding itself already correctly classifies this as
  `external_action_required`: *"This is a harness/SDK-level defect (the
  download command's own cleanup-on-rejected-write-path behavior), not
  something workflow plan/step config can change."*
- A working documented workaround already existed *before* this pass
  (`learnings/_global/references/icici-portal.md` line 42: plain
  click/eval-click on the download control, then copy the browser-saved
  file out via shell), confirmed still present.
- A real platform-side fix would mean building a new download-proxy layer
  ourselves — always downloading to a known-safe location and having our
  own code perform the folder-guard-validated copy, bypassing
  `agent-browser`'s built-in destination-writing entirely — a meaningfully
  sized new feature, not a small patch, and not something to build
  speculatively against two occurrences without broader confirmation this
  is a recurring operational cost worth the investment.

## Explicitly not done

- No code change — nothing on this repo's side causes or can cheaply
  prevent the loss; the mechanism lives inside the third-party binary.
- Did not design or build a download-proxy feature to work around
  `agent-browser`'s own behavior — flagged as a real, available option if
  this recurs often enough to justify it, not ruled out permanently.
- Did not attempt to patch or file upstream against `agent-browser` itself
  — out of scope for this repo's own harness register.

## Verification

- Confirmed via direct code read that no platform-side interception of the
  `download` destination path exists in `pkg/browser/executor.go`.
- Confirmed the documented workaround (`icici-portal.md`) is present and
  unchanged.
- No code changed by this ticket — nothing to build or test.

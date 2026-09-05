# Schedule runtime projection fix

Review follow-up: durable receipt provenance is passed explicitly into the
merge. An authoritative same-run partial/error result replaces stale success;
workflow-only history still cannot overwrite the final Pulse result. Errors
from a different outcome are no longer attached to the retained status.

Local follow-up for G35 / PUL-3565D07C (PLAT-080/219 related). No live
workflow schedules, run records, or finding tracking rows were changed.

- Preserve in-memory running status only while that exact active run still
  has a scheduler run-context registration. Session IDs are not ownership:
  resumed sessions can span multiple runs.
- Without ownership, adopt the latest terminal evidence for the same run
  instead of indefinitely retaining stale running status. Clear stale errors
  and active-run IDs when adopting the terminal record.
- Prefer the durable scheduler terminal receipt over workflow-only history,
  preserving partial/stopped Pulse outcomes.
- Sort history explicitly by start time before selecting the newest record.
- Calculate next_run from loaded enabled jobs at projection time and again
  after Pulse finishes, instead of reusing the timestamp from before Pulse.
  Calendar jobs select the earliest remaining future item; disabled, deleted,
  or exhausted schedules have no next occurrence. Workflow scopes stay isolated.

Regression coverage exercises the actual GetRuntimeStateForWorkflow reader
against a mock workspace API, active-run registration/release, same-time
terminal history, reused sessions, unsorted history, durable partial Pulse
results, long Pulse delays, calendar exhaustion, disabled jobs, and workspace
scope isolation. The individual-step retry recovery issue G67 is separate and
is not changed by this patch. This is not a claim of deployed verification or
a historical SQLite repair.

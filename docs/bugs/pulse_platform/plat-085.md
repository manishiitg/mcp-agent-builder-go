[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-085 — direct HTML report rollout hid primary dashboards that lived outside `db/reports/`

| Coordination | Value |
|---|---|
| Assigned agent | Codex |
| Ticket state | `implemented` — contract upgrade added; live migration pending restart/run |
| Last synchronized | `2026-08-11` |

- **Priority:** P1 — workflows retain their durable report data and HTML, but
  the Report UI can omit the primary user-facing dashboard after the platform
  changes its discovery contract.
- **Observed on:** Instagram. Its primary live dashboard remains at
  `db/report.html` and is the first enabled document in
  `reports/report_plan.json`. The new viewer discovers only immediate
  `db/reports/*.html`, so it shows seven secondary host/setup pages and omits
  the real Instagram workflow snapshot.

## Root cause

The frontend moved from the legacy `reports/report_plan.json` layout registry
to direct standalone `db/reports/*.html` pages. The rollout changed the shared
viewer and authoring guidance, but no workflow contract version performed the
corresponding per-workflow content migration. This cannot be solved safely by
blindly copying one conventional filename: the old plan contains the ordered,
enabled list, titles, and source paths, and existing workflows use different
locations and report sets.

## Fix

Workflow contract `1.0.23` adds one bounded, agent-run migration:

1. Read the complete old report plan and every enabled HTML source.
2. Preserve immediate `db/reports/*.html` pages that were not in the plan.
3. Compose one complete `db/reports/index.html` without replacing the old
   queries, scripts, styles, media references, or visible content.
4. Let that workflow-owned HTML choose its own tabs, sidebar, anchored sections,
   or scrolling layout; the platform does not manufacture navigation from files.
5. Validate the consolidated document and prove the primary dashboard and all
   intended sections remain reachable before removing the retired plan and
   superseded HTML files.

The migration explicitly rejects wrappers, iframes, generic placeholders, and
empty navigation shells. A missing or ambiguous source blocks the version
stamp rather than silently losing the report.

## Verification boundary

Source-level regression tests cover the `1.0.22 -> 1.0.23` route and the
prompt's preservation/validation requirements. Per operator instruction, no
build or test suite was run during this UI session. Live verification requires
the updated server to start and Instagram's workflow upgrade preflight to
complete; the expected result is that the real `Instagram Reel Workflow —
Snapshot` document appears as a report page alongside the host/setup pages.

## Acceptance

- Every enabled HTML document from the old plan remains represented and
  reachable from the single workflow-owned report.
- Primary dashboards outside `db/reports/` are migrated, not omitted.
- The workflow HTML—not the platform—chooses its reporting navigation.
- The workflow version is not stamped if any source is missing, ambiguous, or
  fails HTML validation.

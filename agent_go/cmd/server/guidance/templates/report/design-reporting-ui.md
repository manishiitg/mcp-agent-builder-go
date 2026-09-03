Design the workflow's reporting UI from the ground up. Load
`read_skill(skills=[{"name":"builder-reference","path":"references/reporting-policy.md"},{"name":"builder-reference","path":"references/html-output.md"}])`
first and follow it. A workflow report is one complete HTML experience at
`db/reports/index.html`; its own HTML chooses tabs, sections, sidebar, or a
single scrolling layout. There is no `report_plan.json`, JSON layout, report
widget, or platform-generated report navigation.{{if .Focus}}

Focus on: {{.Focus}}.{{end}}

Every report must include one section, as its own top-level tab — not a
subsection scrolled past within another tab, and not merely an anchored
region on a single scrolling page — that answers "what did this workflow
actually do," in plain, non-technical language: recent runs and the
actions taken in each, in the order a non-technical reader would want
them, with no raw JSON, internal IDs, or state codes. Name it for
the workflow's real run cadence: `Daily Action` (or `Today's Actions`) for a
workflow that genuinely runs daily, `Recent Activity` or `Latest Run` for
one that runs hourly, weekly, or on demand. Even a report with no other
distinct views needs this one top-level tab; the rest of the report's
content becomes a second tab rather than the whole page staying tab-less.

**Build it from the run summaries you already send, by default.**
`notify_user(notification_kind="run_summary")` already writes a structured
row (title, status, message, fields, timestamp) into
`org_dashboard_notifications` in the same `db/db.sqlite`, for every run.
Query `notification_kind = 'run_summary'` ordered by `created_at desc` for
this tab — that is the default, and it needs no new step, table, or
column. Its `message` is agent-written markdown: pass it through
`window.report.renderMarkdown` so headings, bullets and inline code read
properly instead of showing raw `###` and backticks. Show "no runs recorded yet" when the table has no rows rather
than treating it as an error. Only design something custom — a bespoke
table, richer per-run detail — when the parent has explicitly asked for a
different or more detailed activity view than the run summaries give
them; never add a step or table whose only purpose is feeding this tab.

1. Decide the reader's questions and the durable DB/asset evidence that answers
   them. Design one coherent reporting experience; use internal views only for
   genuinely distinct questions.
2. Inspect the real DB schema and sample rows before authoring. Do not invent
   values or make a workflow run regenerate a report.
3. Write the complete experience as `db/reports/index.html`. Include a
   meaningful `<title>` and accessible internal navigation when needed. Use
   `window.report.query` for live data, inline CSS/JS, responsive layout, clear
   empty/error states, no external CDN, no fixed body height, and no nested
   scrolling.
4. Call `validate_report_html` after editing; repair every error.
5. When visual review is requested, inspect the Report tab at desktop/tablet/
   mobile widths and both themes; otherwise stop after validation.

Before writing a large report, briefly state the sections/views you will create
and what each answers, including the required activity/actions section above.
The report should lead with goal progress and key risks, then evidence and
detail—not raw JSON or a generic data dump.

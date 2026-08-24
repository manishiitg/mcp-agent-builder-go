Design the workflow's reporting UI from the ground up. Load
`read_skill(skills=[{"name":"builder-reference","path":"references/reporting-policy.md"},{"name":"builder-reference","path":"references/html-output.md"}])`
first and follow it. A workflow report is one complete HTML experience at
`db/reports/index.html`; its own HTML chooses tabs, sections, sidebar, or a
single scrolling layout. There is no `report_plan.json`, JSON layout, report
widget, or platform-generated report navigation.{{if .Focus}}

Focus on: {{.Focus}}.{{end}}

Every report must include one section — its own tab, panel, or anchored
region; the overall layout is still the HTML's choice — that answers "what
did this workflow actually do," in plain, non-technical language: recent
runs and the actions taken in each, in the order a non-technical reader
would want them, with no raw JSON, internal IDs, or state codes. Name it for
the workflow's real run cadence: `Daily Action` (or `Today's Actions`) for a
workflow that genuinely runs daily, `Recent Activity` or `Latest Run` for
one that runs hourly, weekly, or on demand. This is a content requirement,
not a widget — build it the same way as any other section, reading from
`db/db.sqlite` via `window.report.query`.

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

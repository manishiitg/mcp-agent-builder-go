Design the workflow's reporting UI from the ground up. Load
`read_skill(skills=[{"name":"builder-reference","path":"references/reporting-policy.md"},{"name":"builder-reference","path":"references/html-output.md"}])`
first and follow it. A workflow report is one complete HTML experience at
`db/reports/index.html`; its own HTML chooses tabs, sections, sidebar, or a
single scrolling layout. There is no `report_plan.json`, JSON layout, report
widget, or platform-generated report navigation.{{if .Focus}}

Focus on: {{.Focus}}.{{end}}

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
and what each answers. The report should lead with goal progress and key risks,
then evidence and detail—not raw JSON or a generic data dump.

Design the workflow's reporting UI from the ground up. Load
`read_skill(skills=[{"name":"builder-reference","path":"references/reporting-policy.md"},{"name":"builder-reference","path":"references/html-output.md"}])`
first and follow it. Reports are standalone HTML pages under `db/reports/`;
the Report tab selects one page at a time. There is no `report_plan.json`, JSON
layout, report widget, or report preview tool.{{if .Focus}}

Focus on: {{.Focus}}.{{end}}

1. Decide the reader's questions and the durable DB/asset evidence that answers
   them. One page should be a coherent briefing; use more pages only for truly
   distinct audiences or subjects.
2. Inspect the real DB schema and sample rows before authoring. Do not invent
   values or make a workflow run regenerate a report.
3. Write each complete page directly as `db/reports/<name>.html`. Include a
   meaningful `<title>` and optional `report-order` meta tag. Use
   `window.report.query` for live data, inline CSS/JS, responsive layout, clear
   empty/error states, no external CDN, no fixed body height, and no nested
   scrolling.
4. Call `validate_report_html` after each page edit; repair every error.
5. When visual review is requested, inspect the Report tab at desktop/tablet/
   mobile widths and both themes; otherwise stop after validation.

Before writing a large report, briefly state the pages you will create and
what each answers. The report should lead with goal progress and key risks,
then evidence and detail—not raw JSON or a generic data dump.

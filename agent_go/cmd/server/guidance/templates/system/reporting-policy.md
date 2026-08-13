## HTML-only reporting policy

The Report tab renders one workflow-owned document: `db/reports/index.html`.
That HTML owns the entire reporting experience and decides whether to use tabs,
a sidebar, anchored sections, expandable panels, or one scrolling briefing.
The platform does not manufacture navigation from filenames. There is no report
plan, widget registry, JSON layout, report-native decision control, or report
generation step.

### Page contract

- Write one complete, self-contained `db/reports/index.html` document.
- Include a non-empty `<title>` and accessible internal navigation when the
  report has multiple views or sections.
- Keep CSS and JavaScript inline. Do not pin body height or create a nested
  scroll container.
- Read durable live data with `window.report.query`, `get`, `getText`,
  `getHtml`, `renderMarkdown`, `fileUrl`, and `openFile`. Do not bake changing
  run results into the document or add a step that regenerates it each run.
- After editing, call `validate_report_html()`.
  Open the Report tab to verify visual layout only when requested or needed.

### Workshop and Run boundaries

Workshop owns report authoring and may create or edit `db/reports/index.html`.
Keep report-only changes presentational unless the user also asked to change
workflow behavior or evaluation. Run mode never authors report pages; it only
produces the durable data those pages read.

Human decisions belong in the Pulse panel and chat, not inside reports.

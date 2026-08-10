## HTML-only reporting policy

The Report tab discovers each immediate `db/reports/*.html` file as one
top-level page. It renders only the selected page in one iframe; the outer
Report pane owns scrolling. There is no report plan, widget registry, JSON
layout, report-native decision control, or report generation step.

### Page contract

- Write a complete, self-contained HTML document directly under `db/reports/`.
- A non-empty `<title>` is the page label. Use
  `<meta name="report-order" content="10">` only to override filename order.
- Keep CSS and JavaScript inline. Do not pin body height or create a nested
  scroll container.
- Read durable live data with `window.report.query`, `get`, `getText`,
  `getHtml`, `renderMarkdown`, `fileUrl`, and `openFile`. Do not bake changing
  run results into the document or add a step that regenerates it each run.
- After editing, call `validate_report_html(path="db/reports/<page>.html")`.
  Open the Report tab to verify visual layout only when requested or needed.

### Workshop and Run boundaries

Workshop owns report authoring and may create or edit `db/reports/*.html`.
Keep report-only changes presentational unless the user also asked to change
workflow behavior or evaluation. Run mode never authors report pages; it only
produces the durable data those pages read.

Human decisions belong in the Pulse panel and chat, not inside reports.

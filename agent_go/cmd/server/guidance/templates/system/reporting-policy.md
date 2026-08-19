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

### Referenced files must live under `db/`

A report may only reference paths under `db/`. That is the durable store the
Report tab reads; anything else is invisible to it.

A step's own execution folder (`runs/iteration-N/<group>/execution/...`) is
per-run scratch, not a referenceable location. A screenshot, PDF, or export
captured there is not reachable from the report just because it exists on disk
— the step must copy it into `db/` (for example `db/reports/<key>/proof.png`)
for the report to show it.

Referencing a path that was never published produces a broken-image icon and
nothing else: no error, no log line, no failed run. Verify the file exists at
its `db/` path, not only at the path the step wrote.

`validate_report_html()` does not catch this. It parses the document; it does
not execute it or resolve the paths it references.

### Workshop and Run boundaries

Workshop owns report authoring and may create or edit `db/reports/index.html`.
Keep report-only changes presentational unless the user also asked to change
workflow behavior or evaluation. Run mode never authors report pages; it only
produces the durable data those pages read.

Human decisions belong in the Pulse panel and chat, not inside reports.

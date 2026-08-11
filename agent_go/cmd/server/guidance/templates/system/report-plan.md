## WORKFLOW REPORT — db/reports/index.html

The workflow owns one complete live reporting experience at
`db/reports/index.html`. There is no JSON report plan, widget registry, or
platform-generated page navigation.

- The HTML decides whether to use tabs, a sidebar, anchored sections,
  expandable panels, or one scrolling briefing.
- Read current durable data through `window.report.query`, `get`, `getText`,
  `getHtml`, `renderMarkdown`, `fileUrl`, and `openFile`.
- Keep CSS and JavaScript inline. Use `db/assets/` for durable media.
- Do not add a workflow step that regenerates the report. Steps update durable
  data; the report reads it live.
- Human decisions belong in Pulse/chat, not report-native controls.
- After editing, call `validate_report_html()` and repair every error.

Load `references/reporting-policy.md` for the complete authoring contract.

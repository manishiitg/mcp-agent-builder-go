## WORKFLOW REPORT — db/reports/index.html

The workflow owns one complete live reporting experience at
`db/reports/index.html`. There is no JSON report plan, widget registry, or
platform-generated page navigation.

- The HTML decides whether to use tabs, a sidebar, anchored sections,
  expandable panels, or one scrolling briefing.
- Read current durable data through `window.report.query`, `get`, `getText`,
  `getHtml`, `renderMarkdown`, `fileUrl`, and `openFile`. Write a plain field
  on an existing row (e.g. an inline Approve button) with
  `window.report.updateField`/`updateFields` — see `reporting-policy.md`.
- Gate ALL data calls behind `window.report.ready(fn)` — never `DOMContentLoaded`,
  `window.onload`, or a bare top-level call/await. `window.report` is injected
  after the page's own script runs, so those fire too early.
- Keep CSS and JavaScript inline. Use `db/assets/` for durable media.
- Do not add a workflow step that regenerates the report. Steps update durable
  data; the report reads it live.
- Full human-decision flows (question, options, approve/dismiss/consume)
  belong in Pulse/chat, not report-native controls — `updateField`/
  `updateFields` cover only a plain field write on existing data.
- After editing, call `validate_report_html()` and repair every error.

Load `references/reporting-policy.md` for the complete authoring contract.

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
  `getHtml`, `renderMarkdown`, `fileUrl`, and `openFile`. Write a business
  field on an already-existing row with `window.report.updateField`/
  `updateFields` (see below). Do not bake changing run results into the
  document or add a step that regenerates it each run.
- After editing, call `validate_report_html()`.
  Open the Report tab to verify visual layout only when requested or needed.
- Always include one section, as its own top-level tab (not a subsection
  scrolled past within another tab, and not merely an anchored region on a
  single scrolling page), that reports what the workflow actually did, in
  plain non-technical language (recent runs and the actions taken in
  each) — named for the workflow's real run cadence (e.g. `Daily Action`
  for a daily workflow, `Recent Activity` for hourly/weekly/on-demand ones).
  A report with no other tabs still needs this one; the rest of its content
  becomes a second tab. See the `design-reporting-ui` skill for the full
  authoring requirement.

### Data lifecycle: always gate on `window.report.ready(fn)`

`window.report` is injected into the page AFTER the report's own `<script>`
has already parsed and started running — it does not exist on the page's
first line. Wrap every use of `window.report.*` in:

```js
window.report.ready(function () {
  // window.report.query/get/getText/getHtml/fileUrl are live here.
  // Runs once on load, and again on every later data refresh.
});
```

Do NOT gate data calls on `DOMContentLoaded`, `window.onload`, or a bare
top-level `(async () => { await window.report.query(...) })()`. All three can
run before injection, before `window.report.query` is truly live — the single
most common cause of a report that shows a "data loading error" or is stuck on
"Loading…" the first time it is opened. `validate_report_html()` cannot catch
this: it parses the markup, it does not execute the page. `.ready()` is the
only pattern that is safe regardless of when it runs.

### Writing back: `window.report.updateField`/`updateFields`

A report may write a plain business field on a row it already reads via
`query` — for example an inline Approve/Reject button that flips a
`status` column, or a bulk list where each row edits independently. This
is a narrow, structured write, not a general database API:

```js
// One cell:
await window.report.updateField('emails', row.id, 'status', 'approved')
// Several columns on the same row, applied atomically (all or none) — a
// form submit:
await window.report.updateFields('emails', row.id, { status: 'approved', note: 'looks good' })
```

Both resolve `{ oldValue(s), newValue(s) }` once committed, or reject with
a clear error. The backend validates every call against the table's own
live schema before writing — there is no way to pass raw SQL through
either function, and no declaration step is needed first:

- the target table must exist, have exactly one primary-key column (the
  row is matched on it), and must not be a platform-owned table
  (`report_human_inputs`, `report_human_input_events`,
  `schema_migration_log`);
- every named column must exist on that table, and must not be the
  primary key, end in `_id`, or be named `created_at`/`updated_at` — a
  column that identifies or timestamps the row is never a legitimate
  write target;
- every value must be a plain string, number, boolean, or null — no
  objects/arrays.

This is deliberately NOT a decision UI. A full human-decision flow — a
question with options, free text, an approve/dismiss/consume lifecycle,
and its own audit trail — still belongs in the Pulse panel and chat via
`create_human_input_request`/the report's own `report_human_inputs`
panel, not inside the report page. `updateField`/`updateFields` cover the
narrower case of a report editing a plain field on its own already-
existing data (an email's `status`, a lead's `contacted` flag), not
building a new decision surface.

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

Full human-decision flows (a question, options, approve/dismiss/consume,
audit trail) belong in the Pulse panel and chat, not inside reports. A
report may still write a plain business field via `window.report.
updateField`/`updateFields` — see "Writing back" above.

// Dispatched on window to make the report view re-read db/reports/index.html.
// Lives in its own module so the workflow store can raise it without
// importing the view component.
export const WORKFLOW_REPORT_REFRESH_EVENT = 'workflow-report-refresh-requested'

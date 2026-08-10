[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-063 — the report pane flashed mid-run because a view flip remounts it

| Coordination | Value |
|---|---|
| Assigned agent | `Claude Code` |
| Ticket state | `done` — user-confirmed on the live UI |
| Last synchronized | `2026-08-09` |

- **Priority:** P3 (cosmetic; no data loss)
- **Owner:** workflow canvas mounting / workspace-view sync
- **Reported as:** "something refreshes the reporting page… when a step runs"

## Problem

`WorkflowCanvasWithProvider` (`WorkflowCanvas.tsx:3050`) returns **three
different component types** depending on view state:

```js
if (workflowWorkspaceView === 'files')       return <WorkflowFilesCanvasInner/>
if (viewMode === 'report'|'log'|'soul')      return <WorkflowReportCanvasInner/>
return <ReactFlowProvider><WorkflowCanvasInner/></ReactFlowProvider>
```

React destroys and rebuilds a subtree when the component *type* at a position
changes. So changing `workflowWorkspaceView` does not re-render the pane — it
**unmounts and remounts** it.

`WorkflowLayout.tsx:1231` flipped that value in both directions:

```js
if (!workspaceMinimized && workflowWorkspaceView !== 'files')
    setWorkflowWorkspaceView('files')          // report unmounts
if (workspaceMinimized && workflowWorkspaceView === 'files')
    setWorkflowWorkspaceView(canvasViewMode)   // report remounts
```

Two remounts back to back — a visible flash.

## Why it read as a data refresh but was not

The rebuilt viewer repopulates **synchronously from the module-level
`reportDataCache`** (`ReportViewer.tsx`), so no fetch occurs and `loading` never
shows. That is precisely why it was *fast*, and why instrumenting all four
report events (`workflow-report-data-stale`,
`workflow-report-refresh-requested`, the preference and export events) produced
**zero logs** while the flashing continued. The negative probe result was the
decisive clue: whatever it was, it was not the refresh path.

## Two wrong theories worth recording

1. **`orchestrator_end` fires per step.** It does not — there is exactly one
   caller (`workflow_orchestrator.go:678`, `execution_mode:"workflow_execution"`).
   Acting on this would have been actively harmful: the proposed fix was to
   narrow `EVENT_TYPES.COMPLETION` to `workflow_end`, and…
2. **…`workflow_end` and `batch_execution_end` are never emitted by any Go
   code.** Both appear in the frontend `COMPLETION` list; only `orchestrator_end`
   actually fires. Narrowing to `workflow_end` would have meant *nothing ever
   completes*. **This is a real latent defect and is NOT fixed by this ticket** —
   completion detection rests entirely on one event with two dead entries beside
   it that look like redundancy and provide none.

## Implementation (2026-08-09)

`WorkflowLayout.tsx:1231` — the "un-minimizing the workspace switches to Files"
rule is now skipped while a preview view (`report`/`log`/`soul`) is open, so the
preview is never torn down to show Files and immediately restored.

**Tradeoff, accepted:** un-minimizing the workspace while on a preview view no
longer auto-switches to Files; the user clicks Files. One-line revert if that
reads wrong in use.

Frontend-only — no Go rebuild, no server restart.

## Verification

`npx tsc --noEmit` clean, and **the user confirmed on the live UI that the
flashing stopped**. That is why this ships `done` rather than the usual
`runtime_reverify`.

## Follow-up worth doing separately

- The type-swap in `WorkflowCanvasWithProvider` remains a standing hazard: *any*
  future view-state flip will remount rather than re-render. Rendering one
  component type that switches its inner content would remove the whole class.
- The dead `workflow_end` / `batch_execution_end` entries in `EVENT_TYPES.COMPLETION`
  (see above) deserve their own ticket.

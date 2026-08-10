import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('open report stability', () => {
  it('invalidates terminal-run data without auto-refreshing the mounted report', () => {
    const source = readFileSync('src/components/workflow/ReportViewer.tsx', 'utf8')

    expect(source).toContain('reportDataCache.delete(wf.workspacePath)')
    expect(source).toContain('new CustomEvent(REPORT_DATA_STALE_EVENT')
    expect(source).not.toContain('addEventListener(REPORT_DATA_STALE_EVENT')
  })

  it('keeps report reload behind the explicit toolbar refresh action', () => {
    const canvas = readFileSync('src/components/workflow/canvas/WorkflowCanvas.tsx', 'utf8')

    expect(canvas).toContain('window.dispatchEvent(new CustomEvent(WORKFLOW_REPORT_REFRESH_EVENT))')
    expect(canvas).toContain('onRefresh={handleReportRefresh}')
  })
})

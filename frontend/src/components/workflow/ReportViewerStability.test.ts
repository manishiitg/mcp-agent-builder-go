import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('open report stability', () => {
  it('uses HTML pages directly rather than a report-plan registry', () => {
    const source = readFileSync('src/components/workflow/ReportViewer.tsx', 'utf8')

    expect(source).toContain('db/reports')
    expect(source).toContain('parsePageMetadata')
    expect(source).not.toContain('report_plan.json')
    expect(source).not.toContain('reportPlanParser')
  })

  it('keeps report reload behind the explicit toolbar refresh action', () => {
    const canvas = readFileSync('src/components/workflow/canvas/WorkflowCanvas.tsx', 'utf8')

    expect(canvas).toContain('window.dispatchEvent(new CustomEvent(WORKFLOW_REPORT_REFRESH_EVENT))')
    expect(canvas).toContain('onRefresh={handleReportRefresh}')
  })
})

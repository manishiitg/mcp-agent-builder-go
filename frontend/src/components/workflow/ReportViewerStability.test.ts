import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('open report stability', () => {
  it('loads one workflow-owned HTML reporting experience', () => {
    const source = readFileSync('src/components/workflow/ReportViewer.tsx', 'utf8')

    expect(source).toContain('db/reports/index.html')
    expect(source).toContain('loadReportDocument')
    expect(source).not.toContain('ReportPageNavigation')
    expect(source).not.toContain('report_plan.json')
  })

  it('keeps report reload behind the explicit toolbar refresh action', () => {
    const canvas = readFileSync('src/components/workflow/canvas/WorkflowCanvas.tsx', 'utf8')

    expect(canvas).toContain('window.dispatchEvent(new CustomEvent(WORKFLOW_REPORT_REFRESH_EVENT))')
    expect(canvas).toContain('onRefresh={handleReportRefresh}')
  })

  it('does not let outer polling remount the report iframe', () => {
    const viewer = readFileSync('src/components/workflow/ReportViewer.tsx', 'utf8')
    const frame = readFileSync('src/components/workflow/reportWidgets/HtmlWidgetFrame.tsx', 'utf8')

    expect(viewer).toContain('refreshToken={refreshNonce}')
    expect(frame).toContain('export const HtmlReportFrame = memo(HtmlReportFrameComponent)')
    expect(frame).toContain('const refreshRequested = injectedRefreshTokenRef.current !== refreshToken')
    expect(frame).toContain('useLayoutEffect(() => {')
    expect(frame).toContain('frame.srcdoc = html')
    expect(frame).not.toContain('srcDoc={html}')
  })

  it('preserves report scroll across unsolicited outer-shell resets', () => {
    const viewer = readFileSync('src/components/workflow/ReportViewer.tsx', 'utf8')

    expect(viewer).toContain('lastStableScrollTopRef')
    expect(viewer).toContain('unexpected scroll reset restored')
    expect(viewer).toContain('onScroll={handleReportScroll}')
    expect(viewer).toContain('onWheelCapture={noteUserScrollIntent}')
    expect(viewer).toContain('aria-label="Report content"')
  })
})

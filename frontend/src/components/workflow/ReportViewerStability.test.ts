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

  it('does not fight native scrolling with a corrective reset loop', () => {
    const viewer = readFileSync('src/components/workflow/ReportViewer.tsx', 'utf8')

    expect(viewer).toContain('aria-label="Report content"')
    expect(viewer).not.toContain('unexpected scroll reset restored')
    expect(viewer).not.toContain('onScroll={handleReportScroll}')
  })

  it('keeps decision-status polling visually silent when nothing changed', () => {
    const panel = readFileSync('src/components/workflow/ReportHumanInputPanel.tsx', 'utf8')

    expect(panel).toContain('keepPreviousInputsWhenUnchanged')
    expect(panel).toContain('loadInputs(undefined, false)')
    expect(panel).toContain('if (showLoading) setLoading(true)')
  })
})

import { describe, expect, it, vi } from 'vitest'
import {
  DEFAULT_REPORT_PREVIEW_DEVICE,
  readReportPreviewPreference,
  writeReportPreviewPreference,
} from './reportPreviewPreference'

describe('report preview preference', () => {
  it('has one Tablet default', () => {
    expect(DEFAULT_REPORT_PREVIEW_DEVICE).toBe('tablet')
  })

  it('reads and writes a workflow-scoped explicit choice', () => {
    const values = new Map<string, string>()
    vi.stubGlobal('window', {
      localStorage: {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => values.set(key, value),
      },
      dispatchEvent: vi.fn(),
    })

    expect(readReportPreviewPreference('Workflow/example')).toBe('tablet')
    writeReportPreviewPreference('Workflow/example', 'desktop')
    expect(readReportPreviewPreference('Workflow/example')).toBe('desktop')

    vi.unstubAllGlobals()
  })
})

import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { PulseLoopClosureFinding } from '../../services/api-types'
import { PulseLoopClosureFindingRow } from './PulseLoopClosureNotice'
import {
  PULSE_LOOP_CLOSURE_PREVIEW_LIMIT,
  pulseLoopClosureReference,
  visiblePulseLoopClosureFindings,
} from './pulseLoopClosureNoticeUtils'

function finding(index: number, kind = 'concern_keeps_recurring'): PulseLoopClosureFinding {
  return {
    kind,
    severity: 'high',
    subject: `Full subject ${index}`,
    detail: `Full detail ${index}`,
    evidence: `Full evidence ${index}`,
    age_days: index,
    id: `record-${index}`,
  }
}

describe('PulseLoopClosureNotice', () => {
  it('limits the preview to four findings and exposes the complete list on request', () => {
    const findings = Array.from({ length: 10 }, (_, index) => finding(index))

    expect(PULSE_LOOP_CLOSURE_PREVIEW_LIMIT).toBe(4)
    expect(visiblePulseLoopClosureFindings(findings, false)).toEqual(findings.slice(0, 4))
    expect(visiblePulseLoopClosureFindings(findings, true)).toEqual(findings)
  })

  it('labels the underlying record as a decision or finding', () => {
    expect(pulseLoopClosureReference(finding(1, 'answer_not_applied'))).toEqual({
      label: 'Linked decision',
      value: 'record-1',
    })
    expect(pulseLoopClosureReference(finding(2))).toEqual({
      label: 'Linked finding',
      value: 'record-2',
    })
  })

  it('renders every requested field without truncation when a row is expanded', () => {
    const html = renderToStaticMarkup(
      <PulseLoopClosureFindingRow
        finding={finding(7)}
        findingKey="record-7"
        expanded
        onToggle={() => undefined}
      />,
    )

    expect(html).toContain('Full subject 7')
    expect(html).toContain('Full detail 7')
    expect(html).toContain('Full evidence 7')
    expect(html).toContain('7 days')
    expect(html).toContain('Linked finding')
    expect(html).toContain('record-7')
    expect(html).not.toContain('line-clamp-2')
  })
})

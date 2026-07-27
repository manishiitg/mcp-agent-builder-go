import { describe, expect, it } from 'vitest'
import { completionTitle } from './agentEventDisplayLabels'

describe('completionTitle', () => {
  it('hides internal message-sequence labels and IDs', () => {
    expect(completionTitle('message-sequence-math-solver', true, 'Step')).toBe('Completed')
  })

  it('keeps meaningful names for normal agent completion events', () => {
    expect(completionTitle('Daily latency collector', false, 'Sub-Agent')).toBe('Sub-Agent completed: Daily latency collector')
  })
})

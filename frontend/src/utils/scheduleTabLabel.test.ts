import { describe, expect, it } from 'vitest'
import { scheduleTabLabel } from './scheduleTabLabel'

describe('scheduleTabLabel', () => {
  it('falls back to "Schedule" only when there is no name', () => {
    expect(scheduleTabLabel(undefined)).toBe('Schedule')
    expect(scheduleTabLabel('')).toBe('Schedule')
    expect(scheduleTabLabel('   ')).toBe('Schedule')
  })

  it('keeps a short name intact', () => {
    expect(scheduleTabLabel('Daily execution')).toBe('Daily execution')
  })

  // The real names that motivated this: the parenthetical is schedule timing,
  // already shown in the panel below, and it is the least identifying part.
  it('drops a trailing parenthetical before clipping', () => {
    expect(scheduleTabLabel('Daily Execution x3 (10:00 / 15:00 / 20:00 IST)')).toBe('Daily Execution x3')
    expect(scheduleTabLabel('Lead finding — US (Mon/Wed/Fri)')).toBe('Lead finding — US')
  })

  it('clips an over-long name on a word boundary', () => {
    const label = scheduleTabLabel('Weekly Strategy Discovery Proposer Pass')
    expect(label.endsWith('…')).toBe(true)
    expect(label.length).toBeLessThanOrEqual(23)
    expect(label).not.toMatch(/\s…$/)
  })

  it('distinguishes two schedules that used to render identically', () => {
    const a = scheduleTabLabel('Daily Execution x3 (10:00 / 15:00 / 20:00 IST)')
    const b = scheduleTabLabel('Daily Measurement & Critqueue')
    expect(a).not.toBe(b)
  })
})

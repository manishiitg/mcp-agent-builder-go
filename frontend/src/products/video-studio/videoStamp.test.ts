import { describe, expect, it } from 'vitest'
import { videoStamp } from './videoStamp'

describe('videoStamp', () => {
  // Presentations carry updatedAt from the workspace row, but a missing or
  // malformed value must not render "Invalid Date" into the panel — the caller
  // falls back to the revision label when short is empty.
  it('returns empty strings for missing or unparseable timestamps', () => {
    expect(videoStamp('')).toEqual({ short: '', full: '' })
    expect(videoStamp('not-a-date')).toEqual({ short: '', full: '' })
  })

  it('shows only the time for a video presented today, so the panel stays scannable', () => {
    const today = new Date()
    today.setHours(14, 35, 0, 0)
    const { short, full } = videoStamp(today.toISOString())
    expect(short).not.toContain(',')
    expect(short.length).toBeGreaterThan(0)
    expect(full.length).toBeGreaterThan(short.length)
  })

  it('includes the date once the video is not from today', () => {
    const earlier = new Date()
    earlier.setDate(earlier.getDate() - 3)
    const { short } = videoStamp(earlier.toISOString())
    expect(short).toContain(',')
  })
})

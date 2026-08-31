import { describe, expect, it } from 'vitest'
import {
  LARGE_PASTE_MIN_CHARS,
  LARGE_PASTE_MIN_LINES,
  shouldUsePastedTextAttachment,
} from './chatPasteBehavior'

describe('shouldUsePastedTextAttachment', () => {
  it('keeps ordinary short pastes inline', () => {
    expect(shouldUsePastedTextAttachment('Full watchlist & signal breakdown')).toBe(false)
    expect(shouldUsePastedTextAttachment('first line\nsecond line')).toBe(false)
  })

  it('turns long text into a pasted attachment', () => {
    expect(shouldUsePastedTextAttachment('x'.repeat(LARGE_PASTE_MIN_CHARS))).toBe(true)
  })

  it('turns tall multi-line text into a pasted attachment', () => {
    expect(shouldUsePastedTextAttachment(Array.from({ length: LARGE_PASTE_MIN_LINES }, (_, index) => `line ${index}`).join('\n'))).toBe(true)
  })
})

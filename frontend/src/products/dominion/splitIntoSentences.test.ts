import { describe, expect, it } from 'vitest'
import { splitIntoSentences } from './splitIntoSentences'

describe('splitIntoSentences', () => {
  it('splits on sentence-ending punctuation followed by whitespace', () => {
    const text = 'Conviction 23 is below the 55 actionable threshold. Price is bearish. No entry is set.'
    expect(splitIntoSentences(text)).toEqual([
      'Conviction 23 is below the 55 actionable threshold.',
      'Price is bearish.',
      'No entry is set.',
    ])
  })

  it('does not split on decimal numbers, since none of this dataset has a space after the decimal point', () => {
    const text = 'Entry=49.3 is 0.27 above the prior-day high. Stop=48.3844, target=50.2156, rr=1 (0.5x ATR14=1.8311 stop).'
    expect(splitIntoSentences(text)).toEqual([
      'Entry=49.3 is 0.27 above the prior-day high.',
      'Stop=48.3844, target=50.2156, rr=1 (0.5x ATR14=1.8311 stop).',
    ])
  })

  it('splits before a lowercase-starting clause too, unlike an uppercase-only heuristic would', () => {
    const text = 'Size and stop discipline should account for this. data_completeness=price:ok,options:ok.'
    expect(splitIntoSentences(text)).toEqual([
      'Size and stop discipline should account for this.',
      'data_completeness=price:ok,options:ok.',
    ])
  })

  it('returns a single-element array for text with no sentence boundary', () => {
    expect(splitIntoSentences('No entry (standing aside)')).toEqual(['No entry (standing aside)'])
  })

  it('returns an empty array for empty input', () => {
    expect(splitIntoSentences('')).toEqual([])
  })
})

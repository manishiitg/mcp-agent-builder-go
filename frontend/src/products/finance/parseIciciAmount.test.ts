import { describe, expect, it } from 'vitest'
import { parseIciciAmount } from './parseIciciAmount'

describe('parseIciciAmount', () => {
  it('parses the real observed format: prefix, Indian comma grouping, CR suffix glued on', () => {
    expect(parseIciciAmount('INR 12,34,567.89CR')).toBeCloseTo(1234567.89)
  })

  it('treats a DR (debit/overdrawn) balance as negative', () => {
    expect(parseIciciAmount('INR 5,000.00DR')).toBeCloseTo(-5000)
  })

  it('handles a value with no comma grouping', () => {
    expect(parseIciciAmount('INR 100.00CR')).toBeCloseTo(100)
  })

  it('is case-insensitive on the prefix and suffix', () => {
    expect(parseIciciAmount('inr 100.00cr')).toBeCloseTo(100)
  })

  it('returns null, not NaN or 0, for empty/missing input', () => {
    expect(parseIciciAmount('')).toBeNull()
    expect(parseIciciAmount(null)).toBeNull()
    expect(parseIciciAmount(undefined)).toBeNull()
  })

  it('returns null for unparseable garbage rather than silently coercing', () => {
    expect(parseIciciAmount('not a number')).toBeNull()
    expect(parseIciciAmount('INR CR')).toBeNull()
  })
})

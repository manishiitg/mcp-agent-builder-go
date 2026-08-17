import { describe, expect, it } from 'vitest'
import { extractDateFromLatestTransaction } from './extractDateFromLatestTransaction'

describe('extractDateFromLatestTransaction', () => {
  it('extracts the date from the real observed JSON-string shape', () => {
    const raw = '{"date": "2026-04-30", "description": "SWEEP-IN CREDIT", "debit_amount": 0.0, "credit_amount": 354.0, "closing_balance": 3.42}'
    expect(extractDateFromLatestTransaction(raw)).toBe('2026-04-30')
  })

  it('handles an already-parsed object, not just a JSON string', () => {
    expect(extractDateFromLatestTransaction({ date: '2026-01-01' })).toBe('2026-01-01')
  })

  it('returns undefined rather than a raw JSON blob for unparseable input', () => {
    expect(extractDateFromLatestTransaction('not json')).toBeUndefined()
    expect(extractDateFromLatestTransaction(null)).toBeUndefined()
    expect(extractDateFromLatestTransaction(undefined)).toBeUndefined()
    expect(extractDateFromLatestTransaction('{}')).toBeUndefined()
  })
})

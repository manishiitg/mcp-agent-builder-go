import { describe, expect, it } from 'vitest'
import { normalizeDdMmYyyy } from './normalizeDdMmYyyy'

describe('normalizeDdMmYyyy', () => {
  it('converts the real observed DD/MM/YYYY format to ISO YYYY-MM-DD', () => {
    expect(normalizeDdMmYyyy('20/07/2026')).toBe('2026-07-20')
  })

  it('returns null for a value not in that format, rather than a wrong date', () => {
    expect(normalizeDdMmYyyy('2026-07-20')).toBeNull()
    expect(normalizeDdMmYyyy('')).toBeNull()
    expect(normalizeDdMmYyyy('not a date')).toBeNull()
  })
})

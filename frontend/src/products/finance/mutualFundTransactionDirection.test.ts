import { describe, expect, it } from 'vitest'
import { mutualFundTransactionDirection } from './mutualFundTransactionDirection'

describe('mutualFundTransactionDirection', () => {
  // Every distinct transaction_type value actually observed in the real
  // workflow data (2026-08-16) -- not synthetic examples.
  const observedTypes = [
    'Redemption', 'Switch', 'Purchase', 'Creation of units - Segregated Portfolio',
    'SIP Purchase', 'SWITCH IN', 'SWITCH OUT', 'PURCHASE', 'Switch Out',
    'PURCHASES', 'Switch In', 'REDEEM', 'SWITCH', 'REDEMPTION', 'SIP PURCHASE',
    'PURCHASE SYSTEMATIC', 'SYSTEMATIC PURCHASE (CONTINUOUS OFFER)',
    'PAYMENT - UNITS EXTINGUISHED',
  ]

  it('never throws on any observed value', () => {
    for (const type of observedTypes) {
      expect(() => mutualFundTransactionDirection(type)).not.toThrow()
    }
  })

  it('treats every redemption variant as credit', () => {
    expect(mutualFundTransactionDirection('Redemption')).toBe('credit')
    expect(mutualFundTransactionDirection('REDEEM')).toBe('credit')
    expect(mutualFundTransactionDirection('REDEMPTION')).toBe('credit')
  })

  it('treats every purchase variant as debit', () => {
    expect(mutualFundTransactionDirection('Purchase')).toBe('debit')
    expect(mutualFundTransactionDirection('PURCHASE')).toBe('debit')
    expect(mutualFundTransactionDirection('SIP Purchase')).toBe('debit')
    expect(mutualFundTransactionDirection('PURCHASE SYSTEMATIC')).toBe('debit')
    expect(mutualFundTransactionDirection('SYSTEMATIC PURCHASE (CONTINUOUS OFFER)')).toBe('debit')
  })

  it('leaves switches and corporate actions unlabeled rather than guessing', () => {
    expect(mutualFundTransactionDirection('Switch')).toBeUndefined()
    expect(mutualFundTransactionDirection('SWITCH IN')).toBeUndefined()
    expect(mutualFundTransactionDirection('SWITCH OUT')).toBeUndefined()
    expect(mutualFundTransactionDirection('Creation of units - Segregated Portfolio')).toBeUndefined()
    expect(mutualFundTransactionDirection('PAYMENT - UNITS EXTINGUISHED')).toBeUndefined()
  })
})

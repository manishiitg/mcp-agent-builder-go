import { describe, expect, it } from 'vitest'
import { cleanRationale } from './cleanRationale'

describe('cleanRationale', () => {
  it('strips the observed API-failure noise from a real SOUN rationale, keeping the trading-relevant content', () => {
    const raw = "Stand aside: conviction 22.8 is below the 55 actionable threshold. price 20.0 is bearish and data-quality partial (Polygon snapshot/last-trade entitlement gap; trend ok 27.2, -66.261% from 52w high); options carried no real signal this run -- neutral-injected at 50.0 after Unusual Whales returned 401 Unauthorized on every endpoint (status=failed), fully excluded from the weighted score rather than merely degraded; social 86.1 is constructive (StockTwits sentiment 0.889, status partial); insider 85.0 is constructive (mspr=100)."

    const cleaned = cleanRationale(raw)

    expect(cleaned).not.toContain('401 Unauthorized')
    expect(cleaned).not.toContain('status=failed')
    expect(cleaned).not.toContain('status partial')
    expect(cleaned).not.toContain('entitlement gap')
    // Trading-relevant content survives.
    expect(cleaned).toContain('conviction 22.8')
    expect(cleaned).toContain('trend ok 27.2')
    expect(cleaned).toContain('social 86.1 is constructive')
    expect(cleaned).toContain('insider 85.0 is constructive (mspr=100)')
    expect(cleaned).toContain('options data was unavailable for this run')
  })

  it('strips the observed Polygon rate-limit noise from a price sub-score failure, keeping surrounding content', () => {
    const raw = "Long entry at $12.40, stop $12.05, target $13.10. price data FAILED this run (Polygon 429 rate-limit / possible 403 on snapshot) - no gap%, VWAP, rel-volume, or ATR available; price sub-score excluded from conviction. options 71.2 is constructive (call_put_ratio=2.1)."

    const cleaned = cleanRationale(raw)

    expect(cleaned).not.toContain('429')
    expect(cleaned).not.toContain('403')
    expect(cleaned).not.toContain('rate-limit')
    expect(cleaned).not.toContain('sub-score excluded from conviction')
    // Trading-relevant content survives.
    expect(cleaned).toContain('Long entry at $12.40')
    expect(cleaned).toContain('options 71.2 is constructive (call_put_ratio=2.1)')
    expect(cleaned).toContain('price data was unavailable for this run')
  })

  it('leaves text with none of the known noise patterns unchanged', () => {
    const raw = 'Long entry at $49.30, stop $48.38, target $50.22. Conviction 66.6 driven by strong insider buying.'
    expect(cleanRationale(raw)).toBe(raw)
  })

  it('is idempotent -- cleaning already-clean text is a no-op', () => {
    const raw = 'Stand aside: conviction 22.8 is below the 55 actionable threshold.'
    expect(cleanRationale(cleanRationale(raw))).toBe(cleanRationale(raw))
  })
})

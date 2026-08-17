// The workflow's own rationale text sometimes leaks raw data-provider
// integration failures (a 401 from a market-data API, an entitlement gap)
// into what's meant to be a trading explanation -- e.g. "...after Unusual
// Whales returned 401 Unauthorized on every endpoint (status=failed)...".
// That's engineering-debug language, not something a trader can act on.
//
// This is a best-effort, pattern-based cleanup of the specific noise
// observed in real data (see cleanRationale.test.ts for the exact source
// string) -- not a general translator. It will not catch every future
// variant the workflow's own scoring script might produce; text that
// doesn't match a known pattern passes through unchanged rather than being
// mangled by an over-eager regex.
const REPLACEMENTS: Array<[RegExp, string]> = [
  // "(Polygon snapshot/last-trade entitlement gap; trend ok ...)" -> "(trend ok ...)"
  [/\(Polygon snapshot\/last-trade entitlement gap;\s*/gi, '('],
  // "options carried no real signal this run -- neutral-injected at 50.0 after
  // Unusual Whales returned 401 Unauthorized on every endpoint (status=failed),
  // fully excluded from the weighted score rather than merely degraded"
  [/options carried no real signal this run -- neutral-injected at [\d.]+ after Unusual Whales returned 401 Unauthorized on every endpoint \(status=failed\),?\s*fully excluded from the weighted score rather than merely degraded/gi, 'options data was unavailable for this run'],
  // "price data FAILED this run (Polygon 429 rate-limit / possible 403 on
  // snapshot) - no gap%, VWAP, rel-volume, or ATR available; price sub-score
  // excluded from conviction." -- the parenthetical reason is left general
  // (Polygon's own wording varies: 429 vs 403, "rate-limit" vs "snapshot
  // entitlement", etc.) since the surrounding template is what's reliably
  // observed.
  [/price data FAILED this run \([^)]*\)\s*-\s*no gap%,\s*VWAP,\s*rel-volume,\s*or ATR available;\s*price sub-score excluded from conviction\.?/gi, 'price data was unavailable for this run.'],
  // ", status partial)" / ", status=partial)" -> ")"
  [/,\s*status[= ]partial\)/gi, ')'],
  // ", status=failed)" -> ")"
  [/,\s*status=failed\)/gi, ')'],
]

export function cleanRationale(text: string): string {
  let cleaned = text
  for (const [pattern, replacement] of REPLACEMENTS) {
    cleaned = cleaned.replace(pattern, replacement)
  }
  return cleaned.replace(/[ \t]{2,}/g, ' ').trim()
}

import { BRAND_MARKS } from './brandMarks'

/**
 * Server names that do not normalize cleanly onto their brand slug.
 * Keys are the normalized server name (lowercased, non-alphanumerics stripped).
 */
const SLUG_ALIASES: Record<string, string> = {
  mondaycom: 'monday',
  cloudflareobservability: 'cloudflare',
}

/**
 * Resolves an MCP server's display name to a `BRAND_MARKS` slug.
 *
 * Most names match once lowercased and stripped of punctuation ("PayPal" →
 * "paypal"). The rest are aliased above. Servers with no published mark —
 * Plaid, Close, Fireflies, Globalping, custom servers — resolve to undefined
 * and fall back to the neutral glyph in ConnectionIcon.
 */
export function brandSlugFor(serverName: string): string | undefined {
  const normalized = serverName.toLowerCase().replace(/[^a-z0-9]/g, '')
  const slug = SLUG_ALIASES[normalized] ?? normalized
  return slug in BRAND_MARKS ? slug : undefined
}

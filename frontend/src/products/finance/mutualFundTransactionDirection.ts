// Verified against the real distinct transaction_type values in the
// Mutual-Fund workflow's own data (2026-08-16): Redemption/REDEEM/REDEMPTION
// always means cash out of the fund and into the portfolio (credit);
// Purchase/SIP variants always mean cash going into the fund (debit).
// Switch In/Out, "Creation of units - Segregated Portfolio", and "PAYMENT -
// UNITS EXTINGUISHED" are deliberately left unlabeled -- a switch moves
// money between two funds, not in or out of the portfolio, and the other
// two are corporate actions with no confident reading. Guessing wrong here
// would mislabel a transaction in a real money dashboard.
export function mutualFundTransactionDirection(rawType: string): 'credit' | 'debit' | undefined {
  const type = rawType.toLowerCase()
  // "redeem" and "redemption" diverge after the shared "rede" root --
  // neither word is a substring of the other, so both stems are matched
  // explicitly rather than relying on one regex to catch both.
  if (/redeem|redempt/.test(type)) return 'credit'
  if (/purchase/.test(type)) return 'debit'
  return undefined
}

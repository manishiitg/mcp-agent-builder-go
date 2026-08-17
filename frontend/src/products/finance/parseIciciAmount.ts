// ICICI's total_balance_inr column is formatted text, not a number, e.g.
// "INR 12,34,567.89CR" -- a currency prefix, Indian comma grouping, and a
// credit/debit suffix glued directly onto the number with no space. Comma
// stripping is safe regardless of grouping (parseFloat doesn't care where
// commas fall), but the CR/DR suffix must be stripped first or parseFloat
// silently returns NaN instead of throwing, which would corrupt Net Worth
// with no visible error. Returns null (not NaN, not 0) when the input
// can't be parsed, so a caller must decide explicitly how to handle it
// rather than silently summing a wrong number.
export function parseIciciAmount(raw: string | null | undefined): number | null {
  if (!raw) return null
  const trimmed = raw.trim()
  // DR (a debit/overdrawn balance) is a negative contribution to Net Worth;
  // CR (credit, the normal case) is positive. Getting this backwards would
  // silently overstate net worth on an overdrawn account, so it is handled
  // explicitly rather than left as "whatever the absolute number parses to".
  const isDebit = /DR$/i.test(trimmed)
  const cleaned = trimmed
    .replace(/^INR\s*/i, '')
    .replace(/CR$|DR$/i, '')
    .replace(/,/g, '')
    .trim()
  if (!cleaned) return null
  const value = Number(cleaned)
  if (!Number.isFinite(value)) return null
  return isDebit ? -value : value
}

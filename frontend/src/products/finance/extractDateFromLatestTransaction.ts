// HDFC's transaction_summary.latest_transaction is not a date string -- it's
// a serialized JSON object of the full transaction row (confirmed real:
// {"date": "2026-04-30", "description": "...", "debit_amount": ..., ...}).
// Extracts just the date field; returns undefined rather than the raw JSON
// if the shape doesn't match, since showing a JSON blob as a date is worse
// than showing nothing.
export function extractDateFromLatestTransaction(raw: unknown): string | undefined {
  if (raw == null) return undefined
  try {
    const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw
    const date = (parsed as { date?: unknown })?.date
    return typeof date === 'string' ? date : undefined
  } catch {
    return undefined
  }
}

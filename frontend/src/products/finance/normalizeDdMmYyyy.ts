// gst_return_status.due_date is DD/MM/YYYY in this source. Converts to ISO
// YYYY-MM-DD so it can be compared to today's date; returns null (not a
// wrong date) for anything not in that exact shape.
export function normalizeDdMmYyyy(value: string): string | null {
  const match = /^(\d{2})\/(\d{2})\/(\d{4})$/.exec(value)
  if (!match) return null
  const [, day, month, year] = match
  return `${year}-${month}-${day}`
}

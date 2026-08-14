// Several renders of the same brief look alike in the panel — same title, same
// first frame — so the only way to tell which one is newest was the revision
// number, which does not distinguish two separately presented videos at all.
// The timestamp is already carried on the presentation; it just was not shown.
export function videoStamp(iso: string): { short: string; full: string } {
  const when = new Date(iso)
  if (!iso || Number.isNaN(when.getTime())) return { short: '', full: '' }
  const now = new Date()
  const sameDay = when.toDateString() === now.toDateString()
  const time = when.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  const short = sameDay ? time : `${when.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })}, ${time}`
  return { short, full: when.toLocaleString() }
}

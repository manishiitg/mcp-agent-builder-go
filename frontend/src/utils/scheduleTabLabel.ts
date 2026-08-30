// Every scheduled-run tab used to be labelled the literal string "Schedule".
// The real schedule name was captured into metadata.scheduledJobName but only
// ever reached a tooltip, so several concurrent runs produced a row of
// identical tabs that could only be told apart by hovering each one.
//
// The name is the only thing that distinguishes these tabs, so it belongs in
// the label. It still has to fit a tab, and real schedule names are long and
// front-loaded with redundancy — "Daily Execution x3 (10:00 / 15:00 / 20:00
// IST)", "Lead finding — US (Mon/Wed/Fri)" — so the parenthetical schedule-time
// detail is dropped first (it is already shown in the panel below) and only
// then is the remainder clipped.

const MAX_SCHEDULE_TAB_LABEL = 22

/**
 * scheduleTabLabel renders a schedule's name for a tab, falling back to
 * "Schedule" only when there is genuinely no name to show.
 *
 * The full name always remains available in the tab's tooltip, so clipping
 * here never loses information — it only keeps the tab strip readable.
 */
export function scheduleTabLabel(jobName?: string | null): string {
  const trimmed = (jobName || '').trim()
  if (!trimmed) return 'Schedule'

  // Drop a trailing parenthetical (cron times, day lists): it is the least
  // identifying part of the name and the most expensive in tab width.
  const withoutParenthetical = trimmed.replace(/\s*\([^()]*\)\s*$/, '').trim() || trimmed
  if (withoutParenthetical.length <= MAX_SCHEDULE_TAB_LABEL) return withoutParenthetical

  // Clip on a word boundary when one is close to the limit, so a label breaks
  // between words rather than mid-word.
  const clipped = withoutParenthetical.slice(0, MAX_SCHEDULE_TAB_LABEL)
  const lastSpace = clipped.lastIndexOf(' ')
  const body = lastSpace > MAX_SCHEDULE_TAB_LABEL - 8 ? clipped.slice(0, lastSpace) : clipped
  return `${body.trimEnd()}…`
}

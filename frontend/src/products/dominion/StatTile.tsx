type StatTileProps = {
  label: string
  value: string
  detail?: string
  tone?: 'neutral' | 'positive' | 'negative'
}

const TONE_CLASSES: Record<NonNullable<StatTileProps['tone']>, string> = {
  neutral: 'text-white',
  positive: 'text-emerald-400',
  negative: 'text-red-400',
}

// Generalizes the inline KPI-tile markup already used ad hoc elsewhere
// (e.g. EmployeeDashboard's "Selected run cost" block) into a single
// reusable component, since no shared StatTile/KpiCard exists yet.
// Unconditionally dark, matching DominionSurface's own navy/glass theme --
// this surface doesn't branch on light mode the way Finance's does.
export function StatTile({ label, value, detail, tone = 'neutral' }: StatTileProps) {
  return (
    <div className="rounded-2xl border border-white/10 bg-[#0d111c] px-5 py-4 shadow-xl shadow-black/20">
      <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</div>
      <div className={`mt-1.5 text-3xl font-bold tracking-tight tabular-nums ${TONE_CLASSES[tone]}`}>{value}</div>
      {detail && <div className="mt-1.5 text-xs text-slate-500">{detail}</div>}
    </div>
  )
}

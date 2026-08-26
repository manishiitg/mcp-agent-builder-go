import { Activity } from 'lucide-react'
import type { TradeIdea, TradeOutcome } from './types'
import { CARD, SectionHeader } from './DominionSurface'

type ActionItem =
  | { kind: 'outcome'; at: string; data: TradeOutcome }
  | { kind: 'idea'; at: string; data: TradeIdea }

const RESULT_LABEL: Record<TradeOutcome['result'], string> = {
  win: 'Won',
  loss: 'Lost',
  flat: 'Flat',
  no_fill: 'No fill',
  open: 'Opened',
  retired: 'Retired',
}

const RESULT_TONE: Record<TradeOutcome['result'], string> = {
  win: 'text-emerald-400',
  loss: 'text-rose-400',
  flat: 'text-slate-400',
  no_fill: 'text-slate-500',
  open: 'text-indigo-400',
  retired: 'text-slate-500',
}

function formatTime(value: string): string {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
}

function formatDayLabel(value: string): string {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleDateString('en-US', { weekday: 'long', month: 'short', day: 'numeric' })
}

// "Today" for a paper-trading workflow is the most recent trading day that
// actually produced data, not the calendar date -- there is no action to
// show for a weekend or a day the run hasn't fired yet.
function latestRunDate(outcomes: TradeOutcome[], ideas: TradeIdea[]): string | null {
  let latest: string | null = null
  for (const row of outcomes) {
    if (!latest || row.runDate > latest) latest = row.runDate
  }
  for (const row of ideas) {
    if (!latest || row.runDate > latest) latest = row.runDate
  }
  return latest
}

export function DailyActions({ outcomes, ideas }: { outcomes: TradeOutcome[]; ideas: TradeIdea[] }) {
  const runDate = latestRunDate(outcomes, ideas)
  if (!runDate) return null

  const dayOutcomes = outcomes.filter((o) => o.runDate === runDate && o.result !== 'open')
  const dayIdeas = ideas.filter((i) => i.runDate === runDate)

  const items: ActionItem[] = [
    ...dayOutcomes.map((data): ActionItem => ({ kind: 'outcome', at: data.exitTime ?? data.runDate, data })),
    ...dayIdeas.map((data): ActionItem => ({ kind: 'idea', at: data.createdAt, data })),
  ].sort((a, b) => b.at.localeCompare(a.at))

  return (
    <section>
      <SectionHeader icon={Activity} title="Daily Action" count={items.length} />
      <div className={CARD}>
        <p className="mb-4 text-xs text-slate-500">{formatDayLabel(runDate)} · what this workflow actually did</p>
        {items.length === 0 ? (
          <p className="text-sm text-slate-500">No trades closed or signals generated this session.</p>
        ) : (
          <ul className="divide-y divide-white/5">
            {items.map((item, index) => (
              <li key={`${item.kind}-${item.data.symbol}-${index}`} className="flex items-center justify-between gap-4 py-2.5 text-sm">
                {item.kind === 'outcome' ? (
                  <>
                    <div className="min-w-0 flex-1">
                      <span className="font-semibold text-slate-100">{item.data.symbol}</span>
                      <span className="ml-2 text-slate-500">{item.data.direction}</span>
                      {item.data.note ? <span className="ml-2 truncate text-slate-500">{item.data.note}</span> : null}
                    </div>
                    <span className={`shrink-0 font-medium ${RESULT_TONE[item.data.result]}`}>
                      {RESULT_LABEL[item.data.result]}
                      {item.data.rMultiple != null ? ` · ${item.data.rMultiple >= 0 ? '+' : ''}${item.data.rMultiple.toFixed(2)}R` : ''}
                    </span>
                  </>
                ) : (
                  <>
                    <div className="min-w-0 flex-1">
                      <span className="font-semibold text-slate-100">{item.data.symbol}</span>
                      <span className="ml-2 text-slate-500">
                        new {item.data.direction} signal · conviction {item.data.conviction}
                      </span>
                    </div>
                    <span className="shrink-0 text-indigo-400">Signal</span>
                  </>
                )}
                <time className="w-14 shrink-0 text-right text-xs text-slate-500">{formatTime(item.at)}</time>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  )
}

import { useEffect, useMemo, useState } from 'react'
import { ArrowLeft } from 'lucide-react'
import { loadSymbolHistory } from './adapters/symbolHistory'
import { computeWinRate } from './adapters/winRate'
import { cleanRationale } from './cleanRationale'
import { CARD } from './DominionSurface'
import { splitIntoSentences } from './splitIntoSentences'
import type { StockGroup } from './stockGroups'
import type { TradeIdea, TradeOutcome } from './types'

const TIER_BADGE: Record<string, string> = {
  large: 'bg-indigo-500/10 text-indigo-300',
  mid: 'bg-violet-500/10 text-violet-300',
  small: 'bg-fuchsia-500/10 text-fuchsia-300',
}

const RESULT_BADGE: Record<TradeOutcome['result'], string> = {
  win: 'bg-emerald-500/10 text-emerald-300',
  loss: 'bg-red-500/10 text-red-300',
  flat: 'bg-white/5 text-slate-300',
  no_fill: 'bg-white/5 text-slate-500',
  open: 'bg-amber-500/10 text-amber-300',
  retired: 'bg-white/5 text-slate-500',
}

const DIRECTION_BADGE: Record<string, string> = {
  long: 'bg-emerald-500/10 text-emerald-300',
  short: 'bg-red-500/10 text-red-300',
  stand_aside: 'bg-white/5 text-slate-400',
}

function formatUsd(value: number): string {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 }).format(value)
}

function formatDateTime(value: string | null): string {
  if (!value) return '—'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
}

function StatTile({ label, value, tone = 'neutral' }: { label: string; value: string; tone?: 'neutral' | 'positive' | 'negative' }) {
  const toneClass = tone === 'positive' ? 'text-emerald-400' : tone === 'negative' ? 'text-red-400' : 'text-white'
  return (
    <div className="rounded-xl border border-white/10 bg-white/[0.03] px-4 py-3">
      <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</div>
      <div className={`mt-1 text-xl font-bold ${toneClass}`}>{value}</div>
    </div>
  )
}

type LoadState = { ideas: TradeIdea[]; outcomes: TradeOutcome[]; loading: boolean; error: string | null }

type StockDetailViewProps = {
  symbol: string
  group: StockGroup | undefined
  onBack: () => void
}

export function StockDetailView({ symbol, group, onBack }: StockDetailViewProps) {
  const [state, setState] = useState<LoadState>({ ideas: [], outcomes: [], loading: true, error: null })
  const [showAllSignals, setShowAllSignals] = useState(false)

  useEffect(() => {
    let cancelled = false
    setState({ ideas: [], outcomes: [], loading: true, error: null })
    setShowAllSignals(false)
    ;(async () => {
      try {
        const { ideas, outcomes } = await loadSymbolHistory(symbol)
        if (cancelled) return
        setState({ ideas, outcomes, loading: false, error: null })
      } catch (err) {
        if (cancelled) return
        setState((prev) => ({ ...prev, loading: false, error: err instanceof Error ? err.message : String(err) }))
      }
    })()
    return () => { cancelled = true }
  }, [symbol])

  const winRate = useMemo(() => computeWinRate(state.outcomes), [state.outcomes])
  const actionableIdeas = useMemo(() => state.ideas.filter((idea) => idea.direction !== 'stand_aside'), [state.ideas])
  const standAsideCount = state.ideas.length - actionableIdeas.length
  const visibleIdeas = showAllSignals ? state.ideas : actionableIdeas

  return (
    <div className="mx-auto w-full max-w-[1600px] space-y-8 p-8">
      <div>
        <button
          type="button"
          onClick={onBack}
          className="flex items-center gap-1.5 text-sm font-medium text-slate-400 transition hover:text-white"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Stocks
        </button>
        <div className="mt-4 flex items-center gap-3">
          <h1 className="text-3xl font-bold text-white">{symbol}</h1>
          {group?.tier && (
            <span className={`rounded-full px-2.5 py-1 text-xs font-bold uppercase tracking-wide ${TIER_BADGE[group.tier]}`}>{group.tier}</span>
          )}
        </div>
      </div>

      {!state.loading && !state.error && (
        <section className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <StatTile label="Trades" value={String(winRate.decided)} />
          <StatTile
            label="Win Rate"
            value={winRate.winRatePct != null ? `${winRate.winRatePct.toFixed(1)}%` : '—'}
          />
          <StatTile
            label="Total R"
            value={`${winRate.sumR >= 0 ? '+' : ''}${winRate.sumR.toFixed(2)}R`}
            tone={winRate.sumR > 0 ? 'positive' : winRate.sumR < 0 ? 'negative' : 'neutral'}
          />
          <StatTile
            label="Position"
            value={group?.position ? `${group.position.unrealizedPl >= 0 ? '+' : ''}${formatUsd(group.position.unrealizedPl)}` : 'None'}
            tone={group?.position ? (group.position.unrealizedPl >= 0 ? 'positive' : 'negative') : 'neutral'}
          />
        </section>
      )}

      {group?.position && (
        <section className={CARD}>
          <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Open Position</div>
          <div className="mt-2 flex items-center justify-between">
            <div className="text-sm text-slate-300">{group.position.qty} sh @ {formatUsd(group.position.avgEntryPrice)}</div>
            <div className={`text-xl font-bold ${group.position.unrealizedPl >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
              {group.position.unrealizedPl >= 0 ? '+' : ''}{formatUsd(group.position.unrealizedPl)}
            </div>
          </div>
          {group.position.stop != null && group.position.target != null && (
            <div className="mt-1 text-xs text-slate-500">stop {formatUsd(group.position.stop)} · target {formatUsd(group.position.target)}</div>
          )}
        </section>
      )}

      {state.loading ? (
        <div className="grid h-40 place-items-center text-sm text-slate-500">Loading history…</div>
      ) : state.error ? (
        <div className="grid h-40 place-items-center text-sm text-red-400">Failed to load: {state.error}</div>
      ) : (
        <div className="grid grid-cols-1 gap-8 xl:grid-cols-2">
          <section>
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-base font-semibold text-slate-100">
                Signal History <span className="text-sm font-normal text-slate-500">({visibleIdeas.length})</span>
              </h2>
              {standAsideCount > 0 && (
                <button
                  type="button"
                  onClick={() => setShowAllSignals((v) => !v)}
                  className="text-xs font-medium text-indigo-400 transition hover:text-indigo-300"
                >
                  {showAllSignals ? `Hide ${standAsideCount} stand-aside` : `Show ${standAsideCount} stand-aside`}
                </button>
              )}
            </div>
            <div className="space-y-3">
              {visibleIdeas.map((idea, i) => (
                <div key={i} className={CARD}>
                  <div className="flex items-center justify-between">
                    <span className={`rounded-full px-2.5 py-1 text-xs font-bold uppercase tracking-wide ${DIRECTION_BADGE[idea.direction]}`}>
                      {idea.direction.replace('_', ' ')}
                    </span>
                    <span className="text-xs text-slate-500">{formatDateTime(idea.createdAt)}</span>
                  </div>
                  <div className="mt-2 flex items-baseline gap-3">
                    <span className="text-lg font-bold text-white">{idea.conviction.toFixed(0)}</span>
                    <span className="text-sm text-slate-500">
                      {idea.direction === 'stand_aside' ? 'no entry' : `${formatUsd(idea.entry)} → ${formatUsd(idea.target)}`}
                    </span>
                  </div>
                  <div className="mt-2 space-y-1.5">
                    {splitIntoSentences(cleanRationale(idea.rationale)).map((sentence, si) => (
                      <p key={si} className="text-sm leading-relaxed text-slate-400">{sentence}</p>
                    ))}
                  </div>
                </div>
              ))}
              {visibleIdeas.length === 0 && standAsideCount > 0 && (
                <div className={`${CARD} text-center text-sm text-slate-500`}>
                  No actual trade signals yet -- {standAsideCount} stand-aside signal{standAsideCount === 1 ? '' : 's'} hidden.
                </div>
              )}
              {visibleIdeas.length === 0 && standAsideCount === 0 && (
                <div className={`${CARD} text-center text-sm text-slate-500`}>No signal history.</div>
              )}
            </div>
          </section>

          <section>
            <h2 className="mb-4 text-base font-semibold text-slate-100">
              Trade History <span className="text-sm font-normal text-slate-500">({state.outcomes.length})</span>
            </h2>
            <div className="space-y-3">
              {state.outcomes.map((o, i) => (
                <div key={i} className={CARD}>
                  <div className="flex items-center justify-between">
                    <span className={`rounded-full px-2.5 py-1 text-xs font-bold uppercase tracking-wide ${RESULT_BADGE[o.result]}`}>
                      {o.result.replace('_', ' ')}
                    </span>
                    <span className={`text-lg font-bold ${
                      o.rMultiple == null ? 'text-slate-500' : o.rMultiple >= 0 ? 'text-emerald-400' : 'text-red-400'
                    }`}>
                      {o.rMultiple != null ? `${o.rMultiple >= 0 ? '+' : ''}${o.rMultiple.toFixed(2)}R` : '—'}
                    </span>
                  </div>
                  <div className="mt-2 text-sm text-slate-400">
                    {o.direction} · entry {formatUsd(o.entry)}{o.exitPrice != null ? ` → exit ${formatUsd(o.exitPrice)}` : ''}
                  </div>
                  <div className="mt-1 text-xs text-slate-500">{formatDateTime(o.exitTime)}</div>
                  {o.note && <p className="mt-2 text-xs text-slate-500">{o.note}</p>}
                </div>
              ))}
              {state.outcomes.length === 0 && (
                <div className={`${CARD} text-center text-sm text-slate-500`}>No trade history.</div>
              )}
            </div>
          </section>
        </div>
      )}
    </div>
  )
}

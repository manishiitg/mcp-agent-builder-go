import { Trash2 } from 'lucide-react'
import type { StockGroup } from './stockGroups'
import type { TradeOutcome } from './types'

const TIER_BADGE: Record<string, string> = {
  large: 'bg-indigo-500/10 text-indigo-300',
  mid: 'bg-violet-500/10 text-violet-300',
  small: 'bg-fuchsia-500/10 text-fuchsia-300',
}

const DIRECTION_BADGE: Record<string, string> = {
  long: 'bg-emerald-500/10 text-emerald-300',
  short: 'bg-red-500/10 text-red-300',
  stand_aside: 'bg-white/5 text-slate-400',
}

const RESULT_DOT: Record<TradeOutcome['result'], string> = {
  win: 'bg-emerald-400',
  loss: 'bg-red-400',
  flat: 'bg-slate-400',
  no_fill: 'bg-slate-600',
  open: 'bg-amber-400',
  retired: 'bg-slate-600',
}

function formatUsd(value: number): string {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 }).format(value)
}

type StockTableProps = {
  groups: StockGroup[]
  onSelectSymbol: (symbol: string) => void
  onRequestRemove: (symbol: string) => void
}

export function StockTable({ groups, onSelectSymbol, onRequestRemove }: StockTableProps) {
  return (
    <div className="overflow-hidden rounded-2xl border border-white/10 bg-[#0d111c] shadow-xl shadow-black/20">
      <div className="overflow-x-auto">
        <table className="w-full min-w-[860px] text-sm">
          <thead>
            <tr className="border-b border-white/10 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
              <th className="px-5 py-3">Symbol</th>
              <th className="px-5 py-3">Signal</th>
              <th className="px-5 py-3">Conviction</th>
              <th className="px-5 py-3">Entry → Target</th>
              <th className="px-5 py-3">Position</th>
              <th className="px-5 py-3">Recent</th>
              <th className="px-5 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-white/5">
            {groups.map((group) => (
              <tr
                key={group.symbol}
                onClick={() => onSelectSymbol(group.symbol)}
                className="cursor-pointer transition hover:bg-white/[0.03]"
              >
                <td className="px-5 py-3.5">
                  <div className="flex items-center gap-2">
                    <span className="font-bold text-white">{group.symbol}</span>
                    {group.tier && (
                      <span className={`rounded-full px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide ${TIER_BADGE[group.tier]}`}>
                        {group.tier}
                      </span>
                    )}
                  </div>
                </td>
                <td className="px-5 py-3.5">
                  {group.idea ? (
                    <span className={`rounded-full px-2.5 py-1 text-xs font-bold uppercase tracking-wide ${DIRECTION_BADGE[group.idea.direction]}`}>
                      {group.idea.direction.replace('_', ' ')}
                    </span>
                  ) : (
                    <span className="text-xs text-slate-600">—</span>
                  )}
                </td>
                <td className="px-5 py-3.5 font-semibold text-white">
                  {group.idea ? group.idea.conviction.toFixed(0) : <span className="text-xs text-slate-600">—</span>}
                </td>
                <td className="px-5 py-3.5 text-xs text-slate-400">
                  {group.idea
                    ? group.idea.direction === 'stand_aside' ? 'no entry' : `${formatUsd(group.idea.entry)} → ${formatUsd(group.idea.target)}`
                    : <span className="text-slate-600">—</span>}
                </td>
                <td className="px-5 py-3.5">
                  {group.position ? (
                    <div className="text-xs">
                      <div className="text-slate-300">{group.position.qty} sh @ {formatUsd(group.position.avgEntryPrice)}</div>
                      <div className={`font-semibold ${group.position.unrealizedPl >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
                        {group.position.unrealizedPl >= 0 ? '+' : ''}{formatUsd(group.position.unrealizedPl)}
                      </div>
                    </div>
                  ) : (
                    <span className="text-xs text-slate-600">—</span>
                  )}
                </td>
                <td className="px-5 py-3.5">
                  {group.recentOutcomes.length > 0 ? (
                    <div className="flex items-center gap-1.5">
                      {group.recentOutcomes.map((o, i) => (
                        <span
                          key={i}
                          title={`${o.result} · ${o.rMultiple != null ? o.rMultiple.toFixed(2) + 'R' : 'n/a'}`}
                          className={`h-2 w-2 rounded-full ${RESULT_DOT[o.result]}`}
                        />
                      ))}
                    </div>
                  ) : (
                    <span className="text-xs text-slate-600">—</span>
                  )}
                </td>
                <td className="px-5 py-3.5 text-right">
                  {group.tier && (
                    <button
                      type="button"
                      onClick={(e) => { e.stopPropagation(); onRequestRemove(group.symbol) }}
                      className="text-slate-500 transition hover:text-red-400"
                      title={`Remove ${group.symbol} from watchlist`}
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {groups.length === 0 && (
              <tr>
                <td colSpan={7} className="px-5 py-10 text-center text-sm text-slate-500">No symbols on the watchlist yet.</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

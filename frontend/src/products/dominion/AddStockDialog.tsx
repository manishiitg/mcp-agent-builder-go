import { useState } from 'react'
import { Loader2 } from 'lucide-react'
import { Modal } from './Modal'
import type { WatchlistItem, WatchlistTier } from './types'

const TIERS: WatchlistTier[] = ['large', 'mid', 'small']

type AddStockDialogProps = {
  existing: WatchlistItem[]
  onAdd: (symbol: string, tier: WatchlistTier) => Promise<void>
  onClose: () => void
}

export function AddStockDialog({ existing, onAdd, onClose }: AddStockDialogProps) {
  const [symbol, setSymbol] = useState('')
  const [tier, setTier] = useState<WatchlistTier>('mid')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const cleaned = symbol.trim().toUpperCase()
  const isDuplicate = cleaned.length > 0 && existing.some((item) => item.symbol === cleaned)

  const handleAdd = async () => {
    if (!cleaned || isDuplicate) return
    setIsSubmitting(true)
    setError(null)
    try {
      await onAdd(cleaned, tier)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
      setIsSubmitting(false)
    }
  }

  return (
    <Modal title="Add Stock" onClose={onClose}>
      <div className="space-y-4">
        <div>
          <label className="text-xs font-semibold uppercase tracking-wide text-slate-500">Symbol</label>
          <input
            type="text"
            autoFocus
            value={symbol}
            onChange={(e) => setSymbol(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') void handleAdd() }}
            placeholder="e.g. NVDA"
            className="mt-1.5 w-full rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2 font-mono text-sm uppercase text-white placeholder-slate-600 focus:outline-none focus:ring-2 focus:ring-indigo-500/50"
          />
          {isDuplicate && <p className="mt-1.5 text-xs text-amber-400">{cleaned} is already on the watchlist.</p>}
        </div>
        <div>
          <label className="text-xs font-semibold uppercase tracking-wide text-slate-500">Tier</label>
          <select
            value={tier}
            onChange={(e) => setTier(e.target.value as WatchlistTier)}
            className="mt-1.5 w-full rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2 text-sm text-slate-200 focus:outline-none focus:ring-2 focus:ring-indigo-500/50"
          >
            {TIERS.map((t) => <option key={t} value={t}>{t}</option>)}
          </select>
        </div>
        {error && <p className="text-xs text-red-400">{error}</p>}
        <div className="flex justify-end gap-2 pt-1">
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="rounded-lg border border-white/10 px-3 py-1.5 text-xs font-medium text-slate-300 transition hover:bg-white/5 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleAdd}
            disabled={isSubmitting || !cleaned || isDuplicate}
            className="flex items-center gap-1.5 rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {isSubmitting && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            Add
          </button>
        </div>
      </div>
    </Modal>
  )
}

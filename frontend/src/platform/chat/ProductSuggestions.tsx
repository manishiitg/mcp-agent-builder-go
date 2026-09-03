import { ArrowUpRight } from 'lucide-react'
import { latestSuggestions } from '../../../shared/session/interactions'
import type { PollingEvent } from '../../../shared/session/types'

/**
 * Tappable next steps under the latest reply: the shared rendering a tool
 * binding asks for with `interaction: { kind, render: chat.suggestions }` in
 * product.yaml. Painted with the product's tokens.
 */
export function ProductSuggestions({ events, kind = 'suggestions', onSubmit, hidden }: { events: PollingEvent[]; kind?: string; onSubmit: (message: string) => void; hidden?: boolean }) {
  const actions = hidden ? [] : latestSuggestions(events, kind)
  if (actions.length === 0) return null
  return (
    <div className="flex flex-wrap gap-2 px-4 pb-3 pt-1" aria-label="Suggested next steps">
      {actions.map((a, i) => (
        <button
          key={`${i}-${a.label}`}
          type="button"
          className="group inline-flex items-center gap-1.5 rounded-full border border-primary/15 bg-primary/5 px-3.5 py-1.5 text-[13px] font-medium text-primary transition-colors hover:border-primary/30 hover:bg-primary/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          onClick={() => onSubmit(a.message)}
        >
          <span>{a.label}</span>
          <ArrowUpRight className="h-3.5 w-3.5 opacity-50 transition-opacity group-hover:opacity-90" aria-hidden="true" />
        </button>
      ))}
    </div>
  )
}

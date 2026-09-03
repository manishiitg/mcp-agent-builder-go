import { latestSuggestions } from '../../../shared/session/interactions'
import type { PollingEvent } from '../../../shared/session/types'

/**
 * Tappable next steps under the latest reply, for any product whose
 * product.yaml declares `ui_panels.suggestions` and whose tool emits the
 * `suggestions` interaction. Styled with the product's tokens.
 */
export function ProductSuggestions({ events, onSubmit, hidden }: { events: PollingEvent[]; onSubmit: (message: string) => void; hidden?: boolean }) {
  const actions = hidden ? [] : latestSuggestions(events)
  if (actions.length === 0) return null
  return (
    <div className="flex flex-wrap gap-2 px-4 pb-3 pt-1" aria-label="Suggested next steps">
      {actions.map((a, i) => (
        <button
          key={`${i}-${a.label}`}
          type="button"
          className="rounded-full border border-border bg-card px-4 py-1.5 text-sm font-semibold text-foreground shadow-sm transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          onClick={() => onSubmit(a.message)}
        >
          {a.label}
        </button>
      ))}
    </div>
  )
}

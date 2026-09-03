import { useMemo } from 'react'
import { useChatStore } from '../../stores/useChatStore'
import { parseProductInteraction, type ProductInteraction } from '../../../shared/session/interactions'

export { latestSuggestions, parseProductInteraction } from '../../../shared/session/interactions'
export type { ProductInteraction, SuggestedAction } from '../../../shared/session/interactions'

/**
 * A session's product interactions, in event order, from the same event
 * stream ChatArea keeps (no second connection) — the interaction twin of
 * usePresentationEvents. Filter by `kinds` for the ones a surface reacts to.
 */
export function useProductInteractions(sessionId: string | undefined, kinds?: string[]): ProductInteraction[] {
  const events = useChatStore((s) => (sessionId ? s.tabEvents[sessionId] : undefined))
  return useMemo(() => {
    if (!events || events.length === 0) return []
    const filter = kinds && kinds.length > 0 ? new Set(kinds) : null
    const out: ProductInteraction[] = []
    for (const e of events) {
      const it = parseProductInteraction(e)
      if (it && (!filter || filter.has(it.kind))) out.push(it)
    }
    return out
  }, [events, kinds])
}

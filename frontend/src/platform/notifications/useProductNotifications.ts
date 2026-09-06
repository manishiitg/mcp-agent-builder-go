import { useMemo } from 'react'
import { useProductInteractions } from '../interactions/useProductInteractions'

/**
 * A message the agent sent the user through notify_user. The tool delivers
 * to whatever external channels are configured and, on the same call,
 * emits the in-app copy as a product_interaction of kind `notify` — the
 * generic side channel every product.yaml UI tool uses (pins, scenes,
 * suggestions), so a surface can keep the message on screen until the
 * user dismisses it. Read from the session's event stream: it fires
 * because the tool ran, whichever coding CLI drove the turn.
 */
export type ProductNotification = {
  /** The interaction event id: stable across hydration, so a dismissal can be remembered. */
  id: string
  title: string
  message: string
}

export const NOTIFY_INTERACTION_KIND = 'notify'
const NOTIFY_KINDS = [NOTIFY_INTERACTION_KIND]

export function useProductNotifications(sessionId: string | undefined): ProductNotification[] {
  const interactions = useProductInteractions(sessionId, NOTIFY_KINDS)
  return useMemo(() => {
    const out: ProductNotification[] = []
    for (const it of interactions) {
      const message = typeof it.payload.message === 'string' ? it.payload.message.trim() : ''
      if (!it.id || !message) continue
      const title = typeof it.payload.title === 'string' ? it.payload.title.trim() : ''
      out.push({ id: it.id, title, message })
    }
    return out
  }, [interactions])
}

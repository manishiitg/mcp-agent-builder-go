// Product interactions: side-channel events a product's tools emit for its
// own surface (product_interaction). The transcript never renders them; the
// surface reads them. One kind is a platform convention, `suggestions`, so
// every product can offer tappable next steps under a reply.
import type { PollingEvent } from './types'

export const PRODUCT_INTERACTION_EVENT_TYPE = 'product_interaction'

export type ProductInteraction = { id: string; product: string; kind: string; payload: Record<string, unknown> }
export type SuggestedAction = { label: string; message: string }

export function parseProductInteraction(event: PollingEvent): ProductInteraction | null {
  const type = event.type ?? (event.data as { type?: string } | undefined)?.type
  if (type !== PRODUCT_INTERACTION_EVENT_TYPE) return null
  const p = ((event.data as { data?: Record<string, unknown> } | undefined)?.data ?? {}) as Record<string, unknown>
  const kind = typeof p.kind === 'string' ? p.kind : ''
  if (!kind) return null
  const payload = p.payload && typeof p.payload === 'object' ? (p.payload as Record<string, unknown>) : {}
  return { id: event.id, product: typeof p.product === 'string' ? p.product : '', kind, payload }
}

/** The suggestion buttons for the latest reply: the last `suggestions` interaction after the last user message. */
export function latestSuggestions(events: PollingEvent[] | undefined): SuggestedAction[] {
  let out: SuggestedAction[] = []
  for (const e of events ?? []) {
    const type = e.type ?? (e.data as { type?: string } | undefined)?.type
    if (type === 'user_message') { out = []; continue }
    const it = parseProductInteraction(e)
    if (!it || it.kind !== 'suggestions') continue
    const actions = Array.isArray(it.payload.actions) ? (it.payload.actions as { label?: unknown; message?: unknown }[]) : []
    out = actions
      .map((a) => ({ label: String(a.label ?? '').trim(), message: String(a.message ?? '').trim() }))
      .filter((a) => a.label && a.message)
  }
  return out
}

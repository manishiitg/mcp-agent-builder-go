import type { SavedLLM } from '../services/api-types'

export interface LLMChoice {
  provider: string
  model_id: string
}

export interface EffectiveLLM extends LLMChoice {
  /** True when the lock replaced the caller's choice with the published default. */
  forcedByLock: boolean
}

const norm = (value: string | undefined | null) => (value ?? '').trim().toLowerCase()

/**
 * What a workflow or chat actually runs on when the deployment's LLM settings
 * are locked. Mirrors the server (resolveLockedLLM): under LLM_CONFIG_LOCKED a
 * saved choice only counts if the published list names it; anything else runs
 * on the first published entry. The UI used to show the saved choice regardless
 * (a workflow synced from another machine kept showing claude-sonnet-5 on a
 * Cursor-only deployment), so every model label should go through this.
 */
export function effectiveLLMUnderLock(
  choice: Partial<LLMChoice> | null | undefined,
  locked: boolean,
  published: SavedLLM[],
): EffectiveLLM | null {
  const provider = choice?.provider?.trim() || ''
  const modelId = choice?.model_id?.trim() || ''
  const own: EffectiveLLM | null = provider && modelId ? { provider, model_id: modelId, forcedByLock: false } : null
  if (!locked || published.length === 0) return own
  const allowed = published.some(
    (entry) => norm(entry.provider) === norm(provider) && norm(entry.model_id) === norm(modelId),
  )
  if (own && allowed) return own
  const fallback = published[0]
  if (!fallback?.provider || !fallback?.model_id) return own
  return { provider: fallback.provider, model_id: fallback.model_id, forcedByLock: true }
}

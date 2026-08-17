import { agentApi } from '../../../services/api'
import type { VariablesManifest } from '../../../services/api-types'
import type { WatchlistItem, WatchlistTier } from '../types'

export const DOMINION_WORKSPACE_PATH = 'Workflow/tectonicusadaytrading'
const TICKERS_VARIABLE_NAME = 'TICKERS'

function parseTickers(value: string | undefined): WatchlistItem[] {
  if (!value) return []
  try {
    const parsed: unknown = JSON.parse(value)
    if (!Array.isArray(parsed)) return []
    return parsed
      .filter((item): item is { symbol: string; tier?: string } => !!item && typeof item.symbol === 'string')
      .map((item) => ({
        symbol: item.symbol.toUpperCase(),
        tier: (item.tier as WatchlistTier) ?? 'mid',
      }))
  } catch {
    return []
  }
}

export async function loadWatchlist(): Promise<WatchlistItem[]> {
  const response = await agentApi.getVariableGroups(DOMINION_WORKSPACE_PATH)
  if (!response.success) {
    throw new Error(response.error || 'Dominion: failed to load the watchlist.')
  }
  const tickersVariable = response.manifest?.variables?.find((v) => v.name === TICKERS_VARIABLE_NAME)
  return parseTickers(tickersVariable?.value)
}

// Re-fetches the manifest immediately before writing rather than accepting
// one from the caller, so an edit made elsewhere to the other variables
// (BRIEFING_EMAIL_TO/CC, BOT_NAME) between load and save isn't clobbered by
// a stale copy held in component state.
export async function saveWatchlist(items: WatchlistItem[]): Promise<void> {
  const response = await agentApi.getVariableGroups(DOMINION_WORKSPACE_PATH)
  if (!response.success || !response.manifest) {
    throw new Error(response.error || 'Dominion: failed to load the watchlist before saving.')
  }
  const serialized = JSON.stringify(items.map((item) => ({ symbol: item.symbol, tier: item.tier })))

  const manifest: VariablesManifest = {
    ...response.manifest,
    variables: (response.manifest.variables ?? []).map((v) =>
      v.name === TICKERS_VARIABLE_NAME ? { ...v, value: serialized } : v
    ),
    groups: (response.manifest.groups ?? []).map((g) => ({
      ...g,
      values: { ...g.values, [TICKERS_VARIABLE_NAME]: serialized },
    })),
  }

  const saveResponse = await agentApi.updateVariableGroups(DOMINION_WORKSPACE_PATH, manifest)
  if (!saveResponse.success) {
    throw new Error(saveResponse.message || 'Dominion: failed to save the watchlist.')
  }
}

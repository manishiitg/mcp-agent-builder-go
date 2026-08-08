import type { PollingEvent } from '../../services/api-types'

export interface ProjectAgentUsage {
  totalTokens: number
  inputTokens: number
  outputTokens: number
  cacheTokens: number
  reasoningTokens: number
  totalCostUSD: number | null
  /** `session` is provider-supplied cumulative usage for this running agent
   * session; `visible` covers only retained chat events. */
  scope: 'session' | 'visible'
  /** CLI transports without tokenizer telemetry must be shown honestly. */
  isEstimated: boolean
}

type UnknownRecord = Record<string, unknown>

function asRecord(value: unknown): UnknownRecord | null {
  return value !== null && typeof value === 'object' ? value as UnknownRecord : null
}

function finiteNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function payloadFor(event: PollingEvent): UnknownRecord | null {
  const wrapper = asRecord(event.data)
  return asRecord(wrapper?.data) ?? wrapper
}

function usageFrom(payload: UnknownRecord): ProjectAgentUsage | null {
  const info = asRecord(payload.generation_info)
  const additional = asRecord(info?.Additional) ?? asRecord(info?.additional)
  const provider = typeof payload.provider === 'string' ? payload.provider.toLowerCase() : ''
  const input = finiteNumber(info?.cumulative_prompt_tokens) ?? finiteNumber(payload.prompt_tokens) ?? 0
  const output = finiteNumber(info?.cumulative_completion_tokens) ?? finiteNumber(payload.completion_tokens) ?? 0
  const reasoning = finiteNumber(info?.cumulative_reasoning_tokens) ?? finiteNumber(payload.reasoning_tokens) ?? 0
  const cache = finiteNumber(info?.cumulative_cache_tokens) ?? 0
  const totalTokens = finiteNumber(payload.total_tokens) ?? input + output + reasoning + cache
  const totalCostUSD = finiteNumber(info?.cumulative_total_cost) ?? finiteNumber(payload.total_cost_usd) ?? finiteNumber(payload.cost_estimate)

  if (totalTokens <= 0 && totalCostUSD === null) return null
  const isEstimated = payload.token_usage_estimated === true
    || payload.usage_estimated === true
    || additional?.token_usage_estimated === true
    || additional?.usage_estimated === true
    || typeof additional?.token_usage_source === 'string'
    // Cursor's tmux transport has no tokenizer/billing telemetry. Older
    // persisted events predate the explicit marker, so keep those honest too.
    || provider === 'cursor-cli'
  return { totalTokens, inputTokens: input, outputTokens: output, cacheTokens: cache, reasoningTokens: reasoning, totalCostUSD, scope: 'visible', isEstimated }
}

/** Returns the latest cumulative usage when available, otherwise sums per-call events. */
export function summarizeProjectAgentUsage(events: PollingEvent[]): ProjectAgentUsage | null {
  const usageEvents = events
    .filter(event => event.type === 'token_usage')
    .map(event => ({ payload: payloadFor(event), cumulative: payloadFor(event)?.context === 'conversation_total' }))
    .filter((entry): entry is { payload: UnknownRecord; cumulative: boolean } => entry.payload !== null)

  // Cumulative events are emitted by the bridge separately from the provider
  // turn. Carry the provider's estimate marker forward so a cumulative event
  // cannot accidentally make Cursor's character-count fallback look exact.
  const hasEstimatedUsage = usageEvents.some((entry) => usageFrom(entry.payload)?.isEstimated)

  for (let index = usageEvents.length - 1; index >= 0; index -= 1) {
    const entry = usageEvents[index]
    if (entry.cumulative) {
      const usage = usageFrom(entry.payload)
      if (usage) return { ...usage, scope: 'session', isEstimated: usage.isEstimated || hasEstimatedUsage }
    }
  }

  let totalTokens = 0
  let inputTokens = 0
  let outputTokens = 0
  let cacheTokens = 0
  let reasoningTokens = 0
  let totalCostUSD = 0
  let hasCost = false
  let hasUsage = false
  let isEstimated = false
  for (const entry of usageEvents) {
    const usage = usageFrom(entry.payload)
    if (!usage) continue
    hasUsage = true
    isEstimated ||= usage.isEstimated
    totalTokens += usage.totalTokens
    inputTokens += usage.inputTokens
    outputTokens += usage.outputTokens
    cacheTokens += usage.cacheTokens
    reasoningTokens += usage.reasoningTokens
    if (usage.totalCostUSD !== null) {
      totalCostUSD += usage.totalCostUSD
      hasCost = true
    }
  }
  return hasUsage
    ? { totalTokens, inputTokens, outputTokens, cacheTokens, reasoningTokens, totalCostUSD: hasCost ? totalCostUSD : null, scope: 'visible', isEstimated }
    : null
}

export function formatProjectAgentTokens(tokens: number): string {
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(2).replace(/\.00$/, '')}M`
  if (tokens >= 1_000) return `${(tokens / 1_000).toFixed(1).replace(/\.0$/, '')}k`
  return String(tokens)
}

export function formatProjectAgentCost(cost: number): string {
  if (cost === 0) return '$0'
  if (cost < 0.01) return `$${cost.toFixed(4)}`
  if (cost < 1) return `$${cost.toFixed(3)}`
  return `$${cost.toFixed(2)}`
}

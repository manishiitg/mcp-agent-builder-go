import { describe, expect, it } from 'vitest'
import type { PollingEvent } from '../../services/api-types'
import { formatProjectAgentCost, formatProjectAgentTokens, summarizeProjectAgentUsage } from './projectAgentUsage'

function usage(data: Record<string, unknown>): PollingEvent {
  return { id: crypto.randomUUID(), type: 'token_usage', timestamp: '2026-08-08T00:00:00Z', data: { type: 'token_usage', data } as PollingEvent['data'] }
}

describe('summarizeProjectAgentUsage', () => {
  it('uses the latest conversation total rather than double-counting individual calls', () => {
    const result = summarizeProjectAgentUsage([
      usage({ prompt_tokens: 100, completion_tokens: 50, total_cost_usd: 0.01 }),
      usage({ context: 'conversation_total', generation_info: { cumulative_prompt_tokens: 400, cumulative_completion_tokens: 125, cumulative_total_cost: 0.045 } }),
    ])

    expect(result).toEqual({ totalTokens: 525, inputTokens: 400, outputTokens: 125, cacheTokens: 0, reasoningTokens: 0, totalCostUSD: 0.045, scope: 'session', isEstimated: false })
  })

  it('sums individual calls when no cumulative total exists', () => {
    const result = summarizeProjectAgentUsage([
      usage({ total_tokens: 100, total_cost_usd: 0.01 }),
      usage({ total_tokens: 220, total_cost_usd: 0.02 }),
    ])

    expect(result).toEqual({ totalTokens: 320, inputTokens: 0, outputTokens: 0, cacheTokens: 0, reasoningTokens: 0, totalCostUSD: 0.03, scope: 'visible', isEstimated: false })
  })

  it('labels character-count CLI usage as an estimate and never as a conversation total', () => {
    const result = summarizeProjectAgentUsage([
      usage({ prompt_tokens: 13, completion_tokens: 114, total_tokens: 127, generation_info: { Additional: { token_usage_estimated: true, token_usage_source: 'character_estimate' } } }),
    ])

    expect(result).toMatchObject({ totalTokens: 127, inputTokens: 13, outputTokens: 114, scope: 'visible', isEstimated: true })
  })

  it('treats legacy Cursor events as estimates when their explicit marker is absent', () => {
    const result = summarizeProjectAgentUsage([usage({ provider: 'cursor-cli', prompt_tokens: 10, completion_tokens: 5 })])
    expect(result).toMatchObject({ totalTokens: 15, isEstimated: true })
  })

  it('propagates an estimated provider turn to the bridge cumulative event', () => {
    const result = summarizeProjectAgentUsage([
      usage({ prompt_tokens: 10, completion_tokens: 5, generation_info: { Additional: { token_usage_estimated: true } } }),
      usage({ context: 'conversation_total', generation_info: { cumulative_prompt_tokens: 10, cumulative_completion_tokens: 5 } }),
    ])

    expect(result).toMatchObject({ totalTokens: 15, scope: 'session', isEstimated: true })
  })

  it('formats concise header values', () => {
    expect(formatProjectAgentTokens(12_500)).toBe('12.5k')
    expect(formatProjectAgentCost(0.0042)).toBe('$0.0042')
  })
})

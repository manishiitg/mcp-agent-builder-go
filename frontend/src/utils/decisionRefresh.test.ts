import { describe, expect, it } from 'vitest'
import type { PollingEvent } from '../services/api-types'
import { decisionMutationNeedsRefresh } from './decisionRefresh'

const workspace = 'Workflow/example'
function event(type = 'tool_call_end', name = 'answer_human_input_request', result = JSON.stringify({
  status: 'answered', input: { id: 'decision-1', workspace_path: workspace },
})): PollingEvent {
  return { id: 'receipt-1', type, data: { type, data: { tool_name: name, result } } } as PollingEvent
}

describe('decision receipt refresh', () => {
  it('refreshes on a saved answer before the chat turn completes', () => {
    expect(decisionMutationNeedsRefresh(event(), workspace)).toBe(true)
    expect(decisionMutationNeedsRefresh(event('tool_execution'), workspace)).toBe(true)
  })
  it('ignores discussion, tool starts, failures and unrelated tools', () => {
    expect(decisionMutationNeedsRefresh(event('tool_call_start'), workspace)).toBe(false)
    expect(decisionMutationNeedsRefresh(event('tool_call_error'), workspace)).toBe(false)
    expect(decisionMutationNeedsRefresh(event('tool_call_end', 'get_human_input_request'), workspace)).toBe(false)
    for (const result of ['Failed to save', '{}', 'null', '{"status":"failed"}']) {
      expect(decisionMutationNeedsRefresh(event('tool_call_end', 'answer_human_input_request', result), workspace)).toBe(false)
    }
  })
  it('never refreshes another workflow or an unknown workspace', () => {
    expect(decisionMutationNeedsRefresh(event(), 'Workflow/other')).toBe(false)
    expect(decisionMutationNeedsRefresh(event(), '')).toBe(false)
  })
})

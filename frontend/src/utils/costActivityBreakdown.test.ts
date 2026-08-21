import { describe, expect, it } from 'vitest'

import type { CostAggregate, CostSummary, WorkflowActivityTimingSummary } from '../services/api-types'
import { buildCostActivityBreakdown } from './costActivityBreakdown'

const cost = (amount: number, calls = 1): CostAggregate => ({
  prompt_tokens: 100,
  completion_tokens: 20,
  reasoning_tokens: 0,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  total_cost_usd: amount,
  call_count: calls,
  llm_generation_duration_ms: 1_000,
})

describe('buildCostActivityBreakdown', () => {
  it('produces the four product categories and merges chat into builder', () => {
    const summary: CostSummary = {
      total: cost(10, 5),
      by_date: {},
      by_model: {},
      by_scope: {
        builder: { ...cost(1), by_execution: { 'builder-turn': cost(1) } },
        chat: { ...cost(2), by_execution: { 'chat-turn': cost(2) } },
        pulse: { ...cost(3), by_execution: { 'bg-engineering-review': cost(3) } },
        workflow_execution: {
          ...cost(4),
          by_execution: {
            'exec-step-fetch-1786495485849093000': cost(2),
            'exec-step-fetch-1786495485849093001': cost(2),
          },
        },
      },
    }

    const timing: WorkflowActivityTimingSummary = {
      by_date: {},
      by_scope: {
        pulse: {
          duration_ms: 12_000,
          llm_duration_ms: 0,
          tool_duration_ms: 0,
          by_execution: {
            'bg-engineering-review-123': { duration_ms: 12_000, llm_duration_ms: 0, tool_duration_ms: 0 },
          },
        },
        workflow_execution: {
          duration_ms: 30_000,
          llm_duration_ms: 25_000,
          tool_duration_ms: 2_000,
          by_execution: {
            'step:fetch': { duration_ms: 30_000, llm_duration_ms: 25_000, tool_duration_ms: 2_000 },
          },
        },
      },
    }

    const result = buildCostActivityBreakdown(summary, timing)
    expect(result.map(category => category.id)).toEqual(['builder', 'pulse', 'workflow', 'evaluation'])
    expect(result[0].total.total_cost_usd).toBe(3)
    expect(result[0].executions.map(execution => execution.id)).toEqual(['interactive workflow chat', 'workflow builder'])
    expect(result[1].executions[0].label).toBe('engineering review and fix')
    expect(result[1].timing.duration_ms).toBe(12_000)
    expect(result[1].executions[0].timing.duration_ms).toBe(12_000)
    expect(result[1].total.llm_generation_duration_ms).toBe(1_000)
    expect(result[2].executions[0].label).toBe('fetch')
    expect(result[2].executions[0].cost.total_cost_usd).toBe(4)
    expect(result[2].executions[0].timing.duration_ms).toBe(30_000)
    expect(result[3].total.total_cost_usd).toBe(0)
  })

  it('joins direct workflow execution ids to persisted step timing ids', () => {
    const summary: CostSummary = {
      total: cost(4),
      by_date: {},
      by_model: {},
      by_scope: {
        workflow_execution: {
          ...cost(4),
          by_execution: { 'exec-execute-find-opportunities': cost(4) },
        },
      },
    }
    const timing: WorkflowActivityTimingSummary = {
      by_date: {},
      by_scope: {
        workflow_execution: {
          duration_ms: 30_000,
          llm_duration_ms: 25_000,
          tool_duration_ms: 2_000,
          by_execution: {
            'step:execute-find-opportunities': { duration_ms: 30_000, llm_duration_ms: 25_000, tool_duration_ms: 2_000 },
          },
        },
      },
    }

    const workflow = buildCostActivityBreakdown(summary, timing).find(category => category.id === 'workflow')!
    expect(workflow.executions).toHaveLength(1)
    expect(workflow.executions[0].cost.total_cost_usd).toBe(4)
    expect(workflow.executions[0].timing.duration_ms).toBe(30_000)
  })

  // PLAT-166
  it('carries a by_phase breakdown through into the grouped execution row', () => {
    const summary: CostSummary = {
      total: cost(1.25),
      by_date: {},
      by_model: {},
      by_scope: {
        workflow_execution: {
          ...cost(1.25),
          by_execution: {
            'exec-review-measure': {
              ...cost(1.25),
              by_phase: {
                execution_only: cost(1.0),
                reflection: cost(0.25),
              },
            },
          },
        },
      },
    }

    const workflow = buildCostActivityBreakdown(summary, null).find(category => category.id === 'workflow')!
    expect(workflow.executions).toHaveLength(1)
    const [execution] = workflow.executions
    expect(execution.cost.total_cost_usd).toBe(1.25)
    expect(execution.cost.by_phase?.execution_only.total_cost_usd).toBe(1.0)
    expect(execution.cost.by_phase?.reflection.total_cost_usd).toBe(0.25)
  })

  // PLAT-166
  it('sums by_phase per-phase when executionGroup merges dispatched retry ids', () => {
    const summary: CostSummary = {
      total: cost(2),
      by_date: {},
      by_model: {},
      by_scope: {
        workflow_execution: {
          ...cost(2),
          by_execution: {
            'exec-step-fetch-1786495485849093000': {
              ...cost(1),
              by_phase: { execution_only: cost(0.8), reflection: cost(0.2) },
            },
            'exec-step-fetch-1786495485849093001': {
              ...cost(1),
              by_phase: { execution_only: cost(0.7), reflection: cost(0.3) },
            },
          },
        },
      },
    }

    const workflow = buildCostActivityBreakdown(summary, null).find(category => category.id === 'workflow')!
    expect(workflow.executions).toHaveLength(1)
    const [execution] = workflow.executions
    expect(execution.cost.total_cost_usd).toBe(2)
    expect(execution.cost.by_phase?.execution_only.total_cost_usd).toBeCloseTo(1.5)
    expect(execution.cost.by_phase?.reflection.total_cost_usd).toBeCloseTo(0.5)
  })

  // PLAT-166
  it('omits by_phase for an execution that never carried one', () => {
    const summary: CostSummary = {
      total: cost(1),
      by_date: {},
      by_model: {},
      by_scope: {
        chat: { ...cost(1), by_execution: { 'chat-turn': cost(1) } },
      },
    }

    const builder = buildCostActivityBreakdown(summary, null).find(category => category.id === 'builder')!
    expect(builder.executions[0].cost.by_phase).toBeUndefined()
  })
})

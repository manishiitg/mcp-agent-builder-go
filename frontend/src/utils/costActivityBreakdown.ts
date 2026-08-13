import type { CostAggregate, CostSummary, WorkflowActivityTimingAggregate, WorkflowActivityTimingSummary } from '../services/api-types'

export interface ActivityTiming {
  duration_ms: number
  llm_duration_ms: number
  tool_duration_ms: number
}

export interface CostActivityCategory {
  id: 'builder' | 'pulse' | 'workflow' | 'evaluation'
  label: string
  description: string
  total: CostAggregate
  timing: ActivityTiming
  executions: Array<{ id: string; label: string; cost: CostAggregate; timing: ActivityTiming }>
}

const emptyCost = (): CostAggregate => ({
  prompt_tokens: 0,
  completion_tokens: 0,
  reasoning_tokens: 0,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  total_cost_usd: 0,
  call_count: 0,
  llm_generation_duration_ms: 0,
})

const addCost = (target: CostAggregate, source?: Partial<CostAggregate>) => {
  if (!source) return
  target.prompt_tokens += source.prompt_tokens || 0
  target.completion_tokens += source.completion_tokens || 0
  target.reasoning_tokens += source.reasoning_tokens || 0
  target.cache_read_tokens += source.cache_read_tokens || 0
  target.cache_write_tokens += source.cache_write_tokens || 0
  target.total_cost_usd += source.total_cost_usd || 0
  target.call_count += source.call_count || 0
  target.llm_generation_duration_ms = (target.llm_generation_duration_ms || 0) + (source.llm_generation_duration_ms || 0)
}

const emptyTiming = (): ActivityTiming => ({ duration_ms: 0, llm_duration_ms: 0, tool_duration_ms: 0 })

const addTiming = (target: ActivityTiming, source?: Partial<WorkflowActivityTimingAggregate>) => {
  if (!source) return
  target.duration_ms += source.duration_ms || 0
  target.llm_duration_ms += source.llm_duration_ms || 0
  target.tool_duration_ms += source.tool_duration_ms || 0
}

const executionLabel = (id: string) => {
  if (id === 'unattributed') return 'Unattributed work'
  if (id.startsWith('session:')) return 'Interactive session'
  if (id.startsWith('step:')) return id.slice('step:'.length).replace(/[-_]+/g, ' ')
  if (id.startsWith('evaluation:')) return id.slice('evaluation:'.length).replace(/[-_]+/g, ' ')
  return id
    .replace(/^(exec|bg|eval-full|query)[-_:]+/i, '')
    .replace(/[-_]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim() || id
}

const executionGroup = (category: CostActivityCategory['id'], scope: string, id: string) => {
  if (category === 'builder') {
    return scope === 'chat' ? 'interactive workflow chat' : 'workflow builder'
  }
  if (category === 'workflow') {
    const canonicalStep = (value: string) => value.replace(/^step:/, '').replace(/^step-/, '')
    if (id.startsWith('step:')) return `step:${canonicalStep(id)}`
    const dispatchedStep = id.match(/^exec-step-(.+)-\d{16,}$/)?.[1]
    if (dispatchedStep) return `step:${canonicalStep(dispatchedStep)}`
    const directStep = id.match(/^exec-(.+?)(?:-\d{16,})?$/)?.[1]
    return directStep ? `step:${canonicalStep(directStep)}` : id.replace(/-\d{16,}$/, '')
  }
  if (category === 'evaluation') {
    const run = id.match(/^eval-full-(.+)-\d{16,}$/)?.[1]
    return run ? `evaluation:${run}` : id.replace(/-\d{16,}$/, '')
  }
  if (category === 'pulse') {
    if (/engineering-review|workflow-review/i.test(id)) return 'engineering review and fix'
    if (/strategy-auditor/i.test(id)) return 'strategy auditor'
    if (/goal-advisor/i.test(id)) return 'goal advisor'
    if (/pulse-fixer|fixer/i.test(id)) return 'pulse fixer'
    if (/stores-health/i.test(id)) return 'stores health'
    if (/^query[_-]/i.test(id)) return 'pulse main turn'
  }
  return id.replace(/-\d{16,}$/, '')
}

const definitions: Array<Omit<CostActivityCategory, 'total' | 'timing' | 'executions'> & { scopes: string[] }> = [
  {
    id: 'builder',
    label: 'Builder',
    description: 'Workflow building and interactive workflow chat',
    scopes: ['builder', 'chat'],
  },
  {
    id: 'pulse',
    label: 'Pulse',
    description: 'Includes Pulse background agents: reviews, fixes, strategy, and goal advice',
    scopes: ['pulse'],
  },
  {
    id: 'workflow',
    label: 'Workflow',
    description: 'Production runs and their individual steps',
    scopes: ['workflow_execution'],
  },
  {
    id: 'evaluation',
    label: 'Evaluation',
    description: 'Evaluation runs and scoring steps',
    scopes: ['evaluation'],
  },
]

export const buildCostActivityBreakdown = (
  summary?: Pick<CostSummary, 'by_scope'> | null,
  timingSummary?: Pick<WorkflowActivityTimingSummary, 'by_scope'> | null,
): CostActivityCategory[] => {
  const byScope = summary?.by_scope || {}
  const timingByScope = timingSummary?.by_scope || {}
  return definitions.map(definition => {
    const total = emptyCost()
    const timing = emptyTiming()
    const byExecution = new Map<string, CostAggregate>()
    const timingByExecution = new Map<string, ActivityTiming>()

    for (const scopeName of definition.scopes) {
      const scope = byScope[scopeName]
      addCost(total, scope)
      for (const [executionID, cost] of Object.entries(scope?.by_execution || {})) {
        const groupID = executionGroup(definition.id, scopeName, executionID)
        const aggregate = byExecution.get(groupID) || emptyCost()
        addCost(aggregate, cost)
        byExecution.set(groupID, aggregate)
      }
      const scopeTiming = timingByScope[scopeName]
      addTiming(timing, scopeTiming)
      for (const [executionID, executionTiming] of Object.entries(scopeTiming?.by_execution || {})) {
        const groupID = executionGroup(definition.id, scopeName, executionID)
        const aggregate = timingByExecution.get(groupID) || emptyTiming()
        addTiming(aggregate, executionTiming)
        timingByExecution.set(groupID, aggregate)
      }
    }

    const executionIDs = new Set([...byExecution.keys(), ...timingByExecution.keys()])
    const executions = Array.from(executionIDs)
      .map(id => ({
        id,
        label: executionLabel(id),
        cost: byExecution.get(id) || emptyCost(),
        timing: timingByExecution.get(id) || emptyTiming(),
      }))
      .sort((a, b) => b.cost.total_cost_usd - a.cost.total_cost_usd || b.timing.duration_ms - a.timing.duration_ms)

    return { ...definition, total, timing, executions }
  })
}

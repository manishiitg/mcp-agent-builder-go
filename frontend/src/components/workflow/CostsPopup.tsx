import React, { useEffect, useState, useMemo, useRef } from 'react'
import {
  X,
  Loader2,
  ChevronRight,
  ChevronDown,
  AlertCircle,
  DollarSign,
  Coins,
  List,
  Award,
  TrendingUp,
  TrendingDown,
  RefreshCw,
} from 'lucide-react'
import { agentApi } from '../../services/api'
import { formatStartedAt } from '../../utils/duration'
import {
  classifyPhase,
  formatPhaseTitle,
} from '../../utils/dailyCostBreakdown'
import { buildCostActivityBreakdown, phaseLabel as costPhaseLabel } from '../../utils/costActivityBreakdown'
import type {
  CostSummary,
  TokenUsageFile,
  StepExecutionLogs,
  PhaseTokenUsageFile,
  WorkflowRunCostsEntry,
  WorkflowPhaseDailyCostsEntry,
  WorkflowRunDailyCostsEntry,
  WorkflowActivityTimingSummary,
} from '../../services/api-types'
import ModalPortal from '../ui/ModalPortal'

interface CostsPopupProps {
  isOpen: boolean
  onClose: () => void
  workspacePath: string | null
  runFolders: string[] // Available run folders
  selectedRunFolder: string | null // Currently selected run folder
  startedAt?: string | null
  embedded?: boolean
}

// Format cost in USD
const formatUSD = (amount?: number) => {
  if (amount === undefined || amount === null) return '$0.00'
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 4,
    maximumFractionDigits: 4
  }).format(amount)
}

// Format token count (e.g., 1,234,567 -> 1.23M)
const formatTokens = (count?: number) => {
  if (!count) return '0'
  if (count >= 1000000) {
    return (count / 1000000).toFixed(2) + 'M'
  }
  if (count >= 1000) {
    return (count / 1000).toFixed(1) + 'K'
  }
  return count.toString()
}

const formatDuration = (milliseconds?: number) => {
  if (!milliseconds || milliseconds <= 0) return '—'
  const seconds = Math.round(milliseconds / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remainingSeconds = seconds % 60
  if (minutes < 60) return remainingSeconds ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`
  const hours = Math.floor(minutes / 60)
  const remainingMinutes = minutes % 60
  return remainingMinutes ? `${hours}h ${remainingMinutes}m` : `${hours}h`
}

interface RunCosts {
  runFolder: string
  tokenUsage: TokenUsageFile | null
  evaluationTokenUsage?: TokenUsageFile | null
  steps?: Record<string, StepExecutionLogs> // Store steps for title lookup
  costSummary: {
    totalCost: number
    totalLLMCost: number
    totalToolCost: number
    totalInputTokens: number
    totalOutputTokens: number
    totalTokens: number
    totalLLMCalls: number
    totalCacheReadTokens: number
    totalCacheWriteTokens: number
    totalReasoningTokens: number
    stageCosts: {
      execution: number
      learning: number
      reflection: number
      evaluation: number
      knowledgebase: number   // kb_update / kb_reorganize / kb_consolidate
      routing: number         // deterministic routing / todo_task orchestration
      workshop: number        // review_step_code / goal advisor / other workshop agents
      other: number
    }
    stepCosts: Array<{
      stepID: string        // Step ID (e.g., "fetch-pr-data" or phase name for phase-only agents)
      stepTitle: string     // Display title
      stepNum: number       // Step number (for sorting, 0 for non-step entries)
      execution: number
      learning: number
      reflection: number
      evaluation: number
      knowledgebase: number
      routing: number
      workshop: number
      totalCost: number
      inputTokens: number
      outputTokens: number
      llmCalls: number
    }>
  } | null
}

interface PhaseCostSummary {
  totalCost: number
  totalInputTokens: number
  totalOutputTokens: number
  totalTokens: number
  totalLLMCalls: number
  totalCacheReadTokens: number
  totalCacheWriteTokens: number
  totalReasoningTokens: number
  createdAt: string | null
  updatedAt: string | null
  phaseCosts: Array<{
    phaseID: string
    phaseTitle: string
    totalCost: number
    inputTokens: number
    outputTokens: number
    llmCalls: number
  }>
  modelCosts: Array<{
    modelID: string
    provider: string
    totalCost: number
    inputTokens: number
    outputTokens: number
    llmCalls: number
  }>
}

interface PhaseDailyCostSummaryEntry {
  date: string
  tokenUsage: PhaseTokenUsageFile
  summary: PhaseCostSummary
}

interface RunDailyCostSummaryEntry {
  date: string
  scope: string
  groupFolder: string
  runFolder: string
  updatedAt: string | null
  tokenUsage: TokenUsageFile
  summary: NonNullable<RunCosts['costSummary']>
}

interface CombinedDailyCostSummaryEntry {
  date: string
  workflowCost: number
  evaluationCost: number
  builderCost: number
  pulseCost: number | null
  totalCost: number
  totalTokens: number
  llmDurationMS: number
  runCount: number
}

const getRunFolderDisplayName = (runFolder: string) => {
  // Show "iteration-N · group" so both dimensions are visible. Single-segment
  // values (legacy layout, or iteration-only) fall through unchanged.
  const parts = runFolder.split('/').filter(Boolean)
  if (parts.length >= 2) {
    return `${parts[0]} · ${parts.slice(1).join('/')}`
  }
  return parts[parts.length - 1] || runFolder
}

// classifyPhase maps a backend phase name to one of the UI stage buckets.
// Order matters: check specific prefixes before falling through to includes().
// Known phases (see comment in calculateCostSummary):
//   execution_only, success_learning, failure_learning,
//   routing, todo_task, kb_update, kb_reorganize,
//   kb_consolidate, review_step_code,
//   Goal Advisor proposal/application work, evaluation_scoring.
const getRunTimestamp = (runCost: Pick<RunCosts, 'tokenUsage' | 'evaluationTokenUsage'>) => {
  const timestamp =
    runCost.tokenUsage?.updated_at ||
    runCost.evaluationTokenUsage?.updated_at ||
    runCost.tokenUsage?.created_at ||
    runCost.evaluationTokenUsage?.created_at

  if (!timestamp) return null

  const parsed = new Date(timestamp)
  if (Number.isNaN(parsed.getTime())) return null
  return parsed
}

const formatRunTimestampLabel = (runCost: Pick<RunCosts, 'tokenUsage' | 'evaluationTokenUsage'>) => {
  const timestamp = getRunTimestamp(runCost)
  if (!timestamp) return ''

  return new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit'
  }).format(timestamp)
}

const formatTimestampLabel = (timestamp?: string | null) => {
  if (!timestamp) return ''
  const parsed = new Date(timestamp)
  if (Number.isNaN(parsed.getTime())) return ''

  return new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit'
  }).format(parsed)
}

const compareRunCosts = (a: RunCosts, b: RunCosts, selectedRunFolder: string | null) => {
  if (selectedRunFolder) {
    if (a.runFolder === selectedRunFolder && b.runFolder !== selectedRunFolder) return -1
    if (b.runFolder === selectedRunFolder && a.runFolder !== selectedRunFolder) return 1
  }

  const timestampA = getRunTimestamp(a)
  const timestampB = getRunTimestamp(b)
  if (timestampA && timestampB && timestampA.getTime() !== timestampB.getTime()) {
    return timestampB.getTime() - timestampA.getTime()
  }
  if (timestampA && !timestampB) return -1
  if (!timestampA && timestampB) return 1

  const displayCompare = getRunFolderDisplayName(a.runFolder).localeCompare(getRunFolderDisplayName(b.runFolder))
  if (displayCompare !== 0) return displayCompare

  return b.runFolder.localeCompare(a.runFolder)
}

const getRunFolderSecondaryLabel = (runCost: RunCosts) => {
  const timestampLabel = formatRunTimestampLabel(runCost)
  if (timestampLabel) return timestampLabel

  const displayName = getRunFolderDisplayName(runCost.runFolder)
  return displayName === runCost.runFolder ? '' : runCost.runFolder
}

const getRunFolderTitle = (runCost: RunCosts) => {
  const secondary = getRunFolderSecondaryLabel(runCost)
  return secondary ? `${runCost.runFolder}\n${secondary}` : runCost.runFolder
}

const getRunBadgeLabel = (runCost: RunCosts) => {
  const timestamp = getRunTimestamp(runCost)
  if (timestamp) {
    return new Intl.DateTimeFormat('en-US', {
      month: 'short',
      day: 'numeric'
    }).format(timestamp)
  }

  return 'Run'
}

const CostsPopup: React.FC<CostsPopupProps> = ({
  isOpen,
  onClose,
  workspacePath,
  selectedRunFolder,
  startedAt,
  embedded = false,
}) => {
  const [loading, setLoading] = useState(false)
  const [runCosts, setRunCosts] = useState<RunCosts[]>([])
  const [phaseCostSummary, setPhaseCostSummary] = useState<PhaseCostSummary | null>(null)
  const [phaseDailyCostSummaries, setPhaseDailyCostSummaries] = useState<PhaseDailyCostSummaryEntry[]>([])
  const [runDailyCostSummaries, setRunDailyCostSummaries] = useState<RunDailyCostSummaryEntry[]>([])
  const [scopedCosts, setScopedCosts] = useState<CostSummary | null>(null)
  const [activityTiming, setActivityTiming] = useState<WorkflowActivityTimingSummary | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [expandedRunFolders, setExpandedRunFolders] = useState<Set<string>>(new Set())
  const [expandedCostModels, setExpandedCostModels] = useState<Set<string>>(new Set())
  const [costViewMode, setCostViewMode] = useState<Record<string, 'step' | 'model'>>({})
  const [expandedDailyDate, setExpandedDailyDate] = useState<string | null>(null)
  const [costHistory, setCostHistory] = useState<{ hasMore: boolean; nextBefore?: string } | null>(null)
  const [loadingOlder, setLoadingOlder] = useState(false)
  const loadGenerationRef = useRef(0)
  const activityBreakdown = useMemo(() => buildCostActivityBreakdown(scopedCosts, activityTiming), [scopedCosts, activityTiming])
  const hasScopedActivity = Object.keys(scopedCosts?.by_scope || {}).length > 0

  // Calculate cost summary from token usage
  const calculateCostSummary = (tokenUsage: TokenUsageFile | null, evaluationTokenUsage: TokenUsageFile | null | undefined, steps?: Record<string, StepExecutionLogs>): RunCosts['costSummary'] => {
    if (!tokenUsage?.by_model && !tokenUsage?.by_tool && !evaluationTokenUsage?.by_model && !evaluationTokenUsage?.by_tool) return null

    let totalCost = 0
    let totalLLMCost = 0
    let totalToolCost = 0
    let totalInputTokens = 0
    let totalOutputTokens = 0
    let totalLLMCalls = 0
    let totalCacheReadTokens = 0
    let totalCacheWriteTokens = 0
    let totalReasoningTokens = 0

    const stageCosts = {
      execution: 0,
      learning: 0,
      reflection: 0,
      evaluation: 0,
      knowledgebase: 0,
      routing: 0,
      workshop: 0,
      other: 0
    }

    const stepCosts: Record<string, {
      stepID: string
      stepNum: number
      stepTitle: string
      execution: number
      learning: number
      reflection: number
      evaluation: number
      knowledgebase: number
      routing: number
      workshop: number
      totalCost: number
      inputTokens: number
      outputTokens: number
      llmCalls: number
    }> = {}

    // Calculate totals from by_model
    if (tokenUsage?.by_model) {
      Object.values(tokenUsage.by_model).forEach(usage => {
        totalCost += usage.total_cost_usd || 0
        totalLLMCost += usage.total_cost_usd || 0
        totalInputTokens += usage.input_tokens || 0
        totalOutputTokens += usage.output_tokens || 0
        totalLLMCalls += usage.llm_call_count || 0
        totalCacheReadTokens += usage.cache_read_tokens || usage.cache_tokens || 0
        totalCacheWriteTokens += usage.cache_write_tokens || 0
        totalReasoningTokens += usage.reasoning_tokens || 0
      })
    }

    if (tokenUsage?.by_tool) {
      Object.values(tokenUsage.by_tool).forEach(usage => {
        const cost = usage.total_cost_usd || 0
        totalCost += cost
        totalToolCost += cost
      })
      if (!tokenUsage.by_step_and_tool) {
        stageCosts.execution += Object.values(tokenUsage.by_tool).reduce((sum, usage) => sum + (usage.total_cost_usd || 0), 0)
      }
    }

    // Helper to find step number and title from stepID
    const findStepInfo = (stepID: string): { stepNum: number, stepTitle: string } => {
      // Try to find the step in the steps data by matching the step ID
      if (steps) {
        for (const [key, stepData] of Object.entries(steps)) {
          if (stepData.step_id === stepID) {
            // Extract step number from key (e.g., "step-1" -> 1)
            const match = key.match(/step-(\d+)/)
            const stepNum = match ? parseInt(match[1], 10) : 0
            return { stepNum, stepTitle: stepData.title || stepID }
          }
        }
      }
      // If not found, it might be a phase-only agent (use phase name as display)
      return { stepNum: 0, stepTitle: stepID }
    }

    // Calculate stage costs and step-wise costs from by_step_and_model
    if (tokenUsage?.by_step_and_model) {
      Object.entries(tokenUsage.by_step_and_model).forEach(([key, modelMap]) => {
        const parts = key.split(':')
        const phase = parts[0]
        const stepID = parts[1] || ''  // New format: stepID instead of index

        let cost = 0
        let inputTokens = 0
        let outputTokens = 0
        let llmCalls = 0
        Object.values(modelMap).forEach(u => {
          cost += u.total_cost_usd || 0
          inputTokens += u.input_tokens || 0
          outputTokens += u.output_tokens || 0
          llmCalls += u.llm_call_count || 0
        })

        // Stage dispatch — must stay in sync with the Go phases that actually emit
        // token usage. See controller_execution.go (execution_only), controller_learning.go
        // (success_learning/failure_learning), deterministic routing,
        // controller_todo_task.go (todo_task), controller_kb_update.go (kb_*),
        // interactive_workshop_manager.go (review_step_code /
        // Goal Advisor proposal/application work), evaluation_scoring agents.
        const stageBucket = classifyPhase(phase)
        stageCosts[stageBucket] += cost

        // Step-wise costs - group by stepID
        const { stepNum, stepTitle } = findStepInfo(stepID)
        const stepKey = stepID  // Use stepID as the key

        if (!stepCosts[stepKey]) {
          stepCosts[stepKey] = {
            stepID,
            stepNum,
            stepTitle,
            execution: 0,
            learning: 0,
            reflection: 0,
            evaluation: 0,
            knowledgebase: 0,
            routing: 0,
            workshop: 0,
            totalCost: 0,
            inputTokens: 0,
            outputTokens: 0,
            llmCalls: 0
          }
        }
        stepCosts[stepKey].totalCost += cost
        stepCosts[stepKey].inputTokens += inputTokens
        stepCosts[stepKey].outputTokens += outputTokens
        stepCosts[stepKey].llmCalls += llmCalls
        if (stageBucket !== 'other') {
          stepCosts[stepKey][stageBucket] += cost
        }
      })
    }

    if (tokenUsage?.by_step_and_tool) {
      Object.entries(tokenUsage.by_step_and_tool).forEach(([key, toolMap]) => {
        const parts = key.split(':')
        const phase = parts[0]
        const stepID = parts[1] || ''
        const cost = Object.values(toolMap).reduce((sum, u) => sum + (u.total_cost_usd || 0), 0)
        const stageBucket = classifyPhase(phase)
        stageCosts[stageBucket] += cost
        const { stepNum, stepTitle } = findStepInfo(stepID)
        const stepKey = stepID || key
        if (!stepCosts[stepKey]) {
          stepCosts[stepKey] = {
            stepID: stepKey,
            stepNum,
            stepTitle: stepTitle || stepKey,
            execution: 0,
            learning: 0,
            reflection: 0,
            evaluation: 0,
            knowledgebase: 0,
            routing: 0,
            workshop: 0,
            totalCost: 0,
            inputTokens: 0,
            outputTokens: 0,
            llmCalls: 0
          }
        }
        stepCosts[stepKey].totalCost += cost
        if (stageBucket !== 'other') {
          stepCosts[stepKey][stageBucket] += cost
        }
      })
    }

    // Process evaluation token usage
    if (evaluationTokenUsage?.by_model) {
      Object.values(evaluationTokenUsage.by_model).forEach(usage => {
        totalCost += usage.total_cost_usd || 0
        totalLLMCost += usage.total_cost_usd || 0
        totalInputTokens += usage.input_tokens || 0
        totalOutputTokens += usage.output_tokens || 0
        totalLLMCalls += usage.llm_call_count || 0
        totalCacheReadTokens += usage.cache_read_tokens || usage.cache_tokens || 0
        totalCacheWriteTokens += usage.cache_write_tokens || 0
        totalReasoningTokens += usage.reasoning_tokens || 0
        // All evaluation by_model costs go to evaluation stage
        stageCosts.evaluation += usage.total_cost_usd || 0
      })
    }

    if (evaluationTokenUsage?.by_tool) {
      const evalToolCost = Object.values(evaluationTokenUsage.by_tool).reduce((sum, usage) => sum + (usage.total_cost_usd || 0), 0)
      totalCost += evalToolCost
      totalToolCost += evalToolCost
      if (!evaluationTokenUsage.by_step_and_tool) {
        stageCosts.evaluation += evalToolCost
      }
    }

    // Process evaluation step-wise costs
    if (evaluationTokenUsage?.by_step_and_model) {
      Object.entries(evaluationTokenUsage.by_step_and_model).forEach(([key, modelMap]) => {
        const parts = key.split(':')
        const stepID = parts[1] || parts[0]  // Use stepID from phase:stepID format

        let cost = 0
        let inputTokens = 0
        let outputTokens = 0
        let llmCalls = 0
        Object.values(modelMap).forEach(u => {
          cost += u.total_cost_usd || 0
          inputTokens += u.input_tokens || 0
          outputTokens += u.output_tokens || 0
          llmCalls += u.llm_call_count || 0
        })

        // Step-wise costs - group by stepID with "eval-" prefix
        const { stepNum, stepTitle } = findStepInfo(stepID)
        const stepKey = `eval-${stepID}`  // Prefix with eval- to distinguish from regular steps

        if (!stepCosts[stepKey]) {
          stepCosts[stepKey] = {
            stepID: stepKey,
            stepNum: stepNum > 0 ? stepNum + 1000 : 0, // Put eval steps after regular steps
            stepTitle: `[Eval] ${stepTitle}`,
            execution: 0,
            learning: 0,
            reflection: 0,
            evaluation: 0,
            knowledgebase: 0,
            routing: 0,
            workshop: 0,
            totalCost: 0,
            inputTokens: 0,
            outputTokens: 0,
            llmCalls: 0
          }
        }
        stepCosts[stepKey].totalCost += cost
        stepCosts[stepKey].inputTokens += inputTokens
        stepCosts[stepKey].outputTokens += outputTokens
        stepCosts[stepKey].llmCalls += llmCalls
        stepCosts[stepKey].evaluation += cost
      })
    }

    if (evaluationTokenUsage?.by_step_and_tool) {
      Object.entries(evaluationTokenUsage.by_step_and_tool).forEach(([key, toolMap]) => {
        const parts = key.split(':')
        const stepID = parts[1] || parts[0]
        const cost = Object.values(toolMap).reduce((sum, u) => sum + (u.total_cost_usd || 0), 0)
        const { stepNum, stepTitle } = findStepInfo(stepID)
        const stepKey = `eval-${stepID}`
        if (!stepCosts[stepKey]) {
          stepCosts[stepKey] = {
            stepID: stepKey,
            stepNum: stepNum > 0 ? stepNum + 1000 : 0,
            stepTitle: `[Eval] ${stepTitle}`,
            execution: 0,
            learning: 0,
            reflection: 0,
            evaluation: 0,
            knowledgebase: 0,
            routing: 0,
            workshop: 0,
            totalCost: 0,
            inputTokens: 0,
            outputTokens: 0,
            llmCalls: 0
          }
        }
        stageCosts.evaluation += cost
        stepCosts[stepKey].totalCost += cost
        stepCosts[stepKey].evaluation += cost
      })
    }

    // Sort by step number, then by stepID
    const sortedStepCosts = Object.values(stepCosts).sort((a, b) => {
      if (a.stepNum !== b.stepNum) return a.stepNum - b.stepNum
      return a.stepID.localeCompare(b.stepID)
    })

    return {
      totalCost,
      totalLLMCost,
      totalToolCost,
      totalInputTokens,
      totalOutputTokens,
      totalTokens: totalInputTokens + totalOutputTokens,
      totalLLMCalls,
      totalCacheReadTokens,
      totalCacheWriteTokens,
      totalReasoningTokens,
      stageCosts,
      stepCosts: sortedStepCosts
    }
  }

  const calculatePhaseCostSummary = (tokenUsage: PhaseTokenUsageFile | null): PhaseCostSummary | null => {
    if (!tokenUsage?.by_model) return null

    let totalCost = 0
    let totalInputTokens = 0
    let totalOutputTokens = 0
    let totalLLMCalls = 0
    let totalCacheReadTokens = 0
    let totalCacheWriteTokens = 0
    let totalReasoningTokens = 0

    Object.values(tokenUsage.by_model).forEach(usage => {
      totalCost += usage.total_cost_usd || 0
      totalInputTokens += usage.input_tokens || 0
      totalOutputTokens += usage.output_tokens || 0
      totalLLMCalls += usage.llm_call_count || 0
      totalCacheReadTokens += usage.cache_read_tokens || usage.cache_tokens || 0
      totalCacheWriteTokens += usage.cache_write_tokens || 0
      totalReasoningTokens += usage.reasoning_tokens || 0
    })

    const phaseCosts = Object.entries(tokenUsage.by_phase_and_model || {})
      .map(([phaseID, modelMap]) => {
        let cost = 0
        let inputTokens = 0
        let outputTokens = 0
        let llmCalls = 0

        Object.values(modelMap).forEach(usage => {
          cost += usage.total_cost_usd || 0
          inputTokens += usage.input_tokens || 0
          outputTokens += usage.output_tokens || 0
          llmCalls += usage.llm_call_count || 0
        })

        return {
          phaseID,
          phaseTitle: formatPhaseTitle(phaseID),
          totalCost: cost,
          inputTokens,
          outputTokens,
          llmCalls
        }
      })
      .sort((a, b) => {
        if (b.totalCost !== a.totalCost) return b.totalCost - a.totalCost
        return a.phaseTitle.localeCompare(b.phaseTitle)
      })

    const modelCosts = Object.entries(tokenUsage.by_model || {})
      .map(([modelID, usage]) => ({
        modelID,
        provider: usage.provider || 'unknown',
        totalCost: usage.total_cost_usd || 0,
        inputTokens: usage.input_tokens || 0,
        outputTokens: usage.output_tokens || 0,
        llmCalls: usage.llm_call_count || 0
      }))
      .sort((a, b) => {
        if (b.totalCost !== a.totalCost) return b.totalCost - a.totalCost
        return a.modelID.localeCompare(b.modelID)
      })

    return {
      totalCost,
      totalInputTokens,
      totalOutputTokens,
      totalTokens: totalInputTokens + totalOutputTokens,
      totalLLMCalls,
      totalCacheReadTokens,
      totalCacheWriteTokens,
      totalReasoningTokens,
      createdAt: tokenUsage.created_at || null,
      updatedAt: tokenUsage.updated_at || null,
      phaseCosts,
      modelCosts
    }
  }

  // Load costs for all workflow runs
  useEffect(() => {
    if ((isOpen || embedded) && workspacePath) {
      loadAllCosts()
    } else {
      setRunCosts([])
      setPhaseCostSummary(null)
      setPhaseDailyCostSummaries([])
      setRunDailyCostSummaries([])
      setScopedCosts(null)
      setActivityTiming(null)
      setError(null)
      setExpandedRunFolders(new Set())
      setExpandedCostModels(new Set())
      setCostViewMode({})
      setExpandedDailyDate(null)
      setCostHistory(null)
      setLoadingOlder(false)
    }
    return () => {
      loadGenerationRef.current += 1
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, embedded, workspacePath])

  // Auto-expand selected run folder when it changes
  useEffect(() => {
    if ((isOpen || embedded) && selectedRunFolder && runCosts.some(c => c.runFolder === selectedRunFolder)) {
      setExpandedRunFolders(prev => {
        if (prev.has(selectedRunFolder!)) return prev
        const next = new Set(prev)
        next.add(selectedRunFolder!)
        return next
      })
    }
  }, [isOpen, embedded, selectedRunFolder, runCosts])

  const loadAllCosts = async () => {
    if (!workspacePath) return

    const generation = ++loadGenerationRef.current
    setLoading(true)
    setError(null)
    try {
      const summaryResponse = await agentApi.getCosts(workspacePath, { view: 'summary', days: 30 })
      if (generation !== loadGenerationRef.current) return
      const summaryScopes = summaryResponse.scoped_costs?.by_scope || {}
      const hasAuthoritativeCosts = ['builder', 'chat', 'pulse', 'workflow_execution', 'evaluation']
        .some(scope => !!summaryScopes[scope])
      if (hasAuthoritativeCosts) {
        setScopedCosts(summaryResponse.scoped_costs ?? null)
        setActivityTiming(summaryResponse.activity_timing ?? null)
        setRunCosts([])
        setPhaseCostSummary(null)
        setPhaseDailyCostSummaries([])
        setRunDailyCostSummaries([])
        setCostHistory({
          hasMore: summaryResponse.history?.has_more ?? false,
          nextBefore: summaryResponse.history?.next_before,
        })
        setExpandedRunFolders(new Set())
        return
      }

      // Compatibility fallback for old workspaces that predate the canonical
      // event ledger. This deliberately remains off the normal first-paint path.
      const costsResponse = await agentApi.getCosts(workspacePath)
      if (generation !== loadGenerationRef.current) return
      setScopedCosts(costsResponse.scoped_costs ?? null)
      setActivityTiming(costsResponse.activity_timing ?? null)
      setCostHistory(null)
      const costEntriesByRunFolder = new Map<string, WorkflowRunCostsEntry>(
        (costsResponse.runs || []).map(entry => [entry.run_folder, entry])
      )
      let nextPhaseCostSummary: PhaseCostSummary | null = null
      let nextPhaseDailyCostSummaries: PhaseDailyCostSummaryEntry[] = []
      let nextRunDailyCostSummaries: RunDailyCostSummaryEntry[] = []
      const costs: RunCosts[] = []

      nextPhaseCostSummary = calculatePhaseCostSummary(costsResponse.phase_token_usage ?? null)
      nextPhaseDailyCostSummaries = (costsResponse.phase_daily_costs || [])
        .map((entry: WorkflowPhaseDailyCostsEntry) => {
          if (!entry.token_usage) return null
          const summary = calculatePhaseCostSummary(entry.token_usage ?? null)
          if (!summary) return null
          return {
            date: entry.date,
            tokenUsage: entry.token_usage,
            summary
          }
        })
        .filter((entry): entry is PhaseDailyCostSummaryEntry => entry !== null)
        .sort((a, b) => b.date.localeCompare(a.date))

      const runDailyEntriesByKey = new Map<string, WorkflowRunDailyCostsEntry>()
      ;(costsResponse.run_daily_costs || []).forEach((entry: WorkflowRunDailyCostsEntry) => {
        runDailyEntriesByKey.set(`${entry.date}:${entry.scope}:${entry.run_folder}`, entry)
      })

      nextRunDailyCostSummaries = Array.from(runDailyEntriesByKey.values())
        .map((entry: WorkflowRunDailyCostsEntry) => {
          if (!entry.token_usage) return null
          const summary = calculateCostSummary(entry.token_usage ?? null, null)
          if (!summary) return null
          return {
            date: entry.date,
            scope: entry.scope,
            groupFolder: entry.group_folder,
            runFolder: entry.run_folder,
            updatedAt: entry.token_usage?.updated_at || null,
            tokenUsage: entry.token_usage,
            summary
          }
        })
        .filter((entry): entry is RunDailyCostSummaryEntry => entry !== null)
        .sort((a, b) => {
          if (a.date !== b.date) return b.date.localeCompare(a.date)
          return a.runFolder.localeCompare(b.runFolder)
        })
      
      // Costs are stored keyed by "{iteration}/{group}" (e.g. "iteration-0/xspaces") —
      // multi-group workflows produce one entry per iteration-group pair. The
      // runFolders prop passed by parents can be stale or iteration-only (no group
      // info), so always trust the API cost keys as the source of truth.
      const allCostKeys = Array.from(costEntriesByRunFolder.keys())
      const foldersToLoad = allCostKeys

      for (const runFolder of foldersToLoad) {
        try {
          const data = costEntriesByRunFolder.get(runFolder)
          if (data?.token_usage || data?.evaluation_token_usage) {
            // Also fetch steps to get step titles for cost breakdown
            let steps: Record<string, StepExecutionLogs> | undefined
            try {
              const logsData = await agentApi.getExecutionLogs(workspacePath, runFolder)
              steps = logsData.steps
            } catch (err) {
              // If we can't get steps, continue without them (costs will still work)
              console.warn(`Failed to load steps for ${runFolder}:`, err)
            }
            const costSummary = calculateCostSummary(data.token_usage ?? null, data.evaluation_token_usage, steps)
            costs.push({
              runFolder,
              tokenUsage: data.token_usage ?? null,
              evaluationTokenUsage: data.evaluation_token_usage,
              steps, // Store steps for later use in model breakdown
              costSummary
            })
          }
        } catch (err) {
          console.warn(`Failed to process costs for ${runFolder}:`, err)
          // Continue loading other run folders
        }
      }

      costs.sort((a, b) => compareRunCosts(a, b, selectedRunFolder))

      setRunCosts(costs)
      setPhaseCostSummary(nextPhaseCostSummary)
      setPhaseDailyCostSummaries(nextPhaseDailyCostSummaries)
      setRunDailyCostSummaries(nextRunDailyCostSummaries)

      // Auto-expand selected run folder if provided
      if (selectedRunFolder && costs.some(c => c.runFolder === selectedRunFolder)) {
        setExpandedRunFolders(new Set([selectedRunFolder]))
      }
    } catch (err) {
      if (generation !== loadGenerationRef.current) return
      console.error('Failed to load costs:', err)
      setError('Failed to load cost data')
    } finally {
      if (generation === loadGenerationRef.current) setLoading(false)
    }
  }

  const loadOlderCosts = async () => {
    if (!workspacePath || !costHistory?.hasMore || !costHistory.nextBefore || loadingOlder) return
    setLoadingOlder(true)
    try {
      const response = await agentApi.getCosts(workspacePath, {
        view: 'summary',
        days: 30,
        before: costHistory.nextBefore,
      })
      if (response.scoped_costs) {
        setScopedCosts(current => current ? {
          ...current,
          by_date: {
            ...current.by_date,
            ...response.scoped_costs!.by_date,
          },
        } : response.scoped_costs!)
      }
      setCostHistory({
        hasMore: response.history?.has_more ?? false,
        nextBefore: response.history?.next_before,
      })
    } catch (err) {
      console.error('Failed to load older cost data:', err)
      setError('Failed to load older cost data')
    } finally {
      setLoadingOlder(false)
    }
  }

  const toggleRunFolder = (runFolder: string) => {
    setExpandedRunFolders(prev => {
      const next = new Set(prev)
      if (next.has(runFolder)) {
        next.delete(runFolder)
      } else {
        next.add(runFolder)
      }
      return next
    })
  }

  const toggleCostModel = (modelId: string) => {
    setExpandedCostModels(prev => {
      const next = new Set(prev)
      if (next.has(modelId)) {
        next.delete(modelId)
      } else {
        next.add(modelId)
      }
      return next
    })
  }

  const setViewModeForRunFolder = (runFolder: string, mode: 'step' | 'model') => {
    setCostViewMode(prev => ({
      ...prev,
      [runFolder]: mode
    }))
  }

  // Calculate aggregate summary across all visible run folders
  const aggregateSummary = useMemo(() => {
    if (runCosts.length === 0) return null

    let totalCost = 0
    let totalLLMCost = 0
    let totalToolCost = 0
    let totalInputTokens = 0
    let totalOutputTokens = 0
    let totalLLMCalls = 0
    let totalCacheReadTokens = 0
    let totalCacheWriteTokens = 0
    let totalReasoningTokens = 0
    const stageCosts = {
      execution: 0,
      learning: 0,
      reflection: 0,
      evaluation: 0,
      knowledgebase: 0,
      routing: 0,
      workshop: 0,
      other: 0
    }
    let highestCost = 0
    let lowestCost = Infinity

    runCosts.forEach(runCost => {
      if (runCost.costSummary) {
        totalCost += runCost.costSummary.totalCost
        totalLLMCost += runCost.costSummary.totalLLMCost
        totalToolCost += runCost.costSummary.totalToolCost
        totalInputTokens += runCost.costSummary.totalInputTokens
        totalOutputTokens += runCost.costSummary.totalOutputTokens
        totalLLMCalls += runCost.costSummary.totalLLMCalls
        totalCacheReadTokens += runCost.costSummary.totalCacheReadTokens
        totalCacheWriteTokens += runCost.costSummary.totalCacheWriteTokens
        totalReasoningTokens += runCost.costSummary.totalReasoningTokens
        stageCosts.execution += runCost.costSummary.stageCosts.execution
        stageCosts.learning += runCost.costSummary.stageCosts.learning
        stageCosts.reflection += runCost.costSummary.stageCosts.reflection
        stageCosts.evaluation += runCost.costSummary.stageCosts.evaluation
        stageCosts.knowledgebase += runCost.costSummary.stageCosts.knowledgebase
        stageCosts.routing += runCost.costSummary.stageCosts.routing
        stageCosts.workshop += runCost.costSummary.stageCosts.workshop
        stageCosts.other += runCost.costSummary.stageCosts.other
        
        if (runCost.costSummary.totalCost > highestCost) {
          highestCost = runCost.costSummary.totalCost
        }
        if (runCost.costSummary.totalCost < lowestCost) {
          lowestCost = runCost.costSummary.totalCost
        }
      }
    })

    return {
      totalCost,
      totalLLMCost,
      totalToolCost,
      totalInputTokens,
      totalOutputTokens,
      totalTokens: totalInputTokens + totalOutputTokens,
      totalLLMCalls,
      totalCacheReadTokens,
      totalCacheWriteTokens,
      totalReasoningTokens,
      stageCosts,
      highestCost: highestCost === 0 ? 0 : highestCost,
      lowestCost: lowestCost === Infinity ? 0 : lowestCost,
      totalRuns: runCosts.length
    }
  }, [runCosts])

  const overallSummary = useMemo(() => {
    if (scopedCosts && Object.keys(scopedCosts.by_scope || {}).length > 0) {
      return {
        totalCost: scopedCosts.total.total_cost_usd,
        totalTokens: scopedCosts.total.prompt_tokens + scopedCosts.total.completion_tokens,
        totalRuns: aggregateSummary?.totalRuns || 0
      }
    }
    if (!aggregateSummary && !phaseCostSummary) return null

    return {
      totalCost: (aggregateSummary?.totalCost || 0) + (phaseCostSummary?.totalCost || 0),
      totalTokens: (aggregateSummary?.totalTokens || 0) + (phaseCostSummary?.totalTokens || 0),
      totalRuns: aggregateSummary?.totalRuns || 0
    }
  }, [aggregateSummary, phaseCostSummary, scopedCosts])

  const combinedDailyCostSummaries = useMemo(() => {
    const byDate = scopedCosts?.by_date || {}
    const hasDailyScopeAttribution = Object.values(byDate).some(total => Object.keys(total.by_scope || {}).length > 0)
    if (!hasDailyScopeAttribution) {
      const legacyByDate = new Map<string, CombinedDailyCostSummaryEntry & { runKeys: Set<string> }>()
      const ensureLegacyEntry = (date: string) => {
        let entry = legacyByDate.get(date)
        if (!entry) {
          entry = {
            date,
            builderCost: 0,
            pulseCost: null,
            workflowCost: 0,
            evaluationCost: 0,
            totalCost: 0,
            totalTokens: 0,
            llmDurationMS: 0,
            runCount: 0,
            runKeys: new Set<string>(),
          }
          legacyByDate.set(date, entry)
        }
        return entry
      }

      phaseDailyCostSummaries.forEach(daily => {
        const entry = ensureLegacyEntry(daily.date)
        entry.builderCost += daily.summary.totalCost
        entry.totalCost += daily.summary.totalCost
        entry.totalTokens += daily.summary.totalTokens
      })
      runDailyCostSummaries.forEach(daily => {
        const entry = ensureLegacyEntry(daily.date)
        if (daily.scope === 'evaluation') entry.evaluationCost += daily.summary.totalCost
        else entry.workflowCost += daily.summary.totalCost
        entry.totalCost += daily.summary.totalCost
        entry.totalTokens += daily.summary.totalTokens
        entry.runKeys.add(`${daily.scope}:${daily.runFolder}`)
        entry.runCount = entry.runKeys.size
      })
      Object.entries(activityTiming?.by_date || {}).forEach(([date, timing]) => {
        const entry = ensureLegacyEntry(date)
        entry.llmDurationMS = Object.values(timing.by_scope || {}).reduce(
          (sum, scopeTiming) => sum + (scopeTiming.llm_duration_ms || 0),
          0,
        )
      })
      return Array.from(legacyByDate.values())
        .map(({ runKeys: _runKeys, ...entry }) => entry)
        .sort((a, b) => b.date.localeCompare(a.date))
    }

    return Object.entries(byDate)
      .map(([date, total]): CombinedDailyCostSummaryEntry => {
        const byScope = total.by_scope || {}
        const costFor = (scope: string) => byScope[scope]?.total_cost_usd || 0
        return {
          date,
          builderCost: costFor('builder') + costFor('chat'),
          pulseCost: costFor('pulse'),
          workflowCost: costFor('workflow_execution'),
          evaluationCost: costFor('evaluation'),
          totalCost: total.total_cost_usd || 0,
          totalTokens: (total.prompt_tokens || 0) + (total.completion_tokens || 0),
          llmDurationMS: total.llm_generation_duration_ms || 0,
          runCount: total.workflow_run_count || 0,
        }
      })
      .sort((a, b) => b.date.localeCompare(a.date))
  }, [activityTiming, phaseDailyCostSummaries, runDailyCostSummaries, scopedCosts])

  const dailyActivityBreakdown = useMemo(() => {
    const details = new Map<string, ReturnType<typeof buildCostActivityBreakdown>>()
    Object.entries(scopedCosts?.by_date || {}).forEach(([date, total]) => {
      if (Object.keys(total.by_scope || {}).length > 0) {
        details.set(date, buildCostActivityBreakdown(
          { by_scope: total.by_scope },
          activityTiming?.by_date?.[date] || null,
        ))
      }
    })
    return details
  }, [activityTiming, scopedCosts])

  if (!embedded && !isOpen) return null

  const shell = (
      <div className={embedded
        ? 'flex h-full min-h-0 w-full flex-col bg-background'
        : 'bg-background rounded-lg shadow-xl w-full max-w-6xl max-h-[calc(100dvh-1rem)] sm:max-h-[90vh] flex flex-col border border-border relative'}>
        {/* Header */}
        <div className="flex items-start justify-between gap-3 px-4 py-3 border-b border-border sm:px-6 sm:py-4">
          <div className="flex-1 min-w-0">
            <h2 className="text-lg font-semibold text-foreground flex items-center gap-2">
              <DollarSign className="w-5 h-5 text-primary" />
              Cost Analysis
              {startedAt && (
                <span className="text-xs font-normal text-muted-foreground">{formatStartedAt(startedAt)}</span>
              )}
            </h2>
            <div className="flex flex-wrap items-center gap-2 mt-1 sm:gap-4">
              {overallSummary && (
                <div className="flex flex-wrap items-center gap-2 text-xs sm:gap-3">
                  <div className="flex items-center gap-1.5 text-green-600 dark:text-green-400 font-medium">
                    <DollarSign className="w-3.5 h-3.5" />
                    {formatUSD(overallSummary.totalCost)}
                  </div>
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <Coins className="w-3.5 h-3.5" />
                    {formatTokens(overallSummary.totalTokens)} tokens
                  </div>
                  {aggregateSummary && (
                    <div className="text-muted-foreground">
                      {aggregateSummary.totalRuns} run{aggregateSummary.totalRuns !== 1 ? 's' : ''}
                    </div>
                  )}
                  {aggregateSummary && aggregateSummary.totalToolCost > 0 && (
                    <div className="text-muted-foreground">
                      LLM {formatUSD(aggregateSummary.totalLLMCost)} | Tools {formatUSD(aggregateSummary.totalToolCost)}
                    </div>
                  )}
                  {phaseCostSummary && (
                    <div className="text-amber-600 dark:text-amber-400 font-medium">
                      Builder {formatUSD(phaseCostSummary.totalCost)}
                    </div>
                  )}
                </div>
              )}
              <button
                onClick={loadAllCosts}
                disabled={loading}
                className="p-1.5 rounded-md hover:bg-muted transition-colors text-muted-foreground hover:text-foreground"
                title="Refresh"
              >
                <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              </button>
            </div>
          </div>
          {!embedded && <button
            onClick={onClose}
            className="p-2 rounded-full hover:bg-accent hover:text-accent-foreground transition-colors ml-4"
          >
            <X className="w-5 h-5 text-muted-foreground" />
          </button>}
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-6 bg-background">
          {loading ? (
            <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
              <Loader2 className="w-8 h-8 animate-spin mb-3 text-primary" />
              <p>Loading cost data...</p>
            </div>
          ) : error ? (
            <div className="flex flex-col items-center justify-center py-12 text-destructive">
              <AlertCircle className="w-12 h-12 mb-3" />
              <p>{error}</p>
              <button
                onClick={loadAllCosts}
                className="mt-4 px-4 py-2 bg-destructive/10 text-destructive rounded-md hover:bg-destructive/20 transition-colors text-sm font-medium"
              >
                Retry
              </button>
            </div>
          ) : runCosts.length === 0 && !phaseCostSummary && !hasScopedActivity ? (
            <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
              <DollarSign className="w-12 h-12 mb-3 opacity-50" />
              <p>No cost data found.</p>
              <p className="text-sm mt-2">Run the automation to see cost data here.</p>
            </div>
          ) : (
            <div className="space-y-6">
              {/* Canonical product activity hierarchy */}
              {hasScopedActivity && (
                <section className="space-y-3">
                  <div>
                    <h3 className="text-sm font-semibold text-foreground">Cost by activity</h3>
                    <p className="mt-1 text-xs text-muted-foreground">
                      Builder, Pulse, workflow, and evaluation costs from the authoritative event ledger.
                    </p>
                  </div>
                  <div className="grid grid-cols-1 gap-3 xl:grid-cols-2">
                    {activityBreakdown.map(category => {
                      const tokenTotal = category.total.prompt_tokens + category.total.completion_tokens
                      return (
                      <div key={category.id} className="flex items-center gap-3 rounded-lg border border-border bg-card p-4 shadow-sm">
                        <div className="min-w-0 flex-1">
                          <div className="font-semibold text-foreground">{category.label}</div>
                          <div className="truncate text-xs text-muted-foreground">{category.description}</div>
                        </div>
                          <div className="text-right">
                            <div className="font-mono font-semibold text-foreground">{formatUSD(category.total.total_cost_usd)}</div>
                            <div className="text-xs text-muted-foreground">{formatTokens(tokenTotal)} tokens</div>
                            <div className="text-xs text-muted-foreground">LLM time: {formatDuration(category.total.llm_generation_duration_ms)}</div>
                          </div>
                      </div>
                      )
                    })}
                  </div>
                </section>
              )}

              {/* Daily Costs */}
              {combinedDailyCostSummaries.length > 0 && (
                <div className="bg-card border border-border rounded-lg p-4 shadow-sm">
                  <div className="flex items-start justify-between gap-4 mb-4">
                    <div>
                      <h3 className="text-sm font-semibold text-foreground mb-1 flex items-center gap-2">
                        <TrendingUp className="w-4 h-4 text-primary" />
                        Daily Cost Breakdown
                      </h3>
                      <p className="text-xs text-muted-foreground">
                        Daily totals using the same Builder, Pulse, Workflow, and Evaluation categories above.
                      </p>
                    </div>
                  </div>

                  <div className="overflow-x-auto">
                    <table className="w-full text-xs">
                      <thead>
                        <tr className="text-muted-foreground border-b border-border pb-2">
                          <th className="text-left font-medium pb-2">Date</th>
                          <th className="text-right font-medium pb-2">Runs</th>
                          <th className="text-right font-medium pb-2">Builder</th>
                          <th className="text-right font-medium pb-2">Pulse</th>
                          <th className="text-right font-medium pb-2">Workflow</th>
                          <th className="text-right font-medium pb-2">Evaluation</th>
                          <th className="text-right font-medium pb-2">LLM time</th>
                          <th className="text-right font-medium pb-2">Tokens</th>
                          <th className="text-right font-medium pb-2">Total</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-border">
                        {combinedDailyCostSummaries.map(entry => {
                          const isExpanded = expandedDailyDate === entry.date
                          const categories = dailyActivityBreakdown.get(entry.date)
                          return (
                            <React.Fragment key={entry.date}>
                              <tr className="hover:bg-accent/50 transition-colors">
                                <td className="py-2">
                                  <div className="flex items-center gap-2">
                                    <span className="font-medium text-foreground">{entry.date}</span>
                                    <button
                                      type="button"
                                      onClick={() => setExpandedDailyDate(current => current === entry.date ? null : entry.date)}
                                      className="rounded border border-border px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground hover:bg-accent hover:text-foreground"
                                      aria-expanded={isExpanded}
                                    >
                                      <span aria-hidden="true" className="text-sm leading-none">{isExpanded ? '−' : '+'}</span>
                                      <span className="sr-only">{isExpanded ? 'Hide daily activity details' : 'Show daily activity details'}</span>
                                    </button>
                                  </div>
                                </td>
                                <td className="py-2 text-right font-mono text-muted-foreground">{entry.runCount.toLocaleString()}</td>
                                <td className="py-2 text-right font-mono text-muted-foreground">{formatUSD(entry.builderCost)}</td>
                                <td className="py-2 text-right font-mono text-muted-foreground">{entry.pulseCost === null ? '—' : formatUSD(entry.pulseCost)}</td>
                                <td className="py-2 text-right font-mono text-muted-foreground">{formatUSD(entry.workflowCost)}</td>
                                <td className="py-2 text-right font-mono text-muted-foreground">{formatUSD(entry.evaluationCost)}</td>
                                <td className="py-2 text-right font-mono text-muted-foreground">{formatDuration(entry.llmDurationMS)}</td>
                                <td className="py-2 text-right font-mono text-muted-foreground">{formatTokens(entry.totalTokens)}</td>
                                <td className="py-2 text-right font-bold text-green-600 dark:text-green-400">{formatUSD(entry.totalCost)}</td>
                              </tr>
                              {isExpanded && (
                                <tr className="bg-muted/20">
                                  <td colSpan={9} className="p-3">
                                    {!categories ? (
                                      <p className="text-xs text-muted-foreground">This older daily record has totals but no activity attribution.</p>
                                    ) : (
                                      <div className="space-y-3">
                                        {categories.map(category => {
                                          const tokenTotal = category.total.prompt_tokens + category.total.completion_tokens
                                          return (
                                            <div key={category.id} className="overflow-hidden rounded-md border border-border bg-card">
                                              <div className="flex items-center justify-between gap-3 border-b border-border px-3 py-2">
                                                <div className="text-xs font-semibold text-foreground">
                                                  {category.id === 'pulse' ? 'Pulse (including background agents)' : category.label}
                                                </div>
                                                  <div className="text-right text-[10px] text-muted-foreground">
                                                    <div className="font-mono text-foreground">{formatUSD(category.total.total_cost_usd)}</div>
                                                    <div>{formatTokens(tokenTotal)} tokens</div>
                                                    <div>LLM time: {formatDuration(category.total.llm_generation_duration_ms)}</div>
                                                </div>
                                              </div>
                                              {category.executions.length === 0 ? (
                                                <p className="px-3 py-2 text-[10px] text-muted-foreground">No activity recorded.</p>
                                              ) : (
                                                <div className="max-h-48 overflow-y-auto px-3">
                                                  {category.executions.map(execution => {
                                                    // PLAT-166/PLAT-167: an execution row's own combined total can hide a
                                                    // phase breakdown underneath — a workflow step's reflection turn
                                                    // sharing the step's execution id, or a message_sequence step's
                                                    // individual items each tagged with their own "item:<id>" phase.
                                                    // Most executions (chat, builder, evaluation, Pulse, a step with no
                                                    // reflection turn) never populate by_phase at all — the backend
                                                    // only writes an entry for a turn it explicitly tagged, not a
                                                    // catch-all "the rest" bucket (PLAT-166 scope-fix).
                                                    const phaseEntries = Object.entries(execution.cost.by_phase || {})
                                                      .filter(([, phaseCost]) => (
                                                        phaseCost.total_cost_usd > 0 ||
                                                        phaseCost.prompt_tokens + phaseCost.completion_tokens > 0 ||
                                                        (phaseCost.llm_generation_duration_ms || 0) > 0
                                                      ))
                                                      .sort(([, a], [, b]) => b.total_cost_usd - a.total_cost_usd)
                                                    // Show the breakdown whenever it has more than one tagged phase
                                                    // (e.g. several message_sequence items), or exactly one tagged
                                                    // phase that doesn't already account for the row's whole total
                                                    // (e.g. a reflection turn alongside untagged execution work) —
                                                    // comparing token counts rather than float cost to stay exact.
                                                    const taggedTokens = phaseEntries.reduce((sum, [, c]) => sum + c.prompt_tokens + c.completion_tokens, 0)
                                                    const totalTokens = execution.cost.prompt_tokens + execution.cost.completion_tokens
                                                    const showPhaseBreakdown = phaseEntries.length > 1 || (phaseEntries.length === 1 && taggedTokens < totalTokens)
                                                    return (
                                                      <React.Fragment key={execution.id}>
                                                        <div className="flex items-center gap-2 border-b border-border/70 py-2 text-[10px] last:border-0" title={execution.id}>
                                                          <span className="min-w-0 flex-1 truncate text-foreground">{execution.label}</span>
                                                          <span className="shrink-0 font-mono text-muted-foreground">{formatTokens(execution.cost.prompt_tokens + execution.cost.completion_tokens)}</span>
                                                          <span className="shrink-0 font-mono text-muted-foreground">LLM time: {formatDuration(execution.cost.llm_generation_duration_ms)}</span>
                                                          <span className="shrink-0 font-mono text-foreground">{formatUSD(execution.cost.total_cost_usd)}</span>
                                                        </div>
                                                        {showPhaseBreakdown && phaseEntries.map(([phase, phaseCost]) => (
                                                          <div key={phase} className="flex items-center gap-2 border-b border-border/70 py-1 pl-4 text-[10px] text-muted-foreground last:border-0" title="Included in the total above">
                                                            <span className="min-w-0 flex-1 truncate">↳ {costPhaseLabel(phase)}</span>
                                                            <span className="shrink-0 font-mono">{formatTokens(phaseCost.prompt_tokens + phaseCost.completion_tokens)}</span>
                                                            <span className="shrink-0 font-mono">LLM time: {formatDuration(phaseCost.llm_generation_duration_ms)}</span>
                                                            <span className="shrink-0 font-mono">{formatUSD(phaseCost.total_cost_usd)}</span>
                                                          </div>
                                                        ))}
                                                      </React.Fragment>
                                                    )
                                                  })}
                                                </div>
                                              )}
                                            </div>
                                          )
                                        })}
                                      </div>
                                    )}
                                  </td>
                                </tr>
                              )}
                            </React.Fragment>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>
                  {costHistory?.hasMore && (
                    <div className="mt-4 flex justify-center">
                      <button
                        type="button"
                        onClick={loadOlderCosts}
                        disabled={loadingOlder}
                        className="inline-flex items-center gap-2 rounded-md border border-border px-3 py-2 text-xs font-medium text-foreground transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-60"
                      >
                        {loadingOlder && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                        {loadingOlder ? 'Loading older days…' : 'Load older days'}
                      </button>
                    </div>
                  )}
                </div>
              )}

              {/* Automation Builder / Phase Costs */}
              {!hasScopedActivity && phaseCostSummary && (
                <div className="bg-card border border-border rounded-lg p-4 shadow-sm">
                  <div className="flex items-start justify-between gap-4 mb-4">
                    <div>
                      <h3 className="text-sm font-semibold text-foreground mb-1 flex items-center gap-2">
                        <DollarSign className="w-4 h-4 text-amber-500" />
                        Automation Builder Costs
                      </h3>
                      <p className="text-xs text-muted-foreground">
                        Costs captured outside run folders, including workflow builder and other phase-only sessions.
                      </p>
                      {phaseCostSummary.updatedAt && (
                        <p className="text-[10px] text-muted-foreground mt-1">
                          Last updated: {formatTimestampLabel(phaseCostSummary.updatedAt)}
                        </p>
                      )}
                    </div>
                  </div>

                  <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    <div className="bg-amber-100 dark:bg-amber-900/30 rounded-lg p-3">
                      <div className="text-xs text-muted-foreground uppercase tracking-wider mb-1">Builder Total</div>
                      <div className="text-2xl font-bold text-amber-600 dark:text-amber-400">
                        {formatUSD(phaseCostSummary.totalCost)}
                      </div>
                      <div className="text-xs text-muted-foreground mt-1">
                        {formatTokens(phaseCostSummary.totalTokens)} tokens
                      </div>
                    </div>

                    <div className="bg-card border border-border rounded-lg p-3">
                      <div className="text-xs text-muted-foreground uppercase tracking-wider mb-1">Automation Builder</div>
                      <div className="text-2xl font-bold text-foreground">
                        {formatUSD(phaseCostSummary.phaseCosts.find(phase => phase.phaseID === 'workflow-builder')?.totalCost)}
                      </div>
                    </div>

                    <div className="bg-card border border-border rounded-lg p-3">
                      <div className="text-xs text-muted-foreground uppercase tracking-wider mb-1">LLM Calls</div>
                      <div className="text-2xl font-bold text-foreground">
                        {phaseCostSummary.totalLLMCalls.toLocaleString()}
                      </div>
                    </div>

                    <div className="bg-card border border-border rounded-lg p-3">
                      <div className="text-xs text-muted-foreground uppercase tracking-wider mb-1">Tracked Phases</div>
                      <div className="text-2xl font-bold text-foreground">
                        {phaseCostSummary.phaseCosts.length}
                      </div>
                    </div>
                  </div>

                  {phaseCostSummary.phaseCosts.length > 0 && (
                    <div className="mt-4 overflow-x-auto">
                      <table className="w-full text-xs">
                        <thead>
                          <tr className="text-muted-foreground border-b border-border pb-2">
                            <th className="text-left font-medium pb-2">Phase</th>
                            <th className="text-right font-medium pb-2">Calls</th>
                            <th className="text-right font-medium pb-2">Tokens</th>
                            <th className="text-right font-medium pb-2">Cost</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-border">
                          {phaseCostSummary.phaseCosts.map(phase => (
                            <tr key={phase.phaseID} className="hover:bg-accent/50 transition-colors">
                              <td className="py-2">
                                <div className="font-medium text-foreground">{phase.phaseTitle}</div>
                                <div className="text-[10px] text-muted-foreground font-mono">{phase.phaseID}</div>
                              </td>
                              <td className="py-2 text-right font-mono text-muted-foreground">
                                {phase.llmCalls.toLocaleString()}
                              </td>
                              <td className="py-2 text-right font-mono text-muted-foreground">
                                {(phase.inputTokens + phase.outputTokens).toLocaleString()}
                              </td>
                              <td className="py-2 text-right font-bold text-amber-600 dark:text-amber-400">
                                {formatUSD(phase.totalCost)}
                              </td>
                            </tr>
                          ))}
                          <tr className="bg-muted/30 font-semibold">
                            <td className="py-2 text-foreground">Total</td>
                            <td className="py-2 text-right font-mono text-muted-foreground">
                              {phaseCostSummary.totalLLMCalls.toLocaleString()}
                            </td>
                            <td className="py-2 text-right font-mono text-muted-foreground">
                              {phaseCostSummary.totalTokens.toLocaleString()}
                            </td>
                            <td className="py-2 text-right font-bold text-amber-600 dark:text-amber-400">
                              {formatUSD(phaseCostSummary.totalCost)}
                            </td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  )}

                  {phaseCostSummary.modelCosts.length > 0 && (
                    <div className="mt-5 overflow-x-auto">
                      <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-2">
                        LLM Breakdown
                      </div>
                      <table className="w-full text-xs">
                        <thead>
                          <tr className="text-muted-foreground border-b border-border pb-2">
                            <th className="text-left font-medium pb-2">Model</th>
                            <th className="text-right font-medium pb-2">Provider</th>
                            <th className="text-right font-medium pb-2">Calls</th>
                            <th className="text-right font-medium pb-2">Tokens</th>
                            <th className="text-right font-medium pb-2">Cost</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-border">
                          {phaseCostSummary.modelCosts.map(model => (
                            <tr key={model.modelID} className="hover:bg-accent/50 transition-colors">
                              <td className="py-2">
                                <div className="font-medium text-foreground font-mono">{model.modelID}</div>
                              </td>
                              <td className="py-2 text-right text-muted-foreground">
                                {model.provider}
                              </td>
                              <td className="py-2 text-right font-mono text-muted-foreground">
                                {model.llmCalls.toLocaleString()}
                              </td>
                              <td className="py-2 text-right font-mono text-muted-foreground">
                                {(model.inputTokens + model.outputTokens).toLocaleString()}
                              </td>
                              <td className="py-2 text-right font-bold text-amber-600 dark:text-amber-400">
                                {formatUSD(model.totalCost)}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}

                  {phaseDailyCostSummaries.length === 0 && (
                    <p className="mt-5 text-xs text-muted-foreground">
                      Daily builder history appears only for phase costs written to the new daily ledger. Older builder totals remain included in the aggregate above.
                    </p>
                  )}
                </div>
              )}

              {/* Aggregate Summary */}
              {!hasScopedActivity && aggregateSummary && (
                <div className="bg-card border border-border rounded-lg p-4 shadow-sm">
                  <h3 className="text-sm font-semibold text-foreground mb-4 flex items-center gap-2">
                    <Award className="w-4 h-4 text-primary" />
                    Aggregate Summary ({aggregateSummary.totalRuns} runs)
                  </h3>
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    {/* Total Cost */}
                    <div className="bg-green-100 dark:bg-green-900/30 rounded-lg p-3">
                      <div className="text-xs text-muted-foreground uppercase tracking-wider mb-1">Total Cost</div>
                      <div className="text-2xl font-bold text-green-600 dark:text-green-400">
                        {formatUSD(aggregateSummary.totalCost)}
                      </div>
                      <div className="text-xs text-muted-foreground mt-1">
                        {formatTokens(aggregateSummary.totalTokens)} tokens
                      </div>
                    </div>

                    {/* Highest Cost */}
                    <div className="bg-blue-100 dark:bg-blue-900/30 rounded-lg p-3">
                      <div className="text-xs text-muted-foreground uppercase tracking-wider mb-1 flex items-center gap-1">
                        <TrendingUp className="w-3 h-3" />
                        Highest
                      </div>
                      <div className="text-2xl font-bold text-blue-600 dark:text-blue-400">
                        {formatUSD(aggregateSummary.highestCost)}
                      </div>
                    </div>

                    {/* Lowest Cost */}
                    <div className="bg-purple-100 dark:bg-purple-900/30 rounded-lg p-3">
                      <div className="text-xs text-muted-foreground uppercase tracking-wider mb-1 flex items-center gap-1">
                        <TrendingDown className="w-3 h-3" />
                        Lowest
                      </div>
                      <div className="text-2xl font-bold text-purple-600 dark:text-purple-400">
                        {formatUSD(aggregateSummary.lowestCost)}
                      </div>
                    </div>

                    {/* Total Runs */}
                    <div className="bg-muted rounded-lg p-3">
                      <div className="text-xs text-muted-foreground uppercase tracking-wider mb-1">Runs</div>
                      <div className="text-2xl font-bold text-foreground">
                        {aggregateSummary.totalRuns}
                      </div>
                    </div>
                  </div>

                  {/* Stage Costs Summary */}
                  <div className="mt-4 grid grid-cols-2 md:grid-cols-4 lg:grid-cols-8 gap-3">
                    <div className="bg-card border border-border rounded-lg p-3">
                      <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Execution</div>
                      <div className="text-lg font-bold text-foreground">{formatUSD(aggregateSummary.stageCosts.execution)}</div>
                    </div>
                    <div className="bg-card border border-border rounded-lg p-3">
                      <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Learning</div>
                      <div className="text-lg font-bold text-foreground">{formatUSD(aggregateSummary.stageCosts.learning)}</div>
                    </div>
                    <div className="bg-card border border-border rounded-lg p-3">
                      <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Reflection</div>
                      <div className="text-lg font-bold text-foreground">{formatUSD(aggregateSummary.stageCosts.reflection)}</div>
                    </div>
                    <div className="bg-card border border-border rounded-lg p-3">
                      <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Knowledgebase</div>
                      <div className="text-lg font-bold text-foreground">{formatUSD(aggregateSummary.stageCosts.knowledgebase)}</div>
                    </div>
                    <div className="bg-card border border-border rounded-lg p-3">
                      <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Routing</div>
                      <div className="text-lg font-bold text-foreground">{formatUSD(aggregateSummary.stageCosts.routing)}</div>
                    </div>
                    <div className="bg-card border border-border rounded-lg p-3">
                      <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Workshop</div>
                      <div className="text-lg font-bold text-foreground">{formatUSD(aggregateSummary.stageCosts.workshop)}</div>
                    </div>
                    <div className="bg-card border border-border rounded-lg p-3">
                      <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Evaluation</div>
                      <div className="text-lg font-bold text-foreground">{formatUSD(aggregateSummary.stageCosts.evaluation)}</div>
                    </div>
                    <div className="bg-card border border-border rounded-lg p-3">
                      <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Other</div>
                      <div className="text-lg font-bold text-foreground">{formatUSD(aggregateSummary.stageCosts.other)}</div>
                    </div>
                  </div>
                </div>
              )}

              {/* Legacy per-run explorer: the canonical ledger provides the grouped
                  activity detail above. Retain this only when the ledger is absent. */}
              {!hasScopedActivity && runCosts.length > 0 && (
                <div className="space-y-3">
                <div>
                  <h3 className="text-sm font-semibold text-foreground">Workflow runs</h3>
                  <p className="mt-1 text-xs text-muted-foreground">Open a run only when you need its step or model-level cost detail.</p>
                </div>
                {runCosts.map((runCost) => {
                  const isExpanded = expandedRunFolders.has(runCost.runFolder)
                  const viewMode = costViewMode[runCost.runFolder] || 'step'
                  const costSummary = runCost.costSummary
                  const displayRunFolderName = getRunFolderDisplayName(runCost.runFolder)
                  const secondaryRunFolderLabel = getRunFolderSecondaryLabel(runCost)

                  if (!costSummary) return null

                  return (
                    <div
                      key={runCost.runFolder}
                      className={`border rounded-lg overflow-hidden bg-card ${
                        runCost.runFolder === selectedRunFolder 
                          ? 'border-purple-500/50 ring-1 ring-purple-500/20' 
                          : 'border-border'
                      }`}
                    >
                      {/* Run Folder Header */}
                      <button
                        onClick={() => toggleRunFolder(runCost.runFolder)}
                        title={getRunFolderTitle(runCost)}
                        className={`w-full flex items-center justify-between px-4 py-3 text-left transition-colors ${
                          isExpanded ? 'bg-accent/50' : 'hover:bg-accent/50'
                        } ${runCost.runFolder === selectedRunFolder ? 'bg-purple-50/30 dark:bg-purple-900/10' : ''}`}
                      >
                        <div className="flex items-center gap-3 flex-1 min-w-0">
                          {isExpanded ? (
                            <ChevronDown className="w-4 h-4 text-muted-foreground flex-shrink-0" />
                          ) : (
                            <ChevronRight className="w-4 h-4 text-muted-foreground flex-shrink-0" />
                          )}
                          <div className="min-w-0">
                            <div className="flex items-center gap-2 min-w-0">
                              <span className={`font-mono text-xs px-1.5 py-0.5 rounded ${
                                runCost.runFolder === selectedRunFolder 
                                  ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300 font-bold' 
                                  : 'bg-muted text-foreground'
                              }`}>
                                {displayRunFolderName}
                              </span>
                              {runCost.runFolder === selectedRunFolder && (
                                <span className="flex items-center gap-1 text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded bg-purple-500 text-white shadow-sm">
                                  <TrendingUp className="w-2.5 h-2.5" />
                                  Current
                                </span>
                              )}
                            </div>
                            {secondaryRunFolderLabel && (
                              <div className="mt-1 text-[10px] text-muted-foreground truncate">
                                {secondaryRunFolderLabel}
                              </div>
                            )}
                          </div>
                        </div>

                        {/* Cost Badge */}
                        <div className="flex items-center gap-3 flex-shrink-0 ml-4">
                          <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-green-100 dark:bg-green-900/30">
                            <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                              {getRunBadgeLabel(runCost)}
                            </span>
                            <DollarSign className="w-4 h-4 text-green-600 dark:text-green-400" />
                            <span className="text-sm font-semibold text-green-600 dark:text-green-400">
                              {formatUSD(costSummary.totalCost)}
                            </span>
                            <span className="text-xs text-muted-foreground">
                              ({formatTokens(costSummary.totalTokens)})
                            </span>
                          </div>
                        </div>
                      </button>

                      {/* Expanded Content */}
                      {isExpanded && (
                        <div className="border-t border-border p-4 space-y-4">
                          {/* Stage Summary Cards */}
                          <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-7 gap-3">
                            <div className="bg-card border border-border rounded-lg p-3 shadow-sm">
                              <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Execution</div>
                              <div className="text-lg font-bold text-foreground">{formatUSD(costSummary.stageCosts.execution)}</div>
                            </div>
                            <div className="bg-card border border-border rounded-lg p-3 shadow-sm">
                              <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Learning</div>
                              <div className="text-lg font-bold text-foreground">{formatUSD(costSummary.stageCosts.learning)}</div>
                            </div>
                            <div className="bg-card border border-border rounded-lg p-3 shadow-sm">
                              <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Knowledgebase</div>
                              <div className="text-lg font-bold text-foreground">{formatUSD(costSummary.stageCosts.knowledgebase)}</div>
                            </div>
                            <div className="bg-card border border-border rounded-lg p-3 shadow-sm">
                              <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Routing</div>
                              <div className="text-lg font-bold text-foreground">{formatUSD(costSummary.stageCosts.routing)}</div>
                            </div>
                            <div className="bg-card border border-border rounded-lg p-3 shadow-sm">
                              <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Workshop</div>
                              <div className="text-lg font-bold text-foreground">{formatUSD(costSummary.stageCosts.workshop)}</div>
                            </div>
                            <div className="bg-card border border-border rounded-lg p-3 shadow-sm">
                              <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Evaluation</div>
                              <div className="text-lg font-bold text-foreground">{formatUSD(costSummary.stageCosts.evaluation)}</div>
                            </div>
                            <div className="bg-card border border-border rounded-lg p-3 shadow-sm">
                              <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">Other</div>
                              <div className="text-lg font-bold text-foreground">{formatUSD(costSummary.stageCosts.other)}</div>
                            </div>
                          </div>

                          {/* Cost Breakdown Table with View Toggle */}
                          {(runCost.tokenUsage?.by_model || runCost.tokenUsage?.by_tool || runCost.evaluationTokenUsage?.by_model || runCost.evaluationTokenUsage?.by_tool) && (
                            <div className="bg-card border border-border rounded-lg overflow-hidden shadow-sm">
                              <div className="px-4 py-3 bg-muted/30 border-b border-border flex items-center justify-between">
                                <h3 className="text-sm font-semibold flex items-center gap-2">
                                  <DollarSign className="w-4 h-4 text-green-500" />
                                  Cost Breakdown
                                </h3>
                                {/* View Toggle Buttons */}
                                <div className="flex items-center gap-1 bg-muted rounded-md p-0.5">
                                  <button
                                    onClick={() => setViewModeForRunFolder(runCost.runFolder, 'step')}
                                    className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                      viewMode === 'step'
                                        ? 'bg-background text-foreground shadow-sm'
                                        : 'text-muted-foreground hover:text-foreground'
                                    }`}
                                  >
                                    By Step
                                  </button>
                                  <button
                                    onClick={() => setViewModeForRunFolder(runCost.runFolder, 'model')}
                                    className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                      viewMode === 'model'
                                        ? 'bg-background text-foreground shadow-sm'
                                        : 'text-muted-foreground hover:text-foreground'
                                    }`}
                                  >
                                    By Model
                                  </button>
                                </div>
                              </div>

                              {/* Step-wise Cost Breakdown View */}
                              {viewMode === 'step' && costSummary.stepCosts.length > 0 && (
                                <div className="p-4 overflow-x-auto">
                                  <table className="w-full text-xs">
                                    <thead>
                                      <tr className="text-muted-foreground border-b border-border pb-2">
                                        <th className="text-left font-medium pb-2">Step</th>
                                        <th className="text-right font-medium pb-2">Tokens</th>
                                        <th className="text-right font-medium pb-2 text-blue-500">Execution</th>
                                        <th className="text-right font-medium pb-2 text-purple-500">Learning</th>
                                        <th className="text-right font-medium pb-2 text-teal-500">KB</th>
                                        <th className="text-right font-medium pb-2 text-cyan-500">Routing</th>
                                        <th className="text-right font-medium pb-2 text-pink-500">Workshop</th>
                                        <th className="text-right font-medium pb-2 text-amber-500">Evaluation</th>
                                        <th className="text-right font-medium pb-2">Total Cost</th>
                                      </tr>
                                    </thead>
                                    <tbody className="divide-y divide-border">
                                      {costSummary.stepCosts.map((step) => (
                                        <tr key={step.stepID} className="hover:bg-accent/50 transition-colors">
                                          <td className="py-2">
                                            <div className="font-medium text-foreground">
                                              {step.stepNum === 0 || step.stepNum > 1000
                                                ? (
                                                    <span className="flex items-center gap-1.5">
                                                      {step.stepTitle}
                                                      <span className="text-xs text-muted-foreground">
                                                        ({step.stepID})
                                                      </span>
                                                    </span>
                                                  )
                                                : (
                                                    <span>
                                                      Step {step.stepNum}: {step.stepTitle}
                                                      <span className="text-xs text-muted-foreground ml-1">
                                                        ({step.stepID})
                                                      </span>
                                                    </span>
                                                  )
                                              }
                                            </div>
                                          </td>
                                          <td className="py-2 text-right font-mono text-muted-foreground">
                                            {(step.inputTokens + step.outputTokens).toLocaleString()}
                                          </td>
                                          <td className="py-2 text-right font-mono text-blue-600 dark:text-blue-400">
                                            {formatUSD(step.execution)}
                                          </td>
                                          <td className="py-2 text-right font-mono text-purple-600 dark:text-purple-400">
                                            {formatUSD(step.learning)}
                                          </td>
                                          <td className="py-2 text-right font-mono text-teal-600 dark:text-teal-400">
                                            {formatUSD(step.knowledgebase)}
                                          </td>
                                          <td className="py-2 text-right font-mono text-cyan-600 dark:text-cyan-400">
                                            {formatUSD(step.routing)}
                                          </td>
                                          <td className="py-2 text-right font-mono text-pink-600 dark:text-pink-400">
                                            {formatUSD(step.workshop)}
                                          </td>
                                          <td className="py-2 text-right font-mono text-amber-600 dark:text-amber-400">
                                            {formatUSD(step.evaluation)}
                                          </td>
                                          <td className="py-2 text-right font-bold text-foreground">
                                            {formatUSD(step.totalCost)}
                                          </td>
                                        </tr>
                                      ))}
                                      {/* Total Row */}
                                      <tr className="bg-muted/30 font-semibold">
                                        <td className="py-2 text-foreground">Total</td>
                                        <td className="py-2 text-right font-mono text-muted-foreground">
                                          {costSummary.totalTokens.toLocaleString()}
                                        </td>
                                        <td className="py-2 text-right font-mono text-blue-600 dark:text-blue-400">
                                          {formatUSD(costSummary.stageCosts.execution)}
                                        </td>
                                        <td className="py-2 text-right font-mono text-purple-600 dark:text-purple-400">
                                          {formatUSD(costSummary.stageCosts.learning)}
                                        </td>
                                        <td className="py-2 text-right font-mono text-teal-600 dark:text-teal-400">
                                          {formatUSD(costSummary.stageCosts.knowledgebase)}
                                        </td>
                                        <td className="py-2 text-right font-mono text-cyan-600 dark:text-cyan-400">
                                          {formatUSD(costSummary.stageCosts.routing)}
                                        </td>
                                        <td className="py-2 text-right font-mono text-pink-600 dark:text-pink-400">
                                          {formatUSD(costSummary.stageCosts.workshop)}
                                        </td>
                                        <td className="py-2 text-right font-mono text-amber-600 dark:text-amber-400">
                                          {formatUSD(costSummary.stageCosts.evaluation)}
                                        </td>
                                        <td className="py-2 text-right font-bold text-green-600 dark:text-green-400">
                                          {formatUSD(costSummary.totalCost)}
                                        </td>
                                      </tr>
                                    </tbody>
                                  </table>
                                </div>
                              )}

                              {/* Model-wise Cost Breakdown View */}
                              {viewMode === 'model' && (runCost.tokenUsage || runCost.evaluationTokenUsage) && (
                                <div className="p-4 overflow-x-auto">
                                  <table className="w-full text-xs">
                                    <thead>
                                      <tr className="text-muted-foreground border-b border-border pb-2">
                                        <th className="w-8"></th>
                                        <th className="text-left font-medium pb-2">Model</th>
                                        <th className="text-right font-medium pb-2">Input</th>
                                        <th className="text-right font-medium pb-2">Cached In</th>
                                        <th className="text-right font-medium pb-2">Cache Write</th>
                                        <th className="text-right font-medium pb-2">Reasoning</th>
                                        <th className="text-right font-medium pb-2">Output</th>
                                        <th className="text-right font-medium pb-2">Cost (USD)</th>
                                      </tr>
                                    </thead>
                                    <tbody className="divide-y divide-border">
                                      {runCost.tokenUsage && Object.entries(runCost.tokenUsage.by_model || {}).map(([modelId, usage]) => {
                                        const cacheRead = usage.cache_read_tokens || usage.cache_tokens || 0
                                        const cacheWrite = usage.cache_write_tokens || 0
                                        const reasoning = usage.reasoning_tokens || 0
                                        const cachePercent = usage.input_tokens > 0 ? (cacheRead / usage.input_tokens) * 100 : 0
                                        const modelKey = `${runCost.runFolder}-${modelId}`
                                        const isModelExpanded = expandedCostModels.has(modelKey)

                                        // Calculate step-wise breakdown for this model
                                        const modelSteps = runCost.tokenUsage && runCost.tokenUsage.by_step_and_model
                                          ? Object.entries(runCost.tokenUsage.by_step_and_model)
                                              .map(([stepKey, modelMap]) => {
                                                const stepUsage = modelMap[modelId]
                                                if (!stepUsage) return null

                                                const parts = stepKey.split(':')
                                                const phase = parts[0]
                                                const stepID = parts[1] || ''  // New format: stepID instead of index

                                                let phaseLabel = ''
                                                if (phase === 'execution_only') { phaseLabel = 'Execution' }
                                                else if (phase.includes('learning')) { phaseLabel = 'Learning' }
                                                else if (phase.startsWith('kb_')) { phaseLabel = 'Knowledgebase' }
                                                else if (phase === 'routing' || phase === 'todo_task') { phaseLabel = 'Routing' }
                                                else if (phase === 'review_step_code' || phase === 'goal_advisor' || phase === 'plan_change' || phase === 'replan_workflow_from_results') { phaseLabel = 'Workshop' }
                                                else if (phase === 'evaluation_scoring' || phase.startsWith('evaluation')) { phaseLabel = 'Evaluation' }
                                                else { phaseLabel = phase }

                                                // Try to find step info from stepID
                                                let stepNum = 0
                                                let stepTitle = stepID
                                                if (runCost.steps) {
                                                  for (const [key, stepData] of Object.entries(runCost.steps)) {
                                                    if (stepData.step_id === stepID) {
                                                      const match = key.match(/step-(\d+)/)
                                                      stepNum = match ? parseInt(match[1], 10) : 0
                                                      stepTitle = stepData.title || stepID
                                                      break
                                                    }
                                                  }
                                                }

                                                let label = ''
                                                if (stepNum > 0) {
                                                  label = `Step ${stepNum}: ${stepTitle} (${phaseLabel})`
                                                } else {
                                                  // Phase-only agent
                                                  label = `${stepTitle} (${phaseLabel})`
                                                }

                                                return { key: stepKey, label, usage: stepUsage, stepNum }
                                              })
                                              .filter((s): s is NonNullable<typeof s> => s !== null)
                                              .sort((a, b) => {
                                                // Sort by step number first, then by label
                                                if (a.stepNum !== b.stepNum) return a.stepNum - b.stepNum
                                                return a.label.localeCompare(b.label)
                                              })
                                          : []

                                        return (
                                          <React.Fragment key={modelId}>
                                            <tr className="hover:bg-accent/50 transition-colors cursor-pointer" onClick={() => toggleCostModel(modelKey)}>
                                              <td className="py-2 pl-2">
                                                {isModelExpanded ? (
                                                  <ChevronDown className="w-3 h-3 text-muted-foreground" />
                                                ) : (
                                                  <ChevronRight className="w-3 h-3 text-muted-foreground" />
                                                )}
                                              </td>
                                              <td className="py-2">
                                                <div className="font-mono text-foreground font-medium">{modelId}</div>
                                                <div className="text-[10px] text-muted-foreground uppercase">{usage.provider}</div>
                                              </td>
                                              <td className="py-2 text-right text-muted-foreground">{usage.input_tokens.toLocaleString()}</td>
                                              <td className="py-2 text-right">
                                                <div className="text-foreground">{cacheRead.toLocaleString()}</div>
                                                {cachePercent > 0 && (
                                                  <div className="text-[10px] text-green-600 dark:text-green-400">({cachePercent.toFixed(0)}%)</div>
                                                )}
                                              </td>
                                              <td className="py-2 text-right text-muted-foreground">{cacheWrite > 0 ? cacheWrite.toLocaleString() : '-'}</td>
                                              <td className="py-2 text-right text-muted-foreground">{reasoning > 0 ? reasoning.toLocaleString() : '-'}</td>
                                              <td className="py-2 text-right text-muted-foreground">{usage.output_tokens.toLocaleString()}</td>
                                              <td className="py-2 text-right text-green-600 dark:text-green-400 font-semibold">{formatUSD(usage.total_cost_usd)}</td>
                                            </tr>
                                            {isModelExpanded && modelSteps.length > 0 && (
                                              <tr className="bg-muted/20">
                                                <td colSpan={8} className="p-0">
                                                  <div className="p-4 space-y-4">
                                                    <div className="border border-border rounded-md overflow-hidden bg-background">
                                                      <div className="bg-muted/50 px-4 py-2 border-b border-border flex justify-between items-center">
                                                        <h4 className="font-semibold text-xs text-foreground flex items-center gap-2">
                                                          <List className="w-3.5 h-3.5" /> Usage by Step
                                                        </h4>
                                                      </div>
                                                      <div className="overflow-x-auto">
                                                        <table className="w-full text-xs">
                                                          <thead>
                                                            <tr className="text-muted-foreground border-b border-border bg-muted/30">
                                                              <th className="px-4 py-2 text-left font-medium">Step</th>
                                                              <th className="px-4 py-2 text-right font-medium">Input</th>
                                                              <th className="px-4 py-2 text-right font-medium">Cached In</th>
                                                              <th className="px-4 py-2 text-right font-medium">Reasoning</th>
                                                              <th className="px-4 py-2 text-right font-medium">Output</th>
                                                              <th className="px-4 py-2 text-right font-medium">Cost</th>
                                                            </tr>
                                                          </thead>
                                                          <tbody className="divide-y divide-border">
                                                            {modelSteps.map((step) => (
                                                              <tr key={step.key} className="hover:bg-muted/30 transition-colors">
                                                                <td className="px-4 py-2">
                                                                  <span className="font-medium text-foreground">{step.label}</span>
                                                                </td>
                                                                <td className="px-4 py-2 text-right text-muted-foreground">{step.usage.input_tokens.toLocaleString()}</td>
                                                                <td className="px-4 py-2 text-right text-muted-foreground">
                                                                  {(step.usage.cache_read_tokens || step.usage.cache_tokens || 0).toLocaleString()}
                                                                </td>
                                                                <td className="px-4 py-2 text-right text-muted-foreground">
                                                                  {(step.usage.reasoning_tokens || 0).toLocaleString()}
                                                                </td>
                                                                <td className="px-4 py-2 text-right text-muted-foreground">{step.usage.output_tokens.toLocaleString()}</td>
                                                                <td className="px-4 py-2 text-right text-green-600 dark:text-green-400 font-medium">{formatUSD(step.usage.total_cost_usd)}</td>
                                                              </tr>
                                                            ))}
                                                          </tbody>
                                                        </table>
                                                      </div>
                                                    </div>
                                                  </div>
                                                </td>
                                              </tr>
                                            )}
                                          </React.Fragment>
                                        )
                                      })}
                                      {runCost.tokenUsage && Object.entries(runCost.tokenUsage.by_tool || {}).map(([toolId, usage]) => (
                                        <tr key={`tool-${toolId}`} className="hover:bg-accent/50 transition-colors">
                                          <td className="py-2 pl-2"></td>
                                          <td className="py-2">
                                            <div className="font-mono text-foreground font-medium">{usage.tool_name || toolId}</div>
                                            <div className="text-[10px] text-muted-foreground uppercase">
                                              {[usage.provider, usage.model_id, usage.estimated ? 'estimated' : ''].filter(Boolean).join(' | ')}
                                            </div>
                                          </td>
                                          <td className="py-2 text-right text-muted-foreground" colSpan={5}>
                                            {usage.quantity ? `${usage.quantity.toFixed(4)} ${usage.unit || ''}` : (usage.unit || 'tool usage')}
                                          </td>
                                          <td className="py-2 text-right text-green-600 dark:text-green-400 font-semibold">{formatUSD(usage.total_cost_usd)}</td>
                                        </tr>
                                      ))}
                                      {runCost.evaluationTokenUsage && Object.entries(runCost.evaluationTokenUsage.by_tool || {}).map(([toolId, usage]) => (
                                        <tr key={`eval-tool-${toolId}`} className="hover:bg-accent/50 transition-colors">
                                          <td className="py-2 pl-2"></td>
                                          <td className="py-2">
                                            <div className="font-mono text-foreground font-medium">{usage.tool_name || toolId}</div>
                                            <div className="text-[10px] text-muted-foreground uppercase">
                                              {['evaluation', usage.provider, usage.model_id, usage.estimated ? 'estimated' : ''].filter(Boolean).join(' | ')}
                                            </div>
                                          </td>
                                          <td className="py-2 text-right text-muted-foreground" colSpan={5}>
                                            {usage.quantity ? `${usage.quantity.toFixed(4)} ${usage.unit || ''}` : (usage.unit || 'tool usage')}
                                          </td>
                                          <td className="py-2 text-right text-green-600 dark:text-green-400 font-semibold">{formatUSD(usage.total_cost_usd)}</td>
                                        </tr>
                                      ))}
                                    </tbody>
                                    <tfoot>
                                      <tr className="border-t-2 border-border font-bold">
                                        <td></td>
                                        <td className="py-3 text-foreground">Total Summary</td>
                                        <td className="py-3 text-right text-muted-foreground">{costSummary.totalInputTokens.toLocaleString()}</td>
                                        <td className="py-3 text-right text-muted-foreground">
                                          {costSummary.totalCacheReadTokens.toLocaleString()}
                                          {costSummary.totalInputTokens > 0 && (
                                            <span className="text-[10px] text-muted-foreground ml-1">
                                              ({((costSummary.totalCacheReadTokens / costSummary.totalInputTokens) * 100).toFixed(0)}%)
                                            </span>
                                          )}
                                        </td>
                                        <td className="py-3 text-right text-muted-foreground">{costSummary.totalCacheWriteTokens.toLocaleString()}</td>
                                        <td className="py-3 text-right text-muted-foreground">{costSummary.totalReasoningTokens.toLocaleString()}</td>
                                        <td className="py-3 text-right text-muted-foreground">{costSummary.totalOutputTokens.toLocaleString()}</td>
                                        <td className="py-3 text-right text-green-600 dark:text-green-400">{formatUSD(costSummary.totalCost)}</td>
                                      </tr>
                                    </tfoot>
                                  </table>
                                </div>
                              )}
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  )
                })}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        {!embedded && <div className="px-6 py-4 border-t border-border flex justify-end bg-background rounded-b-lg">
          <button
            onClick={onClose}
            className="px-4 py-2 bg-secondary text-secondary-foreground rounded-md hover:bg-secondary/80 transition-colors text-sm font-medium"
          >
            Close
          </button>
        </div>}
      </div>
  )

  if (embedded) return shell

  return (
    <ModalPortal>
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/50 backdrop-blur-sm p-2 sm:p-4">
      {shell}
    </div>
    </ModalPortal>
  )
}

export default CostsPopup

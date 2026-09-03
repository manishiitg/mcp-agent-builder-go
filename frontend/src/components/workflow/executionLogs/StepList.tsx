import { useMemo } from 'react'
import type React from 'react'
import {
  ChevronRight,
  ChevronDown,
  FileText,
  Clock,
  Split,
  Route as RouteIcon,
  Gauge,
} from 'lucide-react'
import type { ExecutionLogsResponse, StepExecutionLogs } from '../../../services/api-types'
import { StepMetricChip } from './LogPrimitives'
import {
  formatDuration,
  formatStepStartedAt,
  formatTokenCount,
  getStepFirstActivityMs,
  getStepIcon,
  getStepIndentStyle,
  getStepMetrics,
  getStepModel,
  getStepNestingClass,
  getStepNestingLevel,
  getStepResultPreview,
  getStepStatus,
  getStepTypeBadgeStyle,
  getStepTypeDescription,
  getStepTypeLabel,
  hasKnowledgebaseSignal,
  hasLearningSignal,
  hasStepMetrics,
  sortStepEntriesByExecution,
} from './helpers'

export interface StepListProps {
  logs: ExecutionLogsResponse | null
  focusedStepId: string | undefined
  routeFilterKey: string | null
  expandedSteps: Set<string>
  toggleStep: (stepId: string) => void
  renderStepContent: (stepId: string, stepLogs: StepExecutionLogs) => React.ReactNode
}

export function StepList({ logs, focusedStepId, routeFilterKey, expandedSteps, toggleStep, renderStepContent }: StepListProps) {
  // The sort comparator walks every execution of both steps per comparison;
  // do it once per logs payload, not on every 2.5 s poll re-render.
  const sortedStepEntries = useMemo(
    () => Object.entries(logs?.steps || {}).sort(sortStepEntriesByExecution),
    [logs],
  )
  return (
    <>
              {sortedStepEntries
                .filter(([stepId]) => !focusedStepId || stepId === focusedStepId)
                .filter(([, stepLogs]) =>
                  !routeFilterKey ||
                  (stepLogs.route_kind === 'routing' && `${stepLogs.route_step_id}::${stepLogs.route_id}` === routeFilterKey)
                )
                .map(([stepId, stepLogs]) => {
                  const isExpanded = expandedSteps.has(stepId)
                  const displayId = stepLogs.original_id || stepId
                  const displayTitle = (stepLogs.title && stepLogs.title.trim()) ? stepLogs.title : displayId
                  const resultPreview = getStepResultPreview(stepLogs)
                  const nestingLevel = getStepNestingLevel(stepId)
                  const indentStyle = getStepIndentStyle(nestingLevel)
                  const nestingClass = getStepNestingClass(stepId)
                  const stepMetrics = getStepMetrics(stepLogs.executions || [])
                  const showMetrics = hasStepMetrics(stepMetrics)
                  const stepModel = getStepModel(stepLogs.executions || [])
                  const executionTier = stepLogs.execution_tier?.trim()
                  const stepStartedAtMs = getStepFirstActivityMs(stepLogs)

                  const stepStatus = getStepStatus(stepLogs)

                  // Determine card styles and glow based on execution status
                  let cardBorderClass = 'border-border'
                  let cardBgClass = 'bg-card'
                  let accentBarClass = 'bg-muted-foreground/20'
                  if (stepStatus === 'running') {
                    cardBorderClass = 'border-indigo-500/30 dark:border-indigo-500/40 shadow-[0_0_12px_rgba(99,102,241,0.08)]'
                    cardBgClass = 'bg-indigo-500/[0.02] dark:bg-indigo-500/[0.04] hover:bg-indigo-500/[0.03] dark:hover:bg-indigo-500/[0.05] animate-pulse-subtle'
                    accentBarClass = 'bg-indigo-500'
                  } else if (stepStatus === 'completed') {
                    cardBorderClass = 'border-emerald-500/15 dark:border-emerald-500/25'
                    cardBgClass = 'bg-muted/5 dark:bg-card/20 hover:bg-muted/10 dark:hover:bg-card/40'
                    accentBarClass = 'bg-emerald-500'
                  } else if (stepStatus === 'failed') {
                    cardBorderClass = 'border-rose-500/25 dark:border-rose-500/35 shadow-[0_0_10px_rgba(244,63,94,0.03)]'
                    cardBgClass = 'bg-rose-500/[0.01] dark:bg-rose-500/[0.02] hover:bg-rose-500/[0.02] dark:hover:bg-rose-500/[0.04]'
                    accentBarClass = 'bg-rose-500'
                  } else {
                    cardBorderClass = 'border-border/80 opacity-80'
                    cardBgClass = 'bg-muted/5 dark:bg-card/20 hover:bg-muted/10 dark:hover:bg-card/40'
                    accentBarClass = 'bg-muted-foreground/20'
                  }

                  return (
                    <div key={stepId} className={`relative border ${cardBorderClass} ${cardBgClass} rounded-lg overflow-hidden transition-all duration-300 ${nestingClass}`} style={indentStyle}>
                      {/* Left accent bar indicator */}
                      <div className={`absolute left-0 top-0 bottom-0 w-[4px] ${accentBarClass}`} />

                      <button
                        onClick={() => toggleStep(stepId)}
                        aria-expanded={isExpanded}
                        aria-controls={`execution-step-${stepId}`}
                        aria-label={`${isExpanded ? 'Collapse' : 'Expand'} ${displayTitle}`}
                        className={`
                          w-full flex flex-col gap-2 pl-5 pr-4 py-3 text-left transition-colors
                          ${isExpanded ? 'bg-accent/30' : 'hover:bg-accent/40'}
                        `}
                      >
                        <div className="flex w-full items-start justify-between gap-3">
                          <div className="flex min-w-0 items-start gap-3 overflow-hidden flex-1">
                            {isExpanded ? <ChevronDown className="w-4 h-4 text-muted-foreground flex-shrink-0 mt-0.5" /> : <ChevronRight className="w-4 h-4 text-muted-foreground flex-shrink-0 mt-0.5" />}
                            
                            <div className="flex flex-col items-start text-left min-w-0">
                              <div className="flex items-center gap-2 flex-wrap">
                                <span className="flex-shrink-0" title={getStepTypeDescription(stepLogs.type || 'regular')}>
                                  {getStepIcon(stepLogs.type)}
                                </span>
                                <span className="text-sm font-semibold text-foreground truncate">{displayTitle}</span>
                                <span title={getStepTypeDescription(stepLogs.type || 'regular')} className={`inline-flex items-center px-1.5 py-0.5 rounded-md text-[10px] font-medium border ${getStepTypeBadgeStyle(stepLogs.type)}`}>
                                  {getStepTypeLabel(stepLogs.type)}
                                </span>
                              </div>
                              {resultPreview && (
                                <span className="mt-0.5 w-full truncate pl-6 text-xs text-muted-foreground">{resultPreview}</span>
                              )}
                            </div>
                          </div>
                        </div>
                        
                        <div className="flex w-full flex-wrap items-center gap-1.5 pl-10 text-xs text-muted-foreground">
                          {stepModel && (
                            <StepMetricChip title={`Model used on the most recent attempt: ${stepModel}`}>
                              {stepModel}
                            </StepMetricChip>
                          )}
                          {executionTier && (
                            <StepMetricChip title={`Execution tier pinned in step config: ${executionTier}`}>
                              <Gauge className="h-3 w-3" />
                              {executionTier}
                            </StepMetricChip>
                          )}
                          {showMetrics && (
                            <>
                              {stepMetrics.totalTokens > 0 && (
                                <StepMetricChip title={`Tokens used: ${stepMetrics.totalTokens.toLocaleString()} total (${stepMetrics.inputTokens.toLocaleString()} input, ${stepMetrics.outputTokens.toLocaleString()} output${stepMetrics.reasoningTokens > 0 ? `, ${stepMetrics.reasoningTokens.toLocaleString()} reasoning` : ''}${stepMetrics.cacheTokens > 0 ? `, ${stepMetrics.cacheTokens.toLocaleString()} cache` : ''})`}>
                                  {formatTokenCount(stepMetrics.totalTokens)} tok total
                                </StepMetricChip>
                              )}
                              {stepMetrics.inputTokens > 0 && (
                                <StepMetricChip title={`Input tokens: ${stepMetrics.inputTokens.toLocaleString()}`}>
                                  {formatTokenCount(stepMetrics.inputTokens)} in
                                </StepMetricChip>
                              )}
                              {stepMetrics.outputTokens > 0 && (
                                <StepMetricChip title={`Output tokens: ${stepMetrics.outputTokens.toLocaleString()}${stepMetrics.reasoningTokens > 0 ? ` (includes ${stepMetrics.reasoningTokens.toLocaleString()} reasoning)` : ''}`}>
                                  {formatTokenCount(stepMetrics.outputTokens)} out
                                </StepMetricChip>
                              )}
                              {stepMetrics.cacheTokens > 0 && (
                                <StepMetricChip title={`Cached tokens: ${stepMetrics.cacheTokens.toLocaleString()}`}>
                                  {formatTokenCount(stepMetrics.cacheTokens)} cache
                                </StepMetricChip>
                              )}
                              {stepMetrics.durationMs > 0 && (
                                <StepMetricChip title={`Time taken: ${formatDuration(stepMetrics.durationMs)}${stepMetrics.llmCalls > 0 ? ` across ${stepMetrics.llmCalls} LLM call${stepMetrics.llmCalls !== 1 ? 's' : ''}` : ''}`}>
                                  <Clock className="h-3 w-3" />
                                  {formatDuration(stepMetrics.durationMs)}
                                </StepMetricChip>
                              )}
                            </>
                          )}
                          {stepStartedAtMs > 0 && (
                            <StepMetricChip title={`First recorded activity: ${new Date(stepStartedAtMs).toLocaleString()}`}>
                              <Clock className="h-3 w-3" />
                              {formatStepStartedAt(stepStartedAtMs)}
                            </StepMetricChip>
                          )}
                          {stepLogs.description?.trim() && (
                            <StepMetricChip title={`Authored step instructions: ${stepLogs.description.length.toLocaleString()} characters`}>
                              <FileText className="h-3 w-3" />
                              {stepLogs.description.length >= 1000
                                ? `${(stepLogs.description.length / 1000).toFixed(stepLogs.description.length >= 10_000 ? 0 : 1)}k`
                                : stepLogs.description.length} instr
                            </StepMetricChip>
                          )}
                          {stepLogs.parent_step_title && (
                            <StepMetricChip title={`This route was dispatched by ${stepLogs.parent_step_title}${stepLogs.route_id ? ` (${stepLogs.route_id})` : ''}`}>
                              <Split className="h-3 w-3 text-sky-600 dark:text-sky-300" />
                              ↳ {stepLogs.parent_step_title}
                            </StepMetricChip>
                          )}
                          {stepLogs.route_kind === 'routing' && stepLogs.route_name && (
                            <StepMetricChip title={`Reached via the "${stepLogs.route_name}" route, selected by ${stepLogs.route_step_title || stepLogs.route_step_id}`}>
                              <RouteIcon className="h-3 w-3 text-teal-600 dark:text-teal-300" />
                              ↳ {stepLogs.route_name}
                            </StepMetricChip>
                          )}
                          <span className="whitespace-nowrap">
                            {stepLogs.executions.length} exec
                            {hasLearningSignal(stepLogs) && ' • learning'}
                            {hasKnowledgebaseSignal(stepLogs) && ' • kb'}
                            {stepLogs.todo_task && stepLogs.todo_task.length > 0 && ` • ${stepLogs.todo_task.length} todo`}
                          </span>
                        </div>
                      </button>

                      {isExpanded && (
                        <div id={`execution-step-${stepId}`}>
                          {renderStepContent(stepId, stepLogs)}
                        </div>
                      )}
                    </div>
                  )
                })}
    </>
  )
}

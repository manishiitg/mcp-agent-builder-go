import React from 'react'
import { DollarSign, Award, TrendingUp, TrendingDown } from 'lucide-react'
import { formatUSD, formatTokens, formatTimestampLabel } from './helpers'
import type { CostsData } from './useCostsData'

type CostsSummarySectionsProps = Pick<
  CostsData,
  'hasScopedActivity' | 'phaseCostSummary' | 'phaseDailyCostSummaries' | 'aggregateSummary'
>

const CostsSummarySections: React.FC<CostsSummarySectionsProps> = ({
  hasScopedActivity,
  phaseCostSummary,
  phaseDailyCostSummaries,
  aggregateSummary,
}) => (
  <>
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
  </>
)

export default CostsSummarySections

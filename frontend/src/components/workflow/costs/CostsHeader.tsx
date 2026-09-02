import React from 'react'
import { X, DollarSign, Coins, RefreshCw } from 'lucide-react'
import { formatStartedAt } from '../../../utils/duration'
import { formatUSD, formatTokens } from './helpers'
import type { CostsData } from './useCostsData'

type CostsHeaderProps = Pick<CostsData, 'overallSummary' | 'aggregateSummary' | 'phaseCostSummary' | 'loading' | 'loadAllCosts'> & {
  startedAt?: string | null
  embedded: boolean
  onClose: () => void
}

const CostsHeader: React.FC<CostsHeaderProps> = ({
  startedAt,
  overallSummary,
  aggregateSummary,
  phaseCostSummary,
  loading,
  loadAllCosts,
  embedded,
  onClose,
}) => (
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
)

export default CostsHeader

import React from 'react'
import {
  Loader2,
  AlertCircle,
  DollarSign,
} from 'lucide-react'
import ModalPortal from '../ui/ModalPortal'
import { useCostsData } from './costs/useCostsData'
import CostsHeader from './costs/CostsHeader'
import CostsDailySection from './costs/CostsDailySection'
import CostsSummarySections from './costs/CostsSummarySections'
import RunCostsSection from './costs/RunCostsSection'

interface CostsPopupProps {
  isOpen: boolean
  onClose: () => void
  workspacePath: string | null
  runFolders: string[] // Available run folders
  selectedRunFolder: string | null // Currently selected run folder
  startedAt?: string | null
  embedded?: boolean
}

const CostsPopup: React.FC<CostsPopupProps> = ({
  isOpen,
  onClose,
  workspacePath,
  selectedRunFolder,
  startedAt,
  embedded = false,
}) => {
  const data = useCostsData({ isOpen, embedded, workspacePath, selectedRunFolder })
  const {
    loading,
    error,
    runCosts,
    phaseCostSummary,
    phaseDailyCostSummaries,
    hasScopedActivity,
    activityBreakdown,
    expandedRunFolders,
    expandedCostModels,
    costViewMode,
    routeFilterByRunFolder,
    expandedDailyDate,
    setExpandedDailyDate,
    costHistory,
    loadingOlder,
    loadAllCosts,
    loadOlderCosts,
    toggleRunFolder,
    toggleCostModel,
    setViewModeForRunFolder,
    setRouteFilterForRunFolder,
    aggregateSummary,
    overallSummary,
    combinedDailyCostSummaries,
    dailyActivityBreakdown,
  } = data

  if (!embedded && !isOpen) return null

  const shell = (
      <div className={embedded
        ? 'flex h-full min-h-0 w-full flex-col bg-background'
        : 'bg-background rounded-lg shadow-xl w-full max-w-6xl max-h-[calc(100dvh-1rem)] sm:max-h-[90vh] flex flex-col border border-border relative'}>
        {/* Header */}
        <CostsHeader
          startedAt={startedAt}
          overallSummary={overallSummary}
          aggregateSummary={aggregateSummary}
          phaseCostSummary={phaseCostSummary}
          loading={loading}
          loadAllCosts={loadAllCosts}
          embedded={embedded}
          onClose={onClose}
        />

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
              <CostsDailySection
                hasScopedActivity={hasScopedActivity}
                activityBreakdown={activityBreakdown}
                combinedDailyCostSummaries={combinedDailyCostSummaries}
                dailyActivityBreakdown={dailyActivityBreakdown}
                expandedDailyDate={expandedDailyDate}
                setExpandedDailyDate={setExpandedDailyDate}
                costHistory={costHistory}
                loadingOlder={loadingOlder}
                loadOlderCosts={loadOlderCosts}
              />

              <CostsSummarySections
                hasScopedActivity={hasScopedActivity}
                phaseCostSummary={phaseCostSummary}
                phaseDailyCostSummaries={phaseDailyCostSummaries}
                aggregateSummary={aggregateSummary}
              />

              <RunCostsSection
                hasScopedActivity={hasScopedActivity}
                runCosts={runCosts}
                selectedRunFolder={selectedRunFolder}
                expandedRunFolders={expandedRunFolders}
                expandedCostModels={expandedCostModels}
                costViewMode={costViewMode}
                routeFilterByRunFolder={routeFilterByRunFolder}
                toggleRunFolder={toggleRunFolder}
                toggleCostModel={toggleCostModel}
                setViewModeForRunFolder={setViewModeForRunFolder}
                setRouteFilterForRunFolder={setRouteFilterForRunFolder}
              />
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

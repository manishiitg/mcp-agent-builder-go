import { Settings, RefreshCw, X } from 'lucide-react'
import MCPConfigPopup from '../MCPConfigPopup'
import ConnectorsBrowser from '../connectors/ConnectorsBrowser'
import { useMCPStore } from '../../stores'

export default function MCPServersSection() {
  const {
    isLoadingTools,
    showMCPDetails,
    setShowMCPDetails,
    showConfigEditor,
    setShowConfigEditor,
    refreshTools,
  } = useMCPStore()

  return (
    <>
      {/* Connectors Modal */}
      {showMCPDetails && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="flex h-[85vh] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-gray-200 bg-white shadow-2xl dark:border-gray-700 dark:bg-gray-900">
            {/* Header */}
            <div className="flex shrink-0 items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-800">
              <div className="flex items-center gap-2 text-sm">
                <span className="text-gray-500 dark:text-gray-400">Connectors</span>
                <span className="text-gray-300 dark:text-gray-600">/</span>
                <span className="font-medium text-gray-900 dark:text-gray-100">Directory</span>
              </div>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => refreshTools()}
                  className="rounded-md p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                  title="Refresh connectors"
                  aria-label="Refresh connectors"
                >
                  <RefreshCw className={`h-4 w-4 ${isLoadingTools ? 'animate-spin' : ''}`} />
                </button>
                <button
                  onClick={() => {
                    setShowMCPDetails(false)
                    setShowConfigEditor(true)
                  }}
                  className="rounded-md p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                  title="Configure MCP Server (advanced)"
                  aria-label="Configure MCP Server"
                >
                  <Settings className="h-4 w-4" />
                </button>
                <button
                  onClick={() => setShowMCPDetails(false)}
                  className="rounded-md p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                  aria-label="Close"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            </div>

            <div className="min-h-0 flex-1 px-6 pb-6 pt-5">
              <ConnectorsBrowser />
            </div>
          </div>
        </div>
      )}

      {/* MCP Config Popup Modal */}
      {showConfigEditor && (
        <MCPConfigPopup
          initialView="json"
          onConfigChange={() => {
            // Refresh tools after config change
            refreshTools()
          }}
          onClose={() => {
            setShowConfigEditor(false)
            setShowMCPDetails(true)
          }}
        />
      )}
    </>
  )
}

import { useEffect, useState } from 'react'
import { Loader2, AlertCircle, Settings, RefreshCw, ChevronDown, ChevronRight, X } from 'lucide-react'
import MCPConfigPopup from '../MCPConfigPopup'
import { OAuthStatusBadge } from '../OAuthStatusBadge'
import ConnectionIcon from '../connectors/ConnectionIcon'
import { brandSlugFor } from '../connectors/brandSlug'
import { useMCPStore } from '../../stores'

/**
 * A server awaiting OAuth is not broken — it simply has no token yet. The
 * backend still reports those as "error", so `requires_oauth` is what
 * distinguishes "needs connecting" from a genuine failure.
 */
const isAwaitingAuth = (status: string | undefined, requiresOAuth: boolean | undefined) =>
  status === 'not_connected' || (status !== 'ok' && !!requiresOAuth)

const statusDotClass = (status: string | undefined, requiresOAuth: boolean | undefined) => {
  if (status === 'ok') return 'bg-green-500'
  if (status === 'loading') return 'bg-amber-400'
  if (isAwaitingAuth(status, requiresOAuth)) return 'bg-gray-400'
  return 'bg-red-500'
}

const statusLabel = (status: string | undefined, requiresOAuth: boolean | undefined) => {
  if (status === 'ok') return 'Connected'
  if (status === 'loading') return 'Checking...'
  if (isAwaitingAuth(status, requiresOAuth)) return 'Not connected'
  return 'Error'
}

export default function MCPServersSection() {

  // Store subscriptions
  const {
    toolList,
    isLoadingTools,
    toolsError,
    showMCPDetails,
    setShowMCPDetails,
    showConfigEditor,
    setShowConfigEditor,
    getServerGroups,
    refreshTools,
    serverLogs,
    fetchServerLogs
  } = useMCPStore()

  const [expandedLogs, setExpandedLogs] = useState<Set<string>>(new Set())
  const [loadingLogs, setLoadingLogs] = useState<Set<string>>(new Set())

  const toggleLogs = async (serverName: string) => {
    const newExpanded = new Set(expandedLogs)
    if (newExpanded.has(serverName)) {
      newExpanded.delete(serverName)
      setExpandedLogs(newExpanded)
    } else {
      newExpanded.add(serverName)
      setExpandedLogs(newExpanded)
      setLoadingLogs(prev => new Set([...prev, serverName]))
      await fetchServerLogs(serverName)
      setLoadingLogs(prev => {
        const next = new Set(prev)
        next.delete(serverName)
        return next
      })
    }
  }

  const refreshLogs = async (serverName: string) => {
    setLoadingLogs(prev => new Set([...prev, serverName]))
    await fetchServerLogs(serverName)
    setLoadingLogs(prev => {
      const next = new Set(prev)
      next.delete(serverName)
      return next
    })
  }

  useEffect(() => {
    if (!showMCPDetails || expandedLogs.size === 0) return

    const interval = window.setInterval(() => {
      expandedLogs.forEach((serverName) => {
        fetchServerLogs(serverName)
      })
    }, 3000)

    return () => window.clearInterval(interval)
  }, [expandedLogs, fetchServerLogs, showMCPDetails])

  return (
    <>
      {/* Connectors Modal */}
      {showMCPDetails && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
          <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-xl w-full max-w-2xl max-h-[85vh] overflow-y-auto">
            <div className="flex items-center justify-between px-5 py-4 border-b border-gray-200 dark:border-gray-700 sticky top-0 z-10 bg-white dark:bg-gray-800">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
                Connectors
              </h3>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => refreshTools()}
                  className="p-1.5 text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-md transition-colors"
                  title="Refresh connectors"
                  aria-label="Refresh connectors"
                >
                  <RefreshCw className={`w-4 h-4 ${isLoadingTools ? 'animate-spin' : ''}`} />
                </button>
                <button
                  onClick={() => {
                    setShowMCPDetails(false)
                    setShowConfigEditor(true)
                  }}
                  className="p-1.5 text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-md transition-colors"
                  title="Configure MCP Server (advanced)"
                  aria-label="Configure MCP Server"
                >
                  <Settings className="w-4 h-4" />
                </button>
                <button
                  onClick={() => setShowMCPDetails(false)}
                  className="p-1.5 text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-md transition-colors"
                  aria-label="Close"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>
            </div>

            <div className="p-3 space-y-1">
              {isLoadingTools && toolList.length === 0 && (
                <div className="flex items-center gap-2 px-2 py-3 text-sm text-gray-500 dark:text-gray-400">
                  <Loader2 className="w-4 h-4 animate-spin" />
                  <span>Loading connectors...</span>
                </div>
              )}

              {toolsError && (
                <div className="flex items-center gap-2 px-2 py-3 text-sm text-red-500 dark:text-red-400">
                  <AlertCircle className="w-4 h-4" />
                  <span>Error: {toolsError}</span>
                </div>
              )}

              {Object.entries(getServerGroups()).map(([serverName, tools]) => {
                // eslint-disable-next-line @typescript-eslint/no-explicit-any
                const requiresOAuth = (tools[0] as any).requires_oauth as boolean | undefined

                return (
                <div
                  key={serverName}
                  className="flex flex-col rounded-lg hover:bg-gray-50 dark:hover:bg-gray-900/40 transition-colors"
                >
                  <div className="flex items-center gap-3 px-2 py-2.5">
                    <ConnectionIcon icon={brandSlugFor(serverName)} size="sm" />

                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <span className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">
                          {serverName}
                        </span>
                        <span
                          className={`w-1.5 h-1.5 rounded-full shrink-0 ${statusDotClass(tools[0].status, requiresOAuth)}`}
                        ></span>
                      </div>
                      <span className="text-xs text-gray-500 dark:text-gray-400">
                        {statusLabel(tools[0].status, requiresOAuth)}
                      </span>
                    </div>

                    {/* Connect / Disconnect control */}
                    <OAuthStatusBadge
                      serverName={serverName}
                      requiresOAuth={requiresOAuth}
                      onAuthChange={() => {
                        // Refresh on disconnect too — the row's status text reads
                        // from the store, so skipping this left it claiming
                        // "Connected" after the token was revoked.
                        refreshTools()
                      }}
                    />

                    <button
                      onClick={() => toggleLogs(serverName)}
                      className="p-1.5 text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-md transition-colors shrink-0"
                      title="Connection logs"
                      aria-label="Toggle connection logs"
                    >
                      {expandedLogs.has(serverName) ? (
                        <ChevronDown className="w-3.5 h-3.5" />
                      ) : (
                        <ChevronRight className="w-3.5 h-3.5" />
                      )}
                    </button>
                  </div>

                  {/* Connection Logs Panel */}
                  {expandedLogs.has(serverName) && (
                    <div className="mx-2 mb-2 px-3 py-2 bg-gray-900 dark:bg-black rounded-md">
                      <div className="flex items-center justify-between mb-2">
                        <h5 className="text-[10px] font-semibold text-gray-400 uppercase tracking-wide">
                          Connection Logs
                        </h5>
                        <button
                          onClick={() => refreshLogs(serverName)}
                          disabled={loadingLogs.has(serverName)}
                          className="text-xs text-gray-400 hover:text-gray-200 transition-colors disabled:opacity-50"
                        >
                          {loadingLogs.has(serverName) ? '...' : 'Refresh'}
                        </button>
                      </div>
                      <div className="max-h-40 overflow-y-auto font-mono text-xs space-y-0.5">
                        {loadingLogs.has(serverName) && !serverLogs[serverName]?.length ? (
                          <div className="text-gray-400 flex items-center gap-2">
                            <div className="w-3 h-3 border border-gray-500 border-t-blue-400 rounded-full animate-spin"></div>
                            Loading logs...
                          </div>
                        ) : serverLogs[serverName]?.length ? (
                          serverLogs[serverName].map((log, i) => (
                            <div key={i} className="flex gap-2 py-0.5">
                              <span className="text-gray-500 whitespace-nowrap shrink-0">
                                {new Date(log.timestamp).toLocaleTimeString()}
                              </span>
                              <span className={
                                log.level === 'error' ? 'text-red-400' :
                                log.level === 'warn' ? 'text-yellow-400' :
                                log.level === 'debug' ? 'text-gray-500' :
                                'text-green-400'
                              }>
                                {log.message}
                              </span>
                            </div>
                          ))
                        ) : (
                          <div className="text-gray-500">No logs available yet.</div>
                        )}
                      </div>
                    </div>
                  )}
                </div>
                )
              })}
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
            refreshTools();
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

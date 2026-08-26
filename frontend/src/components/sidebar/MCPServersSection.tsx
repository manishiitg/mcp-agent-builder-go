import { useEffect, useMemo, useState } from 'react'
import {
  Loader2,
  AlertCircle,
  Settings,
  RefreshCw,
  ChevronDown,
  ChevronRight,
  Search,
  X,
} from 'lucide-react'
import MCPConfigPopup from '../MCPConfigPopup'
import { OAuthStatusBadge } from '../OAuthStatusBadge'
import ConnectionIcon from '../connectors/ConnectionIcon'
import { brandSlugFor } from '../connectors/brandSlug'
import { descriptionFor } from '../connectors/catalog'
import { useMCPStore } from '../../stores'

/**
 * A server awaiting OAuth is not broken — it simply has no token yet. The
 * backend still reports those as "error", so `requires_oauth` is what
 * distinguishes "needs connecting" from a genuine failure.
 */
const isAwaitingAuth = (status: string | undefined, requiresOAuth: boolean | undefined) =>
  status === 'not_connected' || (status !== 'ok' && !!requiresOAuth)

const statusLabel = (status: string | undefined, requiresOAuth: boolean | undefined) => {
  if (status === 'ok') return 'Connected'
  if (status === 'loading') return 'Checking...'
  if (isAwaitingAuth(status, requiresOAuth)) return 'Not connected'
  return 'Error'
}

type Filter = 'all' | 'connected' | 'available'

const FILTERS: { value: Filter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'connected', label: 'Connected' },
  { value: 'available', label: 'Available' },
]

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
    fetchServerLogs,
  } = useMCPStore()

  const [expandedLogs, setExpandedLogs] = useState<Set<string>>(new Set())
  const [loadingLogs, setLoadingLogs] = useState<Set<string>>(new Set())
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<Filter>('all')

  const toggleLogs = async (serverName: string) => {
    const newExpanded = new Set(expandedLogs)
    if (newExpanded.has(serverName)) {
      newExpanded.delete(serverName)
      setExpandedLogs(newExpanded)
    } else {
      newExpanded.add(serverName)
      setExpandedLogs(newExpanded)
      setLoadingLogs((prev) => new Set([...prev, serverName]))
      await fetchServerLogs(serverName)
      setLoadingLogs((prev) => {
        const next = new Set(prev)
        next.delete(serverName)
        return next
      })
    }
  }

  const refreshLogs = async (serverName: string) => {
    setLoadingLogs((prev) => new Set([...prev, serverName]))
    await fetchServerLogs(serverName)
    setLoadingLogs((prev) => {
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

  const groups = getServerGroups()

  // Connected first, then alphabetical, so the services in use stay at the top
  // of the grid instead of scattering through it as tokens come and go.
  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return Object.entries(groups)
      .filter(([name, tools]) => {
        const connected = tools[0]?.status === 'ok'
        if (filter === 'connected' && !connected) return false
        if (filter === 'available' && connected) return false
        if (!q) return true
        return (
          name.toLowerCase().includes(q) ||
          descriptionFor(name).toLowerCase().includes(q)
        )
      })
      .sort(([aName, aTools], [bName, bTools]) => {
        const aOk = aTools[0]?.status === 'ok' ? 0 : 1
        const bOk = bTools[0]?.status === 'ok' ? 0 : 1
        return aOk - bOk || aName.localeCompare(bName)
      })
  }, [groups, query, filter])

  const total = Object.keys(groups).length

  return (
    <>
      {/* Connectors Modal */}
      {showMCPDetails && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="flex max-h-[85vh] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-gray-200 bg-white shadow-2xl dark:border-gray-700 dark:bg-gray-900">
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

            {/* Search + filter */}
            <div className="flex shrink-0 items-center gap-2 px-6 pt-5">
              <div className="relative flex-1">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                <input
                  type="text"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Search connectors"
                  aria-label="Search connectors"
                  className="w-full rounded-lg border border-gray-300 bg-white py-2.5 pl-10 pr-3 text-sm text-gray-900 placeholder-gray-400 transition-colors focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100"
                />
              </div>
              <div className="flex shrink-0 items-center rounded-lg border border-gray-300 p-0.5 dark:border-gray-700">
                {FILTERS.map((f) => (
                  <button
                    key={f.value}
                    onClick={() => setFilter(f.value)}
                    className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                      filter === f.value
                        ? 'bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900'
                        : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100'
                    }`}
                  >
                    {f.label}
                  </button>
                ))}
              </div>
            </div>

            {/* Grid */}
            <div className="min-h-0 flex-1 overflow-y-auto px-6 pb-6 pt-5">
              <div className="mb-3 flex items-baseline justify-between">
                <h3 className="text-sm font-semibold text-gray-900 dark:text-gray-100">
                  {filter === 'connected'
                    ? 'Connected'
                    : filter === 'available'
                      ? 'Available connectors'
                      : 'All connectors'}
                </h3>
                <span className="text-xs text-gray-500 dark:text-gray-400">
                  {visible.length === total
                    ? `${total} total`
                    : `${visible.length} of ${total}`}
                </span>
              </div>

              {isLoadingTools && toolList.length === 0 && (
                <div className="flex items-center gap-2 py-8 text-sm text-gray-500 dark:text-gray-400">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  <span>Loading connectors...</span>
                </div>
              )}

              {toolsError && (
                <div className="flex items-center gap-2 py-3 text-sm text-red-500 dark:text-red-400">
                  <AlertCircle className="h-4 w-4" />
                  <span>Error: {toolsError}</span>
                </div>
              )}

              {!isLoadingTools && visible.length === 0 && !toolsError && (
                <p className="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
                  No connectors match “{query}”.
                </p>
              )}

              <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                {visible.map(([serverName, tools]) => {
                  // eslint-disable-next-line @typescript-eslint/no-explicit-any
                  const requiresOAuth = (tools[0] as any).requires_oauth as boolean | undefined
                  const status = tools[0]?.status
                  const isOpen = expandedLogs.has(serverName)

                  return (
                    <div
                      key={serverName}
                      className="flex flex-col rounded-xl border border-gray-200 bg-white transition-colors hover:border-gray-300 dark:border-gray-800 dark:bg-gray-900/60 dark:hover:border-gray-700"
                    >
                      <div className="flex items-start gap-3 p-4">
                        <ConnectionIcon
                          icon={brandSlugFor(serverName)}
                          name={serverName}
                          size="lg"
                        />

                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-1.5">
                            <span className="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">
                              {serverName}
                            </span>
                          </div>
                          <p className="mt-0.5 line-clamp-2 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
                            {descriptionFor(serverName)}
                          </p>
                          <span className="mt-1.5 inline-block text-[11px] text-gray-400 dark:text-gray-500">
                            {statusLabel(status, requiresOAuth)}
                          </span>
                        </div>

                        <div className="flex shrink-0 items-center gap-1">
                          <OAuthStatusBadge
                            serverName={serverName}
                            requiresOAuth={requiresOAuth}
                            variant="icon"
                            onAuthChange={() => {
                              // Refresh on disconnect too — the card's status text
                              // reads from the store, so skipping this left it
                              // claiming "Connected" after the token was revoked.
                              refreshTools()
                            }}
                          />
                          <button
                            onClick={() => toggleLogs(serverName)}
                            className="rounded-md p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                            title="Connection logs"
                            aria-label={`${isOpen ? 'Hide' : 'Show'} logs for ${serverName}`}
                          >
                            {isOpen ? (
                              <ChevronDown className="h-3.5 w-3.5" />
                            ) : (
                              <ChevronRight className="h-3.5 w-3.5" />
                            )}
                          </button>
                        </div>
                      </div>

                      {/* Connection Logs Panel */}
                      {isOpen && (
                        <div className="mx-4 mb-4 rounded-md bg-gray-900 px-3 py-2 dark:bg-black">
                          <div className="mb-2 flex items-center justify-between">
                            <h5 className="text-[10px] font-semibold uppercase tracking-wide text-gray-400">
                              Connection Logs
                            </h5>
                            <button
                              onClick={() => refreshLogs(serverName)}
                              disabled={loadingLogs.has(serverName)}
                              className="text-xs text-gray-400 transition-colors hover:text-gray-200 disabled:opacity-50"
                            >
                              {loadingLogs.has(serverName) ? '...' : 'Refresh'}
                            </button>
                          </div>
                          <div className="max-h-40 space-y-0.5 overflow-y-auto font-mono text-xs">
                            {loadingLogs.has(serverName) && !serverLogs[serverName]?.length ? (
                              <div className="flex items-center gap-2 text-gray-400">
                                <div className="h-3 w-3 animate-spin rounded-full border border-gray-500 border-t-blue-400"></div>
                                Loading logs...
                              </div>
                            ) : serverLogs[serverName]?.length ? (
                              serverLogs[serverName].map((log, i) => (
                                <div key={i} className="flex gap-2 py-0.5">
                                  <span className="shrink-0 whitespace-nowrap text-gray-500">
                                    {new Date(log.timestamp).toLocaleTimeString()}
                                  </span>
                                  <span
                                    className={
                                      log.level === 'error'
                                        ? 'text-red-400'
                                        : log.level === 'warn'
                                          ? 'text-yellow-400'
                                          : log.level === 'debug'
                                            ? 'text-gray-500'
                                            : 'text-green-400'
                                    }
                                  >
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

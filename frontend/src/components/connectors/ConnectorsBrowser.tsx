import { useEffect, useMemo, useState } from 'react'
import {
  Loader2,
  AlertCircle,
  ChevronDown,
  ChevronRight,
  Code2,
  Search,
} from 'lucide-react'
import { OAuthStatusBadge } from '../OAuthStatusBadge'
import { Checkbox } from '../ui/checkbox'
import { isSelectedServer } from '../../utils/mcpServerAlias'
import ConnectionIcon from './ConnectionIcon'
import { brandSlugFor } from './brandSlug'
import { CATEGORY_ORDER, categoryFor, descriptionFor } from './catalog'
import { useMCPStore } from '../../stores'
import MCPConfigPopup from '../MCPConfigPopup'

/**
 * The card label answers "is this mine?", which is what `connection` reports.
 * `status` answers a different question — whether the server is currently
 * reachable — so a connected-but-down server is surfaced as an error against
 * the connected label rather than silently reading as not connected.
 */
const statusLabel = (connection: string | undefined, status: string | undefined) => {
  if (connection === 'connected') {
    if (status === 'error') return 'Connected — unreachable'
    if (status === 'loading') return 'Connected — checking...'
    return 'Connected'
  }
  return 'Not connected'
}

type Filter = 'all' | 'connected' | 'available'

const FILTERS: { value: Filter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'connected', label: 'Connected' },
  { value: 'available', label: 'Available' },
]

interface ConnectorsBrowserProps {
  // The embedded workflow-panel context: defaults the filter to "available"
  // (the framing is "here's what you can add", not connector management)
  // and drops the per-card connection-logs disclosure, which is a debugging
  // affordance that belongs in the full modal, not a tool-selection panel.
  compact?: boolean
  // When provided (the workflow-panel embedding), a connected card also gets
  // a checkbox to add/remove it from this workflow's own tool selection --
  // the point of surfacing "connect a new MCP" here in the first place.
  // Not shown for a not-yet-connected server: there's nothing to select
  // until it's connected via the OAuthStatusBadge action first.
  selectedServers?: string[]
  onToggleServer?: (serverName: string) => void
}

export default function ConnectorsBrowser({ compact = false, selectedServers, onToggleServer }: ConnectorsBrowserProps) {
  const {
    toolList,
    isLoadingTools,
    toolsError,
    getServerGroups,
    refreshTools,
    serverLogs,
    fetchServerLogs,
  } = useMCPStore()

  const [expandedLogs, setExpandedLogs] = useState<Set<string>>(new Set())
  const [loadingLogs, setLoadingLogs] = useState<Set<string>>(new Set())
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<Filter>(compact ? 'available' : 'all')
  // Local to this instance -- deliberately independent of the store's global
  // showConfigEditor/showMCPDetails (which the top-menu modal in
  // MCPServersSection.tsx owns and whose onClose reopens that modal). Reusing
  // that global flag here would either double-render the popup or incorrectly
  // pop the top-menu modal open when this is embedded in a workflow panel.
  const [showJsonConfig, setShowJsonConfig] = useState(false)

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
    if (compact || expandedLogs.size === 0) return

    const interval = window.setInterval(() => {
      expandedLogs.forEach((serverName) => {
        fetchServerLogs(serverName)
      })
    }, 3000)

    return () => window.clearInterval(interval)
  }, [compact, expandedLogs, fetchServerLogs])

  const groups = getServerGroups()

  // Connected first, then alphabetical, so the services in use stay at the top
  // of the grid instead of scattering through it as tokens come and go.
  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return Object.entries(groups)
      .filter(([name, tools]) => {
        const connected = tools[0]?.connection === 'connected'
        if (filter === 'connected' && !connected) return false
        if (filter === 'available' && connected) return false
        if (!q) return true
        return (
          name.toLowerCase().includes(q) ||
          descriptionFor(name).toLowerCase().includes(q)
        )
      })
      .sort(([aName, aTools], [bName, bTools]) => {
        const aOk = aTools[0]?.connection === 'connected' ? 0 : 1
        const bOk = bTools[0]?.connection === 'connected' ? 0 : 1
        return aOk - bOk || aName.localeCompare(bName)
      })
  }, [groups, query, filter])

  // Group into directory sections, keeping the connected-first order inside
  // each one. Only sections holding a visible connector are rendered, so a
  // search matching two services does not leave four empty headings behind.
  const sections = useMemo(() => {
    const byCategory = new Map<string, typeof visible>()
    visible.forEach((entry) => {
      const category = categoryFor(entry[0])
      const bucket = byCategory.get(category)
      if (bucket) {
        bucket.push(entry)
      } else {
        byCategory.set(category, [entry])
      }
    })

    const rank = (category: string) => {
      const i = CATEGORY_ORDER.indexOf(category)
      return i === -1 ? CATEGORY_ORDER.length : i
    }

    return [...byCategory.entries()].sort(
      ([a], [b]) => rank(a) - rank(b) || a.localeCompare(b)
    )
  }, [visible])

  const total = Object.keys(groups).length
  const cardPadding = compact ? 'p-3' : 'p-4'
  const gridGap = compact ? 'gap-2' : 'gap-3'

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* Search + filter */}
      <div className="flex shrink-0 items-center gap-2">
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
        {compact && (
          <button
            type="button"
            onClick={() => setShowJsonConfig(true)}
            className="flex shrink-0 items-center gap-1.5 rounded-lg border border-gray-300 px-3 py-2 text-xs font-medium text-gray-500 transition-colors hover:border-gray-400 hover:text-gray-900 dark:border-gray-700 dark:text-gray-400 dark:hover:border-gray-600 dark:hover:text-gray-100"
            title="Add a custom MCP server by pasting its JSON config"
          >
            <Code2 className="h-3.5 w-3.5" />
            <span>Add via JSON</span>
          </button>
        )}
      </div>

      {/* Grid */}
      <div className="min-h-0 flex-1 overflow-y-auto pt-5">
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
            {query
              ? `No connectors match "${query}".`
              : filter === 'connected'
                ? 'No connectors yet. Pick one from Available and press Connect.'
                : filter === 'available'
                  ? 'Every connector is already connected.'
                  : 'No connectors configured.'}
          </p>
        )}

        {sections.map(([category, entries]) => (
          <section key={category} className={compact ? 'mb-4 last:mb-0' : 'mb-6 last:mb-0'}>
            <h4 className="mb-3 border-b border-gray-200 pb-2 text-sm font-semibold text-gray-900 dark:border-gray-800 dark:text-gray-100">
              {category}
            </h4>
            <div className={`grid grid-cols-1 ${gridGap} md:grid-cols-2`}>
              {entries.map(([serverName, tools]) => {
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            const requiresOAuth = (tools[0] as any).requires_oauth as boolean | undefined
            const status = tools[0]?.status
            const connection = tools[0]?.connection
            const isOpen = expandedLogs.has(serverName)

            return (
              <div
                key={serverName}
                className="flex flex-col rounded-xl border border-gray-200 bg-white transition-colors hover:border-gray-300 dark:border-gray-800 dark:bg-gray-900/60 dark:hover:border-gray-700"
              >
                <div className={`flex items-start gap-3 ${cardPadding}`}>
                  {onToggleServer && connection === 'connected' && (
                    <Checkbox
                      id={`connectors-browser-add-${serverName}`}
                      checked={isSelectedServer(selectedServers || [], serverName)}
                      onCheckedChange={() => onToggleServer(serverName)}
                      className="mt-3.5"
                      aria-label={`${isSelectedServer(selectedServers || [], serverName) ? 'Remove' : 'Add'} ${serverName} for this workflow`}
                    />
                  )}
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
                      {statusLabel(connection, status)}
                    </span>
                  </div>

                  <div className="flex shrink-0 items-center gap-1">
                    <OAuthStatusBadge
                      serverName={serverName}
                      requiresOAuth={requiresOAuth}
                      connection={connection}
                      variant="icon"
                      onAuthChange={() => {
                        // Refresh on disconnect too — the card's status text
                        // reads from the store, so skipping this left it
                        // claiming "Connected" after the token was revoked.
                        refreshTools()
                      }}
                    />
                    {!compact && (
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
                    )}
                  </div>
                </div>

                {/* Connection Logs Panel */}
                {!compact && isOpen && (
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
          </section>
        ))}
      </div>

      {showJsonConfig && (
        <MCPConfigPopup
          initialView="json"
          onConfigChange={() => refreshTools()}
          onClose={() => setShowJsonConfig(false)}
        />
      )}
    </div>
  )
}

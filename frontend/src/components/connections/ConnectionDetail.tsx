import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Loader2,
  Copy,
  Check,
  MoreHorizontal,
  Trash2,
  Activity,
  PlugZap,
  Plug,
  Search,
  Info,
} from 'lucide-react'
import ConnectionIcon from './ConnectionIcon'
import HealthBadge from './HealthBadge'
import ErrorNotice from './ErrorNotice'
import ToolGroup from './ToolGroup'
import ConfirmationDialog from '../ui/ConfirmationDialog'
import {
  connectionsApi,
  ConnectionError,
  type CatalogEntry,
  type Connection,
  type ConnectionTool,
  type FriendlyError,
} from '../../services/connectionsApi'

interface ConnectionDetailProps {
  entry?: CatalogEntry
  connection?: Connection
  isConnecting?: boolean
  isTesting?: boolean
  onConnect: () => void
  onDisconnect: () => void
  onRemove: () => void
  onTest: () => void
}

/**
 * A single connection's page: what it is, whether it is working, and which of
 * its tools agents may use. Tool switches are stored as the OFF set, so every
 * tool — including ones the provider adds later — is on unless turned off here.
 */
export default function ConnectionDetail({
  entry,
  connection,
  isConnecting = false,
  isTesting = false,
  onConnect,
  onDisconnect,
  onRemove,
  onTest,
}: ConnectionDetailProps) {
  const id = entry?.id ?? connection?.id ?? ''
  const name = entry?.name ?? connection?.name ?? 'Unknown'
  const icon = entry?.icon ?? connection?.icon
  const brandColor = entry?.brand_color ?? connection?.brand_color
  const url = entry?.url
  const health = connection?.health ?? 'not_connected'
  const isConnected = health === 'connected'

  const [tools, setTools] = useState<ConnectionTool[]>([])
  const [loadingTools, setLoadingTools] = useState(false)
  const [toolsError, setToolsError] = useState<FriendlyError | null>(null)
  const [saving, setSaving] = useState(false)
  const [query, setQuery] = useState('')
  const [groupsFromAnnotations, setGroupsFromAnnotations] = useState(true)
  const [expanded, setExpanded] = useState({ read_only: true, write: true })
  const [copied, setCopied] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const [confirmRemove, setConfirmRemove] = useState(false)

  const loadTools = useCallback(async () => {
    if (!id || !isConnected) return
    setLoadingTools(true)
    setToolsError(null)
    try {
      const result = await connectionsApi.getTools(id)
      setTools(result.tools)
      setGroupsFromAnnotations(result.groups_from_annotations)
    } catch (err) {
      setToolsError(
        err instanceof ConnectionError
          ? err.friendly
          : {
              code: 'unknown',
              title: 'Could not load tools',
              message: 'The tool list could not be read for this connection.',
              action: 'retry',
              raw: String(err),
            }
      )
    } finally {
      setLoadingTools(false)
    }
  }, [id, isConnected])

  useEffect(() => {
    loadTools()
  }, [loadTools])

  // Persist the OFF set. Optimistic so the switch responds immediately; a
  // failed save reverts and surfaces why.
  const persist = async (next: ConnectionTool[]) => {
    const previous = tools
    setTools(next)
    setSaving(true)
    setToolsError(null)
    try {
      await connectionsApi.setDisabledTools(
        id,
        next.filter(t => !t.enabled).map(t => t.name)
      )
    } catch (err) {
      setTools(previous)
      setToolsError(
        err instanceof ConnectionError
          ? err.friendly
          : {
              code: 'unknown',
              title: 'Could not save tool settings',
              message: 'The change was not saved.',
              action: 'retry',
              raw: String(err),
            }
      )
    } finally {
      setSaving(false)
    }
  }

  const setTool = (toolName: string, enabled: boolean) =>
    persist(tools.map(t => (t.name === toolName ? { ...t, enabled } : t)))

  // Bulk switch scoped to one group, so "turn off everything that writes" is a
  // single action.
  const setGroup = (readOnly: boolean, enabled: boolean) =>
    persist(tools.map(t => (t.read_only === readOnly ? { ...t, enabled } : t)))

  const visibleTools = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return tools
    return tools.filter(
      t => t.name.toLowerCase().includes(q) || t.title.toLowerCase().includes(q)
    )
  }, [tools, query])

  const enabledCount = tools.filter(t => t.enabled).length

  const copyUrl = async () => {
    if (!url) return
    try {
      await navigator.clipboard.writeText(url)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard can be blocked; the URL is visible and selectable regardless.
    }
  }

  return (
    <div className="space-y-5">
      {/* Identity + primary action */}
      <div className="flex items-start gap-3">
        <ConnectionIcon icon={icon} brandColor={brandColor} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-base font-semibold text-gray-900 dark:text-gray-100">
              {name}
            </h3>
            {connection && <HealthBadge health={health} />}
          </div>
          {entry?.tagline && (
            <p className="truncate text-xs text-gray-500 dark:text-gray-400">
              {entry.tagline}
            </p>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-1">
          {isConnected ? (
            <button
              type="button"
              onClick={onDisconnect}
              className="flex items-center gap-1.5 rounded-md border border-gray-300 px-3 py-1.5 text-sm font-medium text-gray-800 transition-colors hover:bg-gray-100 dark:border-slate-600 dark:text-gray-200 dark:hover:bg-slate-700"
            >
              <PlugZap className="h-3.5 w-3.5" aria-hidden="true" />
              Disconnect
            </button>
          ) : (
            <button
              type="button"
              onClick={onConnect}
              disabled={isConnecting}
              className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
            >
              {isConnecting ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
              ) : (
                <Plug className="h-3.5 w-3.5" aria-hidden="true" />
              )}
              {health === 'needs_reconnect' ? 'Reconnect' : 'Connect'}
            </button>
          )}

          {connection && (
            <div className="relative">
              <button
                type="button"
                onClick={() => setMenuOpen(v => !v)}
                className="rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-slate-700"
                aria-label={`More actions for ${name}`}
                aria-expanded={menuOpen}
              >
                <MoreHorizontal className="h-4 w-4" />
              </button>
              {menuOpen && (
                <>
                  <div
                    className="fixed inset-0 z-10"
                    onClick={() => setMenuOpen(false)}
                    aria-hidden="true"
                  />
                  <div className="absolute right-0 z-20 mt-1 w-44 overflow-hidden rounded-md border border-gray-200 bg-white py-1 shadow-lg dark:border-slate-700 dark:bg-slate-800">
                    <button
                      type="button"
                      onClick={() => {
                        setMenuOpen(false)
                        onTest()
                      }}
                      disabled={isTesting}
                      className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-gray-700 transition-colors hover:bg-gray-100 disabled:opacity-50 dark:text-gray-300 dark:hover:bg-slate-700"
                    >
                      {isTesting ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
                      ) : (
                        <Activity className="h-3.5 w-3.5" aria-hidden="true" />
                      )}
                      Test connection
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setMenuOpen(false)
                        setConfirmRemove(true)
                      }}
                      className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-red-600 transition-colors hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                    >
                      <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                      Remove
                    </button>
                  </div>
                </>
              )}
            </div>
          )}
        </div>
      </div>

      {url && (
        <div className="flex items-center gap-2">
          <code className="min-w-0 flex-1 truncate rounded bg-gray-50 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-slate-800 dark:text-gray-400">
            {url}
          </code>
          <button
            type="button"
            onClick={copyUrl}
            className="rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-slate-700"
            aria-label="Copy server URL"
          >
            {copied ? (
              <Check className="h-3.5 w-3.5 text-green-600 dark:text-green-400" />
            ) : (
              <Copy className="h-3.5 w-3.5" />
            )}
          </button>
        </div>
      )}

      {connection?.error && (
        <ErrorNotice
          error={connection.error}
          onAction={action => {
            if (action === 'reconnect' || action === 'connect' || action === 'retry') onConnect()
          }}
          compact
        />
      )}

      {/* Tool permissions */}
      <section>
        <div className="mb-1 flex items-center justify-between gap-2">
          <h4 className="text-sm font-medium text-gray-900 dark:text-gray-100">
            Tool permissions
          </h4>
          {saving && (
            <span className="flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400">
              <Loader2 className="h-3 w-3 animate-spin" aria-hidden="true" />
              Saving
            </span>
          )}
        </div>
        <p className="mb-3 text-xs text-gray-500 dark:text-gray-400">
          Choose which tools agents are allowed to use. Disabled tools are never
          offered to an agent.
        </p>

        {!isConnected ? (
          <p className="rounded-md bg-gray-50 p-3 text-xs text-gray-500 dark:bg-slate-800 dark:text-gray-400">
            Connect {name} to see the tools it provides.
          </p>
        ) : (
          <>
            {toolsError && (
              <div className="mb-3">
                <ErrorNotice error={toolsError} onAction={() => loadTools()} compact />
              </div>
            )}

            {loadingTools && tools.length === 0 && (
              <div className="flex items-center gap-2 py-6 text-sm text-gray-500 dark:text-gray-400">
                <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                Loading tools...
              </div>
            )}

            {tools.length > 0 && (
              <>
                <div className="mb-2 flex items-center gap-2">
                  <div className="relative min-w-0 flex-1">
                    <Search
                      className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-gray-400"
                      aria-hidden="true"
                    />
                    <input
                      type="search"
                      value={query}
                      onChange={e => setQuery(e.target.value)}
                      placeholder={`Search ${tools.length} tools`}
                      aria-label="Search tools"
                      className="w-full rounded-md border border-gray-300 bg-white py-1.5 pl-8 pr-3 text-sm text-gray-900 focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-800 dark:text-gray-100"
                    />
                  </div>
                  <span className="shrink-0 text-xs text-gray-500 dark:text-gray-400">
                    {enabledCount}/{tools.length} on
                  </span>
                </div>

                {!groupsFromAnnotations && (
                  <p className="mb-2 flex items-start gap-1.5 text-xs text-gray-500 dark:text-gray-400">
                    <Info className="mt-0.5 h-3 w-3 shrink-0" aria-hidden="true" />
                    This server does not declare which tools are read-only, so the grouping
                    below is read from the tool names. Check before relying on it.
                  </p>
                )}

                <div className="space-y-2">
                  <ToolGroup
                    kind="read_only"
                    tools={visibleTools.filter(t => t.read_only)}
                    expanded={expanded.read_only}
                    onToggleExpanded={() =>
                      setExpanded(e => ({ ...e, read_only: !e.read_only }))
                    }
                    onSetTool={setTool}
                    onSetAll={enabled => setGroup(true, enabled)}
                  />
                  <ToolGroup
                    kind="write"
                    tools={visibleTools.filter(t => !t.read_only)}
                    expanded={expanded.write}
                    onToggleExpanded={() => setExpanded(e => ({ ...e, write: !e.write }))}
                    onSetTool={setTool}
                    onSetAll={enabled => setGroup(false, enabled)}
                  />
                </div>

                {visibleTools.length === 0 && (
                  <p className="p-4 text-center text-xs text-gray-500 dark:text-gray-400">
                    No tools match &ldquo;{query}&rdquo;
                  </p>
                )}
              </>
            )}

            {!loadingTools && tools.length === 0 && !toolsError && (
              <p className="rounded-md bg-gray-50 p-3 text-xs text-gray-500 dark:bg-slate-800 dark:text-gray-400">
                This connection did not report any tools.
              </p>
            )}
          </>
        )}
      </section>

      <ConfirmationDialog
        isOpen={confirmRemove}
        onClose={() => setConfirmRemove(false)}
        onConfirm={() => {
          setConfirmRemove(false)
          onRemove()
        }}
        title={`Remove ${name}?`}
        message={`This deletes the saved sign-in and the connection settings for ${name}. Agents will lose access immediately. To sign out but keep the connection, use Disconnect instead.`}
        confirmText="Remove"
        type="danger"
      />
    </div>
  )
}

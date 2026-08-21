import { useState } from 'react'
import { Loader2, MoreHorizontal, Activity, AlertTriangle } from 'lucide-react'
import ConnectionIcon from './ConnectionIcon'
import type { CatalogEntry, Connection, ConnectionTransport } from '../../services/connectionsApi'

const TRANSPORT_LABELS: Record<ConnectionTransport, string> = {
  web: 'Web',
  local: 'Local',
}

interface ConnectionRowProps {
  /** Catalog metadata. Absent for custom MCP servers the user added by hand. */
  entry?: CatalogEntry
  /** Present once the integration has been provisioned. */
  connection?: Connection
  isConnecting?: boolean
  isTesting?: boolean
  onConnect: () => void
  onDisconnect: () => void
  onTest: () => void
  onOpen: () => void
}

/**
 * One row of the connections table: identity, transport, and the single action
 * that matters for its current state.
 */
export default function ConnectionRow({
  entry,
  connection,
  isConnecting = false,
  isTesting = false,
  onConnect,
  onDisconnect,
  onTest,
  onOpen,
}: ConnectionRowProps) {
  const [menuOpen, setMenuOpen] = useState(false)

  const name = entry?.name ?? connection?.name ?? 'Unknown'
  const icon = entry?.icon ?? connection?.icon
  const brandColor = entry?.brand_color ?? connection?.brand_color
  const transport = entry?.transport ?? connection?.transport ?? 'web'

  const health = connection?.health ?? 'not_connected'
  const isConnected = health === 'connected'
  const comingSoon = entry?.status === 'coming_soon'
  const setupRequired = entry?.setup_required ?? health === 'setup_required'

  // Blocked states get muted explanatory text rather than a dead button, so the
  // user learns why instead of clicking something that cannot work.
  const blockedLabel = comingSoon
    ? 'Coming soon'
    : setupRequired
      ? 'Setup required'
      : null
  const blockedHint = comingSoon || setupRequired ? entry?.setup_hint : undefined

  return (
    <>
      <div className="group grid grid-cols-[1fr_auto_auto] items-center gap-4 rounded-lg px-3 py-2.5 transition-colors hover:bg-gray-50 dark:hover:bg-slate-700/40">
        {/* Connector — opens the connection's own page */}
        <button
          type="button"
          onClick={onOpen}
          className="flex min-w-0 items-center gap-3 text-left"
        >
          <ConnectionIcon icon={icon} brandColor={brandColor} size="sm" />
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="truncate text-sm font-medium text-gray-900 dark:text-gray-100">
                {name}
              </span>
              {connection?.custom && (
                <span className="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-gray-500 dark:bg-slate-700 dark:text-gray-400">
                  Custom
                </span>
              )}
            </div>
            {/* One line of context: the failure if there is one, else the tagline. */}
            {health === 'needs_reconnect' && connection?.error ? (
              <span className="flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
                <AlertTriangle className="h-3 w-3 shrink-0" aria-hidden="true" />
                {connection.error.title}
              </span>
            ) : blockedHint ? (
              <span className="line-clamp-1 text-xs text-gray-400 dark:text-gray-500">
                {blockedHint}
              </span>
            ) : entry?.tagline ? (
              <span className="truncate text-xs text-gray-500 dark:text-gray-400">
                {entry.tagline}
              </span>
            ) : null}
          </div>
        </button>

        {/* Type */}
        <span className="w-16 text-sm text-gray-500 dark:text-gray-400">
          {TRANSPORT_LABELS[transport]}
        </span>

        {/* Status / action */}
        <div className="flex w-36 items-center justify-end gap-1">
          {blockedLabel ? (
            <span className="text-xs text-gray-400 dark:text-gray-500">{blockedLabel}</span>
          ) : isConnected ? (
            // "Disconnect" already says the connection is live, so no tick
            // beside it. Always visible: signing out is a primary action, not
            // something the user should have to hover to discover.
            <button
              type="button"
              onClick={onDisconnect}
              className="rounded-md border border-gray-300 px-2.5 py-1 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-100 dark:border-slate-600 dark:text-gray-300 dark:hover:bg-slate-700"
            >
              Disconnect
            </button>
          ) : (
            <button
              type="button"
              onClick={onConnect}
              disabled={isConnecting}
              className="flex items-center gap-1.5 rounded-md border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-800 transition-colors hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-600 dark:text-gray-200 dark:hover:bg-slate-700"
            >
              {isConnecting && <Loader2 className="h-3 w-3 animate-spin" aria-hidden="true" />}
              {health === 'needs_reconnect' ? 'Reconnect' : 'Connect'}
            </button>
          )}

          {connection && (
            <div className="relative">
              <button
                type="button"
                onClick={() => setMenuOpen(v => !v)}
                className="rounded-md p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-slate-700 dark:hover:text-gray-200"
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
                  </div>
                </>
              )}
            </div>
          )}
        </div>
      </div>
    </>
  )
}

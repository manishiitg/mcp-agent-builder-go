import { useEffect } from 'react'
import { Plug } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipTrigger } from '../ui/tooltip'
import { iconButtonClass } from '../ui/IconPopover'
import { useConnectionsStore } from '../../stores/useConnectionsStore'

interface ConnectionsControlProps {
  active?: boolean
  onClick: () => void
}

/**
 * Top-level entry point for Connections. Integrations used to sit three levels
 * deep under Workspace tools -> MCP Servers, so this promotes them to the top
 * bar and surfaces health at a glance.
 */
export default function ConnectionsControl({ active = false, onClick }: ConnectionsControlProps) {
  const summary = useConnectionsStore(s => s.summary)
  const loadConnections = useConnectionsStore(s => s.loadConnections)

  useEffect(() => {
    loadConnections()
  }, [loadConnections])

  const needsAttention = summary.needs_attention > 0

  const label = summary.total === 0
    ? 'Connections'
    : `Connections — ${summary.connected} connected${
        needsAttention ? `, ${summary.needs_attention} needs attention` : ''
      }`

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onClick}
          aria-label={label}
          className={`relative ${iconButtonClass} ${
            active ? 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-200' : ''
          }`}
        >
          <Plug className="h-4 w-4" />
          {needsAttention && (
            <span
              className="absolute right-1 top-1 h-2 w-2 rounded-full bg-amber-500 ring-2 ring-white dark:ring-slate-800"
              aria-hidden="true"
            />
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent side="bottom">{label}</TooltipContent>
    </Tooltip>
  )
}

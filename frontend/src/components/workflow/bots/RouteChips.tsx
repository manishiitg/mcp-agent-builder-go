import { ChevronDown, Loader2, MessageSquare, Phone, X } from 'lucide-react'
import { READ_ONLY_TITLE } from '../../../hooks/useCanWriteWorkflow'
import { routeId, type WorkflowRoute } from './types'
import type { WorkflowBots } from './useWorkflowBots'

// One "this workflow answers on" chip with its expandable route options.

export function RouteChip({ bots, route }: { bots: WorkflowBots; route: WorkflowRoute }) {
  const { readOnly, expandedChip, setExpandedChip, routeSaving, removeRoute, updateRoute } = bots

  const id = routeId(route)
  const expanded = expandedChip === id
  const saving = routeSaving === id
  const label = route.kind === 'slack' ? `Slack #${route.key}` : `WhatsApp @${route.key}`
  const Icon = route.kind === 'slack' ? MessageSquare : Phone
  return (
    <div key={id} className="min-w-0">
      <span className="inline-flex max-w-full items-center gap-1.5 rounded-full border border-primary/30 bg-primary/10 py-1 pl-2 pr-1 text-xs font-medium text-primary">
        <button
          type="button"
          onClick={() => setExpandedChip(expanded ? null : id)}
          className="inline-flex min-w-0 items-center gap-1.5"
          aria-expanded={expanded}
          title="Edit route options"
        >
          <Icon className="h-3 w-3 shrink-0" />
          <span className="truncate font-mono">{label}</span>
          <ChevronDown className={`h-3 w-3 shrink-0 transition-transform ${expanded ? 'rotate-180' : ''}`} />
        </button>
        {saving ? (
          <Loader2 className="h-3 w-3 animate-spin" />
        ) : (
          <button
            type="button"
            onClick={() => void removeRoute(route)}
            disabled={readOnly}
            className="rounded-full p-0.5 text-primary/70 transition-colors hover:bg-red-500/15 hover:text-red-500 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent disabled:hover:text-primary/70"
            aria-label={`Stop answering on ${label}`}
            title={readOnly ? READ_ONLY_TITLE : 'Remove from this workflow'}
          >
            <X className="h-3 w-3" />
          </button>
        )}
      </span>
      {expanded && (
        <div className="mt-1.5 flex flex-wrap items-center gap-3 rounded-md border border-border bg-muted/20 px-3 py-2 text-xs">
          <label className="flex items-center gap-1.5 text-muted-foreground">
            Mode
            <select
              value={route.kind === 'slack' ? (route.workshop_mode || '') : 'run'}
              onChange={e => void updateRoute(route, { workshop_mode: e.target.value })}
              disabled={readOnly || saving || route.kind === 'whatsapp'}
              className="px-1.5 py-1 text-xs bg-secondary border border-border rounded focus:outline-none focus:ring-1 focus:ring-primary disabled:opacity-60"
              title="Bot channels always run in Run mode. 'Default' uses the automation manifest's setting (which is also Run for bot deployments)."
            >
              {route.kind === 'slack' && <option value="">Default</option>}
              <option value="run">Run</option>
            </select>
          </label>
          <label className="flex items-center gap-1.5 text-muted-foreground" title="Send detailed automation step/runtime messages to this channel">
            <input
              type="checkbox"
              checked={!!route.send_full_details}
              disabled={readOnly || saving}
              onChange={e => void updateRoute(route, { send_full_details: e.target.checked })}
              className="h-3.5 w-3.5"
            />
            Send full details
          </label>
        </div>
      )}
    </div>
  )
}

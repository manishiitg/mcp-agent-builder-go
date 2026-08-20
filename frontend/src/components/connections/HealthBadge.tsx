import { CheckCircle2, AlertTriangle, Wrench, CircleDashed } from 'lucide-react'
import type { ConnectionHealth } from '../../services/connectionsApi'

const STYLES: Record<
  ConnectionHealth,
  { label: string; icon: typeof CheckCircle2; className: string }
> = {
  connected: {
    label: 'Connected',
    icon: CheckCircle2,
    className:
      'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
  },
  needs_reconnect: {
    label: 'Needs attention',
    icon: AlertTriangle,
    className:
      'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  },
  setup_required: {
    label: 'Setup required',
    icon: Wrench,
    className:
      'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
  },
  not_connected: {
    label: 'Not connected',
    icon: CircleDashed,
    className:
      'bg-gray-100 text-gray-600 dark:bg-slate-700 dark:text-gray-300',
  },
}

export default function HealthBadge({ health }: { health: ConnectionHealth }) {
  const style = STYLES[health] ?? STYLES.not_connected
  const Icon = style.icon

  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${style.className}`}
    >
      <Icon className="h-3 w-3" aria-hidden="true" />
      {style.label}
    </span>
  )
}

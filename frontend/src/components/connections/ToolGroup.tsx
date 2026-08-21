import { ChevronDown, ChevronRight, Eye, PencilLine } from 'lucide-react'
import type { ConnectionTool } from '../../services/connectionsApi'

interface ToolGroupProps {
  kind: 'read_only' | 'write'
  tools: ConnectionTool[]
  expanded: boolean
  onToggleExpanded: () => void
  onToggleTool: (name: string) => void
  /** Turns every tool in this group on or off in one move. */
  onSetAll: (enabled: boolean) => void
}

const LABELS = {
  read_only: {
    title: 'Read-only tools',
    icon: Eye,
    hint: 'These only look at your data.',
  },
  write: {
    title: 'Write/delete tools',
    icon: PencilLine,
    hint: 'These can change or remove your data.',
  },
} as const

/**
 * One collapsible group of tools with a bulk switch. Splitting read-only from
 * write/delete lets someone act on the risky half without reading 28 names.
 */
export default function ToolGroup({
  kind,
  tools,
  expanded,
  onToggleExpanded,
  onToggleTool,
  onSetAll,
}: ToolGroupProps) {
  if (tools.length === 0) return null

  const { title, icon: Icon, hint } = LABELS[kind]
  const enabledCount = tools.filter(t => t.enabled).length
  const allOn = enabledCount === tools.length

  return (
    <div className="overflow-hidden rounded-md border border-gray-200 dark:border-slate-700">
      <div className="flex items-center gap-2 bg-gray-50 px-3 py-2 dark:bg-slate-800">
        <button
          type="button"
          onClick={onToggleExpanded}
          aria-expanded={expanded}
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
        >
          {expanded ? (
            <ChevronDown className="h-4 w-4 shrink-0 text-gray-500" aria-hidden="true" />
          ) : (
            <ChevronRight className="h-4 w-4 shrink-0 text-gray-500" aria-hidden="true" />
          )}
          <Icon
            className={`h-3.5 w-3.5 shrink-0 ${
              kind === 'write'
                ? 'text-amber-600 dark:text-amber-400'
                : 'text-gray-500 dark:text-gray-400'
            }`}
            aria-hidden="true"
          />
          <span className="truncate text-sm font-medium text-gray-900 dark:text-gray-100">
            {title}
          </span>
          <span className="shrink-0 rounded bg-gray-200 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-slate-700 dark:text-gray-300">
            {tools.length}
          </span>
          <span className="truncate text-xs text-gray-400 dark:text-gray-500">{hint}</span>
        </button>

        <span className="shrink-0 text-xs text-gray-500 dark:text-gray-400">
          {enabledCount}/{tools.length} on
        </span>
        <button
          type="button"
          onClick={() => onSetAll(!allOn)}
          className="shrink-0 rounded-md px-2 py-1 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
        >
          {allOn ? 'Turn all off' : 'Turn all on'}
        </button>
      </div>

      {expanded && (
        <div className="divide-y divide-gray-100 dark:divide-slate-800">
          {tools.map(tool => (
            <label
              key={tool.name}
              className="flex cursor-pointer items-start gap-3 p-2.5 transition-colors hover:bg-gray-50 dark:hover:bg-slate-700/40"
            >
              <input
                type="checkbox"
                checked={tool.enabled}
                onChange={() => onToggleTool(tool.name)}
                className="mt-0.5 h-4 w-4 shrink-0 rounded border-gray-300 text-blue-600 focus:ring-2 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-700"
              />
              <span className="min-w-0">
                <span className="block truncate font-mono text-xs text-gray-900 dark:text-gray-100">
                  {tool.name}
                </span>
                {tool.description && (
                  <span className="line-clamp-2 text-xs text-gray-500 dark:text-gray-400">
                    {tool.description}
                  </span>
                )}
              </span>
            </label>
          ))}
        </div>
      )}
    </div>
  )
}

import { ChevronDown, ChevronRight, CircleCheck, Ban } from 'lucide-react'
import type { ConnectionTool } from '../../services/connectionsApi'

interface ToolGroupProps {
  kind: 'read_only' | 'write'
  tools: ConnectionTool[]
  expanded: boolean
  onToggleExpanded: () => void
  onSetTool: (name: string, enabled: boolean) => void
  /** Sets every tool in this group at once. */
  onSetAll: (enabled: boolean) => void
}

const LABELS = {
  read_only: { title: 'Read-only tools', bulkOn: 'Allow all', bulkOff: 'Disable all' },
  write: { title: 'Write/delete tools', bulkOn: 'Allow all', bulkOff: 'Disable all' },
} as const

/**
 * A collapsible group of tools. Each row carries the choice itself — allowed or
 * disabled — as two buttons, so the current state is visible at a glance down
 * the column rather than having to be read one checkbox at a time.
 */
export default function ToolGroup({
  kind,
  tools,
  expanded,
  onToggleExpanded,
  onSetTool,
  onSetAll,
}: ToolGroupProps) {
  if (tools.length === 0) return null

  const { title, bulkOn, bulkOff } = LABELS[kind]
  const enabledCount = tools.filter(t => t.enabled).length
  const allOn = enabledCount === tools.length

  return (
    <div className="overflow-hidden rounded-lg border border-gray-200 dark:border-slate-700">
      <div className="flex items-center gap-2 px-3 py-2.5">
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
          <span className="truncate text-sm font-medium text-gray-900 dark:text-gray-100">
            {title}
          </span>
          <span className="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-slate-700 dark:text-gray-300">
            {tools.length}
          </span>
        </button>

        <button
          type="button"
          onClick={() => onSetAll(!allOn)}
          className="shrink-0 rounded-md border border-gray-300 px-2.5 py-1 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-100 dark:border-slate-600 dark:text-gray-300 dark:hover:bg-slate-700"
        >
          {allOn ? bulkOff : bulkOn}
        </button>
      </div>

      {expanded && (
        <div className="divide-y divide-gray-100 dark:divide-slate-800">
          {tools.map(tool => (
            <div
              key={tool.name}
              className="flex items-center gap-3 px-3 py-2 transition-colors hover:bg-gray-50 dark:hover:bg-slate-700/40"
            >
              {/* The raw name stays reachable on hover so a tool can still be
                  identified exactly without cluttering the row. */}
              <span
                title={tool.name}
                className="min-w-0 flex-1 truncate text-sm text-gray-800 dark:text-gray-200"
              >
                {tool.title || tool.name}
              </span>

              <div
                role="group"
                aria-label={`Permission for ${tool.title || tool.name}`}
                className="flex shrink-0 items-center gap-0.5 rounded-lg bg-gray-100 p-0.5 dark:bg-slate-800"
              >
                <button
                  type="button"
                  onClick={() => onSetTool(tool.name, true)}
                  aria-pressed={tool.enabled}
                  title="Allow"
                  className={`rounded-md p-1.5 transition-colors ${
                    tool.enabled
                      ? 'bg-white text-green-600 shadow-sm dark:bg-slate-700 dark:text-green-400'
                      : 'text-gray-400 hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300'
                  }`}
                >
                  <CircleCheck className="h-4 w-4" aria-hidden="true" />
                  <span className="sr-only">Allow</span>
                </button>
                <button
                  type="button"
                  onClick={() => onSetTool(tool.name, false)}
                  aria-pressed={!tool.enabled}
                  title="Disable"
                  className={`rounded-md p-1.5 transition-colors ${
                    !tool.enabled
                      ? 'bg-white text-red-600 shadow-sm dark:bg-slate-700 dark:text-red-400'
                      : 'text-gray-400 hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300'
                  }`}
                >
                  <Ban className="h-4 w-4" aria-hidden="true" />
                  <span className="sr-only">Disable</span>
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

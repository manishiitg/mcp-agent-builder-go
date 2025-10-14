import React from 'react'
import type { MCPServerConnectionEvent } from '../../../generated/events'

interface MCPServerConnectionEventDisplayProps {
  event: MCPServerConnectionEvent
  mode?: 'compact' | 'detailed'
}

export const MCPServerConnectionEventDisplay: React.FC<MCPServerConnectionEventDisplayProps> = ({
  event,
  mode = 'detailed'
}) => {
  if (mode === 'compact') {
    return (
      <div className="bg-indigo-50 dark:bg-indigo-900/20 border border-indigo-200 dark:border-indigo-800 rounded-md p-2">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-indigo-700 dark:text-indigo-300">
            MCP Server Connection
          </span>
          {event.server_name && (
            <span className="text-xs text-indigo-600 dark:text-indigo-400">
              {event.server_name}
            </span>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="bg-indigo-50 dark:bg-indigo-900/20 border border-indigo-200 dark:border-indigo-800 rounded-md p-4">
      <div className="flex items-center gap-2 mb-3">
        <div className="w-3 h-3 bg-indigo-500 rounded-full"></div>
        <h3 className="text-lg font-semibold text-indigo-700 dark:text-indigo-300">
          MCP Server Connection
        </h3>
      </div>

      <div className="space-y-3">
        {event.server_name && (
          <div>
            <span className="text-sm font-medium text-indigo-600 dark:text-indigo-400">Server Name:</span>
            <span className="ml-2 text-sm text-indigo-700 dark:text-indigo-300">{event.server_name}</span>
          </div>
        )}

        {event.config_path && (
          <div>
            <span className="text-sm font-medium text-indigo-600 dark:text-indigo-400">Config Path:</span>
            <div className="mt-1 p-2 bg-indigo-100 dark:bg-indigo-800/30 rounded text-sm text-indigo-700 dark:text-indigo-300 font-mono">
              {event.config_path}
            </div>
          </div>
        )}

        {event.timeout && (
          <div>
            <span className="text-sm font-medium text-indigo-600 dark:text-indigo-400">Timeout:</span>
            <span className="ml-2 text-sm text-indigo-700 dark:text-indigo-300">{event.timeout}</span>
          </div>
        )}

        {event.operation && (
          <div>
            <span className="text-sm font-medium text-indigo-600 dark:text-indigo-400">Operation:</span>
            <span className="ml-2 text-sm text-indigo-700 dark:text-indigo-300">{event.operation}</span>
          </div>
        )}

        {event.status && (
          <div>
            <span className="text-sm font-medium text-indigo-600 dark:text-indigo-400">Status:</span>
            <span className={`ml-2 text-sm ${
              event.status === 'healthy' || event.status === 'connected'
                ? 'text-green-700 dark:text-green-300'
                : event.status === 'error' || event.status === 'failed'
                ? 'text-red-700 dark:text-red-300'
                : 'text-yellow-700 dark:text-yellow-300'
            }`}>
              {event.status}
            </span>
          </div>
        )}

        {event.tools_count !== undefined && (
          <div>
            <span className="text-sm font-medium text-indigo-600 dark:text-indigo-400">Tools Count:</span>
            <span className="ml-2 text-sm text-indigo-700 dark:text-indigo-300">{event.tools_count}</span>
          </div>
        )}

        {event.connection_time !== undefined && (
          <div>
            <span className="text-sm font-medium text-indigo-600 dark:text-indigo-400">Connection Time:</span>
            <span className="ml-2 text-sm text-indigo-700 dark:text-indigo-300">{event.connection_time}ms</span>
          </div>
        )}

        {event.error && (
          <div>
            <span className="text-sm font-medium text-indigo-600 dark:text-indigo-400">Error:</span>
            <div className="mt-1 p-2 bg-red-100 dark:bg-red-800/30 rounded text-sm text-red-700 dark:text-red-300">
              {event.error}
            </div>
          </div>
        )}

        {event.server_info && (
          <div>
            <span className="text-sm font-medium text-indigo-600 dark:text-indigo-400">Server Info:</span>
            <div className="mt-1 p-2 bg-indigo-100 dark:bg-indigo-800/30 rounded text-sm text-indigo-700 dark:text-indigo-300 font-mono text-xs">
              {JSON.stringify(event.server_info, null, 2)}
            </div>
          </div>
        )}

        {event.timestamp && (
          <div>
            <span className="text-sm font-medium text-indigo-600 dark:text-indigo-400">Timestamp:</span>
            <span className="ml-2 text-sm text-indigo-700 dark:text-indigo-300">
              {new Date(event.timestamp).toLocaleString()}
            </span>
          </div>
        )}
      </div>
    </div>
  )
}

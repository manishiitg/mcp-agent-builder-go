import React from 'react'
import type { ToolCallErrorEvent } from '../../../generated/events'
import { formatDuration } from '../../../utils/duration'

interface ToolCallErrorEventDisplayProps {
  event: ToolCallErrorEvent
}

export const ToolCallErrorEventDisplay: React.FC<ToolCallErrorEventDisplayProps> = ({ event }) => {
  return (
    <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded p-2">
      <div className="flex items-start gap-3">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-red-700 dark:text-red-300">
              Tool Call Error
            </span>
            {event.tool_name && (
              <span className="text-xs text-red-600 dark:text-red-400">
                • {event.tool_name}
              </span>
            )}
            {event.turn && (
              <span className="text-xs text-red-600 dark:text-red-400">
                • Turn {event.turn}
              </span>
            )}
            {event.duration && (
              <span className="text-xs text-red-600 dark:text-red-400">
                • {formatDuration(event.duration)}
              </span>
            )}
          </div>
          
          {event.tool_name && (
            <div className="text-xs text-red-600 dark:text-red-400 mt-1">
              Tool: {event.tool_name}
            </div>
          )}
          
          {event.server_name && (
            <div className="text-xs text-red-600 dark:text-red-400 mt-1">
              Server: {event.server_name}
            </div>
          )}
          
          {/* Error details - always visible for debugging */}
          {event.error && (
            <div className="mt-2">
              <div className="text-xs font-medium text-red-700 dark:text-red-300">
                Error Details:
              </div>
              <div className="mt-1 p-2 bg-red-100 dark:bg-red-800/30 rounded text-xs text-red-700 dark:text-red-300">
                {event.error}
              </div>
            </div>
          )}
          
          {event.duration && (
            <div className="text-xs text-red-600 dark:text-red-400 mt-1">
              Duration: {formatDuration(event.duration)}
            </div>
          )}
          
          {event.timestamp && (
            <div className="text-xs text-red-600 dark:text-red-400 mt-1">
              {new Date(event.timestamp).toLocaleTimeString()}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

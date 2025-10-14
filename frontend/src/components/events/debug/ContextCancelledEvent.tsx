import React from 'react'
import type { ContextCancelledEvent } from '../../../generated/events'
import { formatDuration } from '../../../utils/duration'

interface ContextCancelledEventDisplayProps {
  event: ContextCancelledEvent
}

export const ContextCancelledEventDisplay: React.FC<ContextCancelledEventDisplayProps> = ({
  event
}) => {
  return (
    <div className="bg-gray-50 dark:bg-gray-900/20 border border-gray-200 dark:border-gray-800 rounded p-2">
      <div className="flex items-start gap-3">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
              Context Cancelled
            </span>
            {event.reason && (
              <span className="text-xs text-gray-600 dark:text-gray-400">
                {event.reason.length > 40 ? `${event.reason.substring(0, 40)}...` : event.reason}
              </span>
            )}
          </div>
          
          <div className="space-y-2 mt-2">
            {event.turn !== undefined && (
              <div>
                <span className="text-xs font-medium text-gray-600 dark:text-gray-400">Turn:</span>
                <span className="ml-2 text-xs text-gray-700 dark:text-gray-300">{event.turn}</span>
              </div>
            )}

            {event.duration !== undefined && (
              <div>
                <span className="text-xs font-medium text-gray-600 dark:text-gray-400">Duration:</span>
                <span className="ml-2 text-xs text-gray-700 dark:text-gray-300">{formatDuration(event.duration)}</span>
              </div>
            )}

            {event.timestamp && (
              <div>
                <span className="text-xs font-medium text-gray-600 dark:text-gray-400">Time:</span>
                <span className="ml-2 text-xs text-gray-700 dark:text-gray-300">
                  {new Date(event.timestamp).toLocaleTimeString()}
                </span>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

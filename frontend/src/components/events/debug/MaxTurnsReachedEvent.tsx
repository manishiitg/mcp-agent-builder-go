import React from 'react'
import type { MaxTurnsReachedEvent } from '../../../generated/events'

interface MaxTurnsReachedEventDisplayProps {
  event: MaxTurnsReachedEvent
}

export const MaxTurnsReachedEventDisplay: React.FC<MaxTurnsReachedEventDisplayProps> = ({
  event
}) => {
  return (
    <div className="bg-gray-50 dark:bg-gray-900/20 border border-gray-200 dark:border-gray-800 rounded p-2">
      <div className="flex items-start gap-3">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
              Max Turns Reached
            </span>
            {event.max_turns && (
              <span className="text-xs text-gray-600 dark:text-gray-400">
                {event.max_turns} turns
              </span>
            )}
          </div>
          
          {event.question && (
            <div className="text-xs text-gray-600 dark:text-gray-400 mt-1">
              {event.question.length > 60 ? `${event.question.substring(0, 60)}...` : event.question}
            </div>
          )}

          <div className="space-y-2 mt-2">
            {event.turn !== undefined && (
              <div>
                <span className="text-xs font-medium text-gray-600 dark:text-gray-400">Turn:</span>
                <span className="ml-2 text-xs text-gray-700 dark:text-gray-300">{event.turn}</span>
              </div>
            )}

            {event.agent_mode && (
              <div>
                <span className="text-xs font-medium text-gray-600 dark:text-gray-400">Agent Mode:</span>
                <span className="ml-2 text-xs text-gray-700 dark:text-gray-300">{event.agent_mode}</span>
              </div>
            )}

            {event.final_message && (
              <div>
                <span className="text-xs font-medium text-gray-600 dark:text-gray-400">Final Message:</span>
                <div className="mt-1 p-2 bg-gray-100 dark:bg-gray-800/30 rounded text-xs text-gray-700 dark:text-gray-300 max-h-32 overflow-y-auto">
                  {event.final_message}
                </div>
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

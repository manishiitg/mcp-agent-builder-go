import React from 'react'
import type { UserMessageEvent } from '../../../generated/events'

interface UserMessageEventDisplayProps {
  event: UserMessageEvent
  mode?: 'compact' | 'detailed'
}

export const UserMessageEventDisplay: React.FC<UserMessageEventDisplayProps> = ({ 
  event, 
  mode = 'detailed' 
}) => {
  if (mode === 'compact') {
    return (
      <div className="bg-indigo-50 dark:bg-indigo-900/20 border border-indigo-200 dark:border-indigo-800 rounded p-2">
        <div className="flex items-start gap-2">
          <span className="text-xs font-bold text-indigo-700 dark:text-indigo-300">👤</span>
          <div className="flex-1 min-w-0">
            {event.content ? (
              <div className="text-xs text-indigo-900 dark:text-indigo-100 leading-tight">
                {event.content.length > 80
                  ? `${event.content.substring(0, 80)}...`
                  : event.content
                }
              </div>
            ) : (
              <div className="text-xs text-red-600 dark:text-red-400 italic">
                No message content
              </div>
            )}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="bg-indigo-50 dark:bg-indigo-900/20 border border-indigo-200 dark:border-indigo-800 rounded p-2">
      <div className="flex items-start gap-2">
        <span className="text-xs font-bold text-indigo-700 dark:text-indigo-300">👤</span>
        <div className="flex-1 min-w-0">
          {event.content ? (
            <div className="text-xs text-indigo-900 dark:text-indigo-100 leading-tight whitespace-pre-wrap bg-white dark:bg-indigo-900/30 rounded p-2 border border-indigo-100 dark:border-indigo-700/50">
              {event.content}
            </div>
          ) : (
            <div className="text-xs text-red-600 dark:text-red-400 italic bg-red-50 dark:bg-red-900/30 rounded p-2 border border-red-200 dark:border-red-800">
              No message content
            </div>
          )}

          {event.timestamp && (
            <div className="text-xs text-indigo-600 dark:text-indigo-400 mt-1">
              {new Date(event.timestamp).toLocaleString()}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

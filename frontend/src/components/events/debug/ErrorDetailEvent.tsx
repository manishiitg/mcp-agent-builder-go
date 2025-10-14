import React from 'react'
import type { ErrorDetailEvent } from '../../../generated/events-bridge'
import { formatDuration } from '../../../utils/duration'

interface ErrorDetailEventDisplayProps {
  event: ErrorDetailEvent
}

export const ErrorDetailEventDisplay: React.FC<ErrorDetailEventDisplayProps> = ({ event }) => {
  return (
    <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md p-3">
      <div className="flex items-start space-x-3">
        <div className="flex-shrink-0">
          <div className="w-8 h-8 bg-red-100 dark:bg-red-800 rounded-full flex items-center justify-center">
            <svg className="w-5 h-5 text-red-600 dark:text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
            </svg>
          </div>
        </div>
        <div className="flex-1 min-w-0">
          <div className="text-sm font-medium text-red-800 dark:text-red-200">
            Error Detail
          </div>
          <div className="text-xs text-red-600 dark:text-red-400 mt-1">
            Turn: {event.turn || 'N/A'} • Component: {event.component || 'N/A'} • Operation: {event.operation || 'N/A'}
          </div>
          <div className="text-sm text-red-700 dark:text-red-300 mt-2">
            <div className="font-medium">Error:</div>
            <div className="mt-1 break-words">{event.error || 'No error message'}</div>
          </div>
          {event.error_type && (
            <div className="text-xs text-red-600 dark:text-red-400 mt-1">
              Type: {event.error_type}
            </div>
          )}
          {event.context && (
            <div className="text-xs text-red-600 dark:text-red-400 mt-1">
              Context: {event.context}
            </div>
          )}
          <div className="text-xs text-red-500 dark:text-red-400 mt-2">
            Duration: {event.duration ? formatDuration(event.duration) : 'N/A'} • 
            Recoverable: {event.recoverable ? 'Yes' : 'No'} • 
            Retry Count: {event.retry_count || 0}
          </div>
        </div>
      </div>
    </div>
  )
}

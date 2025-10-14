import React from 'react'
import type { ReActReasoningStartEvent } from '../../../generated/events'

interface ReActReasoningStartEventDisplayProps {
  event: ReActReasoningStartEvent
}

export const ReActReasoningStartEventDisplay: React.FC<ReActReasoningStartEventDisplayProps> = ({
  event
}) => {
  // Single-line layout following design guidelines
  return (
    <div className="bg-gray-50 dark:bg-gray-900/20 border border-gray-200 dark:border-gray-800 rounded p-2">
      <div className="flex items-center justify-between gap-3">
        {/* Left side: Icon and main content */}
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-gray-700 dark:text-gray-300">
              ReAct Reasoning Start{' '}
              <span className="text-xs font-normal text-gray-600 dark:text-gray-400">
                {event.question && `• ${event.question.length > 60 ? `${event.question.substring(0, 60)}...` : event.question}`}
              </span>
            </div>
          </div>
        </div>

        {/* Right side: Time */}
        {event.timestamp && (
          <div className="text-xs text-gray-600 dark:text-gray-400 flex-shrink-0">
            {new Date(event.timestamp).toLocaleTimeString()}
          </div>
        )}
      </div>
    </div>
  )
}

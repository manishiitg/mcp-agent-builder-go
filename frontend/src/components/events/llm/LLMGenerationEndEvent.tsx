import React from 'react'
import { CheckCircle, AlertCircle } from 'lucide-react'
import type { LLMGenerationEndEvent } from '../../../generated/events'
import { ConversationMarkdownRenderer } from '../../ui/MarkdownRenderer'
import { formatDuration } from '../../../utils/duration'

interface LLMGenerationEndEventProps {
  event: LLMGenerationEndEvent
}

export const LLMGenerationEndEventDisplay: React.FC<LLMGenerationEndEventProps> = ({ event }) => {
  
  const isSuccess = true // LLM generation end is typically successful

  const bgColor = isSuccess
    ? 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800'
    : 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800'

  const textColor = isSuccess
    ? 'text-green-700 dark:text-green-300'
    : 'text-red-700 dark:text-red-300'

  const iconColor = isSuccess
    ? 'text-green-600'
    : 'text-red-600'

  const Icon = isSuccess ? CheckCircle : AlertCircle

  return (
    <div className={`${bgColor} border rounded p-2`}>
      <div className={`text-xs ${textColor} space-y-1`}>
        {/* Header with key info */}
        <div className="flex items-center justify-between gap-3">
          {/* Left side: Icon and main content */}
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <Icon className={`w-4 h-4 ${iconColor} flex-shrink-0`} />
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium text-green-700 dark:text-green-300">
                LLM Generation End{' '}
                <span className="text-xs font-normal text-green-600 dark:text-green-400">
                  {event.turn && `• Turn ${event.turn}`}
                  {event.duration && ` • ${formatDuration(event.duration)}`}
                  {event.tool_calls !== undefined && ` • ${event.tool_calls} tool calls`}
                  {event.usage_metrics && ` • Tokens: ${event.usage_metrics.total_tokens || 'N/A'}`}
                </span>
              </div>
            </div>
          </div>
          {/* Right side: Time */}
          {event.timestamp && (
            <div className="text-xs text-green-600 dark:text-green-400 flex-shrink-0">
              {new Date(event.timestamp).toLocaleTimeString()}
            </div>
          )}
        </div>
        
        {/* Content with markdown rendering - always visible */}
        {event.content && (
          <div className="space-y-2">
            <div className="text-xs font-medium text-gray-700 dark:text-gray-300">Content:</div>
            <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-md p-2">
              <ConversationMarkdownRenderer content={event.content} />
            </div>
          </div>
        )}
        
      </div>
    </div>
  )
}

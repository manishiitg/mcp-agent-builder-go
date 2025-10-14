import React from 'react'
import type { TokenUsageEvent } from '../../../generated/events'
import { formatDuration } from '../../../utils/duration'

interface TokenUsageEventDisplayProps {
  event: TokenUsageEvent
}

export const TokenUsageEventDisplay: React.FC<TokenUsageEventDisplayProps> = ({ event }) => {
  return (
    <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded p-2">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-blue-700 dark:text-blue-300">
            Token Usage
          </span>
          {event.operation && (
            <span className="text-xs text-blue-600 dark:text-blue-400">
              • Operation: {event.operation}
            </span>
          )}
          {event.model_id && (
            <span className="text-xs text-blue-600 dark:text-blue-400">
              • Model: {event.model_id}
            </span>
          )}
        </div>
        {event.timestamp && (
          <span className="text-xs text-gray-500 dark:text-gray-400">
            {new Date(event.timestamp).toLocaleTimeString()}
          </span>
        )}
      </div>
      
      <div className="flex items-center gap-2 mt-1">
        <span className="text-xs text-blue-600 dark:text-blue-400">
          Input: {Number(event.generation_info?.PromptTokens ?? event.prompt_tokens ?? 0)} • Output: {Number(event.generation_info?.CompletionTokens ?? event.completion_tokens ?? 0)} • Total: {Number(event.generation_info?.TotalTokens ?? event.total_tokens ?? 0)}
        </span>
        {(Number(event.generation_info?.ReasoningTokens ?? event.reasoning_tokens ?? 0) > 0) && (
          <span className="text-xs text-purple-600 dark:text-purple-400">
            • Reasoning: {Number(event.generation_info?.ReasoningTokens ?? event.reasoning_tokens ?? 0)}
          </span>
        )}
        {event.cache_discount && event.cache_discount !== 0 && (
          <span className={`text-xs ${event.cache_discount > 0 ? 'text-green-600 dark:text-green-400' : 'text-orange-600 dark:text-orange-400'}`}>
            • Cache: {event.cache_discount > 0 ? `-${(event.cache_discount * 100).toFixed(1)}%` : `+${(Math.abs(event.cache_discount) * 100).toFixed(1)}%`}
          </span>
        )}
        {event.provider && (
          <span className="text-xs text-blue-600 dark:text-blue-400">
            • Provider: {event.provider}
          </span>
        )}
        {event.duration && (
          <span className="text-xs text-blue-600 dark:text-blue-400">
            • Duration: {formatDuration(event.duration)}
          </span>
        )}
      </div>
    </div>
  )
}

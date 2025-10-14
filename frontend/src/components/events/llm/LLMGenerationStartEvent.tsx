import React from 'react'
import type { LLMGenerationStartEvent } from '../../../generated/events'

interface LLMGenerationStartEventProps {
  event: LLMGenerationStartEvent
  mode?: 'compact' | 'detailed'
}

export const LLMGenerationStartEventDisplay: React.FC<LLMGenerationStartEventProps> = ({ event, mode = 'compact' }) => {
  if (mode === 'compact') {
    return (
      <div className="p-2 bg-indigo-50 dark:bg-indigo-900/20 border border-indigo-200 dark:border-indigo-800 rounded-md">
        <div className="text-xs text-indigo-700 dark:text-indigo-300 flex items-center gap-2">
          <span className="font-medium">LLM Generation Start</span>
          {event.turn && <span className="text-indigo-600 dark:text-indigo-400">• Turn {event.turn}</span>}
          {event.model_id && <span className="text-indigo-600 dark:text-indigo-400">• {event.model_id}</span>}

        </div>
      </div>
    )
  }

  return (
    <div className="p-3 bg-indigo-50 dark:bg-indigo-900/20 border border-indigo-200 dark:border-indigo-800 rounded-lg">
      <div className="text-xs text-indigo-700 dark:text-indigo-300 space-y-1">
        {/* Header */}
        <div className="flex items-center gap-2">
          <span className="font-medium">LLM Generation Start</span>
        </div>
        
        {/* Turn information */}
        {event.turn && (
          <div className="flex items-center gap-2">
            <span>Turn: {event.turn}</span>
          </div>
        )}
        
        {/* Model information */}
        {event.model_id && (
          <div className="flex items-center gap-2">
            <span>Model: {event.model_id}</span>
          </div>
        )}
        
        {/* Temperature */}
        {event.temperature && (
          <div><strong>Temperature:</strong> {event.temperature}</div>
        )}
        
        {/* Tools count */}
        {event.tools_count && (
          <div><strong>Tools Count:</strong> {event.tools_count}</div>
        )}
        
        {/* Messages count */}
        {event.messages_count && (
          <div className="flex items-center gap-2">
            <span>Messages: {event.messages_count}</span>
          </div>
        )}
        
        {/* Optional metadata fields */}
        {event.timestamp && <div><strong>Timestamp:</strong> {new Date(event.timestamp).toLocaleString()}</div>}
        {event.trace_id && <div><strong>Trace ID:</strong> <code className="text-xs bg-indigo-100 dark:bg-indigo-800 px-1 rounded">{event.trace_id}</code></div>}
        {event.correlation_id && <div><strong>Correlation ID:</strong> <code className="text-xs bg-indigo-100 dark:bg-indigo-800 px-1 rounded">{event.correlation_id}</code></div>}
      </div>
    </div>
  )
}
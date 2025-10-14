import { useState } from 'react'
import type { AgentStartEvent } from '@/generated/events'

interface AgentStartEventProps {
  event: AgentStartEvent
}

export function AgentStartEventComponent({ event }: AgentStartEventProps) {
  const [isExpanded, setIsExpanded] = useState(false)
  
  const formatTimestamp = (timestamp?: string) => {
    if (!timestamp) return 'Unknown time'
    return new Date(timestamp).toLocaleTimeString()
  }

  const hasExpandableContent = event.model_id || event.provider || event.parent_id || event.trace_id

  return (
    <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded p-2">
      <div className="flex items-center justify-between gap-3">
        {/* Left side: Icon and main content */}
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-blue-700 dark:text-blue-300">
              🤖 Agent Started{' '}
              <span className="text-xs font-normal text-blue-600 dark:text-blue-400">
                | Type: {event.agent_type || 'Unknown'} | Model: {event.model_id || 'Unknown'} | Provider: {event.provider || 'Unknown'}
              </span>
            </div>
          </div>
        </div>

        {/* Right side: Time and expand button */}
        <div className="flex items-center gap-2 flex-shrink-0">
          {event.timestamp && (
            <div className="text-xs text-blue-600 dark:text-blue-400">
              {formatTimestamp(event.timestamp)}
            </div>
          )}
          
          {hasExpandableContent && (
            <button 
              onClick={() => setIsExpanded(!isExpanded)}
              className="text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-200"
            >
              {isExpanded ? '▼' : '▶'}
            </button>
          )}
        </div>
      </div>

      {/* Expandable content */}
      {isExpanded && hasExpandableContent && (
        <div className="mt-3 space-y-2">
          {/* Model Information */}
          {(event.model_id || event.provider) && (
            <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-md p-2">
              <div className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Model Details:</div>
              <div className="text-xs text-gray-600 dark:text-gray-400 space-y-1">
                {event.model_id && (
                  <div>Model: <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">{event.model_id}</code></div>
                )}
                {event.provider && (
                  <div>Provider: <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">{event.provider}</code></div>
                )}
              </div>
            </div>
          )}

          {/* Hierarchy Information */}
          {event.parent_id && (
            <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-md p-2">
              <div className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Hierarchy:</div>
              <div className="text-xs text-gray-600 dark:text-gray-400">
                Parent ID: <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">{event.parent_id}</code>
              </div>
            </div>
          )}

          {/* Trace Information */}
          {(event.trace_id || event.span_id || event.event_id) && (
            <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-md p-2">
              <div className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Trace Info:</div>
              <div className="text-xs text-gray-600 dark:text-gray-400 space-y-1">
                {event.trace_id && (
                  <div>Trace: <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">{event.trace_id}</code></div>
                )}
                {event.span_id && (
                  <div>Span: <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">{event.span_id}</code></div>
                )}
                {event.event_id && (
                  <div>Event: <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">{event.event_id}</code></div>
                )}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

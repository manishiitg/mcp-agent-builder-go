import React from 'react'
import type { SmartRoutingEndEvent } from '../../../generated/events'
import { formatDuration } from '../../../utils/duration'
import { ConversationMarkdownRenderer } from '../../ui/MarkdownRenderer'

interface SmartRoutingEndEventDisplayProps {
  event: SmartRoutingEndEvent
}

export const SmartRoutingEndEventDisplay: React.FC<SmartRoutingEndEventDisplayProps> = ({
  event
}) => {
  const { 
    total_tools, 
    filtered_tools, 
    total_servers, 
    relevant_servers, 
    routing_duration, 
    success, 
    error 
  } = event
  const [isExpanded, setIsExpanded] = React.useState(false)

  const hasExpandableContent = event.llm_response || event.selected_servers || event.routing_reasoning || event.llm_model_id || event.llm_provider

  return (
    <div className="bg-gray-50 dark:bg-gray-900/20 border border-gray-200 dark:border-gray-800 rounded p-2">
      <div className="flex items-center justify-between gap-3">
        {/* Left side: Icon and main content */}
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div className="min-w-0 flex-1">
            <div className={`text-sm font-medium ${
              success ? 'text-green-700 dark:text-green-300' : 'text-red-700 dark:text-red-300'
            }`}>
              Smart Routing {success ? 'Completed' : 'Failed'}{' '}
              <span className="text-xs font-normal text-gray-600 dark:text-gray-400">
                | Tools: {total_tools || 0} → {filtered_tools || 0} | Servers: {total_servers || 0} → {relevant_servers?.length || 0}
                {routing_duration && ` | Duration: ${formatDuration(routing_duration)}`}
                {event.llm_model_id && ` | LLM: ${event.llm_model_id}`}
                {event.llm_provider && ` (${event.llm_provider})`}
              </span>
            </div>
          </div>
        </div>

        {/* Right side: Time and expand button */}
        <div className="flex items-center gap-2 flex-shrink-0">
          {event.timestamp && (
            <div className="text-xs text-gray-600 dark:text-gray-400">
              {new Date(event.timestamp).toLocaleTimeString()}
            </div>
          )}
          
          {hasExpandableContent && (
            <button
              onClick={() => setIsExpanded(!isExpanded)}
              className="text-xs text-gray-600 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-200 underline cursor-pointer flex-shrink-0"
            >
              {isExpanded ? '▼' : '▶'}
            </button>
          )}
        </div>
      </div>
      
      {/* Error display */}
      {error && (
        <div className="mt-2 text-xs text-red-600 dark:text-red-400">
          Error: {error.length > 80 ? `${error.substring(0, 80)}...` : error}
        </div>
      )}
      
      {/* Expanded LLM Details */}
      {isExpanded && hasExpandableContent && (
        <div className="mt-3 space-y-3 border-t border-gray-200 dark:border-gray-700 pt-3">
          {/* LLM Configuration */}
          {(event.llm_model_id || event.llm_provider || event.llm_temperature || event.llm_max_tokens) && (
            <div>
              <div className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">⚙️ LLM Configuration:</div>
              <div className="text-xs text-gray-600 dark:text-gray-400 space-y-1">
                {event.llm_model_id && <div>Model: <span className="font-mono">{event.llm_model_id}</span></div>}
                {event.llm_provider && <div>Provider: <span className="font-mono">{event.llm_provider}</span></div>}
                {event.llm_temperature !== undefined && <div>Temperature: <span className="font-mono">{event.llm_temperature}</span></div>}
                {event.llm_max_tokens && <div>Max Tokens: <span className="font-mono">{event.llm_max_tokens}</span></div>}
              </div>
            </div>
          )}
          
          {event.routing_reasoning && (
            <div>
              <div className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">🧠 Routing Reasoning:</div>
              <ConversationMarkdownRenderer content={event.routing_reasoning} />
            </div>
          )}
          
          {event.llm_response && (
            <div>
              <div className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">🤖 LLM Response:</div>
              <ConversationMarkdownRenderer content={event.llm_response} />
            </div>
          )}
          
          {event.selected_servers && (
            <div>
              <div className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Server Selection:</div>
              <ConversationMarkdownRenderer content={event.selected_servers} />
            </div>
          )}
        </div>
      )}
    </div>
  )
}

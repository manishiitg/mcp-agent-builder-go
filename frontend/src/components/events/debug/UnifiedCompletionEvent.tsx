import React from 'react'
import { ConversationMarkdownRenderer } from '../../ui/MarkdownRenderer'

interface UnifiedCompletionEvent {
  timestamp?: string
  trace_id?: string
  span_id?: string
  event_id?: string
  parent_id?: string
  is_end_event?: boolean
  correlation_id?: string
  agent_type?: string
  agent_mode?: string
  question?: string
  final_result?: string
  status?: string
  duration?: number
  turns?: number
  error?: string
  metadata?: Record<string, unknown>
}

interface UnifiedCompletionEventDisplayProps {
  event: UnifiedCompletionEvent
}

export const UnifiedCompletionEventDisplay: React.FC<UnifiedCompletionEventDisplayProps> = ({ event }) => {

  // Note: event.duration is in nanoseconds from Go time.Duration
  const formatDuration = (durationNs: number) => {
    if (!durationNs || durationNs <= 0) {
      return '0ms'
    }

    // Convert nanoseconds to milliseconds
    const durationMs = durationNs / 1000000

    if (durationMs < 1) {
      // Less than 1ms, show in microseconds
      const durationUs = durationNs / 1000
      return `${Math.round(durationUs)}μs`
    } else if (durationMs < 1000) {
      // Less than 1ms, show in milliseconds
      return `${Math.round(durationMs)}ms`
    } else if (durationMs < 60000) {
      // Less than 1 minute, show in seconds
      return `${(durationMs / 1000).toFixed(1)}s`
    } else {
      // 1 minute or more, show in minutes
      return `${(durationMs / 60000).toFixed(1)}m`
    }
  }

  // Single-line layout following design guidelines
  return (
    <div className="bg-gray-50 dark:bg-gray-900/20 border border-gray-200 dark:border-gray-800 rounded p-2">
      <div className="flex items-center justify-between gap-3">
        {/* Left side: Icon and main content */}
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-gray-700 dark:text-gray-300">
              ✅ Unified Completion{' '}
              <span className="text-xs font-normal text-gray-600 dark:text-gray-400">
                {event.agent_mode && `• Mode: ${event.agent_mode}`}
                {event.duration && ` • Duration: ${formatDuration(event.duration)}`}
                {event.turns && ` • Turn: ${event.turns}`}
                {event.status && ` • ${event.status}`}
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

      {/* Result always visible below */}
      {event.final_result && (
        <div className="mt-2">
          <div className="bg-white dark:bg-gray-800 rounded-md p-2">
            <ConversationMarkdownRenderer content={event.final_result} />
          </div>
        </div>
      )}
    </div>
  )
}

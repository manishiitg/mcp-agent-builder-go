import React from 'react'
import type {
  ReActReasoningStepEvent,
  ReActReasoningFinalEvent,
  ReActReasoningStartEvent,
  ReActReasoningEndEvent
} from '../../../generated/events'

type ReActReasoningEvent = ReActReasoningStepEvent | ReActReasoningFinalEvent | ReActReasoningStartEvent | ReActReasoningEndEvent

interface ReActReasoningEventDisplayProps {
  event: ReActReasoningEvent
}

export const ReActReasoningEventDisplay: React.FC<ReActReasoningEventDisplayProps> = ({
  event
}) => {
  // Type guard to determine event type
  const isStepEvent = (e: ReActReasoningEvent): e is ReActReasoningStepEvent =>
    'step_number' in e && 'thought' in e

  const isFinalEvent = (e: ReActReasoningEvent): e is ReActReasoningFinalEvent =>
    'final_answer' in e && 'reasoning' in e

  const isStartEvent = (e: ReActReasoningEvent): e is ReActReasoningStartEvent =>
    'question' in e

  const isEndEvent = (e: ReActReasoningEvent): e is ReActReasoningEndEvent =>
    'total_steps' in e

  // Get event type and content for single-line display
  const getEventTypeAndContent = () => {
    if (isStepEvent(event)) {
      return {
        type: `Step ${event.step_number}`,
        content: event.thought || '',
        icon: '🤔'
      }
    }
    if (isFinalEvent(event)) {
      return {
        type: 'Final Answer',
        content: event.final_answer || '',
        icon: '✅'
      }
    }
    if (isStartEvent(event)) {
      return {
        type: 'Started',
        content: event.question || '',
        icon: 'ReAct'
      }
    }
    if (isEndEvent(event)) {
      return {
        type: 'Ended',
        content: `Completed ${event.total_steps || 0} steps`,
        icon: '🏁'
      }
    }
    return {
      type: 'Unknown',
      content: '',
      icon: '❓'
    }
  }

  const { type, content, icon } = getEventTypeAndContent()

  // Single-line layout following design guidelines
  return (
    <div className="bg-gray-50 dark:bg-gray-900/20 border border-gray-200 dark:border-gray-800 rounded p-2">
      <div className="flex items-center justify-between gap-3">
        {/* Left side: Icon and main content */}
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-gray-700 dark:text-gray-300">
              {icon} ReAct Reasoning {type}{' '}
              <span className="text-xs font-normal text-gray-600 dark:text-gray-400">
                {event.turn && `• Turn: ${event.turn}`}
                {content && ` • ${content.length > 60 ? `${content.substring(0, 60)}...` : content}`}
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

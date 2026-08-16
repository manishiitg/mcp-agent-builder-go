import React, { useState } from 'react'
import { Brain, ChevronDown, ChevronRight } from 'lucide-react'
import type { ConversationThinkingEvent } from '../../../generated/event-types'

interface ConversationThinkingEventProps {
  event: ConversationThinkingEvent
  compact?: boolean
}

/**
 * A readable progress update emitted by a coding-agent transport. This is a
 * structured event, rather than a rendered terminal/tmux frame.
 */
export const ConversationThinkingEventDisplay: React.FC<ConversationThinkingEventProps> = ({
  event,
  compact = false,
}) => {
  const [isExpanded, setIsExpanded] = useState(!compact)
  const thinking = event.thinking?.trim()

  if (!thinking) return null

  return (
    <div className="rounded-md border border-violet-200 bg-violet-50/70 p-2 dark:border-violet-900/70 dark:bg-violet-950/25">
      <button
        type="button"
        onClick={() => setIsExpanded(expanded => !expanded)}
        className="flex w-full items-center gap-2 text-left text-xs text-violet-800 hover:text-violet-950 dark:text-violet-200 dark:hover:text-violet-50"
        aria-expanded={isExpanded}
      >
        <Brain className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
        <span className="font-medium">Thinking</span>
        {event.turn ? <span className="text-violet-600 dark:text-violet-400">• Turn {event.turn}</span> : null}
        <span className="ml-auto">
          {isExpanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        </span>
      </button>
      {isExpanded ? (
        <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-violet-950 dark:text-violet-100">
          {thinking}
        </p>
      ) : null}
    </div>
  )
}

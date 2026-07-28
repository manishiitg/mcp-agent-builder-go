import React, { useState } from 'react'
import type { SystemPromptEvent } from '../../../generated/events'
import { PlainMarkdown } from '../../ui/PlainMarkdown'

interface SystemPromptEventDisplayProps {
  event: SystemPromptEvent
  mode?: 'compact' | 'detailed'
}

export const SystemPromptEventDisplay: React.FC<SystemPromptEventDisplayProps> = ({
  event,
  mode = 'detailed'
}) => {
  const [isExpanded, setIsExpanded] = useState(false)
  const CHAR_LIMIT = 300

  // Check if content is long enough to need expansion
  const shouldShowExpand = event.content && event.content.length > CHAR_LIMIT

  if (mode === 'compact') {
    return (
      <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded p-2">
        <div className="flex items-start gap-2">
          <span className="text-xs font-bold text-blue-700 dark:text-blue-300">System</span>
          <div className="flex-1 min-w-0">
            {event.content ? (
              <div className="text-xs text-blue-900 dark:text-blue-100 leading-tight">
                {isExpanded || event.content.length <= CHAR_LIMIT
                  ? event.content
                  : `${event.content.substring(0, CHAR_LIMIT)}...`
                }
              </div>
            ) : (
              <div className="text-xs text-red-600 dark:text-red-400 italic">
                No prompt content
              </div>
            )}
            {shouldShowExpand && (
              <button
                onClick={() => setIsExpanded(!isExpanded)}
                className="text-xs text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 mt-1"
              >
                {isExpanded ? '↑ Collapse' : '↓ Expand'}
              </button>
            )}
            {event.token_count !== undefined && event.token_count > 0 && (
              <div className="text-xs text-blue-600 dark:text-blue-400 mt-1 font-medium">
                📊 {event.token_count.toLocaleString()} tokens
              </div>
            )}
          </div>
        </div>
      </div>
    )
  }

  const charCount = event.content?.length || 0

  // Kept collapsed by default, unlike the task instructions: a real system
  // prompt here is ~12,900 characters, so opening it would bury the actual
  // conversation. What changed is that the collapsed state now states the size
  // instead of showing a 300-character stump of markdown, and expanding renders
  // it as markdown rather than raw monospace with the syntax visible.
  return (
    <div className="rounded border border-blue-200 bg-blue-50/60 px-2.5 py-1.5 dark:border-blue-800/70 dark:bg-blue-900/15">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className="font-medium text-blue-700 dark:text-blue-300">System prompt</span>
        {charCount > 0 && (
          <span className="text-blue-600/80 dark:text-blue-400/80">
            {charCount.toLocaleString()} chars
          </span>
        )}
        {event.token_count !== undefined && event.token_count > 0 && (
          <span className="text-blue-600/80 dark:text-blue-400/80">
            {event.token_count.toLocaleString()} tokens
          </span>
        )}
        {charCount > 0 && (
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            className="ml-auto text-blue-700 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-200"
          >
            {isExpanded ? 'Hide' : 'Show'}
          </button>
        )}
      </div>
      {!event.content && (
        <div className="mt-1 text-xs italic text-red-600 dark:text-red-400">No prompt content</div>
      )}
      {isExpanded && event.content && (
        <div className="mt-2 max-h-96 overflow-auto rounded border border-blue-200/70 bg-white/70 px-3 py-2 dark:border-blue-800/60 dark:bg-black/30">
          <PlainMarkdown content={event.content} />
        </div>
      )}
    </div>
  )
}

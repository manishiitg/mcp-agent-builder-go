import React from 'react'
import type { CacheEvent } from '../../../generated/events'

interface CacheEventDisplayProps {
  event: CacheEvent
}

export const CacheEventDisplay: React.FC<CacheEventDisplayProps> = ({ event }) => {
  const getOperationIcon = (operation: string) => {
    switch (operation) {
      case 'hit':
        return 'Hit'
      case 'miss':
        return 'Miss'
      case 'write':
        return 'Write'
      case 'expired':
        return 'Expired'
      case 'cleanup':
        return 'Cleanup'
      case 'error':
        return 'Error'
      case 'start':
        return 'Start'
      default:
        return 'Unknown'
    }
  }

  const getOperationDetails = () => {
    switch (event.operation) {
      case 'hit':
        return `Tools: ${event.tools_count || 0} • Age: ${event.age || 'N/A'}`
      case 'miss':
        return `Reason: ${event.reason || 'N/A'}`
      case 'write':
        return `Tools: ${event.tools_count || 0} • Size: ${event.data_size ? `${event.data_size} bytes` : 'N/A'} • TTL: ${event.ttl || 'N/A'}`
      case 'expired':
        return `Age: ${event.age || 'N/A'} • TTL: ${event.ttl || 'N/A'}`
      case 'cleanup':
        return `Type: ${event.cleanup_type || 'N/A'} • Removed: ${event.entries_removed || 0}/${event.entries_total || 0} • Freed: ${event.space_freed ? `${event.space_freed} bytes` : 'N/A'}`
      case 'error':
        return `Error: ${event.error || 'N/A'} • Type: ${event.error_type || 'N/A'}`
      case 'start':
        return `Config: ${event.config_path || 'N/A'}`
      default:
        return ''
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
              {getOperationIcon(event.operation || '')} Cache {event.operation?.toUpperCase() || 'UNKNOWN'}{' '}
              <span className="text-xs font-normal text-gray-600 dark:text-gray-400">
                • Server: {event.server_name || 'N/A'} • Key: {event.cache_key || 'N/A'}
                {getOperationDetails() && ` • ${getOperationDetails()}`}
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

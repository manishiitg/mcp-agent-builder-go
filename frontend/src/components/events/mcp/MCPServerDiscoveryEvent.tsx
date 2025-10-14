import React from 'react'
import { Search, CheckCircle, AlertCircle } from 'lucide-react'
import type { MCPServerDiscoveryEvent } from '../../../generated/events'

interface MCPServerDiscoveryEventProps {
  event: MCPServerDiscoveryEvent
  compact?: boolean
}

export const MCPServerDiscoveryEventDisplay: React.FC<MCPServerDiscoveryEventProps> = ({ event, compact = false }) => {
  const hasError = !!event.error
  const isSuccess = !hasError && event.connected_servers && event.connected_servers > 0

  const bgColor = isSuccess
    ? 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800'
    : hasError
    ? 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800'
    : 'bg-blue-50 dark:bg-blue-900/20 border-blue-200 dark:border-blue-800'

  const textColor = isSuccess
    ? 'text-green-700 dark:text-green-300'
    : hasError
    ? 'text-red-700 dark:text-red-300'
    : 'text-blue-700 dark:text-blue-300'

  const iconColor = isSuccess
    ? 'text-green-600'
    : hasError
    ? 'text-red-600'
    : 'text-blue-600'

  const Icon = isSuccess ? CheckCircle : hasError ? AlertCircle : Search

  if (compact) {
    return (
      <div className={`p-2 ${bgColor} border rounded-md`}>
        <div className={`text-xs ${textColor} flex items-center gap-2`}>
          <Icon className={`w-3 h-3 ${iconColor}`} />
          <span className="font-medium">MCP Server Discovery</span>
          {event.server_name && <span className={`${iconColor} dark:text-opacity-80`}>• {event.server_name}</span>}
          {event.connected_servers && <span className={`${iconColor} dark:text-opacity-80`}>• {event.connected_servers} connected</span>}
          {event.total_servers && <span className={`${iconColor} dark:text-opacity-80`}>• {event.total_servers} total</span>}
          {event.error && <span className="text-red-600 dark:text-red-400">• Error</span>}
        </div>
      </div>
    )
  }

  return (
    <div className={`${bgColor} border rounded-lg p-3`}>
      <div className={`text-xs ${textColor} space-y-1`}>
        {/* Header */}
        <div className="flex items-center gap-2">
          <Icon className={`w-4 h-4 ${iconColor}`} />
          <span className="font-medium">MCP Server Discovery</span>
        </div>
        
        {/* Server information */}
        {event.server_name && (
          <div><strong>Server:</strong> {event.server_name}</div>
        )}
        
        {/* Operation */}
        {event.operation && (
          <div><strong>Operation:</strong> {event.operation}</div>
        )}
        
        {/* Server counts */}
        {event.total_servers && (
          <div><strong>Total Servers:</strong> {event.total_servers}</div>
        )}
        
        {event.connected_servers && (
          <div><strong>Connected Servers:</strong> {event.connected_servers}</div>
        )}
        
        {event.failed_servers && (
          <div><strong>Failed Servers:</strong> {event.failed_servers}</div>
        )}
        
        {/* Discovery metrics */}
        {event.discovery_time && (
          <div><strong>Discovery Time:</strong> {event.discovery_time}ms</div>
        )}
        
        {event.tool_count && (
          <div><strong>Tool Count:</strong> {event.tool_count}</div>
        )}
        
        {/* Error */}
        {event.error && (
          <div className="text-red-700 dark:text-red-300">
            <strong>Error:</strong> {event.error}
          </div>
        )}
        
        {/* Optional metadata fields */}
        {event.timestamp && <div><strong>Timestamp:</strong> {new Date(event.timestamp).toLocaleString()}</div>}
        {event.trace_id && <div><strong>Trace ID:</strong> <code className="text-xs bg-gray-100 dark:bg-gray-800 px-1 rounded">{event.trace_id}</code></div>}
        {event.correlation_id && <div><strong>Correlation ID:</strong> <code className="text-xs bg-gray-100 dark:bg-gray-800 px-1 rounded">{event.correlation_id}</code></div>}
      </div>
    </div>
  )
}

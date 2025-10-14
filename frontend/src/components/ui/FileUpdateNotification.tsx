import React, { useEffect, useState } from 'react'

interface FileUpdateNotificationProps {
  filepath: string
  timestamp: string
  onDismiss: () => void
  onOpenFile?: (filepath: string) => void
  onHighlightFile?: (filepath: string) => void
}

export const FileUpdateNotification: React.FC<FileUpdateNotificationProps> = ({
  filepath,
  timestamp,
  onDismiss,
  onOpenFile,
  onHighlightFile
}) => {
  const [isVisible, setIsVisible] = useState(false)

  // Auto-dismiss after 5 seconds
  useEffect(() => {
    setIsVisible(true)
    const timer = setTimeout(() => {
      setIsVisible(false)
      setTimeout(onDismiss, 300) // Wait for animation to complete
    }, 5000)

    return () => clearTimeout(timer)
  }, [onDismiss])

  const formatTimestamp = (timestamp: string) => {
    try {
      return new Date(timestamp).toLocaleTimeString()
    } catch {
      return 'now'
    }
  }

  const getFileName = (path: string) => {
    return path.split('/').pop() || path
  }

  return (
    <div
      className={`transition-all duration-300 ease-in-out ${
        isVisible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-2'
      }`}
    >
      <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-md px-2 py-1 shadow-sm">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-1.5 min-w-0 flex-1">
            <div className="w-1.5 h-1.5 bg-blue-500 rounded-full flex-shrink-0"></div>
            <div className="min-w-0 flex-1">
              <div className="text-xs font-medium text-blue-700 dark:text-blue-300 truncate" title={filepath}>
                📝 {getFileName(filepath)}
              </div>
              <div className="text-xs text-blue-500 dark:text-blue-500">
                {formatTimestamp(timestamp)}
              </div>
            </div>
          </div>
          
          <div className="flex items-center gap-0.5">
            {/* Highlight file button */}
            {onHighlightFile && (
              <button
                onClick={() => onHighlightFile(filepath)}
                className="text-blue-400 hover:text-blue-600 dark:text-blue-500 dark:hover:text-blue-300 transition-colors p-0.5"
                title="Highlight file in workspace"
              >
                <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                </svg>
              </button>
            )}
            
            {/* Open file button */}
            {onOpenFile && (
              <button
                onClick={() => onOpenFile(filepath)}
                className="text-blue-400 hover:text-blue-600 dark:text-blue-500 dark:hover:text-blue-300 transition-colors p-0.5"
                title="Open file"
              >
                <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                </svg>
              </button>
            )}
            
            {/* Dismiss button */}
            <button
              onClick={() => {
                setIsVisible(false)
                setTimeout(onDismiss, 300)
              }}
              className="text-blue-400 hover:text-blue-600 dark:text-blue-500 dark:hover:text-blue-300 transition-colors p-0.5"
              title="Dismiss notification"
            >
              <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

export default FileUpdateNotification

import React, { useMemo } from 'react'

interface DiffRendererProps {
  diff: string
  maxHeight?: string
}

interface DiffStats {
  additions: number
  removals: number
  context: number
  hunks: number
}

export const DiffRenderer: React.FC<DiffRendererProps> = ({ diff, maxHeight = '400px' }) => {
  const lines = diff.split('\n')
  
  // Calculate diff statistics
  const stats = useMemo((): DiffStats => {
    let additions = 0
    let removals = 0
    let context = 0
    let hunks = 0
    
    lines.forEach(line => {
      if (line.startsWith('+') && !line.startsWith('+++')) additions++
      else if (line.startsWith('-') && !line.startsWith('---')) removals++
      else if (line.startsWith(' ')) context++
      else if (line.startsWith('@@')) hunks++
    })
    
    return { additions, removals, context, hunks }
  }, [lines])
  
  const renderLine = (line: string, index: number) => {
    // Headers (--- and +++)
    if (line.startsWith('---') || line.startsWith('+++')) {
      const isOld = line.startsWith('---')
      return (
        <div key={index} className="flex font-mono text-xs py-0.5">
          <span className={`font-semibold ${isOld ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'}`}>
            {line}
          </span>
        </div>
      )
    }
    
    // Hunk headers (@@ ... @@)
    if (line.startsWith('@@') && line.includes('@@')) {
      return (
        <div key={index} className="flex font-mono text-xs bg-blue-100 dark:bg-blue-900/30 border-l-2 border-blue-500 dark:border-blue-400 py-0.5">
          <span className="text-blue-700 dark:text-blue-300 font-semibold px-2">{line}</span>
        </div>
      )
    }
    
    // Addition lines (start with +)
    if (line.startsWith('+') && !line.startsWith('+++')) {
      return (
        <div key={index} className="flex font-mono text-xs bg-green-50 dark:bg-green-900/20 border-l-2 border-green-500 dark:border-green-400">
          <span className="text-green-700 dark:text-green-400 select-none w-4 flex-shrink-0 text-center">+</span>
          <span className="text-green-800 dark:text-green-300 flex-1 pr-2">{line.substring(1) || ' '}</span>
        </div>
      )
    }
    
    // Removal lines (start with -)
    if (line.startsWith('-') && !line.startsWith('---')) {
      return (
        <div key={index} className="flex font-mono text-xs bg-red-50 dark:bg-red-900/20 border-l-2 border-red-500 dark:border-red-400">
          <span className="text-red-700 dark:text-red-400 select-none w-4 flex-shrink-0 text-center">-</span>
          <span className="text-red-800 dark:text-red-300 flex-1 pr-2">{line.substring(1) || ' '}</span>
        </div>
      )
    }
    
    // Context lines (start with space) - CRITICAL for unified diff
    if (line.startsWith(' ')) {
      return (
        <div key={index} className="flex font-mono text-xs border-l-2 border-gray-200 dark:border-gray-700">
          <span className="text-gray-400 dark:text-gray-600 select-none w-4 flex-shrink-0 text-center">·</span>
          <span className="text-gray-700 dark:text-gray-300 flex-1 pr-2">{line.substring(1)}</span>
        </div>
      )
    }
    
    // Empty lines
    if (line === '') {
      return (
        <div key={index} className="flex font-mono text-xs h-4 border-l-2 border-gray-200 dark:border-gray-700">
          <span className="text-gray-400 dark:text-gray-600 select-none w-4 flex-shrink-0"> </span>
        </div>
      )
    }
    
    // Other lines (shouldn't happen in valid unified diff)
    return (
      <div key={index} className="flex font-mono text-xs border-l-2 border-yellow-500 dark:border-yellow-400 bg-yellow-50 dark:bg-yellow-900/20">
        <span className="text-yellow-700 dark:text-yellow-300 px-2">⚠️ {line}</span>
      </div>
    )
  }
  
  return (
    <div 
      className="border border-gray-200 dark:border-gray-700 rounded-md bg-white dark:bg-gray-900 overflow-hidden"
    >
      {/* Statistics Header */}
      <div className="border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800 px-3 py-2">
        <div className="flex items-center gap-4 text-xs">
          <div className="flex items-center gap-1">
            <span className="text-green-700 dark:text-green-400 font-semibold">+{stats.additions}</span>
            <span className="text-gray-600 dark:text-gray-400">additions</span>
          </div>
          <div className="flex items-center gap-1">
            <span className="text-red-700 dark:text-red-400 font-semibold">-{stats.removals}</span>
            <span className="text-gray-600 dark:text-gray-400">removals</span>
          </div>
          <div className="flex items-center gap-1">
            <span className="text-gray-500 dark:text-gray-400 font-semibold">{stats.context}</span>
            <span className="text-gray-600 dark:text-gray-400">context</span>
          </div>
          <div className="flex items-center gap-1">
            <span className="text-blue-700 dark:text-blue-400 font-semibold">{stats.hunks}</span>
            <span className="text-gray-600 dark:text-gray-400">{stats.hunks === 1 ? 'hunk' : 'hunks'}</span>
          </div>
        </div>
      </div>
      
      {/* Diff Content */}
      <div className="overflow-y-auto overflow-x-auto" style={{ maxHeight }}>
        <div className="p-2">
          {lines.map((line, index) => renderLine(line, index))}
        </div>
      </div>
      
      {/* Legend Footer */}
      <div className="border-t border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800 px-3 py-2">
        <div className="flex items-center gap-4 text-xs flex-wrap">
          <div className="flex items-center gap-1">
            <span className="text-green-700 dark:text-green-400 font-semibold">+</span>
            <span className="text-gray-600 dark:text-gray-400">Addition</span>
          </div>
          <div className="flex items-center gap-1">
            <span className="text-red-700 dark:text-red-400 font-semibold">-</span>
            <span className="text-gray-600 dark:text-gray-400">Removal</span>
          </div>
          <div className="flex items-center gap-1">
            <span className="text-gray-400 dark:text-gray-600 font-semibold">·</span>
            <span className="text-gray-600 dark:text-gray-400">Context</span>
          </div>
          <div className="flex items-center gap-1">
            <span className="text-blue-700 dark:text-blue-400 font-semibold">@@</span>
            <span className="text-gray-600 dark:text-gray-400">Hunk</span>
          </div>
          <div className="flex items-center gap-1">
            <span className="text-yellow-700 dark:text-yellow-400 font-semibold">⚠️</span>
            <span className="text-gray-600 dark:text-gray-400">Invalid</span>
          </div>
        </div>
      </div>
    </div>
  )
}

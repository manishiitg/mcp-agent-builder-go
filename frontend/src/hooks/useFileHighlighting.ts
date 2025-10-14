import { useState, useEffect, useCallback } from 'react'
import type { PlannerFile } from '../services/api-types'
import { findFileInTree } from '../utils/fileUtils'

interface UseFileHighlightingProps {
  files: PlannerFile[]
  onFileHighlight?: (filepath: string) => void
  onRefreshFiles?: () => Promise<void>
}

interface UseFileHighlightingReturn {
  highlightedFile: string | null
  setHighlightedFile: (filepath: string | null) => void
  handleFileHighlight: (filepath: string) => Promise<void>
}

/**
 * Custom hook for managing file highlighting with smart refresh logic
 */
export const useFileHighlighting = ({
  files,
  onFileHighlight,
  onRefreshFiles
}: UseFileHighlightingProps): UseFileHighlightingReturn => {
  const [highlightedFile, setHighlightedFile] = useState<string | null>(null)

  const handleFileHighlight = useCallback(async (filepath: string) => {
    try {
      // Check if file exists in current file tree
      const fileExists = findFileInTree(files, filepath)
      
      if (!fileExists && onRefreshFiles) {
        // File not found in workspace, refreshing
        await onRefreshFiles()
        
        // Wait a bit for state to update after refresh
        setTimeout(() => {
          setHighlightedFile(filepath)
          // Highlighting file after refresh
        }, 100)
      } else {
        setHighlightedFile(filepath)
        // Highlighting existing file
      }
      
      // Auto-clear highlight after 5 seconds
      setTimeout(() => setHighlightedFile(null), 5000)
      
    } catch (error) {
      console.error('[File highlight] Error highlighting file:', error)
    }
  }, [files, onRefreshFiles])

  // Set up global highlight function
  useEffect(() => {
    if (onFileHighlight) {
      window.highlightFile = handleFileHighlight
    }
    
    return () => {
      if (window.highlightFile === handleFileHighlight) {
        delete window.highlightFile
      }
    }
  }, [handleFileHighlight, onFileHighlight])

  return {
    highlightedFile,
    setHighlightedFile,
    handleFileHighlight
  }
}

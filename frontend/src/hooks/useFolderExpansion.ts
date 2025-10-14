import { useState, useCallback } from 'react'
import { extractFolderPaths } from '../utils/fileUtils'
import type { PlannerFile } from '../services/api-types'

interface UseFolderExpansionReturn {
  expandedFolders: Set<string>
  setExpandedFolders: React.Dispatch<React.SetStateAction<Set<string>>>
  expandFoldersForFile: (filepath: string) => void
  toggleFolder: (folderPath: string) => void
  expandFoldersToLevel: (files: PlannerFile[], maxLevel?: number) => void
}

/**
 * Custom hook for managing folder expansion state
 */
export const useFolderExpansion = (): UseFolderExpansionReturn => {
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(new Set())

  const expandFoldersForFile = useCallback((filepath: string) => {
    const foldersToExpand = extractFolderPaths(filepath)
    
    setExpandedFolders(prev => {
      const newExpanded = new Set(prev)
      foldersToExpand.forEach(folder => newExpanded.add(folder))
      return newExpanded
    })
  }, [])

  const toggleFolder = useCallback((folderPath: string) => {
    setExpandedFolders(prev => {
      const newExpanded = new Set(prev)
      if (newExpanded.has(folderPath)) {
        newExpanded.delete(folderPath)
      } else {
        newExpanded.add(folderPath)
      }
      return newExpanded
    })
  }, [])

  const expandFoldersToLevel = useCallback((files: PlannerFile[], maxLevel: number = 2) => {
    const foldersToExpand = new Set<string>()
    
    const collectFoldersAtLevel = (fileList: PlannerFile[], currentLevel: number) => {
      fileList.forEach(file => {
        if (file.type === 'folder' && currentLevel < maxLevel) {
          foldersToExpand.add(file.filepath)
          if (file.children) {
            collectFoldersAtLevel(file.children, currentLevel + 1)
          }
        }
      })
    }
    
    collectFoldersAtLevel(files, 0)
    setExpandedFolders(foldersToExpand)
  }, [])

  return {
    expandedFolders,
    setExpandedFolders,
    expandFoldersForFile,
    toggleFolder,
    expandFoldersToLevel
  }
}

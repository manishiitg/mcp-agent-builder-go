import { useState, useEffect, useCallback } from 'react'
import { WandSparkles, Loader2, AlertCircle, Plus, RefreshCw } from 'lucide-react'
import { skillsApi } from '../../api/skills'
import type { Skill } from '../../types/skills'
import SkillCard from './SkillCard'
import SkillImportDialog from './SkillImportDialog'

interface SkillsManagerPanelProps {
  /** The embedded workflow-panel context: tighter spacing for a narrow side
   * panel instead of the modal's roomier padding. SkillCard is already a
   * full-width block with no column assumptions, so the list itself doesn't
   * need a layout change -- just less padding around it. */
  compact?: boolean
}

export default function SkillsManagerPanel({ compact = false }: SkillsManagerPanelProps) {
  const [skills, setSkills] = useState<Skill[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showImportDialog, setShowImportDialog] = useState(false)

  const loadSkills = useCallback(async () => {
    setIsLoading(true)
    setError(null)
    try {
      const response = await skillsApi.listSkills()
      // Deduplicate by file_path to prevent React duplicate key crashes
      const raw = response.skills || []
      const seen = new Set<string>()
      const unique = raw.filter(s => {
        const key = s.file_path || s.folder_name
        if (seen.has(key)) return false
        seen.add(key)
        return true
      })
      setSkills(unique)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load skills')
      setSkills([])
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    loadSkills()
  }, [loadSkills])

  const handleDelete = async (folderName: string) => {
    if (!confirm(`Are you sure you want to delete the skill "${folderName}"?`)) {
      return
    }

    try {
      await skillsApi.deleteSkill(folderName)
      loadSkills()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete skill')
    }
  }

  const handleImportSuccess = () => {
    setShowImportDialog(false)
    loadSkills()
  }

  return (
    <div className={`flex min-h-0 flex-1 flex-col ${compact ? 'gap-2' : 'gap-3'}`}>
      <div className="flex shrink-0 items-center justify-between">
        <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
          {skills?.length || 0} {(skills?.length || 0) === 1 ? 'Skill' : 'Skills'}
        </span>
        <div className="flex items-center gap-1">
          <button
            onClick={loadSkills}
            className="p-1.5 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 transition-colors"
            title="Refresh skills"
          >
            <RefreshCw className={`w-4 h-4 ${isLoading ? 'animate-spin' : ''}`} />
          </button>
          <button
            onClick={() => setShowImportDialog(true)}
            className="px-2.5 py-1 text-xs font-medium text-purple-600 dark:text-purple-400 bg-purple-50 dark:bg-purple-900/30 hover:bg-purple-100 dark:hover:bg-purple-900/50 rounded-md transition-colors flex items-center gap-1.5"
          >
            <Plus className="w-3.5 h-3.5" />
            Import
          </button>
        </div>
      </div>

      {error && (
        <div className="flex shrink-0 items-center gap-2 text-sm text-red-500 dark:text-red-400">
          <AlertCircle className="w-4 h-4" />
          <span>Error: {error}</span>
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto">
        {isLoading && skills.length === 0 ? (
          <div className="flex items-center gap-2 py-4 text-sm text-gray-500 dark:text-gray-400">
            <Loader2 className="w-4 h-4 animate-spin" />
            <span>Loading skills...</span>
          </div>
        ) : (skills?.length || 0) === 0 ? (
          <div className={`flex flex-col items-center justify-center text-gray-500 dark:text-gray-400 ${compact ? 'py-6' : 'h-full'}`}>
            <WandSparkles className={`${compact ? 'w-8 h-8 mb-2' : 'w-12 h-12 mb-4'} opacity-50`} />
            <p className={`${compact ? 'text-sm' : 'text-lg'} font-medium mb-2`}>No skills installed</p>
            <p className="text-sm text-center mb-4">
              Import skills to extend your agent's capabilities
            </p>
            <button
              onClick={() => setShowImportDialog(true)}
              className="px-4 py-2 text-sm font-medium text-white bg-purple-600 hover:bg-purple-700 rounded-md transition-colors flex items-center gap-2"
            >
              <Plus className="w-4 h-4" />
              Import
            </button>
          </div>
        ) : (
          <div className={`grid ${compact ? 'gap-2' : 'gap-4'}`}>
            {(skills || []).map((skill) => (
              <SkillCard
                key={skill.file_path || skill.folder_name}
                skill={skill}
                onDelete={() => handleDelete(skill.folder_name)}
              />
            ))}
          </div>
        )}
      </div>

      {showImportDialog && (
        <SkillImportDialog
          onClose={() => setShowImportDialog(false)}
          onSuccess={handleImportSuccess}
        />
      )}
    </div>
  )
}

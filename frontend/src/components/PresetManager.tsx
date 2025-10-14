import React, { useState, useCallback } from 'react'
import { Plus, Edit, Trash2, Save, X, Folder } from 'lucide-react'
import { usePresetStore, type FolderPreset } from '../stores/usePresetStore'
import { FolderSelectionDialog } from './FolderSelectionDialog'
import type { PlannerFile } from '../services/api-types'

interface PresetManagerProps {
  category: 'workflow' | 'orchestrator'
  onPresetSelect?: (preset: FolderPreset) => void
  className?: string
}

export const PresetManager: React.FC<PresetManagerProps> = ({
  category,
  onPresetSelect,
  className = ''
}) => {
  const {
    selectedPreset,
    createPreset,
    updatePreset,
    deletePreset,
    selectPreset,
    getPresetsByCategory
  } = usePresetStore()

  const [isCreating, setIsCreating] = useState(false)
  const [isEditing, setIsEditing] = useState<string | null>(null)
  const [showFolderDialog, setShowFolderDialog] = useState(false)
  const [editingPreset, setEditingPreset] = useState<Partial<FolderPreset> | null>(null)
  const [folderDialogPosition, setFolderDialogPosition] = useState({ top: 0, left: 0 })

  const categoryPresets = getPresetsByCategory(category)

  const handleCreatePreset = useCallback(() => {
    setEditingPreset({
      name: '',
      description: '',
      category,
      folders: []
    })
    setIsCreating(true)
  }, [category])

  const handleEditPreset = useCallback((preset: FolderPreset) => {
    setEditingPreset({
      name: preset.name,
      description: preset.description,
      category: preset.category,
      folders: preset.folders
    })
    setIsEditing(preset.id)
  }, [])

  const handleSavePreset = useCallback(() => {
    if (!editingPreset || !editingPreset.name?.trim()) return

    if (isCreating) {
      createPreset(editingPreset as Omit<FolderPreset, 'id' | 'createdAt' | 'updatedAt'>)
    } else if (isEditing) {
      updatePreset(isEditing, editingPreset)
    }

    setEditingPreset(null)
    setIsCreating(false)
    setIsEditing(null)
  }, [editingPreset, isCreating, isEditing, createPreset, updatePreset])

  const handleCancelEdit = useCallback(() => {
    setEditingPreset(null)
    setIsCreating(false)
    setIsEditing(null)
  }, [])

  const handleDeletePreset = useCallback((id: string) => {
    if (confirm('Are you sure you want to delete this preset?')) {
      deletePreset(id)
    }
  }, [deletePreset])

  const handleSelectFolders = useCallback(() => {
    // Calculate position for folder dialog
    const button = document.querySelector('[data-folder-button]') as HTMLElement
    if (button) {
      const rect = button.getBoundingClientRect()
      setFolderDialogPosition({
        top: rect.bottom + window.scrollY + 10,
        left: rect.left + window.scrollX
      })
    }
    setShowFolderDialog(true)
  }, [])

  const handleFolderSelect = useCallback((folder: PlannerFile) => {
    if (!editingPreset) return

    // Check if folder is already selected
    const isAlreadySelected = editingPreset.folders?.some(f => f.filepath === folder.filepath)
    
    if (!isAlreadySelected) {
      setEditingPreset(prev => ({
        ...prev,
        folders: [...(prev?.folders || []), folder]
      }))
    }

    setShowFolderDialog(false)
  }, [editingPreset])

  const handleRemoveFolder = useCallback((folderPath: string) => {
    if (!editingPreset) return

    setEditingPreset(prev => ({
      ...prev,
      folders: prev?.folders?.filter(f => f.filepath !== folderPath) || []
    }))
  }, [editingPreset])

  const handlePresetClick = useCallback((preset: FolderPreset) => {
    selectPreset(preset.id)
    onPresetSelect?.(preset)
  }, [selectPreset, onPresetSelect])

  return (
    <div className={`space-y-4 ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-foreground">
          {category === 'workflow' ? 'Workflow' : 'Deep Search'} Folder Presets
        </h3>
        <button
          onClick={handleCreatePreset}
          className="flex items-center gap-2 px-3 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 transition-colors"
        >
          <Plus className="w-4 h-4" />
          New Preset
        </button>
      </div>

      {/* Preset List */}
      <div className="space-y-2">
        {categoryPresets.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            <Folder className="w-12 h-12 mx-auto mb-4 opacity-50" />
            <p>No presets created yet</p>
            <p className="text-sm">Create your first preset to get started</p>
          </div>
        ) : (
          categoryPresets.map((preset: FolderPreset) => (
            <div
              key={preset.id}
              className={`p-4 border border-border rounded-lg transition-colors ${
                selectedPreset?.id === preset.id
                  ? 'bg-primary/10 border-primary'
                  : 'hover:bg-secondary/50'
              }`}
            >
              <div className="flex items-start justify-between">
                <div className="flex-1 min-w-0">
                  <h4 className="font-medium text-foreground truncate">{preset.name}</h4>
                  {preset.description && (
                    <p className="text-sm text-muted-foreground mt-1">{preset.description}</p>
                  )}
                  <div className="flex items-center gap-2 mt-2">
                    <span className="text-xs text-muted-foreground">
                      {preset.folders.length} folder{preset.folders.length !== 1 ? 's' : ''}
                    </span>
                    <span className="text-xs text-muted-foreground">•</span>
                    <span className="text-xs text-muted-foreground">
                      Updated {preset.updatedAt.toLocaleDateString()}
                    </span>
                  </div>
                </div>
                <div className="flex items-center gap-2 ml-4">
                  <button
                    onClick={() => handlePresetClick(preset)}
                    className="p-2 text-muted-foreground hover:text-foreground transition-colors"
                    title="Select preset"
                  >
                    <Folder className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => handleEditPreset(preset)}
                    className="p-2 text-muted-foreground hover:text-foreground transition-colors"
                    title="Edit preset"
                  >
                    <Edit className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => handleDeletePreset(preset.id)}
                    className="p-2 text-muted-foreground hover:text-destructive transition-colors"
                    title="Delete preset"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            </div>
          ))
        )}
      </div>

      {/* Edit/Create Form */}
      {(isCreating || isEditing) && editingPreset && (
        <div className="p-4 border border-border rounded-lg bg-secondary/50">
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-foreground mb-2">
                Preset Name
              </label>
              <input
                type="text"
                value={editingPreset.name || ''}
                onChange={(e) => setEditingPreset(prev => ({ ...prev, name: e.target.value }))}
                className="w-full px-3 py-2 border border-border rounded-md bg-background text-foreground focus:outline-none focus:ring-2 focus:ring-primary"
                placeholder="Enter preset name..."
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-foreground mb-2">
                Description (Optional)
              </label>
              <textarea
                value={editingPreset.description || ''}
                onChange={(e) => setEditingPreset(prev => ({ ...prev, description: e.target.value }))}
                className="w-full px-3 py-2 border border-border rounded-md bg-background text-foreground focus:outline-none focus:ring-2 focus:ring-primary"
                placeholder="Enter description..."
                rows={2}
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-foreground mb-2">
                Selected Folders
              </label>
              <div className="space-y-2">
                {editingPreset.folders?.map((folder) => (
                  <div
                    key={folder.filepath}
                    className="flex items-center justify-between p-2 bg-background border border-border rounded-md"
                  >
                    <div className="flex items-center gap-2">
                      <Folder className="w-4 h-4 text-primary" />
                      <span className="text-sm text-foreground">{folder.filepath}</span>
                    </div>
                    <button
                      onClick={() => handleRemoveFolder(folder.filepath)}
                      className="p-1 text-muted-foreground hover:text-destructive transition-colors"
                    >
                      <X className="w-4 h-4" />
                    </button>
                  </div>
                ))}
                <button
                  data-folder-button
                  onClick={handleSelectFolders}
                  className="w-full p-3 border-2 border-dashed border-border rounded-md text-muted-foreground hover:text-foreground hover:border-primary transition-colors"
                >
                  <div className="flex items-center justify-center gap-2">
                    <Plus className="w-4 h-4" />
                    <span>Add Folders</span>
                  </div>
                </button>
              </div>
            </div>

            <div className="flex items-center gap-2">
              <button
                onClick={handleSavePreset}
                disabled={!editingPreset.name?.trim()}
                className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              >
                <Save className="w-4 h-4" />
                {isCreating ? 'Create Preset' : 'Save Changes'}
              </button>
              <button
                onClick={handleCancelEdit}
                className="flex items-center gap-2 px-4 py-2 border border-border text-foreground rounded-md hover:bg-secondary transition-colors"
              >
                <X className="w-4 h-4" />
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Folder Selection Dialog */}
      <FolderSelectionDialog
        isOpen={showFolderDialog}
        onClose={() => setShowFolderDialog(false)}
        onSelectFolder={handleFolderSelect}
        searchQuery=""
        position={folderDialogPosition}
      />
    </div>
  )
}

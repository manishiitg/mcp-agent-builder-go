import React, { useState, useCallback } from 'react'
import { ChevronDown, Folder, Plus } from 'lucide-react'
import { usePresetStore, type FolderPreset } from '../stores/usePresetStore'

interface PresetSelectorProps {
  category: 'workflow' | 'orchestrator'
  selectedPreset: FolderPreset | null
  onPresetSelect: (preset: FolderPreset | null) => void
  onCreatePreset?: () => void
  className?: string
  placeholder?: string
}

export const PresetSelector: React.FC<PresetSelectorProps> = ({
  category,
  selectedPreset,
  onPresetSelect,
  onCreatePreset,
  className = '',
  placeholder = 'Select a preset...'
}) => {
  const { getPresetsByCategory } = usePresetStore()
  const [isOpen, setIsOpen] = useState(false)

  const presets = getPresetsByCategory(category)

  const handlePresetClick = useCallback((preset: FolderPreset) => {
    onPresetSelect(preset)
    setIsOpen(false)
  }, [onPresetSelect])

  const handleClearSelection = useCallback(() => {
    onPresetSelect(null)
    setIsOpen(false)
  }, [onPresetSelect])

  const handleCreateClick = useCallback(() => {
    onCreatePreset?.()
    setIsOpen(false)
  }, [onCreatePreset])

  return (
    <div className={`relative ${className}`}>
      {/* Selector Button */}
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="w-full flex items-center justify-between px-3 py-2 border border-border rounded-md bg-background text-foreground hover:bg-secondary transition-colors"
      >
        <div className="flex items-center gap-2 min-w-0 flex-1">
          {selectedPreset ? (
            <>
              <Folder className="w-4 h-4 text-primary flex-shrink-0" />
              <span className="truncate">{selectedPreset.name}</span>
              <span className="text-xs text-muted-foreground">
                ({selectedPreset.folders.length} folder{selectedPreset.folders.length !== 1 ? 's' : ''})
              </span>
            </>
          ) : (
            <span className="text-muted-foreground">{placeholder}</span>
          )}
        </div>
        <ChevronDown className={`w-4 h-4 text-muted-foreground transition-transform ${isOpen ? 'rotate-180' : ''}`} />
      </button>

      {/* Dropdown */}
      {isOpen && (
        <div className="absolute top-full left-0 right-0 mt-1 bg-background border border-border rounded-md shadow-lg z-50 max-h-64 overflow-y-auto">
          {/* Clear Selection */}
          <button
            onClick={handleClearSelection}
            className="w-full px-3 py-2 text-left text-sm text-muted-foreground hover:bg-secondary transition-colors"
          >
            Clear Selection
          </button>

          {/* Divider */}
          <div className="border-t border-border" />

          {/* Preset List */}
          {presets.length === 0 ? (
            <div className="px-3 py-4 text-center text-muted-foreground text-sm">
              No presets available
            </div>
          ) : (
            presets.map((preset) => (
              <button
                key={preset.id}
                onClick={() => handlePresetClick(preset)}
                className={`w-full px-3 py-2 text-left text-sm transition-colors ${
                  selectedPreset?.id === preset.id
                    ? 'bg-primary/10 text-primary'
                    : 'text-foreground hover:bg-secondary'
                }`}
              >
                <div className="flex items-center gap-2">
                  <Folder className="w-4 h-4 text-primary flex-shrink-0" />
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium">{preset.name}</div>
                    {preset.description && (
                      <div className="truncate text-xs text-muted-foreground">
                        {preset.description}
                      </div>
                    )}
                    <div className="text-xs text-muted-foreground">
                      {preset.folders.length} folder{preset.folders.length !== 1 ? 's' : ''}
                    </div>
                  </div>
                </div>
              </button>
            ))
          )}

          {/* Create New Preset */}
          {onCreatePreset && (
            <>
              <div className="border-t border-border" />
              <button
                onClick={handleCreateClick}
                className="w-full px-3 py-2 text-left text-sm text-primary hover:bg-primary/10 transition-colors"
              >
                <div className="flex items-center gap-2">
                  <Plus className="w-4 h-4" />
                  <span>Create New Preset</span>
                </div>
              </button>
            </>
          )}
        </div>
      )}

      {/* Overlay to close dropdown */}
      {isOpen && (
        <div
          className="fixed inset-0 z-40"
          onClick={() => setIsOpen(false)}
        />
      )}
    </div>
  )
}

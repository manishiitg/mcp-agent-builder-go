import { useEffect, useRef, useState } from 'react'
import { Check, ChevronDown, ChevronRight, RotateCcw, Sparkles, Zap } from 'lucide-react'

export type ModelChoice = { id: string; label: string }
export type EngineGroup = { id: string; label: string; models: ModelChoice[] }
export type ReasoningLevel = { id: string; label: string }

interface ModelReasoningControlProps {
  engines: EngineGroup[]
  currentEngineId: string
  currentModelId: string
  /** False once the chat has a turn: a Codex thread and a Claude Code session are separate CLI state, so the engine itself is fixed from here on. */
  engineChangeable: boolean
  reasoningLevels: ReasoningLevel[]
  currentReasoningEffort: string
  defaultReasoningEffort?: string
  onSelect: (engineId: string, modelId: string, reasoningEffort?: string) => void
  disabled?: boolean
}

/**
 * One combined "which model, how hard does it think" control: a pill
 * showing the current model, opening a panel with the model list (and, on a
 * fresh chat, the engine it belongs to) plus a discrete reasoning-effort
 * slider when the engine declares one — the same shape as Codex's and
 * ChatGPT's own picker. Replaces two separate dropdowns sitting side by
 * side: one trigger, one panel, one mental model.
 */
export default function ModelReasoningControl({
  engines, currentEngineId, currentModelId, engineChangeable,
  reasoningLevels, currentReasoningEffort, defaultReasoningEffort,
  onSelect, disabled,
}: ModelReasoningControlProps) {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onMouseDown = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) setOpen(false)
    }
    const onKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape') setOpen(false) }
    document.addEventListener('mousedown', onMouseDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('mousedown', onMouseDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  const currentEngine = engines.find((e) => e.id === currentEngineId) ?? engines[0]
  if (!currentEngine) return null
  const currentModel = currentEngine.models.find((m) => m.id === currentModelId) ?? currentEngine.models[0]
  const visibleEngines = engineChangeable ? engines : engines.filter((e) => e.id === currentEngine.id)
  const levelIndex = Math.max(0, reasoningLevels.findIndex((l) => l.id === currentReasoningEffort))
  const currentLevel = reasoningLevels[reasoningLevels.some((l) => l.id === currentReasoningEffort) ? levelIndex : Math.floor((reasoningLevels.length - 1) / 2)]

  return (
    <div ref={containerRef} className="relative inline-block">
      <button
        type="button"
        className="model-reasoning-trigger h-7 max-w-[11rem] rounded-md border border-border bg-transparent px-2 text-xs text-muted-foreground hover:text-foreground disabled:opacity-50 inline-flex items-center gap-1"
        aria-label="Model and reasoning effort"
        title={engineChangeable ? 'Which AI answers this chat, and how hard it thinks' : `This chat runs on ${currentEngine.label}. Start a new chat to switch.`}
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
      >
        <Sparkles className="w-3 h-3 shrink-0" />
        <span className="truncate">{currentModel?.label ?? currentEngine.label}</span>
        <ChevronDown className="w-3 h-3 shrink-0" />
      </button>
      {open && (
        <div className="model-reasoning-panel absolute bottom-full right-0 mb-2 w-64 rounded-xl border border-border bg-popover p-3 shadow-lg z-50 max-h-[70vh] overflow-y-auto">
          {visibleEngines.length > 1 && (
            <div className="grid grid-cols-2 gap-1 mb-2">
              {visibleEngines.map((engine) => (
                <button
                  key={engine.id}
                  type="button"
                  className={`rounded-md px-2 py-1 text-xs border ${engine.id === currentEngine.id ? 'border-blue-500 text-blue-500 bg-blue-500/10' : 'border-border text-muted-foreground hover:text-foreground'}`}
                  onClick={() => { const model = engine.models[0]?.id ?? ''; onSelect(engine.id, model, currentReasoningEffort || undefined) }}
                >
                  {engine.label}
                </button>
              ))}
            </div>
          )}
          {!engineChangeable && (
            <div className="text-[11px] text-muted-foreground mb-2">Running on {currentEngine.label} — start a new chat to switch.</div>
          )}
          <div className="text-[11px] font-medium text-muted-foreground mb-1">Model</div>
          <div className="flex flex-col gap-0.5 mb-2">
            {currentEngine.models.map((model) => (
              <button
                key={model.id}
                type="button"
                className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent text-left"
                onClick={() => onSelect(currentEngine.id, model.id, currentReasoningEffort || undefined)}
              >
                <span className="truncate">{model.label}</span>
                {model.id === currentModel?.id && <Check className="w-3.5 h-3.5 text-blue-500 shrink-0" />}
              </button>
            ))}
          </div>
          {reasoningLevels.length > 0 && currentLevel && (
            <>
              <div className="h-px bg-border my-2" />
              <div className="flex items-center justify-between mb-2">
                <span className="inline-flex items-center gap-1 text-sm font-medium text-blue-500">
                  <Zap className="w-3.5 h-3.5" />
                  {currentLevel.label}
                  <ChevronRight className="w-3.5 h-3.5 text-muted-foreground" />
                </span>
                {defaultReasoningEffort && defaultReasoningEffort !== currentLevel.id && (
                  <button
                    type="button"
                    className="text-muted-foreground hover:text-foreground"
                    aria-label="Reset to default"
                    title="Reset to default"
                    onClick={() => onSelect(currentEngine.id, currentModel?.id ?? '', defaultReasoningEffort)}
                  >
                    <RotateCcw className="w-3.5 h-3.5" />
                  </button>
                )}
              </div>
              <input
                type="range"
                className="reasoning-effort-slider w-full"
                min={0}
                max={reasoningLevels.length - 1}
                step={1}
                value={levelIndex}
                style={{ ['--reasoning-effort-fill' as string]: `${(levelIndex / Math.max(1, reasoningLevels.length - 1)) * 100}%` }}
                onChange={(e) => onSelect(currentEngine.id, currentModel?.id ?? '', reasoningLevels[Number(e.target.value)]?.id ?? currentLevel.id)}
              />
              <div className="flex justify-between text-[11px] text-muted-foreground mt-1">
                {reasoningLevels.map((l) => <span key={l.id}>{l.label}</span>)}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}

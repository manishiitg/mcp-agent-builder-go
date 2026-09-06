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

// Complete, literal class strings — Tailwind's scanner only generates CSS
// for a class name that appears unbroken in source; building one out of
// concatenated fragments (e.g. `hover:${sharedTextClass}`) silently emits
// no rule at all. Explicit arbitrary colors rather than the ambient
// bg-popover/text-foreground tokens: those follow the app-wide theme class
// on <html>, which a product surface's own chat area does not actually
// follow for its own elements (its CSS renders light regardless via its own
// variables) — so a genuinely-dark <html> class produced a black-on-black
// panel over an otherwise light chat. Same class of bug as MicButton's
// white-on-white banner earlier in this product.
const PANEL = 'model-reasoning-panel absolute bottom-full left-0 mb-2 w-64 rounded-xl border border-[#d7e0ec] dark:border-[#2a2d33] bg-white dark:bg-[#121418] p-3 shadow-lg z-50 max-h-[70vh] overflow-y-auto'
const TRIGGER = 'model-reasoning-trigger h-7 max-w-[11rem] rounded-md border border-[#d7e0ec] dark:border-[#2a2d33] bg-transparent px-2 text-xs text-[#5f708d] dark:text-[#a2a7b0] hover:text-[#1f2937] dark:hover:text-[#e4e7ec] disabled:opacity-50 inline-flex items-center gap-1'
const LABEL = 'text-[11px] font-medium text-[#5f708d] dark:text-[#a2a7b0] mb-1'
const NOTE = 'text-[11px] text-[#5f708d] dark:text-[#a2a7b0] mb-2'
const MODEL_ROW = 'flex items-center justify-between rounded-md px-2 py-1.5 text-sm text-[#1f2937] dark:text-[#e4e7ec] hover:bg-[#f3f6fb] dark:hover:bg-[#1c1f24] text-left'
const ENGINE_PILL_ON = 'rounded-md px-2 py-1 text-xs border border-blue-500 text-blue-500 bg-blue-500/10'
const ENGINE_PILL_OFF = 'rounded-md px-2 py-1 text-xs border border-[#d7e0ec] dark:border-[#2a2d33] text-[#5f708d] dark:text-[#a2a7b0] hover:text-[#1f2937] dark:hover:text-[#e4e7ec]'
const RESET_BTN = 'text-[#5f708d] dark:text-[#a2a7b0] hover:text-[#1f2937] dark:hover:text-[#e4e7ec]'
const LEVEL_LABELS = 'flex justify-between text-[11px] text-[#5f708d] dark:text-[#a2a7b0] mt-1'
const DIVIDER = 'h-px bg-[#d7e0ec] dark:bg-[#2a2d33] my-2'

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
        className={TRIGGER}
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
        <div className={PANEL}>
          {visibleEngines.length > 1 && (
            <div className="grid grid-cols-2 gap-1 mb-2">
              {visibleEngines.map((engine) => (
                <button
                  key={engine.id}
                  type="button"
                  className={engine.id === currentEngine.id ? ENGINE_PILL_ON : ENGINE_PILL_OFF}
                  onClick={() => { const model = engine.models[0]?.id ?? ''; onSelect(engine.id, model, currentReasoningEffort || undefined) }}
                >
                  {engine.label}
                </button>
              ))}
            </div>
          )}
          {!engineChangeable && (
            <div className={NOTE}>Running on {currentEngine.label} — start a new chat to switch.</div>
          )}
          <div className={LABEL}>Model</div>
          <div className="flex flex-col gap-0.5 mb-2">
            {currentEngine.models.map((model) => (
              <button
                key={model.id}
                type="button"
                className={MODEL_ROW}
                onClick={() => onSelect(currentEngine.id, model.id, currentReasoningEffort || undefined)}
              >
                <span className="truncate">{model.label}</span>
                {model.id === currentModel?.id && <Check className="w-3.5 h-3.5 text-blue-500 shrink-0" />}
              </button>
            ))}
          </div>
          {reasoningLevels.length > 0 && currentLevel && (
            <>
              <div className={DIVIDER} />
              <div className="flex items-center justify-between mb-2">
                <span className="inline-flex items-center gap-1 text-sm font-medium text-blue-500">
                  <Zap className="w-3.5 h-3.5" />
                  {currentLevel.label}
                  <ChevronRight className="w-3.5 h-3.5 text-[#5f708d] dark:text-[#a2a7b0]" />
                </span>
                {defaultReasoningEffort && defaultReasoningEffort !== currentLevel.id && (
                  <button
                    type="button"
                    className={RESET_BTN}
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
              <div className={LEVEL_LABELS}>
                {reasoningLevels.map((l) => <span key={l.id}>{l.label}</span>)}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}

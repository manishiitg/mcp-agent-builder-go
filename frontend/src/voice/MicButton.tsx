import { Mic, Loader2 } from 'lucide-react'
import { useVoiceDictation } from './useVoiceDictation'

interface MicButtonProps {
  profileId: string
  onText: (text: string, final: boolean) => void
  /** Fired once, synchronously, when the user starts a NEW dictation session
   * (before mic permission is requested) — the caller's cue to reset whatever
   * "text already in the composer" baseline it diffs partials against. */
  onDictationStart?: () => void
  disabled?: boolean
}

/**
 * Push-to-talk mic control for a product composer. Only mounted when the
 * agent profile declares the shared voice capability (agentprofiles.
 * RuntimeCapabilities.Voice) — see loadAgentProfileCapabilityEnabled and its
 * call site in ChatInput.
 *
 * Ported from SparkQuill's MicButton (learning-app/src/voice/MicButton.tsx):
 * a small icon-only toggle looked identical whether it was recording,
 * connecting, or had silently failed, which is what a live test showed —
 * clicks toggling on and off within a second, because nothing confirmed
 * "yes, this is listening" strongly enough to stop a worried re-click. The
 * banner and level ring are that confirmation, shown the instant the click
 * happens, not after the first transcript arrives.
 */
export function MicButton({ profileId, onText, onDictationStart, disabled }: MicButtonProps) {
  const { state, error, level, liveText, start, stop } = useVoiceDictation(profileId, onText, onDictationStart)
  const listening = state === 'listening'
  const starting = state === 'starting'
  const busy = listening || starting

  const label = starting
    ? 'Getting voice ready…'
    : listening
      ? 'Stop dictation'
      : error || 'Start dictation'

  return (
    <span className="relative inline-flex items-center">
      <button
        type="button"
        onClick={() => (busy ? stop() : start())}
        disabled={disabled}
        title={label}
        aria-label={label}
        data-testid="chat-input-mic-button"
        data-voice-state={state}
        className={`relative inline-flex h-7 w-7 items-center justify-center rounded-md transition-colors ${
          listening
            ? 'bg-red-500/15 text-red-500 dark:text-red-400'
            : 'text-slate-500 hover:bg-slate-200/60 dark:text-slate-400 dark:hover:bg-slate-700/60'
        } ${disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
      >
        {starting ? (
          <Loader2 className="h-4 w-4 animate-spin" />
        ) : (
          <Mic className="h-4 w-4" />
        )}
        {listening && (
          // Scales with LIVE input level, not a fixed pulse — a silent or
          // wrong input device shows a dead ring instead of silently
          // recording nothing, which a live test proved is otherwise
          // invisible: the connection can stay open for a minute receiving
          // real audio CHUNKS that are pure silence, and nothing else in this
          // UI would ever reveal that.
          <span
            aria-hidden="true"
            className="pointer-events-none absolute inset-0 rounded-md border-2 border-red-400"
            style={{ transform: `scale(${1 + level * 0.6})`, opacity: 0.3 + level * 0.5 }}
          />
        )}
      </button>
      {(starting || listening) && (
        <span
          role="status"
          className="absolute left-full top-1/2 z-10 ml-2 flex -translate-y-1/2 items-center gap-1.5 whitespace-nowrap rounded-full border border-slate-700 bg-slate-900 px-2.5 py-1 text-xs text-slate-200 shadow-lg"
        >
          {starting ? (
            <>
              <Loader2 className="h-3 w-3 shrink-0 animate-spin text-violet-400" />
              <span>Getting voice ready — this can take a moment the first time…</span>
            </>
          ) : (
            <>
              <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-red-500" style={{ opacity: 0.5 + level * 0.5 }} />
              <span className="font-medium text-red-400">Listening</span>
              <span className="max-w-[220px] truncate text-slate-400">
                {liveText || 'Go ahead — start talking'}
              </span>
            </>
          )}
        </span>
      )}
      {error && !busy && (
        <span role="status" className="absolute left-full top-1/2 z-10 ml-2 -translate-y-1/2 whitespace-nowrap rounded-full border border-red-900/60 bg-red-950/80 px-2.5 py-1 text-xs text-red-300">
          {error}
        </span>
      )}
    </span>
  )
}

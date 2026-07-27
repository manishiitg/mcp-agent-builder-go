import { useEffect } from 'react'
import { Mic, Square, Loader2 } from 'lucide-react'
import { useMicDictation } from './useMicDictation'

/**
 * Mic button for a composer. Same control in parent and child chat, so it's
 * obvious that talking works everywhere — not only for WhatsApp voice notes.
 *
 * The ring around the button scales with live mic level, so "it's listening"
 * is visibly true rather than just asserted; a muted or wrong input device
 * shows a dead ring instead of silently recording nothing.
 *
 * Keyboard: the shortcut is owned here rather than by each composer, so both
 * chats get it automatically and can't drift apart.
 */
export function MicButton({
  onText,
  disabled,
  shortcutEnabled = true,
}: {
  onText: (text: string) => void
  disabled?: boolean
  /** Only the visible composer should own the global shortcut. */
  shortcutEnabled?: boolean
}) {
  const { state, level, liveText, error, toggle, clearError } = useMicDictation(onText)

  // Cmd/Ctrl+Shift+M — deliberately not a bare key: a child typing an answer
  // must never trigger recording by accident.
  useEffect(() => {
    if (!shortcutEnabled || disabled) return
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'm' || e.key === 'M')) {
        e.preventDefault()
        toggle()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [shortcutEnabled, disabled, toggle])

  const busy = state === 'transcribing'
  const label = state === 'recording'
    ? 'Stop recording'
    : busy
      ? 'Transcribing…'
      : 'Speak your message (⌘⇧M)'

  return (
    <span className="fl-mic-wrap">
      <button
        className={`composer-icon fl-mic-btn${state === 'recording' ? ' is-recording' : ''}`}
        type="button"
        aria-label={label}
        title={label}
        disabled={disabled || busy}
        onClick={toggle}
      >
        {busy
          ? <Loader2 size={19} className="fl-mic-spin" />
          : state === 'recording'
            ? <Square size={17} />
            : <Mic size={19} />}
        {state === 'recording' && (
          // Scales with live input level — see useMicDictation's `level`.
          <span className="fl-mic-level" style={{ transform: `scale(${1 + level * 0.7})` }} aria-hidden="true" />
        )}
      </button>
      {state === 'recording' && (
        // A full-width banner above the WHOLE composer (see .fl-mic-live-banner —
        // anchored to .fl-composer, not this small wrap), not a tooltip easy to
        // miss. Shows "Listening" the INSTANT recording starts — before any text
        // has ever come back — so there's no uncertain gap where a parent has to
        // guess whether it's safe to start talking yet.
        <span className="fl-mic-live-banner" role="status">
          <span className="fl-mic-live-dot" aria-hidden="true" />
          <span className="fl-mic-live-label">Listening</span>
          <span className={`fl-mic-live-text${liveText ? '' : ' is-waiting'}`}>
            {liveText || 'Go ahead — start talking'}
          </span>
        </span>
      )}
      {error && (
        <span className="fl-mic-error" role="status" onClick={clearError} title="Dismiss">{error}</span>
      )}
    </span>
  )
}

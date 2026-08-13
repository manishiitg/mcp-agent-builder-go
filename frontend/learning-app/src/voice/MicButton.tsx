import { forwardRef, useEffect, useImperativeHandle } from 'react'
import { Mic, Square, Loader2 } from 'lucide-react'
import { useMicDictation, type MicState } from './useMicDictation'

export type MicButtonHandle = {
  /** No-op unless currently recording — see useMicDictation's stopAndSubmit. */
  stopAndSubmit: () => void
}

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
export const MicButton = forwardRef(function MicButton({
  onText,
  disabled,
  shortcutEnabled = true,
  onStateChange,
}: {
  onText: (text: string, autoSubmit?: boolean) => void
  disabled?: boolean
  /** Only the visible composer should own the global shortcut. */
  shortcutEnabled?: boolean
  /** Lets the composer know when Enter should stop+submit instead of send. */
  onStateChange?: (state: MicState) => void
}, ref: React.ForwardedRef<MicButtonHandle>) {
  const { state, level, liveText, warmingUp, error, toggle, stopAndSubmit, clearError } = useMicDictation(onText)

  useEffect(() => { onStateChange?.(state) }, [state, onStateChange])
  useImperativeHandle(ref, () => ({ stopAndSubmit }), [stopAndSubmit])

  // Cmd/Ctrl+Shift+M — the explicit, discoverable shortcut.
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

  // TAP Option (Alt) on its own to start, tap it again to stop and send — the
  // one-key dictation toggle, close to hand while typing.
  //
  // A bare modifier was deliberately avoided for the shortcut above, for a
  // reason that still holds: this is the child's composer too, and recording
  // that starts itself while she is typing an answer is worse than no
  // shortcut. So "tap" here is strict, and both conditions have to hold:
  //
  //   1. NOTHING ELSE was pressed while Option was down. Option is macOS's
  //      dead-key modifier (⌥e → é, ⌥n → ñ), so any Option+key combination is
  //      someone typing a character, never reaching for the mic. One other
  //      key and this press is disqualified outright.
  //   2. It was RELEASED QUICKLY (under half a second). Holding Option is what
  //      people do while hunting for the second key of a combination, or
  //      resting a hand — a deliberate tap is short.
  //
  // Auto-repeat (holding Option fires keydown over and over) is ignored via
  // e.repeat, which would otherwise flip the mic on and off continuously.
  useEffect(() => {
    if (!shortcutEnabled || disabled) return
    let downAt = 0
    let soloTap = false
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Alt') {
        if (e.repeat) return
        downAt = Date.now()
        soloTap = true
        return
      }
      soloTap = false // some other key joined it — this is a combination
    }
    const onKeyUp = (e: KeyboardEvent) => {
      if (e.key !== 'Alt') return
      const wasTap = soloTap && Date.now() - downAt < 500
      soloTap = false
      if (wasTap) toggle()
    }
    // Any click/blur mid-press also disqualifies it: releasing Option after
    // switching focus or windows is not a tap aimed at the mic.
    const cancel = () => { soloTap = false }
    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('keyup', onKeyUp)
    window.addEventListener('mousedown', cancel)
    window.addEventListener('blur', cancel)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('keyup', onKeyUp)
      window.removeEventListener('mousedown', cancel)
      window.removeEventListener('blur', cancel)
    }
  }, [shortcutEnabled, disabled, toggle])

  // Stopping the mic sends. Dictating and then pressing Enter was two actions
  // for one intent — reported as "99% of the time I never change anything" —
  // and the transcript the composer receives is the accurate batch pass, not
  // the rough live preview. A wrong one is cheap to correct with another
  // message; an extra keypress on every single dictation is not.
  const stopOrStart = () => {
    if (state === 'recording') stopAndSubmit()
    else toggle()
  }

  const preparing = state === 'preparing'
  // Both non-interactive states show a spinner and refuse clicks. 'preparing'
  // matters most: on a cold start it can last seconds while the model loads,
  // and leaving the button looking idle invites repeat clicks.
  const busy = state === 'transcribing' || preparing
  const label = state === 'recording'
    ? 'Stop and send (tap ⌥)'
    : preparing
      ? 'Getting voice ready…'
      : state === 'transcribing'
        ? 'Transcribing…'
        : 'Speak your message (tap ⌥, or ⌘⇧M)'

  return (
    <span className="fl-mic-wrap">
      <button
        className={`composer-icon fl-mic-btn${state === 'recording' ? ' is-recording' : ''}`}
        type="button"
        aria-label={label}
        title={label}
        disabled={disabled || busy}
        onClick={stopOrStart}
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
      {preparing && (
        // Shown for the whole startup gap, which on a cold start is the model
        // loading (seconds, occasionally much longer on first ever use). The
        // absence of this was read as "nothing is working".
        <span className="fl-mic-live-banner" role="status">
          <Loader2 size={15} className="fl-mic-spin" aria-hidden="true" />
          <span className="fl-mic-live-label">Starting</span>
          <span className="fl-mic-live-text is-waiting">
            Getting voice ready — this can take a moment the first time…
          </span>
        </span>
      )}
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
            {liveText || (warmingUp ? 'Still warming up voice recognition…' : 'Go ahead — start talking')}
          </span>
        </span>
      )}
      {error && (
        <span className="fl-mic-error" role="status" onClick={clearError} title="Dismiss">{error}</span>
      )}
    </span>
  )
})

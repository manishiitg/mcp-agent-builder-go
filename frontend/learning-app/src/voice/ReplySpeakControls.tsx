import { Volume2, Square } from 'lucide-react'
import { persistAutoSpeak } from './speech'

/**
 * Read-aloud controls attached to one agent reply.
 *
 * The play/stop button sits on every reply. The "read every reply" toggle
 * appears ONLY on the newest one — it belongs next to the thing it affects
 * (you turn it on because you're listening to *this* reply and want the next
 * one too), not buried in a Settings panel you'd have to go hunting for.
 * Showing it on every historical reply would just be noise.
 *
 * Clicking the reply that's already playing stops it: with auto-read on,
 * speech starts without anyone pressing anything, so "make it stop" has to be
 * reachable in one click.
 */
export function ReplySpeakControls({
  speaking,
  isLatest,
  autoSpeak,
  onToggleSpeak,
  onAutoSpeakChange,
}: {
  speaking: boolean
  isLatest: boolean
  autoSpeak: boolean
  onToggleSpeak: () => void
  onAutoSpeakChange: (on: boolean) => void
}) {
  return (
    <div className="fl-speak-row">
      <button
        className={`fl-speak-btn${speaking ? ' is-speaking' : ''}`}
        type="button"
        aria-label={speaking ? 'Stop reading' : 'Read this out loud'}
        title={speaking ? 'Stop reading' : 'Read this out loud'}
        onClick={onToggleSpeak}
      >
        {speaking ? <Square size={13} /> : <Volume2 size={13} />}
        <span className="fl-speak-btn-label">{speaking ? 'Stop' : 'Listen'}</span>
      </button>
      {isLatest && (
        <label className="fl-speak-auto" title="Speak each new reply as soon as it's ready">
          <input
            type="checkbox"
            checked={autoSpeak}
            onChange={(e) => { onAutoSpeakChange(e.target.checked); persistAutoSpeak(e.target.checked) }}
          />
          <span>Read every reply</span>
        </label>
      )}
    </div>
  )
}

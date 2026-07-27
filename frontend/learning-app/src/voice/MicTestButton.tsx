import { useState } from 'react'
import { Mic, Square, Loader2 } from 'lucide-react'
import { useMicDictation } from './useMicDictation'

/**
 * "Try it" for speech-to-text, in Settings.
 *
 * The read-aloud tiers can be judged by pressing play; speech-to-text can't —
 * the only way to know whether a model actually understands YOUR family's
 * accents, names and background noise is to say something and read back what
 * it heard. So this records, transcribes, and shows the text verbatim, which
 * is also the honest way to justify a 1.5GB upgrade: hear the mistakes the
 * smaller model makes on your own voice first.
 */
export function MicTestButton({ tier }: { tier?: string }) {
  const [heard, setHeard] = useState<string | null>(null)
  const { state, level, error, toggle } = useMicDictation((text) => setHeard(text), tier)

  const recording = state === 'recording'
  const busy = state === 'transcribing'

  return (
    <div className="fl-mic-test">
      <button
        className={`fl-ghost-btn is-tiny${recording ? ' is-recording' : ''}`}
        type="button"
        disabled={busy}
        onClick={() => { setHeard(null); toggle() }}
      >
        {busy
          ? <><Loader2 size={12} className="fl-mic-spin" /> Working it out…</>
          : recording
            ? <><Square size={12} /> Stop</>
            : <><Mic size={12} /> Try it</>}
      </button>

      {recording && (
        <span className="fl-mic-test-live">
          <span className="fl-mic-test-bar" style={{ transform: `scaleX(${0.15 + level * 0.85})` }} />
          Say something…
        </span>
      )}
      {heard && <p className="fl-mic-test-heard">Heard: “{heard}”</p>}
      {error && <p className="fl-voice-tier-error">{error}</p>}
    </div>
  )
}

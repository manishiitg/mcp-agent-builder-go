import { useCallback, useRef, useState } from 'react'
import { FAMILY_API } from '../apiBase'
import { useVoiceDictation } from '../../../shared/voice/useVoiceDictation'

// 'preparing' covers the gap between clicking the mic and audio actually
// flowing — opening the device, and on a cold start loading (or on a first
// run downloading) the model. Without it that gap looked identical to
// "nothing happened", and every impatient re-click began ANOTHER session.
export type MicState = 'idle' | 'preparing' | 'recording' | 'transcribing'

/**
 * Mic dictation for a composer: record → transcribe on-device → hand back text.
 *
 * A thin adapter over the shared AgentWorks dictation hook
 * (frontend/shared/voice/useVoiceDictation.ts) so this app's composer keeps
 * its own state names, ⌥-tap / ⌘⇧M shortcuts and stop-and-send behaviour,
 * while the capture, transport and engine are exactly what AgentWorks uses:
 * raw 16kHz PCM over the /api/voice/stream WebSocket into pkg/voicestt.
 *
 * `liveText` is the running preview as the engine hears it, punctuated the
 * same way the committed text will be. `onText` fires ONCE per session, on
 * stop, with the whole utterance — the composer sets its value from it and
 * optionally submits (see stopAndSubmit).
 */
export function useMicDictation(onText: (text: string, autoSubmit?: boolean) => void) {
  const autoSubmitRef = useRef(false)
  const streamUrl = useCallback(() => `${FAMILY_API.replace(/^http/, 'ws')}/api/voice/stream`, [])
  const { state, error, level, transcript, start, stop, clearError } = useVoiceDictation({ streamUrl })

  const micState: MicState =
    state === 'starting' ? 'preparing'
      : state === 'listening' ? 'recording'
        : state === 'finishing' ? 'transcribing'
          : 'idle'

  const finish = useCallback(async () => {
    const autoSubmit = autoSubmitRef.current
    autoSubmitRef.current = false
    const text = (await stop()).trim()
    if (text) onText(text, autoSubmit)
  }, [stop, onText])

  const [setupError, setSetupError] = useState<string | null>(null)
  const toggle = useCallback(() => {
    if (micState === 'recording') { void finish(); return }
    if (micState !== 'idle') return
    setSetupError(null)
    // Nudge the engine awake in parallel with the mic permission prompt so a
    // cold start overlaps with the parent reaching for the button. The reply
    // also says whether the one-time model download has happened yet — if
    // not, send the parent to Settings → Voice (which shows progress) rather
    // than leaving the mic on "Getting voice ready" for a silent 690MB fetch.
    void (async () => {
      try {
        const res = await fetch(`${FAMILY_API}/api/voice/warm`, { method: 'POST' })
        const status = (await res.json()) as { available?: boolean; installed?: boolean; size_mb?: number }
        if (status.available === false) {
          setSetupError('Voice isn’t included in this build of SparkQuill.')
          return
        }
        if (!status.installed) {
          setSetupError(`Voice is downloading its one-time ~${status.size_mb ?? 690}MB model — watch progress in Settings → Voice, then try again.`)
          return
        }
      } catch {
        // The server will answer the same question when the stream opens.
      }
      void start()
    })()
  }, [micState, finish, start])

  const stopAndSubmit = useCallback(() => {
    if (micState !== 'recording') return
    autoSubmitRef.current = true
    void finish()
  }, [micState, finish])

  return {
    state: micState,
    level,
    liveText: transcript,
    warmingUp: false,
    error: setupError ?? (error ? friendlyError(error) : null),
    toggle,
    stopAndSubmit,
    clearError: () => { setSetupError(null); clearError() },
  }
}

function friendlyError(message: string): string {
  if (/not reachable|failed to connect/i.test(message)) return 'Voice isn’t available right now — is SparkQuill’s server running?'
  if (/Permission|NotAllowed|denied/i.test(message)) return 'Could not use the microphone. Check permission for this app.'
  return message
}

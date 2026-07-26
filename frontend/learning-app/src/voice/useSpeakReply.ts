import { useState } from 'react'
import { speakText, stopSpeech, currentSpeechAudio } from './speech'

/**
 * Read-aloud control for a list of replies (parent chat or child tutor).
 *
 * Returns which message index is currently being spoken plus a toggle:
 * clicking the reply that's already playing STOPS it, so the same button
 * doubles as "make it stop" — which matters, because with auto-read-aloud on,
 * speech can start without anyone pressing anything.
 *
 * One instance per thread; the underlying player (speech.ts) is module-global,
 * so starting a new utterance anywhere cancels whatever was playing.
 */
export function useSpeakReply() {
  const [speakingIdx, setSpeakingIdx] = useState<number | null>(null)

  const speakReply = (idx: number, text: string) => {
    if (speakingIdx === idx) {
      stopSpeech()
      setSpeakingIdx(null)
      return
    }
    setSpeakingIdx(idx)
    speakText(text)
      .then(() => {
        // speakText resolves when playback STARTS; clear the indicator only
        // once it actually finishes, or the button sits in "speaking" forever.
        const audio = currentSpeechAudio()
        if (!audio) { setSpeakingIdx(null); return }
        const clear = () => setSpeakingIdx((cur) => (cur === idx ? null : cur))
        audio.addEventListener('ended', clear, { once: true })
        audio.addEventListener('error', clear, { once: true })
      })
      .catch(() => setSpeakingIdx(null))
  }

  return { speakingIdx, speakReply }
}

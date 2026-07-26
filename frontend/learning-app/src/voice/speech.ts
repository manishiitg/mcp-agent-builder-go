// On-device text-to-speech: playback plumbing shared by the per-reply speaker
// button, the auto-read-aloud setting, and the Settings voice preview.
//
// Module-level (not per-component) state on purpose: a NEW utterance must
// cancel whichever one is still playing. Two replies talking over each other
// is worse than either alone, and that's exactly what auto-read produces on a
// fast follow-up turn.

import { FAMILY_API } from '../apiBase'

// Auto-read-aloud: speak each tutor reply the moment it finishes, without
// anyone pressing play. Persisted (a child who needs replies read aloud needs
// that every session, not once) and OFF by default — audio that starts on its
// own is intrusive if you didn't ask for it.
const AUTO_SPEAK_KEY = 'sparkquill.auto-speak'

export function readAutoSpeak(): boolean {
  try { return localStorage.getItem(AUTO_SPEAK_KEY) === '1' } catch { return false }
}

export function persistAutoSpeak(on: boolean) {
  try { localStorage.setItem(AUTO_SPEAK_KEY, on ? '1' : '0') } catch { /* best-effort */ }
}

let currentSpeech: HTMLAudioElement | null = null
let currentSpeechURL: string | null = null

/** The <audio> currently playing, if any — lets a caller await its `ended`. */
export function currentSpeechAudio(): HTMLAudioElement | null {
  return currentSpeech
}

export function stopSpeech() {
  if (currentSpeech) { currentSpeech.pause(); currentSpeech = null }
  if (currentSpeechURL) { URL.revokeObjectURL(currentSpeechURL); currentSpeechURL = null }
}

/**
 * Speak one reply. Resolves when playback STARTS (not when it finishes) —
 * callers that need completion should listen for `ended` on
 * currentSpeechAudio(). Rejects if synthesis or playback fails.
 *
 * `tier` forces a specific voice instead of the usual best-installed-wins,
 * so each tier's own sample button plays THAT voice — otherwise every sample
 * would sound identical, which defeats the point of comparing them.
 */
export async function speakText(text: string, tier?: string): Promise<void> {
  const clean = stripForSpeech(text)
  if (!clean) return
  stopSpeech()
  const res = await fetch(`${FAMILY_API}/api/voice/speak`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(tier ? { text: clean, tier } : { text: clean }),
  })
  if (!res.ok) throw new Error(`speak failed: ${res.status}`)
  const url = URL.createObjectURL(await res.blob())
  const audio = new Audio(url)
  currentSpeech = audio
  currentSpeechURL = url
  // Revoke on end AND on error — a blob URL leaks until revoked, and a
  // playback failure would otherwise strand it for the page's lifetime.
  const done = () => {
    if (currentSpeechURL === url) {
      URL.revokeObjectURL(url)
      currentSpeechURL = null
      currentSpeech = null
    }
  }
  audio.onended = done
  audio.onerror = done
  await audio.play()
}

/**
 * Turn a chat reply into something worth listening to: the markdown that makes
 * a reply readable ON SCREEN (**bold**, "- " bullets, `code`) is just noise
 * when a voice reads it out literally.
 */
export function stripForSpeech(text: string): string {
  return (text || '')
    .replace(/```[\s\S]*?```/g, ' ')          // fenced code blocks: unreadable aloud
    .replace(/`([^`]+)`/g, '$1')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/\*([^*]+)\*/g, '$1')
    .replace(/^\s*[-*]\s+/gm, '')
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')  // link text, not the URL
    .replace(/\s+/g, ' ')
    .trim()
}

// On-device text-to-speech: playback plumbing shared by the per-reply speaker
// button, the auto-read-aloud setting, and the Settings voice preview.
//
// Module-level (not per-component) state on purpose: a NEW utterance must
// cancel whichever one is still playing. Two replies talking over each other
// is worse than either alone, and that's exactly what auto-read produces on a
// fast follow-up turn.

import { FAMILY_API } from '../apiBase'

// Auto-read-aloud: speak each new reply the moment it finishes, without
// anyone pressing play. Persisted per THREAD, not globally — a parent who
// wants every reply in the child's tutor read aloud (a child who can't read
// well yet) has no reason to also want their own parent-chat replies read
// aloud automatically, and vice versa. The voice/model used to read aloud
// stays a single shared setting (Settings -> Voice); only whether it plays
// automatically is scoped per thread.
export type AutoSpeakScope = 'parent' | 'child'

function autoSpeakKey(scope: AutoSpeakScope): string {
  return `sparkquill.auto-speak.${scope}`
}

export function readAutoSpeak(scope: AutoSpeakScope): boolean {
  try { return localStorage.getItem(autoSpeakKey(scope)) === '1' } catch { return false }
}

export function persistAutoSpeak(scope: AutoSpeakScope, on: boolean) {
  try { localStorage.setItem(autoSpeakKey(scope), on ? '1' : '0') } catch { /* best-effort */ }
}

let currentSpeech: HTMLAudioElement | null = null
let currentSpeechURL: string | null = null
// Bumped by every stopSpeech() call (including the one at the START of every
// speakText()). Without this, two near-simultaneous speakText() calls (e.g.
// auto-read firing right as someone clicks "Listen") each call stopSpeech()
// BEFORE their own fetch — which does nothing, since neither has set
// currentSpeech yet — then both independently finish their fetch and both
// play, genuinely overlapping. Each call captures the generation right after
// its own stopSpeech(); if that number changed by the time its fetch
// resolves, a NEWER call has since started and this one discards itself
// instead of playing over it.
let speechGeneration = 0

/** The <audio> currently playing, if any — lets a caller await its `ended`. */
export function currentSpeechAudio(): HTMLAudioElement | null {
  return currentSpeech
}

export function stopSpeech() {
  speechGeneration++
  if (currentSpeech) { currentSpeech.pause(); currentSpeech = null }
  if (currentSpeechURL) { URL.revokeObjectURL(currentSpeechURL); currentSpeechURL = null }
}

/**
 * Speak one reply. Resolves when playback STARTS (not when it finishes) —
 * callers that need completion should listen for `ended` on
 * currentSpeechAudio(). Rejects if synthesis or playback fails. Silently
 * discards itself (no error, no audio) if a NEWER speakText()/stopSpeech()
 * call supersedes it while its request was in flight — see speechGeneration.
 *
 * `tier` forces a specific voice instead of the usual best-installed-wins,
 * so each tier's own sample button plays THAT voice — otherwise every sample
 * would sound identical, which defeats the point of comparing them.
 */
export async function speakText(text: string, tier?: string, voice?: string): Promise<void> {
  const clean = stripForSpeech(text)
  if (!clean) return
  stopSpeech()
  const myGeneration = speechGeneration
  const res = await fetch(`${FAMILY_API}/api/voice/speak`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      text: clean,
      ...(tier ? { tier } : {}),
      ...(voice ? { voice } : {}),
    }),
  })
  if (!res.ok) throw new Error(`speak failed: ${res.status}`)
  const blob = await res.blob()
  if (myGeneration !== speechGeneration) return // superseded while fetching — don't play over whatever replaced us
  const url = URL.createObjectURL(blob)
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

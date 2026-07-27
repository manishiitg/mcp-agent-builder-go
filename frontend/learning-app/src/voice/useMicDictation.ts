import { useRef, useState } from 'react'
import { FAMILY_API } from '../apiBase'

export type MicState = 'idle' | 'recording' | 'transcribing'

// How often to refresh the live preview while recording. Each tick
// re-transcribes the whole recording-so-far through the SAME one-shot
// Parakeet call used for the final result — not word-by-word streaming, but
// genuinely live: text appears while you're still talking, not only after
// you stop. Chosen over mlx-audio's own realtime WebSocket server (which
// exists and does true incremental streaming) after that server behaved
// unpredictably with Parakeet specifically in testing — this reuses code
// already proven correct in production instead of an unfamiliar protocol.
//
// 1200ms rather than the original 2500ms: with the models kept warm in a
// persistent worker (see voice_worker.go), a real call now takes ~1.2-1.5s
// instead of ~3.5-3.9s, so a shorter interval actually shortens the gap
// between updates instead of just polling uselessly often against a slower
// floor (the in-flight guard below still skips a tick if the previous
// request hasn't finished, so this can't pile up concurrent requests).
const LIVE_PREVIEW_INTERVAL_MS = 1200

// Auto-stop-on-silence, using the SAME live amplitude signal already driving
// the level meter — no new dependency, no raw-PCM streaming, no separate VAD
// model. This is deliberately simple amplitude thresholding, not a real
// speech classifier (like webrtcvad or Silero VAD): it can be fooled by
// steady background noise (a fan, a TV) reading as "still talking", or by a
// loud clatter reading as speech. That trade was made explicitly in favor of
// shipping something today with zero new dependencies; a real VAD would need
// either raw PCM streamed to Python or an in-browser WASM model — a bigger
// build than this.
const SPEECH_LEVEL_THRESHOLD = 0.16 // above this, "the parent is talking"
const SILENCE_LEVEL_THRESHOLD = 0.08 // below this, "quiet" — lower than the
// speech threshold on purpose, so hovering right at the boundary doesn't
// flicker between the two states tick to tick.
const SILENCE_STOP_MS = 1800 // sustained quiet, AFTER real speech began, before auto-stopping

/**
 * Mic dictation for a composer: record → transcribe on-device → hand back text.
 *
 * Uses MediaRecorder (identical in Electron's Chromium and a real browser, so
 * one implementation serves both). Whatever container the browser picks is
 * sent as-is — Parakeet reads it directly — so there's no codec negotiation
 * here.
 *
 * `level` is a live 0..1 mic amplitude, polled from an AnalyserNode. Without
 * it a recording UI is a lie: you can't tell "listening" from "mic is muted /
 * the wrong input is selected" until after you've already spoken. The same
 * signal also drives auto-stop-on-silence (see SILENCE_STOP_MS above).
 *
 * `liveText` is a running preview of what's been said so far, refreshed every
 * LIVE_PREVIEW_INTERVAL_MS. It can revise itself between ticks (more audio
 * context can change how the model reads an earlier word) — that's expected,
 * the same way any live-captioning UI's in-progress line can shift before it
 * settles. The FINAL result (via onText, on stop) always re-transcribes the
 * complete recording once more for the authoritative version.
 */
export function useMicDictation(onText: (text: string) => void, tier?: string) {
  const [state, setState] = useState<MicState>('idle')
  const [level, setLevel] = useState(0)
  const [liveText, setLiveText] = useState('')
  const [error, setError] = useState<string | null>(null)

  const recorderRef = useRef<MediaRecorder | null>(null)
  const chunksRef = useRef<Blob[]>([])
  const streamRef = useRef<MediaStream | null>(null)
  const audioCtxRef = useRef<AudioContext | null>(null)
  const rafRef = useRef<number | null>(null)
  const previewTimerRef = useRef<number | null>(null)
  const previewBusyRef = useRef(false)
  const hasSpokenRef = useRef(false)
  const silenceStartRef = useRef<number | null>(null)
  const stopRef = useRef<() => void>(() => {})

  // Everything the browser handed us has to be torn down explicitly: leaving
  // the MediaStream open keeps the OS mic indicator lit, which reads as "this
  // app is still listening to me" long after it stopped.
  const teardown = () => {
    if (rafRef.current !== null) { cancelAnimationFrame(rafRef.current); rafRef.current = null }
    if (previewTimerRef.current !== null) { window.clearInterval(previewTimerRef.current); previewTimerRef.current = null }
    streamRef.current?.getTracks().forEach((t) => t.stop())
    streamRef.current = null
    audioCtxRef.current?.close().catch(() => {})
    audioCtxRef.current = null
    setLevel(0)
    hasSpokenRef.current = false
    silenceStartRef.current = null
  }

  const runLivePreview = async (rec: MediaRecorder) => {
    if (previewBusyRef.current || chunksRef.current.length === 0) return
    previewBusyRef.current = true
    try {
      const ext = (rec.mimeType || '').includes('mp4') ? 'mp4' : 'webm'
      const blob = new Blob(chunksRef.current, { type: rec.mimeType || 'audio/webm' })
      const form = new FormData()
      form.append('audio', blob, `preview.${ext}`)
      const res = await fetch(`${FAMILY_API}/api/voice/transcribe`, { method: 'POST', body: form })
      if (!res.ok) return // A transient decode hiccup on a short clip resolves
      // itself once more audio accumulates — skip this tick rather than show
      // an error for what is only a preview.
      const data = await res.json()
      if (data.text?.trim()) setLiveText(data.text.trim())
    } catch {
      // Same reasoning: a preview tick failing silently is fine; the next
      // tick tries again with more audio.
    } finally {
      previewBusyRef.current = false
    }
  }

  const stop = () => {
    const rec = recorderRef.current
    if (rec && rec.state !== 'inactive') rec.stop() // onstop does the transcribe
    else { teardown(); setState('idle') }
  }
  stopRef.current = stop

  const start = async () => {
    setError(null)
    setLiveText('')
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      streamRef.current = stream

      // Live level meter — also drives auto-stop-on-silence, see the
      // constants above.
      const ctx = new AudioContext()
      audioCtxRef.current = ctx
      const analyser = ctx.createAnalyser()
      analyser.fftSize = 512
      ctx.createMediaStreamSource(stream).connect(analyser)
      const data = new Uint8Array(analyser.frequencyBinCount)
      const tick = () => {
        analyser.getByteTimeDomainData(data)
        // Peak deviation from silence (128), normalized — cheaper than RMS and
        // more responsive for a simple "is sound arriving" indicator.
        let peak = 0
        for (const v of data) peak = Math.max(peak, Math.abs(v - 128))
        const normalized = Math.min(1, peak / 96)
        setLevel(normalized)

        if (normalized >= SPEECH_LEVEL_THRESHOLD) {
          hasSpokenRef.current = true
          silenceStartRef.current = null
        } else if (normalized <= SILENCE_LEVEL_THRESHOLD && hasSpokenRef.current) {
          // Only start the silence clock once real speech has actually
          // happened — otherwise someone who hasn't started talking yet
          // (still reading the "go ahead" banner) would get auto-stopped
          // before ever saying anything.
          if (silenceStartRef.current === null) silenceStartRef.current = performance.now()
          else if (performance.now() - silenceStartRef.current > SILENCE_STOP_MS) {
            stopRef.current()
            return // the recorder is stopping; no more ticks needed
          }
        }
        rafRef.current = requestAnimationFrame(tick)
      }
      rafRef.current = requestAnimationFrame(tick)

      const rec = new MediaRecorder(stream)
      recorderRef.current = rec
      chunksRef.current = []
      rec.ondataavailable = (e) => { if (e.data.size > 0) chunksRef.current.push(e.data) }
      rec.onstop = async () => {
        teardown()
        const blob = new Blob(chunksRef.current, { type: rec.mimeType || 'audio/webm' })
        if (blob.size === 0) { setState('idle'); return }
        setState('transcribing')
        try {
          const ext = (rec.mimeType || '').includes('mp4') ? 'mp4' : 'webm'
          const form = new FormData()
          form.append('audio', blob, `dictation.${ext}`)
          // Forces a specific model, so a per-tier "Try it" really tests THAT
          // model rather than whichever one currently wins.
          if (tier) form.append('tier', tier)
          const res = await fetch(`${FAMILY_API}/api/voice/transcribe`, { method: 'POST', body: form })
          const data = await res.json()
          if (!res.ok) throw new Error(data?.error || `transcribe failed: ${res.status}`)
          if (data.text?.trim()) onText(data.text.trim())
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Could not transcribe that')
        } finally {
          setState('idle')
          setLiveText('')
        }
      }
      // A timeslice makes ondataavailable fire periodically instead of once
      // at the end — each new chunk is a valid continuation of the SAME
      // container stream, so everything received so far can always be
      // concatenated into one playable clip for the live preview below.
      rec.start(1000)
      previewTimerRef.current = window.setInterval(() => runLivePreview(rec), LIVE_PREVIEW_INTERVAL_MS)
      setState('recording')
    } catch {
      // Nearly always a denied mic permission — worth saying plainly, since
      // the fix is in the OS/browser and not anywhere in this app.
      setError('Could not use the microphone. Check permission for this app.')
      teardown()
      setState('idle')
    }
  }

  const toggle = () => { if (state === 'recording') stop(); else if (state === 'idle') start() }

  return { state, level, liveText, error, toggle, clearError: () => setError(null) }
}

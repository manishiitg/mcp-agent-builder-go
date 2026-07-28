import { useRef, useState } from 'react'
import { FAMILY_API } from '../apiBase'

export type MicState = 'idle' | 'recording' | 'transcribing'

// How often to refresh the live preview while recording, as a backstop for
// continuous speech with no natural pause (see PAUSE_DETECT_MS below, which
// triggers an EXTRA immediate refresh on a pause — most updates come from
// that, not this fixed clock). Each tick re-transcribes the whole
// recording-so-far through the SAME one-shot Parakeet call used for the
// final result — not word-by-word streaming, but genuinely live: text
// appears while you're still talking, not only after you stop.
//
// 1200ms rather than the original 2500ms: with the models kept warm in a
// persistent worker (see voice_worker.go), a real call now takes ~1.2-1.5s
// instead of ~3.5-3.9s, so a shorter interval actually shortens the gap
// between updates instead of just polling uselessly often against a slower
// floor (the in-flight guard below still skips a tick if the previous
// request hasn't finished, so this can't pile up concurrent requests).
const LIVE_PREVIEW_INTERVAL_MS = 1200

// Live amplitude thresholding — the SAME signal already driving the level
// meter — used ONLY to notice a natural pause and refresh the preview right
// then, so it lines up with how someone actually talks (in phrases, with
// breaks) instead of an arbitrary clock. It does NOT stop the recording:
// closing the mic is the parent's decision alone, never automatic. This is
// deliberately simple amplitude thresholding, not a real speech classifier
// (webrtcvad, Silero VAD) — a proper VAD would need either raw PCM streamed
// to Python or an in-browser WASM model, both bigger builds than reusing the
// level meter that already existed. It can be fooled by steady background
// noise (a fan, a TV) reading as "still talking" — acceptable for a preview
// refresh trigger, since a missed pause just means the fixed interval above
// catches it a little later.
const SPEECH_LEVEL_THRESHOLD = 0.16 // above this, "the parent is talking"
const SILENCE_LEVEL_THRESHOLD = 0.08 // below this, "quiet" — lower than the
// speech threshold on purpose, so hovering right at the boundary doesn't
// flicker between the two states tick to tick.
const PAUSE_DETECT_MS = 450 // brief quiet, AFTER real speech, counts as "paused" — short enough to feel immediate, long enough to not fire mid-word

// How many consecutive live-preview attempts (after she's actually said
// something) may come back empty before we say so. The worker unloads after
// 15 minutes idle (see voiceWorkerIdleTimeout in voice_worker.go) and only
// re-warms once at server startup — so the first recording after any gap
// pays a real, multi-second model-load cost that used to be totally
// invisible: the preview just kept silently coming back empty, and "Go ahead
// — start talking" never changed, indistinguishable from a broken mic. Two
// misses (a little over 2 seconds of real talking with nothing back) is
// enough real signal without flickering on one merely-quiet preview tick.
const WARMING_UP_AFTER_MISSES = 2

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
 * signal also triggers an immediate preview refresh on a natural pause (see
 * PAUSE_DETECT_MS above) — it never stops the recording itself.
 *
 * `liveText` is a running preview of what's been said so far. It can revise
 * itself between refreshes (more audio context can change how the model
 * reads an earlier word) — that's expected, the same way any live-captioning
 * UI's in-progress line can shift before it settles. The FINAL result (via
 * onText, on stop) always re-transcribes the complete recording once more
 * for the authoritative version.
 */
export function useMicDictation(onText: (text: string) => void, tier?: string) {
  const [state, setState] = useState<MicState>('idle')
  const [level, setLevel] = useState(0)
  const [liveText, setLiveText] = useState('')
  const [error, setError] = useState<string | null>(null)
  // True once a few consecutive live-preview attempts have come back empty
  // DESPITE real speech being detected — see WARMING_UP_AFTER_MISSES above.
  const [warmingUp, setWarmingUp] = useState(false)

  const recorderRef = useRef<MediaRecorder | null>(null)
  const chunksRef = useRef<Blob[]>([])
  const streamRef = useRef<MediaStream | null>(null)
  const audioCtxRef = useRef<AudioContext | null>(null)
  const rafRef = useRef<number | null>(null)
  const previewTimerRef = useRef<number | null>(null)
  const previewBusyRef = useRef(false)
  const hasSpokenRef = useRef(false)
  const silenceStartRef = useRef<number | null>(null)
  const pauseFiredRef = useRef(false)
  const runPreviewRef = useRef<() => void>(() => {})
  // How many recorder chunks the LAST successful preview transcript was built
  // from, and that exact text. If stop() finds the recording still has
  // exactly that many chunks — meaning not one byte of new audio arrived
  // since — the preview's text IS the final answer: same underlying audio,
  // so re-sending it for an identical answer would just be wasted latency.
  // Any new chunk (the common case; someone almost always adds a beat of
  // trailing audio before clicking stop) falls through to a full re-transcribe.
  const lastPreviewChunkCountRef = useRef(-1)
  const lastPreviewTextRef = useRef('')
  // Consecutive live-preview attempts, AFTER real speech was detected, that
  // came back with no text (empty response OR a failed request) — see
  // WARMING_UP_AFTER_MISSES.
  const previewMissStreakRef = useRef(0)
  // True for exactly as long as a recording is active — guards the
  // self-rescheduling preview chain below against re-arming itself after
  // stop() has already torn everything down (its in-flight call can still
  // resolve after teardown clears the pending timer).
  const recordingActiveRef = useRef(false)

  // Everything the browser handed us has to be torn down explicitly: leaving
  // the MediaStream open keeps the OS mic indicator lit, which reads as "this
  // app is still listening to me" long after it stopped.
  const teardown = () => {
    recordingActiveRef.current = false
    if (rafRef.current !== null) { cancelAnimationFrame(rafRef.current); rafRef.current = null }
    if (previewTimerRef.current !== null) { window.clearTimeout(previewTimerRef.current); previewTimerRef.current = null }
    streamRef.current?.getTracks().forEach((t) => t.stop())
    streamRef.current = null
    audioCtxRef.current?.close().catch(() => {})
    audioCtxRef.current = null
    setLevel(0)
    hasSpokenRef.current = false
    silenceStartRef.current = null
    pauseFiredRef.current = false
    previewMissStreakRef.current = 0
    setWarmingUp(false)
  }

  const runLivePreview = async (rec: MediaRecorder) => {
    if (previewBusyRef.current || chunksRef.current.length === 0) return
    previewBusyRef.current = true
    // Captured BEFORE the request, not after — more chunks can arrive while
    // this is in flight, and this must reflect exactly what the blob below
    // was actually built from.
    const chunkCountForThisBlob = chunksRef.current.length
    try {
      const ext = (rec.mimeType || '').includes('mp4') ? 'mp4' : 'webm'
      const blob = new Blob(chunksRef.current, { type: rec.mimeType || 'audio/webm' })
      const form = new FormData()
      form.append('audio', blob, `preview.${ext}`)
      const res = await fetch(`${FAMILY_API}/api/voice/transcribe`, { method: 'POST', body: form })
      if (!res.ok) { registerPreviewMiss(); return } // A transient decode
      // hiccup on a short clip resolves itself once more audio accumulates —
      // skip this tick rather than show an error for what is only a preview.
      const data = await res.json()
      if (data.text?.trim()) {
        setLiveText(data.text.trim())
        lastPreviewChunkCountRef.current = chunkCountForThisBlob
        lastPreviewTextRef.current = data.text.trim()
        previewMissStreakRef.current = 0
        setWarmingUp(false)
      } else {
        registerPreviewMiss()
      }
    } catch {
      // Same reasoning: a preview tick failing silently is fine; the next
      // tick tries again with more audio.
      registerPreviewMiss()
    } finally {
      previewBusyRef.current = false
    }
  }

  // A miss only counts once she's actually said something — the very first
  // tick or two of a recording legitimately has too little audio yet, which
  // is "hasn't spoken" not "stuck warming up," and must not flip this on.
  const registerPreviewMiss = () => {
    if (!hasSpokenRef.current) return
    previewMissStreakRef.current += 1
    if (previewMissStreakRef.current >= WARMING_UP_AFTER_MISSES) setWarmingUp(true)
  }

  const stop = () => {
    const rec = recorderRef.current
    if (rec && rec.state !== 'inactive') rec.stop() // onstop does the transcribe
    else { teardown(); setState('idle') }
  }

  const start = async () => {
    setError(null)
    setLiveText('')
    setWarmingUp(false)
    lastPreviewChunkCountRef.current = -1
    lastPreviewTextRef.current = ''
    previewMissStreakRef.current = 0

    // Fire-and-forget: if the worker unloaded from 15 minutes idle (see
    // voice_worker.go), this overlaps its cold-start model-load with the
    // parent already talking, instead of that cost landing invisibly on
    // whichever live-preview tick happens to be first.
    fetch(`${FAMILY_API}/api/voice/warm`, { method: 'POST' }).catch(() => {})

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      streamRef.current = stream

      // Live level meter — also used to notice a natural pause and refresh
      // the preview right then (see PAUSE_DETECT_MS). Never stops the
      // recording — only the parent does that.
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
          pauseFiredRef.current = false // speaking again — the next pause can trigger a fresh refresh
        } else if (normalized <= SILENCE_LEVEL_THRESHOLD && hasSpokenRef.current) {
          if (silenceStartRef.current === null) {
            silenceStartRef.current = performance.now()
          } else if (!pauseFiredRef.current && performance.now() - silenceStartRef.current > PAUSE_DETECT_MS) {
            // A natural pause — refresh right now instead of waiting for the
            // fixed interval, so the preview lines up with how someone
            // actually talks. Fires once per pause (not on every tick while
            // they stay quiet) via pauseFiredRef.
            pauseFiredRef.current = true
            runPreviewRef.current()
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
        const finalChunkCount = chunksRef.current.length
        teardown()
        const blob = new Blob(chunksRef.current, { type: rec.mimeType || 'audio/webm' })
        if (blob.size === 0) { setState('idle'); return }

        // The last live preview already transcribed this EXACT audio (not one
        // new chunk arrived since) — its text already IS the final answer.
        // Never taken when `tier` is set: that path forces a specific model
        // for testing, but the live preview always used the auto-selected
        // one, so reusing it here would silently test the wrong model.
        if (!tier && finalChunkCount === lastPreviewChunkCountRef.current && lastPreviewTextRef.current) {
          onText(lastPreviewTextRef.current)
          setState('idle')
          setLiveText('')
          return
        }

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
      runPreviewRef.current = () => runLivePreview(rec)
      // Self-rescheduling, NOT a fixed setInterval: a real transcription call
      // measured at ~2.2-2.4s warm (more when cold) — longer than this
      // interval — so a plain setInterval would fire its NEXT tick while the
      // previous call was still in flight, and previewBusyRef would silently
      // drop it. During continuous speech with no natural pause to trigger
      // an extra refresh, that meant updates arrived every ~2.4s instead of
      // ~1.2s, invisibly. Waiting for each call to actually finish before
      // scheduling the next removes the silent drops — cadence becomes
      // "as fast as transcription genuinely allows," never faster, but never
      // silently slower either.
      const scheduleNextPreview = () => {
        previewTimerRef.current = window.setTimeout(async () => {
          await runLivePreview(rec)
          if (recordingActiveRef.current) scheduleNextPreview()
        }, LIVE_PREVIEW_INTERVAL_MS)
      }
      recordingActiveRef.current = true
      scheduleNextPreview()
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

  return { state, level, liveText, warmingUp, error, toggle, clearError: () => setError(null) }
}

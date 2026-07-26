import { useRef, useState } from 'react'
import { FAMILY_API } from '../apiBase'

export type MicState = 'idle' | 'recording' | 'transcribing'

/**
 * Mic dictation for a composer: record → transcribe on-device → hand back text.
 *
 * Uses MediaRecorder (identical in Electron's Chromium and a real browser, so
 * one implementation serves both). Whatever container the browser picks is
 * sent as-is — ffmpeg normalizes it server-side — so there's no codec
 * negotiation here.
 *
 * `level` is a live 0..1 mic amplitude, polled from an AnalyserNode. Without
 * it a recording UI is a lie: you can't tell "listening" from "mic is muted /
 * the wrong input is selected" until after you've already spoken.
 */
export function useMicDictation(onText: (text: string) => void) {
  const [state, setState] = useState<MicState>('idle')
  const [level, setLevel] = useState(0)
  const [error, setError] = useState<string | null>(null)

  const recorderRef = useRef<MediaRecorder | null>(null)
  const chunksRef = useRef<Blob[]>([])
  const streamRef = useRef<MediaStream | null>(null)
  const audioCtxRef = useRef<AudioContext | null>(null)
  const rafRef = useRef<number | null>(null)

  // Everything the browser handed us has to be torn down explicitly: leaving
  // the MediaStream open keeps the OS mic indicator lit, which reads as "this
  // app is still listening to me" long after it stopped.
  const teardown = () => {
    if (rafRef.current !== null) { cancelAnimationFrame(rafRef.current); rafRef.current = null }
    streamRef.current?.getTracks().forEach((t) => t.stop())
    streamRef.current = null
    audioCtxRef.current?.close().catch(() => {})
    audioCtxRef.current = null
    setLevel(0)
  }

  const stop = () => {
    const rec = recorderRef.current
    if (rec && rec.state !== 'inactive') rec.stop() // onstop does the transcribe
    else { teardown(); setState('idle') }
  }

  const start = async () => {
    setError(null)
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      streamRef.current = stream

      // Live level meter.
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
        setLevel(Math.min(1, peak / 96))
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
          const res = await fetch(`${FAMILY_API}/api/voice/transcribe`, { method: 'POST', body: form })
          const data = await res.json()
          if (!res.ok) throw new Error(data?.error || `transcribe failed: ${res.status}`)
          if (data.text?.trim()) onText(data.text.trim())
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Could not transcribe that')
        } finally {
          setState('idle')
        }
      }
      rec.start()
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

  return { state, level, error, toggle, clearError: () => setError(null) }
}

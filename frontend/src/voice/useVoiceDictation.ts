import { useCallback, useRef, useState } from 'react'
import { getApiBaseUrl, getAuthToken } from '../services/api'
import { startPcmCapture, type PcmCapture } from './pcmCapture'

export type VoiceDictationState = 'idle' | 'starting' | 'listening' | 'error'

type VoiceStreamMessage = {
  type: 'partial' | 'final' | 'error'
  text?: string
  end_of_utterance?: boolean
  error?: string
}

/**
 * Push-to-talk style mic dictation against the shared voicestt backend
 * (cmd/server/voice_stt_routes.go, gated by agentprofiles.RuntimeCapabilities.Voice).
 *
 * `onText` is called with each PARTIAL transcript as it arrives (so a caller
 * can show live text in the composer) and once more with `final: true` when
 * the recognizer's endpoint rule fires or `stop()` is called — mirroring how
 * a push-to-talk button commits a line on release.
 *
 * `level` is a live 0..1 mic amplitude, polled from an AnalyserNode on the
 * SAME stream the recognizer receives — not a decorative animation. A silent
 * or wrong input device shows a flat level with real chunks still arriving,
 * which is the one signal that tells "recording but hearing nothing" apart
 * from "not recording at all" — the confusion a bare icon toggle could not
 * distinguish (a live mic test showed 66s of connected, chunk-receiving
 * silence with no way for the user to tell from the UI alone).
 */
export function useVoiceDictation(
  profileId: string,
  onText: (text: string, final: boolean) => void,
  onStart?: () => void,
) {
  const [state, setState] = useState<VoiceDictationState>('idle')
  const [error, setError] = useState<string | null>(null)
  const [level, setLevel] = useState(0)
  const [liveText, setLiveText] = useState('')
  const wsRef = useRef<WebSocket | null>(null)
  const audioCtxRef = useRef<AudioContext | null>(null)
  const mediaStreamRef = useRef<MediaStream | null>(null)
  const captureRef = useRef<PcmCapture | null>(null)
  const analyserFrameRef = useRef<number | null>(null)

  const cleanup = useCallback(() => {
    if (analyserFrameRef.current !== null) {
      cancelAnimationFrame(analyserFrameRef.current)
      analyserFrameRef.current = null
    }
    setLevel(0)
    captureRef.current?.stop()
    captureRef.current = null
    mediaStreamRef.current?.getTracks().forEach((track) => track.stop())
    mediaStreamRef.current = null
    if (audioCtxRef.current && audioCtxRef.current.state !== 'closed') {
      void audioCtxRef.current.close()
    }
    audioCtxRef.current = null
    if (wsRef.current && wsRef.current.readyState <= WebSocket.OPEN) {
      wsRef.current.close()
    }
    wsRef.current = null
  }, [])

  const start = useCallback(async () => {
    if (state === 'starting' || state === 'listening') return
    onStart?.()
    setState('starting')
    setError(null)
    setLiveText('')
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      mediaStreamRef.current = stream

      const base = getApiBaseUrl().replace(/^http/, 'ws')
      const token = getAuthToken()
      const url = `${base}/api/voice/stream?profile_id=${encodeURIComponent(profileId)}${token ? `&token=${encodeURIComponent(token)}` : ''}`
      const ws = new WebSocket(url)
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      await new Promise<void>((resolve, reject) => {
        ws.onopen = () => resolve()
        ws.onerror = () => reject(new Error('voice websocket failed to connect'))
      })

      ws.onmessage = (event) => {
        let msg: VoiceStreamMessage
        try {
          msg = JSON.parse(event.data as string)
        } catch {
          return
        }
        if (msg.type === 'error') {
          setError(msg.error || 'transcription error')
          return
        }
        if (!msg.text) return
        setLiveText(msg.type === 'final' ? '' : msg.text)
        onText(msg.text, msg.type === 'final')
      }
      ws.onclose = () => {
        if (wsRef.current === ws) {
          wsRef.current = null
          setState('idle')
        }
      }

      const audioCtx = new AudioContext({ sampleRate: 16000 })
      audioCtxRef.current = audioCtx

      // Peak deviation from silence, normalized — cheap and responsive for a
      // simple "is sound arriving" indicator, mirroring the SparkQuill
      // implementation this is a port of (learning-app/src/voice/useMicDictation.ts).
      const analyser = audioCtx.createAnalyser()
      analyser.fftSize = 512
      audioCtx.createMediaStreamSource(stream).connect(analyser)
      const timeDomain = new Uint8Array(analyser.frequencyBinCount)
      const tickLevel = () => {
        analyser.getByteTimeDomainData(timeDomain)
        let peak = 0
        for (const v of timeDomain) peak = Math.max(peak, Math.abs(v - 128))
        setLevel(Math.min(1, peak / 96))
        analyserFrameRef.current = requestAnimationFrame(tickLevel)
      }
      tickLevel()

      captureRef.current = await startPcmCapture(audioCtx, stream, (pcm16) => {
        if (ws.readyState === WebSocket.OPEN) ws.send(pcm16)
      })

      setState('listening')
    } catch (err) {
      cleanup()
      setError(err instanceof Error ? err.message : 'microphone access failed')
      setState('error')
    }
  }, [state, profileId, onText, onStart, cleanup])

  const stop = useCallback(() => {
    // Ask the server to flush and return the final transcript for whatever
    // audio is still buffered before tearing down — closing outright would
    // drop the last partial phrase.
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ action: 'finish' }))
    }
    cleanup()
    setState('idle')
  }, [cleanup])

  return { state, error, level, liveText, start, stop }
}

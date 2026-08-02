import { FAMILY_API } from '../apiBase'

// Raw-PCM capture for the native streaming dictation path
// (see docs/refactor/native_streaming_stt.md).
//
// The MediaRecorder path this sits beside produces a webm/mp4 CONTAINER, whose
// chunks after the first carry no header — which is precisely why that path has
// to resend the whole recording on every preview refresh, and why its cost
// grows with recording length. Raw PCM has no container, so each slice stands
// alone and only new audio is ever sent.

/** 160ms at 16kHz — matches the helper's streaming chunk size. */
export const PCM_CHUNK_SAMPLES = 2560
export const PCM_SAMPLE_RATE = 16000

// Registered from a Blob URL rather than a served file: an AudioWorklet module
// must be fetched by URL, and inlining it keeps this dependency-free and
// immune to asset-path differences between the Vite dev server and the packaged
// app, where the frontend is served by family-server from a different root.
const WORKLET_SOURCE = `
class PcmCollector extends AudioWorkletProcessor {
  process(inputs) {
    const channel = inputs[0] && inputs[0][0]
    // A copy is required: the render quantum's buffer is reused by the audio
    // thread the moment this returns, so posting it directly would deliver
    // whatever the next quantum overwrote it with.
    if (channel) this.port.postMessage(new Float32Array(channel))
    return true
  }
}
registerProcessor('pcm-collector', PcmCollector)
`

export type PcmCapture = { stop: () => void }

/**
 * Streams `stream`'s audio to `onChunk` in PCM_CHUNK_SAMPLES slices.
 * The caller owns both the AudioContext and the MediaStream.
 */
export async function startPcmCapture(
  ctx: AudioContext,
  stream: MediaStream,
  onChunk: (pcm: Float32Array) => void,
): Promise<PcmCapture> {
  const url = URL.createObjectURL(new Blob([WORKLET_SOURCE], { type: 'application/javascript' }))
  try {
    await ctx.audioWorklet.addModule(url)
  } finally {
    URL.revokeObjectURL(url)
  }

  const source = ctx.createMediaStreamSource(stream)
  const node = new AudioWorkletNode(ctx, 'pcm-collector')

  // The worklet emits one render quantum (128 frames) at a time — ~8ms, far too
  // small to be worth a request each. Coalesce up to a real chunk first.
  let pending: number[] = []
  node.port.onmessage = (event: MessageEvent<Float32Array>) => {
    const samples = event.data
    for (let i = 0; i < samples.length; i++) pending.push(samples[i])
    while (pending.length >= PCM_CHUNK_SAMPLES) {
      onChunk(Float32Array.from(pending.slice(0, PCM_CHUNK_SAMPLES)))
      pending = pending.slice(PCM_CHUNK_SAMPLES)
    }
  }

  source.connect(node)
  // Not connected to ctx.destination on purpose — routing the mic to the
  // speakers would echo the speaker back at themselves. An AudioWorkletNode
  // still pulls input without a downstream connection.

  return {
    stop: () => {
      node.port.onmessage = null
      try { source.disconnect() } catch { /* already torn down */ }
      try { node.disconnect() } catch { /* already torn down */ }
    },
  }
}

/**
 * Opens a dictation session on the server.
 * Returns false when the native helper isn't available (the server answers
 * 503), which is the signal to fall back to the MediaRecorder path.
 */
export async function nativeStreamStart(): Promise<boolean> {
  try {
    const res = await fetch(`${FAMILY_API}/api/voice/stream/start`, { method: 'POST' })
    return res.ok
  } catch {
    return false
  }
}

/** Sends one chunk, returning the running transcript so far. */
export async function nativeStreamChunk(pcm: Float32Array): Promise<string> {
  try {
    const res = await fetch(`${FAMILY_API}/api/voice/stream/chunk`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: pcm.buffer as ArrayBuffer,
    })
    if (!res.ok) return ''
    const data = await res.json()
    return typeof data.partial === 'string' ? data.partial : ''
  } catch {
    // A dropped preview tick is not worth surfacing: the helper keeps the
    // utterance, so the next tick returns a transcript covering this audio too.
    return ''
  }
}

/** Closes the session and returns the punctuated, committed transcript. */
export async function nativeStreamFinish(): Promise<string> {
  const res = await fetch(`${FAMILY_API}/api/voice/stream/finish`, { method: 'POST' })
  if (!res.ok) throw new Error(`finish failed: ${res.status}`)
  const data = await res.json()
  // `text` is the batch pass (punctuated); `streamed` is the raw streaming
  // transcript, used only if the batch stage somehow came back empty.
  const text = typeof data.text === 'string' ? data.text : ''
  return text || (typeof data.streamed === 'string' ? data.streamed : '')
}

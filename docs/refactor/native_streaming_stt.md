# Native streaming speech-to-text

**Status:** In progress — end-to-end path implemented, NOT yet verified in the
running app. The helper, server endpoints, and browser capture are all written
and build clean, and the helper itself is verified against real speech
standalone; what has never run is the whole chain inside SparkQuill with a real
microphone. The Python/MLX path remains the automatic fallback whenever the
helper binary is absent, so machines without it are unaffected.
**Date:** 2026-08-02
**Repositories:** `mcp-agent-builder-go` (SparkQuill: `agent_go/cmd/family-server`,
`frontend/learning-app`, `desktop-sparkquill`)
**Related:** `docs/refactor/README.md`

## The problem

Live mic dictation re-transcribes the **entire recording from the start** on
every preview refresh. Cost therefore grows with recording length: a call that
takes ~1.2s a second into talking takes materially longer thirty seconds in,
and the preview drifts further behind the speaker the longer they talk. The
final transcription on stop pays the same full-clip cost again.

This is architectural, not a tuning problem. `LIVE_PREVIEW_INTERVAL_MS` in
`useMicDictation.ts` is already self-rescheduling (it waits for each call to
finish rather than firing on a fixed clock), so there is no polling interval
left to shorten — the floor is the transcription call itself.

## What exists today

- **Capture:** browser `MediaRecorder` (webm/mp4 container), chunked at 1s via
  `rec.start(1000)`.
- **Transport:** each preview posts the whole accumulated blob to
  `POST /api/voice/transcribe` (multipart) — `voice_transcribe_api.go`.
- **Inference:** `voice_worker.go` supervises a persistent Python process
  (`voice_worker.py`, `//go:embed`-ed and written to `mlxVoiceDir()`), one JSON
  request per line over stdin/stdout, models kept warm for
  `voiceWorkerIdleTimeout` (15 min).
- **Model:** `mlx-community/parakeet-tdt-0.6b-v2` via `mlx_audio.stt`, called
  through `generate_transcription` — a one-shot batch call.
- **Install:** `voice_mlx_env.go` builds a ~3.1GB Python venv on first use.

### Why the current stack cannot stream (verified, not assumed)

Inspected the installed package directly in
`/Users/mipl/.sunlit-learning/mlx-voice/.venv`:

- `ParakeetTDT.stream_generate(...)` **takes a complete recording** (a path or a
  full `mx.array`) and chunks it internally, yielding progressive results. It is
  streaming *output over a finished file*, not streaming *input*. Useless for a
  live microphone.
- `ParakeetTDT.decode_chunk(audio_data)` is **stateless** — its whole body is
  `log_mel_spectrogram` then `self.decode(mel)[0]`. No decoder state is carried
  between calls, so consecutive chunks cannot be decoded as one continuous
  utterance.

There is no stateful incremental API in `mlx_audio` to migrate onto.

## Target architecture

Replace the Python/MLX voice worker with a native Swift helper built on
[FluidAudio](https://github.com/FluidInference/FluidAudio) (**Apache-2.0**),
whose `StreamingEouAsrManager` + `EncoderCacheManager` keep encoder/decoder
state across appends — the mechanism that makes genuinely incremental
transcription possible.

Licensing note: FluidAudio is Apache-2.0 and is the only dependency taken. The
FluidVoice *application* that popularised it is GPLv3; none of its code is used
or referenced here.

```
AudioWorklet (raw PCM 16k mono)
      │  POST /api/voice/stream/chunk
      ▼
family-server  ──stdin (JSON + base64 PCM)──▶  voice-helper (Swift)
      ▲                                              │
      └──────── partial in response ◀── JSON lines ──┘
```

(The original sketch here used a WebSocket. It was replaced during
implementation: the audio only crosses loopback to a server on the same
machine at ~6 chunks/second, so a socket bought nothing and would have added
framing, a dependency, and its own backpressure failure modes.)

1. **`desktop-sparkquill/voice-helper/`** — SwiftPM executable depending on
   FluidAudio. Reads JSON lines carrying base64 PCM on stdin, writes JSON
   lines on stdout. The line protocol deliberately mirrors `voice_worker.py`'s shape so
   `voice_worker.go`'s supervision, warm-timeout, and teardown logic carry over
   rather than being rewritten.
2. **Frontend** — replace the `MediaRecorder` preview path with an
   `AudioWorklet` producing raw 16kHz mono PCM. This also removes the container
   problem that makes incremental decode impossible today: chunks after the
   first carry no container header, which is exactly why the current code has to
   resend the whole blob every time.
3. **Server** — three POST endpoints (`start`/`chunk`/`finish`) in
   `voice_stream_api.go`, each forwarding to the helper and returning its
   reply. Every one stays curl-debuggable, like the existing voice endpoints.
4. **CI** — `swift build -c release --arch arm64` in
   `.github/workflows/sparkquill-desktop.yml`, binary staged into
   `extraResources` beside `family-server`. Swift 6.3.3 is present on the
   `macos-15` runner and locally.

### Secondary prize: deleting the Python voice env

If WhatsApp voice-note transcription moves to the same helper, the entire
Python/MLX voice stack can be removed:

- the ~3.1GB venv build in `voice_mlx_env.go` and its install/remove UI,
- `voice_worker.py` and its stdin/stdout JSON protocol,
- the `mx.clear_cache()` unbounded-cache workaround (added 2026-08-02 after
  MLX's cache — unbounded by default, on unified memory shared with the whole
  machine — was identified as a real leak in a process that stays warm 15
  minutes and is hit every ~1.2s during dictation),
- two independent Parakeet installs collapsing into one.

This is the strongest argument for the change and should be treated as part of
it, not a follow-up: leaving both stacks in place is strictly worse than either
alone.

## Measured: the helper works, and latency is not the open question

The Swift helper builds and runs. Verified end-to-end against real speech
(8.8s of `say`-generated audio, fed in 160ms chunks — the shape the live mic
path will use):

| | Native helper | Current Python/MLX |
|---|---|---|
| Per preview refresh | **18–25ms** | ~1.2s and rising with length |
| Trend over a recording | **falls** (25ms → 18ms) | grows |
| Final transcription on stop | **~10ms** | ~1.2–2.4s (full re-transcribe) |
| First call after load | 3.6s (one-time JIT) | — |
| First-run model download + load | 107s | ~3.1GB venv build |

The core claim holds and then some: per-chunk cost is flat-to-falling in
recording length, and `finish()` is effectively free because it flushes live
state instead of re-decoding. Text appears while the speaker is still talking.

### Word accuracy is fine — two earlier "defects" were a bad test

The first run appeared to drop a leading word ("The quick brown fox…" → "quick")
and to garble "photosynthesis" into "photosynthesythesis". **Both were artifacts
of the test audio, not the engine.** That clip began speaking at sample 0; real
microphone input always has a moment of quiet first. Re-run with 0.5s of lead-in
silence, the same model returns:

> the quick brown fox jumps over the lazy dog photosynthesis is how plants make
> their own food using sunlight water and carbon dioxide

Correct throughout, including "jumps" and "photosynthesis". Recorded here
because the original claim is wrong and would otherwise have argued against a
sound approach. Any future test must include lead-in silence.

### The one real gap: no punctuation

Confirmed across **all three** streaming variants — `.ms160`, `.ms320`,
`.ms1280` — on the same padded audio: byte-identical output, no punctuation, no
capitalization. This is a property of FluidAudio's streaming EOU Parakeet
models, not a chunk-size tradeoff, so there is nothing to tune here.
`Documentation/ASR/PostProcessing.md` does not address it either: that is
Inverse Text Normalization (numbers, dates, currency), not punctuation.

### Resolved: two-stage, and it is strictly better than today

Punctuation is **solved**, not traded away. The helper now runs two models:
`StreamingEouAsrManager` for the live preview, and `UnifiedAsrManager`
(parakeet-tdt-0.6b-v2, the same family as the MLX checkpoint in use today) for
the committed text. Measured on the same padded clip:

| | Output |
|---|---|
| Live preview (streaming) | `the quick brown fox jumps over the lazy dog photosynthesis is how plants…` |
| **Committed (batch, 102ms)** | `The quick brown fox jumps over the lazy dog. Photosynthesis is how plants make their own food using sunlight, water and carbon dioxide` |

Capitalization, sentence-final period, and an interior comma — all present. So
the committed message is **no worse than today's text and ~12–24x faster**
(102ms against 1.2–2.4s), while the preview is effectively instant. There is no
remaining quality argument against this path; the earlier "not viable for the
composer" conclusion is withdrawn.

Unpunctuated live text is fine because the preview is explicitly allowed to
revise itself, and this shape matches a product decision already taken
independently: the final transcription should always be a full accurate pass
rather than a reused preview.

Note the API in `Documentation/ASR/GettingStarted.md` is stale against v0.15.5
(`AsrManager.initialize`/`transcribe(_:source:)` do not exist). `UnifiedAsrManager`
— `loadModels()` then `transcribe([Float]) -> String` — is the current surface.

A cold-start cliff remains: the first `audio` call after load took 3.6s (JIT),
so the helper needs the same pre-warm the Python path already does via
`/api/voice/warm`. Subsequent loads with weights cached took 0.6s.

## Risks and open questions

- **First-run model download.** FluidAudio fetches its own CoreML Parakeet
  weights from HuggingFace, separate from the MLX checkpoints already on disk.
  The download UX (progress, failure, retry) needs to match what
  `voice_models.go` does today, and the migration must not leave users holding
  both model sets.
- **WebSocket backpressure.** Audio is produced in real time; if the helper
  stalls, frames must be dropped rather than queued without bound.
- **Platform.** macOS/Apple Silicon only — already true of the current voice
  tier (`voice_hardware.go` gates on architecture), so not a regression.
- **CI cost.** Adds a Swift build to a workflow that already builds Go and the
  frontend.
- **Accuracy parity is unproven.** FluidAudio's streaming Parakeet is a
  different model variant from `parakeet-tdt-0.6b-v2`. Parity must be measured
  on real family audio before the Python path is deleted, not assumed — the
  removal above is contingent on that check.

## Sequencing

The Python path stays fully working until the Swift path is verified.

1. ~~Swift helper + protocol, exercised standalone.~~ Done; measured above.
2. ~~`voice_worker.go` able to drive it.~~ Done — `voice_native.go` reuses
   `voiceWorker` with a different launcher, covered by an opt-in integration
   test that drives the real process (`SPARKQUILL_VOICE_STREAM_TEST=1`).
3. ~~Frontend capture on the new path.~~ Done — `nativePcm.ts` (AudioWorklet →
   raw PCM) and a branch in `useMicDictation`. Three POSTs rather than a
   WebSocket; see `voice_stream_api.go` for why.
4. **Next: use it with a real microphone.** Nothing below this line has been
   exercised in the running app.
5. Then: accuracy/latency comparison against the Python path on family audio.
6. Only then: WhatsApp migration and Python removal.

### Known-unverified, in likely-to-bite order

- **Chunk upload keeping up with speech.** Chunks queue and upload strictly in
  order (never dropped — the batch pass needs the whole utterance). At ~20ms
  per upload against 160ms of audio there is ~8x headroom, but this has only
  been reasoned about, not measured under a real recording.
- **`AudioContext({sampleRate: 16000})`.** Chromium honours it; if a device
  refuses, the worklet would emit at another rate and the helper would receive
  mis-timed audio. No resampling guard exists yet.
- **First-run download inside the app.** `/stream/start` blocks while weights
  download (~96s measured). The UI shows "Listening" throughout, with no
  progress — poor, though only once per machine.

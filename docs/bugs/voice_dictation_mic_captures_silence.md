# Bug: Voice dictation mic captures real silence on at least one dev machine

## Status: OPEN — environment-level, not yet root-caused. Not a code defect as far as
the investigation below could establish.

## Symptom

Voice dictation (`agent_go/pkg/voicestt`, `frontend/src/voice/*`, gated by
`agentprofiles.RuntimeCapabilities.Voice`) connects successfully and streams
audio chunks the whole session, but the recognizer never produces any partial
or final text. Server-side RMS telemetry (`pcmRMS` in
`cmd/server/voice_stt_routes.go`) shows every chunk at `rms=0.0000` — the audio
arriving at the backend is genuine digital silence, not a transport or model
bug.

## What was ruled out

The full pipeline was proven correct independently of the reporter's machine:

- A raw Go WebSocket client (`/tmp/voice-client-spike/main.go`) streamed a real
  `.wav` file through the live `/api/voice/stream` endpoint and got correct
  partial/final transcripts back. Transport, decode, and the sherpa-onnx/
  Nemotron engine all work end-to-end.
- `pkg/voicestt/engine_live_test.go` (opt-in, real model) asserts the
  transcript text *grows* across chunks against a real `.wav` — passed.
- In-browser, the actual `startPcmCapture` (`frontend/src/voice/pcmCapture.ts`)
  was fed a synthetic 440Hz tone via a `getUserMedia` override and produced
  real, varying int16 PCM samples (decoded RMS ≈ 0.71). The JS capture →
  PCM16 encode → WebSocket send path is correct.
- A bare `getUserMedia({audio:true})` call in the reporter's actual tab (no app
  code involved at all) was granted the correct device
  (`MacBook Air Microphone (Built-in)`, not the Zoom virtual device or an
  iPhone mic), returned `muted=false enabled=true`, and an `AnalyserNode`
  wired directly to that raw stream still read `maxPeak=0` over 2+ seconds
  while the reporter spoke.
- Reproduced in a second, completely separate browser window not under
  automation control — rules out the browser-automation tooling
  (`claude-in-chrome`) serving a fake/muted device.
- Reproduced with the SparkQuill Electron app fully quit (`Cmd+Q`, not just
  window-closed) — rules out its native STT engine holding the input device.
- `/Library/Audio/Plug-Ins/HAL/` has no third-party noise-suppression driver
  (e.g. Krisp) that could be intercepting the signal; the only entries are
  `ParrotAudioPlugin.driver` (bundle id `com.apple.audio.ParrotAudioPlugin` —
  an Apple-internal component, no associated user-facing app) and
  `ZoomAudioDevice.driver` (not the granted device).
- macOS Privacy & Security → Microphone has Chrome enabled.
- Ruled out `frontend/learning-app` mic dictation as a divergent implementation
  to compare against — this port intentionally mirrors it 1:1
  (`useVoiceDictation.ts`, `MicButton.tsx`).

## Remaining candidate, unconfirmed

macOS System Settings → Sound → Input has a level **meter** and a separate
**Input volume** slider below it. On some macOS versions the meter reflects
raw hardware signal presence even when the volume slider (the actual gain
handed to apps via CoreAudio) is at or near zero — which would explain every
observation above: real signal at the hardware/meter level, exact digital
silence in every application that requests audio through normal channels.
Asked the reporter to check this slider specifically; not yet confirmed either
way as of this writing.

## Why this is filed as a platform doc and not closed as "not our bug"

The investigation above is the reusable part: if this recurs on another
machine, the same elimination sequence (bypass the app entirely with a raw
`getUserMedia` + `AnalyserNode` check, verify the granted `deviceId`/label,
check for HAL plugins, check Privacy toggle, check Input volume slider) should
be run before re-opening the app-level code as a suspect.

## Follow-up if this recurs

- If the raw `getUserMedia` + `AnalyserNode` bypass test (see "What was ruled
  out" above) reads non-zero on a machine where the in-app mic still shows
  `rms=0.0000`, the bug moves back into `frontend/src/voice/` — re-open here
  with that evidence.
- If the bypass test is also silent, it is environment/OS/hardware, not this
  codebase; the Input volume slider is the next thing to check, followed by
  a full audio driver/kext audit (`system_profiler SPAudioDataType`,
  `/Library/Audio/Plug-Ins/HAL/`).

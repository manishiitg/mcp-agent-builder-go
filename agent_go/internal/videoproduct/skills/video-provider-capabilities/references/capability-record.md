# Capability record

Use this checklist to resolve one exact endpoint. Do not create a static model
catalog: model names, limits, fields, prices, and routes change. Refresh the
record when the endpoint changes, the documented schema changes, or the record
is older than the production's accepted freshness window.

## Discovery order

1. List current candidate endpoints from the configured provider.
2. Select by production intent and required inputs, not by a familiar family
   name or by whether the output happens to be video.
3. Open the exact official endpoint schema and pricing page.
4. Record its canonical model/endpoint ID and source URLs.
5. Resolve higher-level workflows separately from raw model endpoints.

## Contract fields

Capture:

- required and optional fields, exact names, types, defaults, and nesting;
- closed enums and numeric ranges for duration, ratio, resolution, quality,
  seed, guidance, output format, and audio;
- each media role, minimum/maximum count, supported MIME/size constraints, and
  whether local paths need upload, inline encoding, or hosted URLs;
- multi-shot structure, prompt-array/timestamp rules, and total duration cap;
- first-frame, last-frame, general image, video, and audio reference semantics;
- extension/edit modes and what source artifact they require;
- mutually exclusive fields and mode-dependent fields;
- asynchronous submit/status/result shapes, terminal states, and output URLs;
- pricing source and timestamp, currency, billing unit, unit price, base
  estimate, retry reserve, maximum approved cost, and relevant rate limits;
- provider-side adjustment/coercion behavior and structured validation errors.

Probe beyond the basic prompt/duration fields. Useful discovery terms include
structured or multi-shot prompts, first/start and last/end frames, character or
element references, multiple image references, video/audio references, motion
transfer, edit, extend/extension, seed, and generated/native audio. Model-family
names are only search hints: for example, an endpoint branded Kling, Seedance,
or Veo may expose a different subset or field shape than another endpoint in
the same family. Confirm every feature and exact field on the selected route.

Use `null` for a value the official source does not disclose. Use `false` only
when the source confirms lack of support. Never translate one endpoint's role
into another endpoint's guessed field.

## Selection comparison

When two endpoints fit, compare only production-relevant dimensions:

- continuity mechanism and reference capacity;
- maximum useful duration and multi-shot support;
- identity/location/style control;
- native or reference audio and lip-sync needs;
- output resolution/aspect ratio;
- edit/extension capability;
- estimated paid calls, base cost, approved maximum cost, latency, and retry
  exposure.

Prefer the endpoint that satisfies the continuity plan with fewer independent
generations. Do not downgrade merely because an older endpoint is easier to
call. If the user named a model, validate that model first and disclose any
hard incompatibility before proposing another.

## Reference manifest

Write one ordered manifest beside the request:

```json
[
  {"index": 1, "role": "start_frame", "path": "frames/scene-01-start.png", "source": "approved character/location composite"},
  {"index": 2, "role": "end_frame", "path": "frames/scene-01-end.png", "source": "planned transition target"},
  {"index": 3, "role": "audio_reference", "path": "audio/scene-01.wav", "source": "approved narration"}
]
```

Keep the order stable across calls. Never attach a reference merely because
the endpoint allows it; state what property it controls.

---
name: google-ai
description: Generate AI video, image, and narration (text-to-speech) via Google's own Gemini API (Node.js client) -- Gemini image models (publicly nicknamed "Nano Banana"), Veo video models, and Gemini TTS. Use instead of fal-ai when the brief calls for a Google-native model, or when Google is the only provider configured -- this skill can carry a whole production except its music bed. Read alongside video-model-selection, which owns which model to pick.
---

# Google Gemini generation

Google's own image, video, and speech models (Gemini image generation,
publicly nicknamed "Nano Banana"; Veo for video; Gemini TTS for narration)
are reached directly through Google's Gemini API, not through fal.ai. Use
this skill instead of `fal-ai` when the brief specifically calls for a
Google-native model, or when Google is the only provider configured -- it
covers everything a production needs except a music bed. The two skills share the
same operational discipline (never invent a model ID, cost awareness, output
handling) -- read `fal-ai`'s equivalent sections too if this is the first
generation skill you're reading this session, since the rules are stated
fully there and only the provider-specific parts differ here.

This skill owns the client call once a model is already chosen and the shot's
direction is already decided. Read `video-model-selection` first to choose
between this provider and `fal-ai`, and `video-cinematography` to turn the
storyboard beat into the actual prompt/camera/lighting direction you pass in.

## Never invent a model ID

"Nano Banana" and "Veo" are colloquial names, not API model identifiers. Do
not guess the exact model string from memory. Before the first generation
call in a new production:

1. Ask the user which model they want if the brief does not already say.
2. If the user has no preference, resolve the current model ID and its
   required request shape from Google's own Gemini API reference -- not from
   this skill, which is deliberately silent on exact IDs and changes on
   Google's own schedule.
3. Record the resolved model ID and the exact request used in
   `production.json` (see "Make the work resumable" in `video-creation`), so
   a revision reruns the same model rather than whatever is current later.

A run whose model ID cannot be confirmed is a blocker to report, not a guess
to make.

## Authentication

The user stores the key as a workflow secret named `GEMINI_API_KEY` (via
`set_workflow_secret`) -- the same env var name this platform's own LLM
provider configuration already uses for Gemini, kept consistent rather than
introducing a second name for the same kind of credential. The
secret-injection mechanism prefixes every secret name with `SECRET_` in the
shell environment, so the variable actually present is
`$SECRET_GEMINI_API_KEY`, not `$GEMINI_API_KEY`. Bridge the two explicitly:

```bash
export GEMINI_API_KEY="$SECRET_GEMINI_API_KEY"
node generate.mjs
```

or pass it directly to the client instead of relying on ambient env:

```js
import { GoogleGenAI } from "@google/genai";

const ai = new GoogleGenAI({
  apiKey: process.env.SECRET_GEMINI_API_KEY ?? process.env.GEMINI_API_KEY,
});
```

Accept either name: `SECRET_GEMINI_API_KEY` is what the injection mechanism
provides, but a key set directly in the environment as `GEMINI_API_KEY` is
equally valid and should not be treated as missing. If neither is set, stop
and report the blocker -- do not proceed without generation credentials, and
never print or log the key value itself.

## Environment

Google's officially supported client for this is the Node.js SDK
(`@google/genai`), matching this product's existing preference for a managed
Node toolchain over ad hoc scripts in another language. No project-local
`package.json`/`node_modules` is assumed to exist yet:

```bash
npm init -y --silent
npm install --silent @google/genai
```

Write generation scripts as plain `.mjs` files and run them with `node`.

## The generation call

Confirm the exact method name and request shape against the model reference
you resolved -- image and video generation are separate calls on this client
and their request/response shapes differ by model:

```js
import { GoogleGenAI } from "@google/genai";

const ai = new GoogleGenAI({ apiKey: process.env.SECRET_GEMINI_API_KEY });

const response = await ai.models.generateContent({
  model: "<resolved-model-id>",
  contents: "<prompt, and any reference image per the model's input spec>",
});
// Confirm the exact response shape (inline image bytes vs. a hosted URL vs.
// a long-running operation to poll) against the resolved model's own
// reference -- this differs between image models and Veo's video models,
// and moves independently of this skill.
```

## Generating narration (text-to-speech)

Gemini generates speech through the same client, which is what lets a
production run entirely on Google without a fal.ai key. Two things about
the output differ from every other call in this skill and both will produce
a broken file if missed:

- **The audio comes back as raw PCM, not a playable file.** It is base64
  16-bit PCM at 24 kHz, mono. Writing those bytes to `.wav` unchanged
  produces a file nothing will play -- wrap it in a WAV header first (or
  convert with ffmpeg, `-f s16le -ar 24000 -ac 1`). Confirm the returned
  sample rate rather than assuming, since it is stated per model.
- **Quality drifts on long passages.** Google's own guidance is to split
  anything beyond a few minutes into chunks. This matches what
  `video-editing` already requires for a different reason -- narration is
  generated per beat or per chapter so one bad beat can be regenerated
  without re-timing everything after it -- so generate per segment and
  measure each with `ffprobe`, never as one long pass.

```js
const response = await ai.models.generateContent({
  model: "<resolved-tts-model-id>",
  contents: [{ parts: [{ text: "<the exact narration line>" }] }],
  config: {
    responseModalities: ["AUDIO"],
    speechConfig: {
      voiceConfig: { prebuiltVoiceConfig: { voiceName: "<voice>" } },
    },
  },
});
// base64 PCM -- wrap as WAV before writing, per above.
const pcm = response.candidates[0].content.parts[0].inlineData.data;
```

Confirm the model id, the voice list, the exact response path, and whether
your SDK version prefers this or a newer speech endpoint against Google's
own current speech-generation reference before relying on it -- this
surface is in preview and moves faster than the rest of the API. Keep one
voice for the whole production unless the script genuinely calls for more
than one speaker; multi-speaker output is supported but capped at two.

## Music is not covered here

Google's API surface in this skill covers video, image, and speech -- not a
music bed. A production that needs one and has no fal.ai key should use an
uploaded or licensed track (see `video-editing` for the levels to mix it
at), or run without music rather than substituting something whose licence
is unclear. Say which of those is happening rather than quietly shipping a
silent mix the brief did not ask for.

## Sending a local file as input

Unlike `fal-ai`, which uploads a file and passes back a URL, Gemini takes
image input inline as base64 bytes alongside the prompt. This is how a
character reference image produced per `video-cinematography` gets
conditioned on:

```js
import { readFile } from "node:fs/promises";

const bytes = await readFile("work/characters/<character-name>.png");

const response = await ai.models.generateContent({
  model: "<resolved-model-id>",
  contents: [
    { inlineData: { mimeType: "image/png", data: bytes.toString("base64") } },
    { text: "<prompt>" },
  ],
});
```

Confirm the exact `contents` shape against the resolved model's own
reference before relying on it -- part ordering, field names, and whether a
model accepts more than one reference image all vary, and the larger-file
path (an upload/file API rather than inline bytes) differs again. Record
which reference image conditioned which shot in `production.json` (see
`video-creation`) so a revision reuses the same one.

Video generation (Veo) is typically a longer-running operation than image
generation -- confirm whether the resolved model returns synchronously or
requires polling an operation handle, and do not assume one shape for both
capabilities.

Treat a call that errors or times out as a real failure to report with the
error payload -- do not silently retry with different input hoping one
succeeds, and do not fabricate a result if generation fails.

## Output handling

- Depending on the model, output arrives as inline bytes or a hosted URL --
  check which before writing download/decode logic. Either way, get every
  asset you intend to keep into `work/` (or the current stage's execution
  folder, per `video-creation`'s workflow-stage rules) before the session
  ends.
- Use stable, descriptive filenames per shot/asset, and record them in
  `production.json` alongside the model ID and request that produced them.
- Verify what was actually generated before treating a call as done: check
  dimensions, and for video the duration and that the file is not silently
  truncated or corrupt, with `ffprobe` -- a successful API response is not
  proof the asset is usable.

## Cost awareness

Every call here is a paid API request, more so than any other tool this
product uses. Follow `video-creation`'s existing rule strictly: avoid paid
multi-variant generation unless the user explicitly asked for alternatives,
and cache every successful generation so a later step (assembly, QA,
revision) reuses the downloaded file instead of regenerating it. Re-running a
generation call merely to re-inspect its own output is a cost bug, not a
verification step -- inspect the file you already downloaded.

## Where this fits

This skill produces raw generated assets only. It does not assemble a video
or judge whether one is ready to present:

- Use `video-creation` to plan the shot list, own the overall brief, and
  decide between this skill and `fal-ai` per model.
- Use `video-editing` to assemble generated clips, narration, and music into
  a final cut.
- Use `video-quality` before presenting any version as complete.

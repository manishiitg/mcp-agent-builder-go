---
name: google-ai
description: Generate AI video and image clips via Google's own Gemini API (Node.js client) for long-form narrative video productions -- Gemini image models (publicly nicknamed "Nano Banana") and Veo video models. Use instead of fal-ai specifically when the brief calls for a Google-native model rather than a third-party model fal.ai hosts. Not for short product-led explainers -- those stay on HyperFrames typography/composition (see hyperframes, product-infographic).
---

# Google Gemini generation

Google's own image and video models (Gemini image generation, publicly
nicknamed "Nano Banana"; Veo for video) are reached directly through Google's
Gemini API, not through fal.ai. Use this skill instead of `fal-ai` when the
brief specifically calls for a Google-native model. The two skills share the
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

const ai = new GoogleGenAI({ apiKey: process.env.SECRET_GEMINI_API_KEY });
```

If `$SECRET_GEMINI_API_KEY` is unset, stop and report the blocker -- do not
proceed without generation credentials, and never print or log the key value
itself.

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

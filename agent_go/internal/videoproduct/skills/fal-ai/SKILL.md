---
name: fal-ai
description: Generate AI video, image, voice/TTS, and music clips via fal.ai's Node.js client for cinematic video productions. Use when a brief calls for footage, reference imagery, narration, or a music bed that cannot come from uploaded assets or a selective deterministic HyperFrames insert.
---

# fal.ai generation

fal.ai hosts many independent, frequently-changing models behind one client
pattern (submit a job, poll or subscribe, retrieve result URLs). This skill
teaches the pattern and the operational rules; it does not pin a model
catalog, because model IDs, capabilities, and pricing change on their own
schedule and going stale here would silently break every production that
trusted it.

This skill owns the client call once a model is already chosen and the shot's
direction is already decided. Read `video-model-selection` first to choose
between this provider and `google-ai`, and `video-cinematography` to turn the
storyboard beat into the actual prompt/camera/lighting direction you pass in.

fal.ai's officially supported client is Node.js (`@fal-ai/client`), not
Python. `npx` is already this product's proven way to run a managed Node CLI
non-interactively (see `hyperframes-cli`) -- use the same pattern here rather
than reaching for a Python SDK that isn't fal.ai's first-class offering.

## Never invent a model ID

Do not guess a fal.ai model slug from memory or pattern-match one that
"sounds right." Before the first generation call in a new production:

1. Ask the user which model they want for each capability (video, image,
   voice, music) if the brief does not already say, the same way an
   unresolved factual brief detail is never invented elsewhere in this
   product.
2. If the user has no preference, resolve the current model ID and its
   required `input` shape from fal.ai's own model page or API reference for
   that capability -- not from this skill, which is deliberately silent on
   exact IDs.
3. Record the resolved model ID, version, and the exact input used in
   `production.json` (see "Make the work resumable" in `video-creation`), so
   a revision reruns the same model rather than whatever is current later.

A run whose model ID cannot be confirmed is a blocker to report, not a guess
to make.

**Video Studio reference-pack exception:** its system policy already selects
`fal-ai/flux-2-max` for a new cinematic character/background master and
`fal-ai/flux-2-max/edit` for an approved derivative. Verify those IDs and the
live schema, then use them; do not ask the user to reselect a still-image
model or silently substitute a cheaper route. A different still-image model
requires the user's explicit approval.

## Authentication

The user stores the fal.ai key as a workflow secret named `FAL_KEY` (via
`set_workflow_secret`). The secret-injection mechanism prefixes every secret
name with `SECRET_` in the shell environment, so the variable actually
present is `$SECRET_FAL_KEY`, not `$FAL_KEY`. fal.ai's client reads the
unprefixed `FAL_KEY` by default, so bridge the two explicitly rather than
assuming the client will find it on its own:

```bash
export FAL_KEY="$SECRET_FAL_KEY"
node generate.mjs
```

or pass it directly to the client instead of relying on ambient env:

```js
import { fal } from "@fal-ai/client";

fal.config({
  credentials: process.env.SECRET_FAL_KEY ?? process.env.FAL_KEY,
});
```

Accept either name: `SECRET_FAL_KEY` is what the injection mechanism
provides, but a key set directly in the environment as `FAL_KEY` is equally
valid and should not be treated as missing. If neither is set, stop and
report the blocker -- do not proceed without generation credentials, and
never print or log the key value itself.

## Environment

No project-local `package.json`/`node_modules` is assumed to exist yet. Set
one up once per production and reuse it for the whole run rather than
reinstalling per call:

```bash
npm init -y --silent
npm install --silent @fal-ai/client
```

Write generation scripts as plain `.mjs` files (ESM, matching the package's
own `import` style) and run them with `node`, the same way HyperFrames
compositions in this product are driven by a local Node toolchain rather than
one-off inline snippets.

## The submit-and-wait pattern

Every fal.ai model call shares this shape regardless of capability.
`subscribe` blocks and streams progress logs -- the right default for one job
at a time:

```js
import { fal } from "@fal-ai/client";

fal.config({ credentials: process.env.SECRET_FAL_KEY });

const result = await fal.subscribe("<resolved-model-id>", {
  input: {
    // capability-specific: prompt, image_url, duration, aspect_ratio,
    // voice, seed, etc. -- copy the exact shape from the model's own
    // reference page, do not guess field names.
  },
  logs: true,
  onQueueUpdate: (update) => {}, // replace to surface progress if useful
});
```

### Long-running video jobs: use a durable request, not a short blocking call

Video generation is asynchronous provider work, not an instant shell command.
For a single H3 shot, budget up to **15 minutes** when the calling tool permits
it, surface queue/progress updates, and keep one request alive rather than
creating another paid job. Do not declare a job failed merely because a short
local command or tool wait expires.

For a video job, prefer the submit/poll flow even when only one shot is in
flight: it gives the production a durable `request_id` to resume. Persist the
model ID, request ID, input, and submission timestamp in `production.json`
immediately after submission. Poll the same request at a measured interval
(for example, 5--10 seconds), report meaningful state changes (`IN_QUEUE`,
`IN_PROGRESS`, `COMPLETED`), and retrieve the result exactly once when it is
complete. If a local wait times out or the chat reconnects, resume with that
request ID; never submit a replacement until fal reports this request failed
or the user expressly approves a paid retry.

For several independent jobs in flight at once (e.g. generating multiple
shots in parallel), use the non-blocking submit/poll pair instead so one slow
job does not serialize the rest:

```js
const { request_id } = await fal.queue.submit("<resolved-model-id>", {
  input: { /* ... */ },
});
// Save request_id durably before waiting. Then use
// fal.queue.status(modelId, { requestId: request_id }) to poll, and
// fal.queue.result(modelId, { requestId: request_id }) once completed;
// confirm the exact method names against the installed package version,
// since client APIs move independently of this skill.
```

Treat a provider-reported failure as a real failure to report with the request
ID and error payload -- do not silently retry with different input hoping one
succeeds, and do not fabricate a result if generation fails. A local timeout
is not a provider failure: rejoin and poll the existing request first.

## Sending a local file as input

Input fields like `image_url` take a URL, not a filesystem path. A local
file -- most importantly a character reference image produced per
`video-cinematography` -- has to be uploaded first, and the client returns
the CDN URL to pass in:

```js
import { readFile } from "node:fs/promises";

const bytes = await readFile("work/productions/<slug>/characters/<name>.png");
const referenceUrl = await fal.storage.upload(
  new File([bytes], "<character-name>.png", { type: "image/png" }),
);

const result = await fal.subscribe("<resolved-model-id>", {
  input: { prompt: "...", image_url: referenceUrl },
  logs: true,
});
```

Upload each reference image once and reuse the returned URL across every
shot that conditions on it, rather than re-uploading per call. Record that
URL in `production.json` next to the character's local path (see
`video-creation`), so a later revision conditions on the same reference
instead of regenerating one that drifts.

Field names vary by model -- some take `image_url`, others a differently
named field or an array of references. Copy the exact shape from the
model's own reference page. Some models also accept a `data:` URI for
inline content, but an uploaded URL is the form that works everywhere.

## Output handling

- `result.data` (or the completed job's payload) carries one or more hosted
  URLs, not local files. Download every asset you intend to keep into `work/`
  (or the current stage's execution folder, per `video-creation`'s
  workflow-stage rules) before the session ends -- a hosted URL is not a
  durable artifact.
- Use stable, descriptive filenames per shot/asset (`shot-02-hook.mp4`,
  `voice-en-v1.wav`), and record them in `production.json` alongside the
  model ID and input that produced them.
- Verify what was actually generated before treating a job as done: check
  duration, dimensions, and (for video) that the file is not silently
  truncated or corrupt, with `ffprobe` -- a completed job status is not proof
  the asset is usable. This is the per-clip receipt, not the final full QA
  suite: do not create a contact sheet or run the final quality report for a
  normal preview.

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
  decide between this skill and `google-ai` per model -- fal.ai hosts many
  third-party models; use `google-ai` instead when the brief specifically
  calls for a Google-native model (Gemini image generation, Veo).
- Use `video-editing` to assemble generated clips, narration, and music into
  a final cut.
- Use `video-quality` before presenting any version as complete.

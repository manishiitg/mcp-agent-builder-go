---
name: minimax-h3-video
description: "Plan and generate Video Studio's MiniMax H3 Max routes through fal.ai: Text-to-Video for prompt-only shots, Image-to-Video for approved first/last-frame control, and Reference-to-Video for identity and continuity. Read with video-provider-capabilities and fal-ai before any paid H3 Max call."
---

# Use MiniMax H3 Max deliberately

Read `video-provider-capabilities`, `fal-ai`, `video-cinematography`, and the
local prompting reference at `references/fal-h3-max-prompting.md`, then the
live machine-readable guide for the selected route before every paid call:

- `https://fal.ai/models/minimax/h3-max/text-to-video/llms.txt`;
- `https://fal.ai/models/minimax/h3-max/image-to-video/llms.txt`;
- `https://fal.ai/models/minimax/h3-max/reference-to-video/llms.txt`.

## Use the guarded H3 runner

For every paid H3 Max request, use `scripts/h3-max-runner.mjs`; do not write a
new inline Fal client call or use `fal.subscribe()`. The runner permits only
the three approved H3 Max endpoints, validates route-specific inputs before a
paid submit, saves the request ID and resolved non-secret input immediately,
and emits JSON-lines console progress plus a sibling `*.log.jsonl` file.

Write a non-secret job JSON with `endpoint`, `prompt`, and the selected route's
controls. Then validate, submit once, and wait/rejoin the *same* state file:

```bash
BASE="work/<production>/h3/<shot-id>"
node .claude/skills/minimax-h3-video/scripts/h3-max-runner.mjs validate --input "$BASE/job.json"
node .claude/skills/minimax-h3-video/scripts/h3-max-runner.mjs submit --input "$BASE/job.json" --state "$BASE/request.json"
node .claude/skills/minimax-h3-video/scripts/h3-max-runner.mjs wait --state "$BASE/request.json" --output "$BASE/candidate.mp4" --timeout-seconds 900
```

`SECRET_FAL_KEY`, `SECRET_FAL_AI_KEY`, `FAL_KEY`, or `FAL_AI_KEY` must be
present in the environment. Never put a secret in the job or state JSON. If a local wait ends, run `status` or `wait`
again with the existing state file; never submit a replacement. Read the
runner's `request.json` and `request.json.log.jsonl` for the exact endpoint,
resolved input, request ID, queue states, provider logs, result timings, and
download path. After its `completed` event, run the normal file receipt
(`ffprobe`, stable-frame inspection, then `show_video`) before accepting it.

Use H3 Max only. Do not substitute standard H3, another H3 Max route, or
another provider.

Every H3 Max route generates 5–15-second clips at `480P` or `768P`. Video
Studio defaults to `resolution: "480P"` and
`prompt_expansion_mode: "balanced"`. Fal recommends balanced for H3 Max;
quality can spend up to 30 seconds rewriting a prompt, so use it only when the
user explicitly requests that slower treatment. Use 768P only when the user
explicitly requests and approves the higher cost; do not offer or use 2K or
4K. Verify live rates before estimating a run.

Treat an explicit user per-clip duration as a hard creative constraint, not a
suggestion to optimise away. For example, when the user asks for 5-second
clips, request 5 seconds for every applicable H3 generation; do not replace
them with longer 10–15-second takes to reduce cost or seam count. Recommend a
duration tradeoff only when the user has not fixed one.

## Prompt from an H3 Max shot contract

Before writing an H3 prompt, write `shot-contracts/<shot-id>.md` and present it
with `show_document` for review. It is a required pre-generation contract, not
a post-hoc explanation. Include route, exact duration, aspect ratio, each
reference asset and its job, subject identity and invariants, and the sections
below. Then apply the eight rules in `references/fal-h3-max-prompting.md`. Do
not turn a user's precise camera or dialogue direction into a looser generic
cinematic prompt.

### Required shot-contract detail

- **Purpose, success, and priority:** name the story/emotional beat, one
  observable success criterion, and a ranked conflict policy. State what wins
  if H3 cannot satisfy every request—for example: `1. identity and exact
  dialogue/lip-sync; 2. true-POV framing; 3. continuity; 4. performance;
  5. set dressing.` Never let the model silently choose the tradeoff.
- **Subject and performance:** identity reference, face/hair/wardrobe, body
  position, gaze/eyeline, expression at the beginning and end of each timed
  beat, exact emotional turn, hand/prop action, and things the subject must
  not do. Describe an expression concretely: for example, "brow slightly
  drawn, lips relaxed and closed, concern in the eyes; no smile" rather than
  "warm and patient."
- **Place and set dressing:** foreground, midground, and background; named
  objects, their screen side/relative placement, practical lights, time of
  day, weather, lighting direction, palette, texture, and atmosphere. Identify
  which details come from the approved location reference and which must remain
  absent.
- **Camera:** shot scale, camera height, side, distance, viewpoint, lens
  character when relevant, composition, focus behavior, movement/vector or
  explicit static lock, and prohibited reframing. A true POV must say whose
  eyes, what body/reflection/shadow must never appear, and the relative
  eye-level and distance to each visible subject.
- **Timed action and sound:** a `[0–N seconds]` list covering visual action,
  expression, camera, and sound. Record exact dialogue, speaker, language,
  delivery, native lip-sync, voice/performance reference, ambience, foley,
  music, and explicit exclusions. Mark whether dialogue is verbatim and
  confirm that its words fit the clip duration naturally before generating.
- **Continuity and negatives:** approved reference roles, the prior clip's
  exact outgoing state and required incoming match, plus named visual/audio
  failures to prevent—text, subtitles, watermark, smile, body parts, extra
  people, a wrong prop, camera drift, an unwanted cut, or a change of location.
- **Approval, retry, and delivery:** exact approved source paths and a
  `do-not-regenerate/change` marker for protected references; aspect ratio and
  safe-area/text/caption rules; preview versus final status; the approved cost
  and retry allowance; and the frames/audio checks that decide acceptance.

The visible contract is the approval boundary: do not send a paid request until
the user approves it, unless the user has explicitly asked to skip that review
for the current shot.

For a single static dialogue beat, use one timed block rather than a long
unstructured paragraph. State the speaker, exact spoken line, performance,
room tone, and prohibited sound separately. Do not put a second, competing
gesture or action into a tightly constrained performance unless the user asked
for it. If the required dialogue cannot be spoken naturally in the requested
duration, flag that before spending rather than silently rushing, truncating,
or inventing new words.

## Choose the route by control need

1. **Prompt-only establishing shot:** use
   `minimax/h3-max/text-to-video` only when no approved visual, motion, audio,
   identity, or continuity reference needs to control the result. Set the
   delivery `aspect_ratio` explicitly (21:9, 16:9, 4:3, 1:1, 3:4, or 9:16).
2. **Opening/closing-frame control:** use
   `minimax/h3-max/image-to-video` when an approved `image_url` must define
   the first frame; add `end_image_url` only when a specific ending frame must
   be reached. Output follows the supplied start image's aspect ratio. Do not
   use this route to fabricate a bridge between two generated clips.
3. **Identity, motion, voice, or continuation:** use
   `minimax/h3-max/reference-to-video` whenever a shot needs subject/style
   locking, a predecessor's motion or performance, reference audio, or any
   multi-asset conditioning. For a continuation, send the accepted immediate
   predecessor in `reference_video_urls` as Video 1 and describe the exact
   handoff from its final stable motion. Reference each asset in prompt order:
   Image 1, Video 1, Audio 1 — never arbitrary JSON labels.

Reference-to-Video accepts at most 9 images, 3 videos, and 3 audio clips, with
12 files total. Reference videos and audio are each 2–15 seconds with a
15-second combined limit; audio cannot be the only reference, so accompany it
with an image or video. Do not use Image-to-Video's prompt-only fallback when
Text-to-Video states the intent more accurately, and do not use an edit route
as a fallback for a broken source shot.

## Assign every reference one job

Build a reference ledger before the request. For each image, video, or audio
input, record its order, accepted media constraints, and purpose: subject
identity, start composition, end state, motion, camera language, environment,
voice, music, ambience, or style. Use the exact positional tokens and ordering
shown by the live endpoint schema.

Current H3 documentation describes a unified multimodal context with images,
video clips, and audio tracks, but its published counts and combined-duration
limits are discovery hints rather than a frozen request contract. Validate them
against the selected endpoint immediately before generation. Audio references
may require an accompanying image or video; never send audio alone unless the
live schema explicitly permits it.

Use start and end frames for temporal boundaries, not as substitutes for a
subject reference. Use reference media for identity or performance, not as an
unexplained pile of examples. Remove near-duplicates and conflicting donors.

## Prompt motion, camera, and sound

State one principal action, its start and end state, environment, camera
framing and movement, lighting, and continuity obligations. Refer to supplied
media using the endpoint's exact tokens rather than prose such as “the third
image” when tokens are available.

H3 can generate native stereo audio on supported routes. Specify dialogue,
speaker, language, delivery, foley, ambience, and score separately. If using a
voice or performance reference, say what transfers and what must not transfer.
Do not trust native speech for exact regulated or brand-critical wording
without transcription and human review.

For localized editing, name the smallest intended change and list invariants:
subject identity, wardrobe, framing, camera path, timing, background, lighting,
and audio. Treat every edit as a new candidate that must be reviewed; do not
overwrite the last accepted version.

## Continue and review

For a continuous sequence, use Reference-to-Video from the accepted immediate
predecessor as Video 1. Carry forward the accepted output, endpoint route,
reference order, subject state, screen direction, lighting, audio bed, and the
precise next camera/action handoff. Prefer this generation-time continuity over
manual stitching or a bridge workaround. H3 Max produces a new bounded file;
it does not append footage to its predecessor.

Persist the queue request ID immediately and rejoin it after timeouts. Download
the result once and perform a clip receipt: `ffprobe` the actual file and
inspect its stable opening and ending frames. Call `show_video` for each
reviewable candidate. Do not run a full contact-sheet, black/freeze, audio,
prompt, and seam audit for every preview. If a visible boundary is wrong,
regenerate or redesign the successor through Reference-to-Video with a more
specific prompt/reference set; do not hide it with a bridge clip, crossfade,
blend, reframe, zoom, or other creative FFmpeg repair. The full inspection is
run once on the final direct-concatenated delivery MP4.

## Treat speed and playback honestly

H3 Max may report a very short `inference` duration, but that is model time
only. Do not turn it into a fixed end-to-end promise: queueing, reference-media
upload, output download, local validation, and human review also take time.
Record and report those intervals separately.

The API returns a completed MP4. It does not stream partial playable footage,
append a continuation onto the predecessor file, or guarantee that a successor
will be ready before the current clip ends. A UI may begin playing each
completed clip immediately and can provide gapless playlist playback only when
the next completed clip is already buffered. For Reference-to-Video, wait until
the immediate predecessor exists; do not pretend the next request can start
from footage that has not been returned. Video Studio requires a lightweight
boundary receipt before accepting a successor, and one full QA pass only after
the final assembly exists.

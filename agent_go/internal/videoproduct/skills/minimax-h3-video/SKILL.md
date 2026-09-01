---
name: minimax-h3-video
description: Plan and generate Video Studio's MiniMax H3 Max routes through fal.ai: Text-to-Video for prompt-only shots, Image-to-Video for approved first/last-frame control, and Reference-to-Video for identity and continuity. Read with video-provider-capabilities and fal-ai before any paid H3 Max call.
---

# Use MiniMax H3 Max deliberately

Read `video-provider-capabilities`, `fal-ai`, `video-cinematography`, and the
live machine-readable guide for the selected route before every paid call:

- `https://fal.ai/models/minimax/h3-max/text-to-video/llms.txt`;
- `https://fal.ai/models/minimax/h3-max/image-to-video/llms.txt`;
- `https://fal.ai/models/minimax/h3-max/reference-to-video/llms.txt`.

Use H3 Max only. Do not substitute standard H3, another H3 Max route, or
another provider.

Every H3 Max route generates 5–15-second clips at `480P` or `768P`. Video
Studio defaults to `resolution: "480P"` and
`prompt_expansion_mode: "balanced"`. Fal recommends balanced for H3 Max;
quality can spend up to 30 seconds rewriting a prompt, so use it only when the
user explicitly requests that slower treatment. Use 768P only when the user
explicitly requests and approves the higher cost; do not offer or use 2K or
4K. Verify live rates before estimating a run.

## Choose the route by control need

1. **Prompt-only establishing shot:** use
   `minimax/h3-max/text-to-video` only when no approved visual, motion, audio,
   identity, or continuity reference needs to control the result. Set the
   delivery `aspect_ratio` explicitly (21:9, 16:9, 4:3, 1:1, 3:4, or 9:16).
2. **Opening/closing-frame control:** use
   `minimax/h3-max/image-to-video` when an approved `image_url` must define
   the first frame; add `end_image_url` only when a specific ending frame must
   be reached. Output follows the supplied start image's aspect ratio. A
   first/last-frame seam bridge remains a separately approved repair after a
   direct-cut seam proof visibly fails; it is not a way to conceal unrelated
   footage.
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

For a continuous sequence, prefer a supported reference-video or boundary-frame
chain over independent prompts. Carry forward the accepted output, endpoint
route, reference order, seed when available, subject state, screen direction,
lighting, and audio bed. A direct-cut seam must be previewed on its actual
boundary frames. If it visibly jumps, mark it failed — never call a hard cut or
crossfade a pass — and offer the first/last-frame bridge as a separately
approved paid repair. Do not create extra clips when one longer supported take
or a motivated in-model cut will satisfy the brief.

Persist the queue request ID immediately and rejoin it after timeouts. Download
the result once, run technical, identity, motion, audio, prompt, and seam checks,
then call `show_video` for every candidate clip the user needs to review. Never
assemble an unreviewed clip.

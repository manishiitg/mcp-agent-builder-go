---
name: minimax-h3-video
description: Plan and generate Video Studio's MiniMax H3 routes through fal.ai: Reference-to-Video for anchors and normal continuations, and Image-to-Video with first/last frames only for a user-approved failed-seam bridge. Read with video-provider-capabilities and fal-ai before any paid H3 call.
---

# Use MiniMax H3 deliberately

Read `video-provider-capabilities`, `fal-ai`, `video-cinematography`, and the
live schema for the selected route before every paid call:

- `https://fal.ai/models/minimax/h3/reference-to-video/api` for anchors and
  normal continuations;
- `https://fal.ai/models/minimax/h3/image-to-video/api` only for a
  user-approved bridge after a direct-cut seam proof visibly fails.

Do not substitute H3 Max, another H3 route, or another provider.

The current H3 Reference-to-Video schema accepts `reference_image_urls`,
`reference_video_urls`, and `reference_audio_urls` as arrays of URL strings.
Refer to them in the prompt by their ordered modality name — Image 1, Video 1,
Audio 1 — never by JSON labels. It accepts at most 9 images, 3 videos, and 3
audio clips, with at most 12 files total; video and audio references are each
2–15 seconds with a 15-second combined limit. Output clips are 5–15 seconds
at 24 FPS. Video Studio defaults to `resolution: "480P"` and
`prompt_expansion_mode: "quality"`. Use 768P only when the user explicitly
requests and approves the higher cost; do not offer or use 2K or 4K. Verify
live rates before estimating a run.

## Choose the route by control need

- Use `minimax/h3/reference-to-video` for every anchor and continuation. For
  an anchor, supply the approved character/style reference images. For a
  continuation, also supply the accepted predecessor as Video 1 and describe
  the immediate handoff from its final motion.
- Use `minimax/h3/image-to-video` only to repair a *visibly failed* direct
  seam. Upload the selected predecessor end frame as `image_url` and the
  successor start frame as `end_image_url`; both must be same-aspect,
  reviewable stable frames. Make one 5–15-second bridge candidate and review
  both inherited joins. Preserve dialogue with the approved predecessor and
  successor audio edit; do not assume newly generated bridge audio is usable.
- Do not route a new scene to text-to-video or image-to-video. The H3
  reference route accepts the approved images directly and keeps one endpoint
  contract for the whole production.
- Do not use an edit route as a fallback. A bridge does not excuse a broken
  source shot or an unrelated cut: regenerate the affected H3 shot from its
  approved reference manifest when the actual continuity state is wrong.

Do not assume the text, image, reference, and edit routes share field names,
reference limits, duration choices, aspect ratios, or audio controls.

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

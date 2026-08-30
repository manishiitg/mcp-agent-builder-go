---
name: minimax-h3-video
description: Plan and generate MiniMax H3 video effectively through current fal.ai Hailuo 03 endpoints. Use when a production selects or considers MiniMax H3 for text-to-video, image-to-video, first/last-frame video, multimodal reference-to-video, native stereo audio, dialogue or voice transfer, localized edits, or continuity between clips. Read with video-provider-capabilities and fal-ai before any paid H3 call.
---

# Use MiniMax H3 deliberately

Read `video-provider-capabilities`, `fal-ai`, `video-cinematography`, and the
selected endpoint's live fal schema first. Use `https://fal.ai/minimax-h3` as
the current official discovery starting point.

MiniMax H3 is the product name, but the current fal endpoint namespace is
`minimax/hailuo-03/...`. Never invent a `minimax/h3` endpoint from the product
name. Resolve and record the exact endpoint ID, mode, schema, price, defaults,
and limits in the production's capability record.

Current H3 output is 2K at 24 FPS for 5–15 second clips. Do not describe an
H3 route as 768p unless the live endpoint schema explicitly changes this.
Its live fal billing unit may be `compute seconds`; that is not output seconds,
so never derive a dollar-per-video-second or a total from clip duration alone.

## Choose the route by control need

- Use `minimax/hailuo-03/text-to-video` for a new scene without an approved
  appearance or boundary frame.
- Use `minimax/hailuo-03/image-to-video` when the opening composition is
  approved. Supply a last frame only when the live route exposes it and the
  requested transition is physically plausible.
- Use `minimax/hailuo-03/reference-to-video` when identity, performance,
  movement, voice, environment, or style must come from several references.
- Use an edit route only when the live catalog exposes one that accepts the
  current source. Give it a narrow change request and explicitly preserve
  everything else.

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

For a continuous sequence, prefer a supported reference-video, edit, or
boundary-frame chain over independent prompts. Carry forward the accepted
output, endpoint route, reference order, seed when available, subject state,
screen direction, lighting, and audio bed. Do not create extra clips when one
longer supported take or a motivated in-model cut will satisfy the brief.

Persist the queue request ID immediately and rejoin it after timeouts. Download
the result once, run technical, identity, motion, audio, prompt, and seam checks,
then call `show_video` for every candidate clip the user needs to review. Never
assemble an unreviewed clip.

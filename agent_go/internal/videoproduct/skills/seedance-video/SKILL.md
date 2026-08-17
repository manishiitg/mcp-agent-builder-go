---
name: seedance-video
description: Plan and generate Seedance video effectively through current fal.ai ByteDance endpoints. Use when a production selects or considers Seedance for text-to-video, image-to-video, first/last-frame control, multimodal reference-to-video, multiple character or product images, reference video or audio, native synchronized audio, multi-shot scenes, video editing, or extension. Read with video-provider-capabilities and fal-ai before any paid Seedance call.
---

# Use Seedance's multimodal controls

Read `video-provider-capabilities`, `fal-ai`, `video-cinematography`, and the
selected endpoint's live fal schema first. Current official starting points:

- `https://fal.ai/models/bytedance/seedance-2.0/text-to-video/api`
- `https://fal.ai/models/bytedance/seedance-2.0/image-to-video/api`
- `https://fal.ai/models/bytedance/seedance-2.0/reference-to-video/api`

Resolve Standard/Fast and later Seedance families independently. A differently
numbered route can be optimized for editing or extension rather than being a
strict quality upgrade. Record the exact endpoint and current schema.

## Choose the narrowest capable route

- Use text-to-video when the scene needs no visual or audio anchor.
- Use image-to-video for an approved opening frame. Use its ending-frame field
  only when a designed final composition is important.
- Use reference-to-video when the request needs several images, a source video,
  audio guidance, identity/product consistency, or reference-driven editing.
- Use a dedicated edit or extension route when exposed by the live catalog;
  never simulate extension by silently generating an unrelated clip.
- Choose Fast for iteration or budget when its resolution ceiling is enough;
  choose Standard when the required resolution/quality justifies it.

Current Seedance 2 reference routes advertise multiple images, videos, and
audio files with a combined total cap. These are discovery hints only. Copy the
selected endpoint's exact per-modality counts, sizes, formats, duration limits,
and total cap into the capability record before constructing a request.

## Build a reference manifest

Order every modality once and preserve the order:

```text
@Image1 = approved recurring character
@Image2 = approved wardrobe/product
@Video1 = motion and camera reference
@Audio1 = approved dialogue or soundtrack timing
```

Use the token spelling shown by the live endpoint. In the prompt, say what to
borrow from each reference. Do not attach a video when only its color palette
is needed, or an audio file when native ambience is enough. When audio input
requires at least one image or video, enforce that precondition locally.

For multiple character views, prefer complementary angles and clear identity
evidence. Keep style donors separate from identity donors. Do not let a style
reference silently replace the approved subject, wardrobe, product, or place.

## Direct motion and internal shots

For image-to-video, prompt the temporal change: subject action, camera path,
physics, timing, end state, and sound. Do not redescribe the static image.

Seedance can create natural cuts inside one generation, but current routes may
express them through the prompt rather than a structured shot array. Use the
exact live schema. Write shot transitions explicitly while preserving the same
subjects and scene state; never invent `multi_prompt` because another model
uses that field.

Prefer one generation for a scene that fits the route's live duration limit.
Use a reference video or supported extension/edit route for continuation; use
the accepted final frame as the next start frame only when that is the best
available continuity mechanism.

## Audio and duration

When native audio is enabled, prompt dialogue, speaker, ambience, effects, and
music separately and concretely. Use audio references for timing, voice/style,
or soundtrack matching only when the selected route supports them. Native
audio and reference audio are different controls; do not translate one into
the other's field.

Use explicit duration when timing must align with narration or an edit. Use
`auto` only when the user accepts model-selected timing. Record any returned
seed, but do not promise perfect determinism.

## Execute and review

Upload reusable references once, save the queue request ID, and rejoin the same
job after a local timeout. Check provider adjustments before accepting the
result. Verify the downloaded clip technically and visually against every
named reference, audio requirement, and boundary state. Call `show_video` for
each clip and wait for approval before assembly.

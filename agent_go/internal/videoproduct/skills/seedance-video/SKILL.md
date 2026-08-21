---
name: seedance-video
description: Plan and generate Seedance video effectively through current Seedance 2.0 and 2.5 endpoints on fal.ai, or supported direct Seeddance models through its separately authenticated API. Use when a production selects or considers Seedance for text-to-video, image-to-video, first/last-frame control, multimodal reference-to-video, multiple character or product images, reference video or audio, native synchronized audio, multi-shot scenes, editing, extension, or fewer longer clips. Read with video-provider-capabilities, video-cinematography, and the selected provider skill before any paid call.
---

# Use Seedance's multimodal controls

Read `video-provider-capabilities`, `video-cinematography`, and exactly one
provider skill before constructing a request: `fal-ai` for fal-hosted routes or
`seeddance-api` for the independent direct service. fal exposes Seedance 2.5;
as checked on 2026-08-17, Seeddance direct does not. Provider credentials,
endpoint IDs, schemas, jobs, and billing are not interchangeable.

Current official fal starting points:

- `https://fal.ai/models/bytedance/seedance-2.0/text-to-video/api`
- `https://fal.ai/models/bytedance/seedance-2.0/image-to-video/api`
- `https://fal.ai/models/bytedance/seedance-2.0/reference-to-video/api`
- `https://fal.ai/models/bytedance/seedance-2.5/text-to-video/api`
- `https://fal.ai/models/bytedance/seedance-2.5/image-to-video/api`
- `https://fal.ai/models/bytedance/seedance-2.5/reference-to-video/api`

## Resolve the version and live route

Resolve Standard, Fast, and every numbered Seedance family independently. A
differently numbered route can optimize different controls rather than being a
strict quality upgrade. Record the provider, exact endpoint, current schema,
price, and account access in a separate capability record.

As checked on 2026-08-17, fal exposes these live Seedance 2.5 endpoint IDs:

```text
bytedance/seedance-2.5/text-to-video
bytedance/seedance-2.5/image-to-video
bytedance/seedance-2.5/reference-to-video
```

Older search results and editorial pages may still call 2.5 “coming.” The live
model page and its generated request schema win over stale index text. Recheck
the exact page at selection time instead of copying these fields by memory.

Current fal 2.5 schemas expose `auto` or 4–30-second duration, synchronized
native audio through `generate_audio`, and only 480p and 720p resolution enums.
Do not promise 1080p or 4K from Seedance 2.5 merely because Seedance 2.0 offers
them. Text and reference routes expose `auto`, 21:9, 16:9, 4:3, 1:1, 3:4, and
9:16 aspect ratios. Image-to-video derives aspect ratio from the image and can
accept `end_image_url`. If landing-page prose and the live schema disagree,
record the discrepancy and use only values the callable schema accepts.

Seedance 2.0 remains a deliberate option: its Standard and Fast route families
have their own pricing and controls, and current Standard schemas can offer 4K.
Choose 2.0 when its resolution, Fast economics, account access, or proven
behaviour fits the shot better; choose 2.5 when its longer takes or larger
reference context materially reduce seams.

## Choose the narrowest capable mode

- Use text-to-video when the scene needs no visual or audio anchor.
- Use image-to-video for an approved opening frame. Supply the ending frame
  only when the model and provider expose that field and a designed boundary
  composition matters.
- Use reference-to-video for several images, motion/video guidance, audio
  guidance, identity or product consistency, or reference-driven editing.
- Use a dedicated edit or extension route when exposed by the selected live
  catalog; never simulate extension with an unrelated generation.

Current fal 2.5 reference schemas accept up to 30 images, 10 video references,
and 10 audio references, with no more than 50 combined. Each video or audio
reference is 1.8–30.2 seconds and the combined duration for each modality is at
most 30.2 seconds. Video references are limited to 200 MB each, 24–60 FPS,
300–6000 pixels per side, and aspect ratio 0.4–2.5; audio references are limited
to 15 MB each. Audio requires at least one image or video. Treat all of these as
dated discovery hints: copy the exact selected provider schema into the
capability record before constructing a request.

## Build one reference manifest

Order every modality once and preserve the order:

```text
@Image1 = approved recurring character
@Image2 = approved wardrobe or product
@Video1 = motion and camera reference
@Audio1 = approved dialogue or soundtrack timing
```

Current fal 2.5 reference prompts use `@Image1`, `@Video1`, and `@Audio1`.
Seeddance direct uses ordered media arrays and the same conceptual references;
follow its current documentation rather than inventing token syntax. Say what
to borrow from every reference. Keep identity donors separate from style
donors, use complementary character views, and never let a style reference
silently replace the approved subject, wardrobe, product, or place.

## Direct motion and internal shots

For image-to-video, prompt the temporal change: subject action, camera path,
physics, timing, end state, and sound. Do not spend the prompt redescribing the
static image.

Seedance can create natural cuts inside one generation, but a route may express
them through prose rather than a structured shot array. Use the exact live
schema. Write shot transitions explicitly while preserving subject identity,
screen direction, geography, light, and scene state; never invent `multi_prompt`
because another model uses that field.

Prefer one generation for a continuous scene that fits the route's duration.
For long-form work, a reviewed 30-second 2.5 take can replace two 15-second
calls, reducing both spend and a visible seam. Longer is not automatically
better: split when the scene contains an intentional cut, the model loses
control over the full take, or a failed 30-second rerun costs more than two
targeted segments. Use accepted boundary frames, reference video, or a supported
edit/extension route to continue scene state.

## Audio, execution, and review

When native audio is enabled, prompt dialogue, speaker, ambience, effects, and
music separately and concretely. Native audio and reference audio are different
controls. Use explicit duration when timing must align with narration or an
edit; use `auto` only when the user accepts model-selected timing.

Upload reusable references once. Persist the provider's request or task ID
before waiting, then rejoin that job after a local timeout; never create a
duplicate paid request merely because polling failed. Record the returned seed
when present without promising perfect determinism.

Download and probe each clip, then review it against every named reference,
audio requirement, and incoming/outgoing boundary state. Call `show_video` for
each distinct MP4 immediately. When a generation returns several clips, call
`show_video` once per clip rather than showing only the final stitch.

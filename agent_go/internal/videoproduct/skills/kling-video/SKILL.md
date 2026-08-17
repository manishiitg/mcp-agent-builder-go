---
name: kling-video
description: Plan and generate Kling video effectively through current fal.ai Kling endpoints. Use when a production selects or considers Kling for text-to-video, image-to-video, reference-to-video, first/last-frame interpolation, structured multi-shot prompts, recurring characters or objects, style references, motion transfer, or native audio. Read with video-provider-capabilities and fal-ai before any paid Kling call.
---

# Use Kling deliberately

Read `video-provider-capabilities`, `fal-ai`, `video-cinematography`, and the
selected endpoint's live fal schema first. Current official starting points:

- `https://fal.ai/models/fal-ai/kling-video/v3/pro/image-to-video/api`
- `https://fal.ai/models/fal-ai/kling-video/v3/pro/text-to-video/api`
- `https://fal.ai/models/fal-ai/kling-video/o3/4k/reference-to-video/api`

Treat these URLs as discovery sources, not frozen contracts. Resolve and record
the exact endpoint ID, tier, schema, price, and defaults in the production's
capability record.

## Choose the endpoint by control need

- Use text-to-video when no existing appearance or boundary frame must be
  preserved.
- Use image-to-video when the opening composition is approved. Add the ending
  frame only when the live route supports it and the transition is plausible.
- Use reference-to-video when recurring characters, objects, motion donors, or
  style references matter more than a single opening frame.
- Use a Pro/high-resolution route only when its output quality is needed; do
  not pay for it merely because the name sounds newer.
- Do not assume V3 and O3 routes share fields, defaults, reference limits,
  audio behavior, or tiers.

## Map references to their actual job

Use `elements` for subject identity or reusable objects when the selected
schema defines it. Use the route's style/reference-image array for appearance,
palette, or environment guidance. Keep arrays ordered and reference them
positionally as the live examples specify, such as `@Element1` or `@Image1`.

For an image-set element, provide one clean frontal anchor plus genuinely
useful alternate views; do not fill every slot with near-duplicates. A video
element can carry motion and, on routes that support it, voice binding. Never
attempt voice binding to an image element when the schema restricts it to
video elements.

Use a start frame for composition, an end frame for the desired final state,
and elements for identity. These controls are complementary only when the
exact endpoint permits the combination. Record every attached reference and
what it is meant to preserve.

## Use multi-shot for intentional cuts

When the endpoint exposes `multi_prompt`, send the structured array rather than
numbered prose. Provide exactly one of `prompt` or `multi_prompt` when the
schema makes them exclusive, set the required shot mode, and validate that the
shot durations match the total duration.

```json
{
  "multi_prompt": [
    {"prompt": "Wide: @Element1 enters the approved location.", "duration": "4"},
    {"prompt": "Medium tracking shot: @Element1 continues the same action.", "duration": "4"}
  ],
  "shot_type": "customize"
}
```

Do not use multi-shot merely because a script has multiple sentences. Use one
prompt for a continuous take. Use multi-shot for a motivated camera cut that
still preserves identity, wardrobe, location, lighting, and action state.

## Prompt and audio effectively

For every shot state the subject token, one principal action, environment,
camera framing/movement, lighting, end state, and audio intent. Keep positional
tokens exact. Describe motion rather than redescribing a supplied start frame.

Check `generate_audio` on the exact endpoint: current Kling routes do not all
share the same default. When audio is enabled, specify dialogue, ambience, and
effects explicitly; otherwise assemble deterministic audio later. Do not rely
on native speech for exact regulated or brand-critical wording without review.

## Continue and review

Prefer a supported reference-video/motion-transfer or boundary-frame chain over
a fresh independent call. Preserve the same model route, element order, seed
when available, and approved references across a continuous arc.

Persist the queue request ID immediately and rejoin it after timeouts. After
download, run the standard per-clip technical, identity, motion, audio, and seam
checks, then call `show_video` for that clip. Never assemble it while review is
pending.

---
name: video-shot-generation
description: Generate consistent AI video shots with strong prompts, reference images, character and product continuity, controlled speech, and cost-aware retries. Use when the user needs new AI footage, talking presenters, product scenes, B-roll, image-to-video, or revisions to generated shots.
---

# Generate AI video shots

Generate short, controllable shots and assemble them later. Do not ask one generation to produce a complete multi-scene edited video.

## Lock appearance with references

- Create or select a neutral reference image for each recurring person, product, and important set.
- Use the image for appearance and the prompt for performance. Keep anchor faces calm, front-facing, evenly lit, and free of baked-in emotion or action.
- Ensure the written description agrees with every reference. Conflicting wardrobe, color, age, or set details cause drift.
- Prefer asset/reference-image conditioning when the provider supports it. Use a start frame only when the exact opening pose or camera composition matters.
- For multiple people, reference both people or use a clear two-shot reference. Review continuity for each person separately.

## Write one prompt per shot

Use this order:

`STYLE + CHARACTER/PRODUCT + SHOT AND FRAMING + ACTION + DIALOGUE/DELIVERY + AMBIENCE + EXCLUSIONS + FORMAT`

Repeat the style and identity blocks verbatim across related shots. Change only the framing, action, and dialogue needed for that beat.

Always specify:

- one subject action and one camera behavior;
- setting, lighting, mood, and intended realism;
- aspect ratio and centered/safe composition;
- either a deliberate camera movement or a static camera;
- `No captions, subtitles, text, logos, or watermarks` when text will be added later.

For dialogue, put spoken words in double quotes and put delivery direction immediately before them, for example: `She says, (warm and assured) "..."`. Do not use separate TTS when the generated presenter already has good native lip-synced speech.

Avoid vague instructions, conflicting styles, multiple scene changes, tiny precise gestures, generated UI, and readable on-screen text.

## Preserve continuity

- Prefer separate referenced shots joined with hard cuts for talking heads and dialogue.
- Do not use last-frame chaining for fast mouth motion; identity drift compounds across hops.
- Use frame chaining only for slow continuous movement where motion continuity is more important than perfect identity.
- For shot/reverse-shot dialogue, lock both characters and cut between views.
- Anchor recurring set pieces as well as faces when the background must remain stable.

## Run generations safely

1. Read credentials only from environment variables. Never print or persist their values.
2. Store prompts and generation settings in `work/production.json` or a reproducible script before calling a provider.
3. Generate the minimum number of shots at draft quality first. Reserve higher-cost quality for selected finals.
4. Capture the first successful response and save it immediately under `work/shots/`.
5. Retry a transient empty response or provider error at most once unless the user asks to keep trying.
6. Never regenerate a usable clip merely to obtain metadata; inspect the saved file locally.

Use current provider documentation already available in the environment rather than guessing an API or model name. Prefer a fast model with reference-image support for drafts.

## Check each shot immediately

Before generating the next expensive variant, sample the start, middle, and end frames and verify:

- identity, product, wardrobe, and set continuity;
- natural hands, faces, mouth motion, and camera motion;
- correct speech and usable audio;
- no garbled text, watermark, black frame, or frozen opening;
- enough clean handles for trimming.

Record acceptance or the exact regeneration reason in `work/production.json`.

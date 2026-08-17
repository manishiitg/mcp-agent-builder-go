---
name: gemini-omni-video
description: Plan and generate or edit video effectively with Google Gemini Omni Flash through current fal.ai endpoints. Use when a production selects or considers Gemini Omni, Google Omni, or Gemini Omni Flash for text-to-video, image-to-video, reference-to-video, conversational video editing, localized revisions, multimodal conditioning, native audio, or iterative scene continuity. Read with video-provider-capabilities and fal-ai before any paid call.
---

# Use Gemini Omni Flash deliberately

Read `video-provider-capabilities`, `fal-ai`, `video-cinematography`, and the
selected endpoint's live fal schema first. Current official starting points:

- `https://fal.ai/models/google/gemini-omni-flash`
- `https://fal.ai/models/google/gemini-omni-flash/image-to-video`
- `https://fal.ai/models/google/gemini-omni-flash/reference-to-video/api`
- `https://fal.ai/models/google/gemini-omni-flash/edit`

This playbook covers the fal-hosted Google model. Use `fal-ai`, not the direct
`google-ai` provider skill, for these `google/gemini-omni-flash...` endpoints.
Resolve and record the exact route, live schema, price, defaults, and limits in
the production's capability record.

## Choose the route by intent

- Use `google/gemini-omni-flash` to create a new scene from text.
- Use `google/gemini-omni-flash/image-to-video` to animate an approved opening
  composition.
- Use `google/gemini-omni-flash/reference-to-video` when supported references
  must anchor subjects, objects, style, environment, or other scene details.
- Use `google/gemini-omni-flash/edit` for an iterative, localized revision of
  an accepted source video instead of regenerating the whole scene.

The product page describes broad text, image, audio, and video understanding,
but each route can expose a narrower input schema. The current reference route,
for example, documents an ordered `image_urls` input and zero-based prompt tags
such as `<IMAGE_REF_0>`. Treat that as a discovery hint: fetch the exact live
schema and use only its fields. Never invent video or audio parameters from the
landing-page description, and never translate zero-based tags into one-based
tokens used by another provider.

## Keep references explicit

Create an ordered reference ledger before generation. Record each input's
purpose and the exact prompt token that addresses it. Remove redundant or
contradictory references. If the live endpoint says references across multiple
videos are unsupported, do not work around the restriction by hiding several
videos in an undocumented field; select one source, extract purposeful frames,
or choose another model.

Validate duration, aspect ratio, media-size, and reference-count constraints
from the live route. Published reference-route values such as 3–10 second clips
and 16:9 or 9:16 output are capability-discovery hints, not constants to copy
blindly into every Gemini Omni request.

## Prefer conversational edits for narrow changes

Use the last accepted output as the edit source. Describe the smallest visible
change in one instruction, then name the invariants to preserve: identity,
wardrobe, background, composition, camera path, timing, lighting, dialogue,
and sound mix. A good edit asks to replace, remove, restyle, or adjust one thing;
it does not rewrite the entire creative brief.

Review each edit before using it as the next source. Preserve prior accepted
versions and the edit lineage so a failed turn can be rolled back. If repeated
edits introduce drift, restart from the last accepted source rather than the
latest degraded candidate.

## Prompt and review

For generation, state subject and reference tokens, one principal action,
environment, camera framing and movement, lighting, end state, and audio intent.
Request dialogue, ambience, effects, and music separately when the live route
supports native audio. Do not assume model reasoning will infer continuity that
is absent from the supplied source or references.

Persist the fal queue request ID immediately and rejoin it after timeouts.
Download each result once, run technical, identity, motion, audio, prompt, and
continuity checks, then call `show_video` for every generated or edited clip the
user should see. Treat billed input and output tokens or other live pricing
units as part of the approved cost estimate.

---
name: video-model-selection
description: Choose which fal.ai or Google Gemini/Veo model fits one shot's requirements -- text-to-video vs image-to-video, character/subject consistency, duration limits, native audio, cost. Use before the first generation call in a long-form production, and again whenever a shot's requirements differ from the ones already resolved. Read alongside fal-ai and google-ai, which own the client mechanics once a model is chosen; this skill owns the choice itself.
---

# Choosing a generation model

`fal-ai` and `google-ai` teach how to call a model once you've picked one.
This skill teaches how to pick it. Neither this skill nor either of those
pins a model catalog or a "best model for X" table -- both providers'
catalogs, capabilities, and pricing change independently and on their own
schedule, and a fixed ranking here would go stale exactly the way a fixed
model ID would.

## Resolve production-level unknowns before the first paid call

`video-creation` already asks one compact question only when a missing choice
would substantially change the result or cost (see "Understand the request").
A long-form AI-generated production has its own unresolved-question shape,
and getting it wrong is more expensive to unwind than a typography choice: a
wrong shot is a paid regeneration, not a free re-render. Before the first
generation call, resolve whatever the brief hasn't already answered:

- **Provider/model preference.** Does the user already know they want
  fal.ai, Google, or a specific model? If not, state the tradeoff briefly
  (see "Decide by requirement" below) rather than picking silently on their
  behalf for a paid call.
- **Character/subject consistency requirements.** Does any character,
  presenter, or product need to recognizably recur across multiple shots?
  This determines which models are even viable (see
  `video-cinematography`'s consistency section) and must be known before the
  first shot of that subject is generated, not discovered after three
  independently-drifting attempts.
- **Shot count vs. budget.** A 60-second production built from many short
  paid generations costs differently than one from a few longer ones. If the
  user hasn't stated a cost ceiling or a maximum acceptable number of paid
  generation calls, ask once, compactly -- do not silently generate an
  open-ended number of variants.
- **Regeneration tolerance.** If a shot doesn't match the brief on the first
  attempt, how many re-prompts is reasonable before stopping to ask the user
  rather than continuing to spend on the same shot? State a default (e.g.
  two re-prompts) and get confirmation rather than assuming unlimited
  budget for one stubborn shot.

Ask these as part of `video-creation`'s existing request-understanding pass,
not as a second separate interview later.
Write the resolved answers into `production.json` so a revision does not
re-ask what was already decided.

## Decide by requirement, not by brand loyalty

For each shot, resolve these dimensions before choosing a model. Get the
answers from the provider's own current model/capability listing (fal.ai's
model explorer, Google's Gemini API model reference), not from memory:

- **Input mode**: text-to-video, image-to-video (animating a reference
  frame), or image+text combined. A shot that must match a specific
  reference image or an established character needs image-to-video support,
  not just a strong text-to-video model.
- **Duration and shot length**: confirm the model's actual max clip length
  against what the storyboard needs for this shot. A 60-second production
  built from several short clips needs models whose per-call duration limit
  is compatible with the planned cut count -- do the arithmetic explicitly
  (shot count x per-shot duration) rather than assuming any model covers an
  arbitrary length.
- **Subject/character consistency across shots**: if the same character,
  product, or object must recognizably recur across multiple generated
  shots, check whether the model supports reference-image conditioning or a
  character-consistency feature before committing to it for that whole arc
  -- switching models mid-arc after shots are already generated is expensive
  rework. See `video-cinematography` for the production-side techniques that
  apply regardless of which model you pick.
- **Native audio**: some video models generate synchronized audio (dialogue,
  ambience, SFX) directly; others produce silent video that needs a separate
  voice/music generation pass (see `fal-ai`'s and `google-ai`'s audio
  capabilities) plus assembly in `video-editing`. Decide this per shot, not
  once for the whole production, since a narrated hook and a silent B-roll
  cutaway may want different models.
- **Cost and generation time**: video generation is priced and timed very
  differently from image generation, and cost typically scales with
  duration and resolution. Confirm current per-call pricing before
  generating anything expensive at full resolution -- a cheap low-res test
  pass first is usually worth it for an unfamiliar model (see the cost
  discipline in `fal-ai`/`google-ai`).
- **Aspect ratio and resolution support**: confirm the model natively
  supports the production's required aspect ratio and resolution rather than
  planning to crop/pad a mismatched output after the fact, which degrades
  framing decided at generation time.

## Provider is a routing decision, not a preference

- Use `fal-ai` when the brief needs a specific third-party model (Kling,
  Runway, Seedance, Hunyuan, and similar) that fal.ai hosts, or when you want
  one aggregator surface across many vendors' models under one key.
- Use `google-ai` when the brief specifically calls for a Google-native
  model -- Gemini image generation or Veo -- rather than a third-party one.
- The same production can legitimately use both: e.g. Google's model for a
  product shot that needs Gemini's specific strengths, fal.ai for a stylized
  B-roll shot from a different vendor's model. Record which model produced
  which shot in `production.json` (see `video-creation`) so a revision knows
  which client/skill to reuse for that specific shot.

## Record the decision, not just the output

For every shot, write down in `production.json`: the resolved model ID, the
provider (`fal-ai` or `google-ai`), the exact input/arguments used, and why
that model was chosen over alternatives if the choice was non-obvious. A
revision to one shot should not have to re-derive this reasoning from
scratch, and a viewer noticing an inconsistency between two shots (different
apparent style, different character appearance) is easiest to diagnose when
the model-per-shot record already exists.

## Where this fits

- Use `video-creation` to plan the shot list and own the overall brief.
- Use this skill to choose a model per shot.
- Use `video-cinematography` to turn the creative intent for that shot into
  the actual prompt/camera/lighting direction handed to the chosen model.
- Use `fal-ai` or `google-ai` (per the chosen provider) to make the call.
- Use `video-editing` to assemble the results, `video-quality` before
  presenting any version as complete.

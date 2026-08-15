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
- **Shot count vs. budget.** A true long-form (8-15+ minute) production is
  assembled from dozens of separately-generated clips -- at roughly 5-15
  seconds of usable video per paid call, a 10-minute piece is on the order
  of 50-100+ generation calls before accounting for re-prompts. That is a
  materially different cost and time commitment than a handful of shots,
  and it compounds with regeneration tolerance below. If the user hasn't
  stated a cost ceiling or a maximum acceptable number of paid generation
  calls, ask once, compactly, with the rough call-count arithmetic shown --
  do not silently generate an open-ended number of variants across a
  production this large.
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
  against what the storyboard needs for this shot. No model generates an
  8-15+ minute clip in one call -- a true long-form piece is always
  assembled from many short generations, so do the arithmetic explicitly
  (total runtime ÷ per-shot duration = roughly how many generation calls
  the production needs) rather than assuming duration is a one-time choice.
  A model with a longer native per-call duration (e.g. Seedance 2.5's
  native 30-second generation, versus a model capped at 5-8s) reduces the
  call count and the number of stitch points for the same total runtime,
  which is a real cost and consistency advantage at this scale, not just a
  convenience. A generated clip rarely comes out at exactly the beat's
  needed length -- see `video-editing`'s "Stitching independently
  AI-generated clips" for how to trim, normalize, and concatenate dozens of
  generated clips into the final assembly.
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
- **Open vs. closed weights**: most hosted models are closed API-only. A
  minority ship open weights, meaning the model can also be self-hosted or
  fine-tuned rather than only called as a paid API -- relevant if the brief
  has licensing constraints, or the user asks specifically for an
  open-weight option rather than assuming every model is equivalent on this
  axis.

## Notable options as of 2026-08-15 -- a starting point, not a source of truth

This list was checked against real search results and Google's own docs on
2026-08-15. It will go stale -- confirm the exact model ID and current
capabilities against the provider's own live reference before calling
anything, per "Never invent a model ID" in `fal-ai` and `google-ai`. Do not
extend this list from memory in a later session; re-check instead.

- **Video, hosted on fal.ai**: Seedance 2.5 (`bytedance/seedance-2.5/text-to-video`,
  `.../image-to-video`, `.../reference-to-video`) supersedes 2.0 -- native
  30-second generation in one pass (relevant to the shot-count arithmetic
  above: fewer, longer calls to cover the same total long-form runtime than
  a model capped at 5-8s per call), up to 50 multimodal references in one
  generation, native audio co-processed in the same latent space as the
  visuals so it's synchronized without a post layering pass, and roughly
  double 2.0's native ceiling -- for image-to-video this shows up most as
  materially better subject/character consistency across the whole clip
  (see `video-cinematography`'s consistency section), which matters more,
  not less, the longer a character's arc runs across a production. MiniMax
  H3 (`minimax/h3/text-to-video`, `.../image-to-video`, `.../reference-to-video`)
  is the notable open-weights option -- self-hostable/fine-tunable, not
  only a closed API -- generating 2K, 5-15s clips at 24 FPS with native
  stereo audio and rich multimodal reference input (up to 9 images for
  subject/style, 3 video clips for motion, 3 audio clips), which also makes
  it a strong reference-conditioning option for character consistency. Also
  available: Veo 3.1 / Veo 3.1 Lite (native audio, lip-synced dialogue),
  Kling 3.0 Pro (cinematic, native audio, multilingual lip-sync), Wan 2.6,
  LTX 2.0.
- **Image, hosted on fal.ai**: FLUX, and Google's Nano Banana models
  re-hosted alongside fal.ai's own catalog.
- **Voice/audio, hosted on fal.ai**: ElevenLabs (text-to-speech, voice
  cloning), plus separate music-generation models.
- **Image, direct via Google's Gemini API**: Nano Banana 2 =
  `gemini-3.1-flash-image` (most versatile); Nano Banana 2 Lite =
  `gemini-3.1-flash-lite-image`.
- **Video, direct via Google's Gemini API**: Veo 3.1. A newer multimodal
  option, `gemini-omni-flash-preview`, was reported rolling out with native
  video generation and conversational editing from text/image/video input,
  priced comparably to Veo 3.1 Fast.

One real overlap worth knowing: Veo 3.1 is reachable through both fal.ai
(hosted) and Google's own API (direct). Where a model is available through
both, the choice is about aggregation convenience and pricing, not
capability -- fal.ai gives one unified surface across many vendors' models
under one key; going direct to Google skips that layer for Google's own
models specifically.

## Prompt length and structure are model-specific

Once a model is chosen, `video-cinematography`'s five-aspect formula
(subject, motion, scene, spatial framing, camera) still applies, but how
much of it to spell out, and how long the prompt should run, varies by
model and changes as models version forward -- re-confirm rather than
reusing these numbers indefinitely. As last checked (dated, may have
shifted with newer versions):

- **Seedance** is unusually literal about camera language and rewards a
  fully-specified, long, structured prompt -- roughly 200-400 words for a
  hero shot, 80-150 for a cutaway/insert. It also supports an explicit
  multi-shot structure inside one generation call (`Shot 1 (...)`,
  `Shot 2 (...)`, ...), which tends to hold subject identity more reliably
  across those shots than separate calls do, since the identity anchor is
  interpreted once per call instead of cold-started per shot -- prefer this
  for a tight sequence of the same subject when the model supports it.
- **Veo** responds to a comprehensive structured prompt (subject, action,
  scene, camera angle, camera movement, lens/optical effects, lighting,
  tone, style, ambiance, pacing, audio, editing terms, negative prompt) and
  natively generates dialogue and synchronized ambient audio/music when
  asked for it in the prompt. Sweet spot is roughly 100-250 words; longer
  prompts tend to stop helping past that. **Veo may add unrequested
  subtitles by default for dialogue** -- add "no subtitles, no captions, no
  text overlays" to the negative prompt to prevent this (see also
  "Overlays are not scene depth" in `video-cinematography`).
- Lighter-weight or lower-latency models tend to plateau on prompt length
  much sooner (well under 100 words) and reward simple, motion-focused
  language over exhaustive detail -- check the specific model's own
  guidance rather than assuming a longer prompt is always safer.

## Provider is a routing decision, not a preference

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
- Use `video-storytelling` to structure the narrative arc and pacing before
  a shot list exists to choose models for.
- Use this skill to choose a model per shot.
- Use `video-cinematography` to turn the creative intent for that shot into
  the actual prompt/camera/lighting direction handed to the chosen model.
- Use `fal-ai` or `google-ai` (per the chosen provider) to make the call.
- Use `video-editing` to assemble the results, `video-quality` before
  presenting any version as complete.

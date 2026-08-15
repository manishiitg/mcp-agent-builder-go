---
name: video-cinematography
description: Turn a storyboard beat into camera, lighting, and framing direction for an AI-generated shot, and keep a character or subject visually consistent across multiple generated shots. Use before calling fal-ai or google-ai for any shot in a long-form production, and whenever a generated shot needs re-prompting because framing, motion, lighting, or character identity drifted from the brief.
---

# Cinematography and consistency for generated video

A generation model responds to specific technical direction far better than
a vague creative brief. This skill translates a storyboard beat into that
direction. It is provider- and model-agnostic -- `video-model-selection`
picks the model, `fal-ai`/`google-ai` make the call; this skill decides what
you tell the model to do.

## Shot composition: map the beat to the frame

For every shot, resolve before generating:

- **What the shot proves.** One shot, one idea -- match the visual change to
  the story beat it serves (see `video-creation`'s "Shape the story"). A shot
  trying to prove two things usually proves neither clearly in a few
  seconds.
- **Framing**: wide (establish context/scale), medium (person/product with
  surroundings), close-up (detail, emotion, a specific proof point), or
  extreme close-up (a single critical detail). Choose the tightest framing
  that still reads clearly -- long-form video loses viewers to shots that
  don't commit to a clear subject.
- **Composition**: rule-of-thirds subject placement, leading lines, negative
  space for text/overlay if the shot will carry on-screen copy, headroom
  appropriate to the framing.

## Camera movement vocabulary

Name the movement explicitly in the prompt rather than describing an effect
and hoping the model infers the technique -- most models respond much more
reliably to named camera language:

- **Static**: no movement. The default for a shot whose value is in what's
  shown, not how it's revealed. Use it deliberately, not by omission.
- **Pan / tilt**: camera pivots horizontally/vertically from a fixed point.
  Use to reveal scale or follow motion without repositioning the camera.
- **Dolly / truck**: camera physically moves forward-back (dolly) or
  side-to-side (truck) while keeping the same look direction. A dolly-in
  builds intensity or draws attention to a detail; a dolly-out reveals
  context.
- **Crane / jib**: camera moves vertically, often combined with a pan --
  reveals scale or transitions between a wide establishing view and a
  grounded one.
- **Rack focus**: focus shifts from one subject/depth to another within the
  shot, directing attention without moving the camera. Effective for a
  two-beat shot (e.g. product in foreground, context in background) inside
  one clip.
- **Dolly zoom**: camera moves while the lens zooms to compensate, keeping
  the subject's size constant while the background compresses/expands --
  a strong, specific effect; use sparingly and only when the beat calls for
  genuine visual tension, not as generic "cinematic" flavor.
- **Handheld / stabilized**: state explicitly which one is wanted. Handheld
  reads as immediate/authentic; a locked-off or gimbal-stabilized shot reads
  as polished/controlled. The wrong default for the brief's tone is a common
  cause of a shot feeling off without an obvious reason why.

## Lighting and atmosphere

State lighting direction as part of the prompt, not as an afterthought:

- **Key light direction and quality**: front, side, back, or top; hard
  (sharp shadows, high contrast, dramatic) or soft (diffused, low contrast,
  flattering). Three-point lighting (key, fill, rim/back) is the reliable
  default for a clear, well-modeled subject when the brief doesn't call for
  something more stylized.
- **Color temperature and mood**: warm (golden hour, incandescent) vs cool
  (overcast, moonlight, fluorescent/clinical) sets emotional register before
  a single word of copy appears on screen.
- **Practical vs motivated light**: whether light sources are visible in
  frame (practicals: a window, a lamp) or implied off-frame -- affects how
  "real" vs "produced" a shot reads.
- Keep lighting continuity across shots in the same scene/location unless a
  time or mood change is deliberate and telegraphed (e.g. a transition beat
  explicitly moving from day to night).

## Keeping a character or subject consistent across shots

This is the hardest part of a multi-shot AI-generated production and the
most common source of a viewer-visible defect (a character's face, outfit,
or a product's exact appearance shifting between shots). Techniques, in
order of reliability -- check which your chosen model actually supports
before committing to an approach for the whole arc (see
`video-model-selection`):

1. **Reference-image conditioning (most reliable).** If the model accepts an
   image input, generate or source one strong reference image of the
   character/subject first, then condition every subsequent shot on that
   same reference rather than re-describing appearance in text each time.
   Text descriptions alone drift across independent generations even with
   an identical prompt. See `video-model-selection`'s dated model notes for
   a current example of a model whose image-to-video mode specifically
   improves consistency this way -- confirm against the live reference
   before relying on it, the same as any other model claim.
2. **A written character/subject sheet.** Maintain one canonical, detailed
   description (face, build, exact outfit, product's exact colors/markings)
   in `production.json` and reuse it verbatim in every prompt for that
   subject -- do not let the description drift by paraphrasing it
   differently per shot.
3. **Seed reuse where supported.** Some models expose a seed parameter that
   improves consistency across generations sharing it. This alone is
   usually not sufficient for a recognizable recurring character; combine it
   with a reference image when available.
4. **Shot-to-shot anchoring.** For an image-to-video model, extracting a
   frame from an approved shot and using it as the reference/first-frame
   input for the next shot in the same sequence can carry appearance
   continuity forward through a scene.
5. **When none of the above holds well enough**, treat that as a real
   constraint to report, not a defect to hide: tell the user the chosen
   model cannot reliably hold the character/subject consistent across this
   many shots, and offer the tradeoff (fewer shots of that subject, a
   model switch, or accepting visible variation) rather than presenting a
   version with drifting identity as finished.

## Verify before calling a shot done

After generating, check the result against what was asked for -- a
successful API response is not proof the shot is right:

- Does the framing/composition match what was specified, not a plausible
  alternative interpretation?
- Does the camera movement match what was named, or did the model default
  to static/generic motion?
- For a shot in an established sequence: does the subject's appearance
  actually match the reference/previous shots, checked by eye against the
  reference image or prior frame, not assumed from a matching prompt?

Re-prompt with more specific, technically-named direction (not just a
stronger adjective) when a shot misses -- most drift traces back to an
under-specified prompt rather than an unreliable model.

## Where this fits

- Use `video-creation` to plan the shot list and own the overall brief.
- Use `video-model-selection` to choose a model per shot.
- Use this skill to turn the beat into camera/lighting/consistency direction
  before calling `fal-ai` or `google-ai`.
- Use `video-editing` to assemble the results, `video-quality` before
  presenting any version as complete.

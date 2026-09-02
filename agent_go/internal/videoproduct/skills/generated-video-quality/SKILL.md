---
name: generated-video-quality
description: Check AI-generated footage for the failure modes generation causes rather than deterministic editing -- identity drift, generation artifacts, motion that breaks physics, temporal discontinuity at stitch points, lip-sync drift, and prompt adherence. Use alongside video-quality for any candidate assembled from fal-ai or google-ai clips; never in place of it.
---

# Quality checks specific to generated footage

Use this only for the final assembled delivery MP4, alongside `video-quality`;
do not run it after each generated preview clip. `video-quality` covers what
applies to every video this product makes:
decode/duration/dimensions, black frames and freezes, legible text, a
coherent edit, and a passing `quality-report.json`. Run it first and in
full -- this skill does not repeat it.

What this skill adds is the failure modes a camera cannot produce, because
they only exist in generated footage: a face that drifts across shots, a
sixth finger, a background that reflows between frames, motion that snaps
rather than continues across a cut. A technical pass and a coherent-edit
pass both miss these, because nothing about them fails to decode, and
nothing about them is a black frame.

Add these checks to the **same** `quality-report.json` `checks` map
`video-quality` writes, alongside `technical`/`visual`/`audio`/`content`/
`captions`/`promise` -- one report, one file, not a second document to
reconcile:

```json
"checks": {
  "character_consistency": {"status": "pass", "evidence": ["officer.png vs shots 3,7,12: consistent"]},
  "generation_artifacts": {"status": "pass", "evidence": ["no extra limbs, warped hands, or gibberish text observed"]},
  "temporal_coherence": {"status": "pass", "evidence": ["motion continues across every stitch point checked"]},
  "motion_plausibility": {"status": "not_applicable", "evidence": ["no complex physics in this cut"]},
  "lip_sync": {"status": "pass", "evidence": ["dialogue in shots 2,5 reads in sync"]},
  "clip_color_consistency": {"status": "pass", "evidence": ["black level/white balance consistent shot to shot"]},
  "prompt_adherence": {"status": "pass", "evidence": ["each shot matches its shot-list framing and camera move"]},
  "narration_alignment": {"status": "pass", "evidence": ["visuals match narration-manifest.md's measured durations"]}
}
```

Same discipline as the base contract: every check here is `pass` or
genuinely `not_applicable` with evidence, never `not_applicable` to hide a
check you didn't run. A candidate with no recurring character still runs
`character_consistency` as `not_applicable` -- it does not get skipped.

## Character/subject consistency

The characteristic defect of this whole route, and the one most likely to
be missed, because it is invisible unless checked against the actual
reference rather than from memory. For every recurring character, presenter,
or product:

- Open its reference image from `characters/` (see `video-cinematography`)
  beside every shot it appears in, side by side -- not sequentially, not
  from what you remember the reference looking like.
- Compare face, build, and the exact outfit/markings the character spec
  named as disambiguating. A close match on everything except the one
  attribute the spec called out to distinguish this character is still a
  fail -- that attribute existed because it is what a viewer would notice.
- Where a shot conditioned on the reference still drifted, that is a
  generation failure to report, not a QA nuance to soften.

## Generation artifacts

Look for the failure signatures specific to generative models, which a
general "does it look right" pass tends to miss because each artifact is
small in isolation:

- warped or extra fingers, merged limbs, an incorrect number of limbs;
- a face that morphs or drifts within a single continuous shot, not just
  across cuts;
- background elements that shift, duplicate, or reflow between frames;
- rendered text that is gibberish, or a logo that is almost-but-not-quite
  the real one;
- an object that changes material, color, or shape mid-shot with no
  in-story reason.

## Temporal coherence at stitch points

A cut between two independently-generated clips can be technically clean
(matching codec, resolution, frame rate -- see `video-editing`) and still
fail here, because the cut point is exactly where two separate generations
meet with no shared motion between them:

- At each cut, does the subject's position, pose, and motion direction
  continue plausibly from the last frame of the outgoing clip to the first
  frame of the incoming one, or does the subject visibly jump or teleport?
- Does lighting and color grade hold across the cut, beyond the technical
  color-consistency check below -- a technically-matched clip can still
  read as a different scene.
- Where `video-cinematography`'s overlapping-prompt technique was used to
  smooth a seam, check whether it actually worked at that specific cut.

## Motion and physics plausibility

`video-cinematography` already warns that complex physics (explosions,
crowds, collisions) is where generation models are least reliable. Check
specifically for it rather than assuming a smooth render means correct
motion:

- Does contact between objects look physically grounded -- a hand actually
  gripping, a foot actually landing -- or does something pass through
  something else?
- Is there garbled or physically incoherent motion during any fast or
  complex action, especially group scenes?
- Mark `not_applicable` with a one-line reason for a cut with no complex
  physics, rather than leaving the check unexplained.

## Lip-sync drift

For any shot with generated dialogue (native model audio or lip-synced
narration):

- Confirm mouth shapes track the actual words at a few points across the
  shot, not just at the start.
- Flag drift that worsens over the shot's duration -- some models sync
  well at the open and drift by the close, which a single-frame check
  would miss entirely.
- Mark `not_applicable` for shots with no on-screen speaker.

## Per-clip color and exposure consistency

`video-editing` describes the fix (a light, consistent grade pass across
the assembly) but nothing verifies whether it actually worked. Confirm it
did:

- Sample a frame from each clip in a scene and compare black level, white
  balance, and saturation against its neighbors.
- A mismatch here often means two clips came from different models or
  providers -- cross-check against `production.json`'s per-shot model
  record, and note it as a scope decision (per `video-cinematography`'s
  one-model-per-arc rule) rather than only a color-grade problem.

## Prompt adherence

Compare each shot against what `SHOTLIST.md` actually asked for -- a
plausible-looking shot that quietly substituted a different framing or
camera move is easy to wave through if you only check that the shot looks
good in isolation:

- Does the framing (wide/medium/close-up) match what was specified?
- Does the camera movement match the named primitive, or did the model
  default to something else -- a static shot when a push-in was asked for
  reads as a miss even though nothing about it looks wrong?
- Where a shot substituted something else, decide whether the substitution
  still serves the beat (accept and note it) or needs a re-prompt (per
  `video-cinematography`'s iteration strategy) -- do not silently accept a
  drift from the plan without recording that a decision was made.

## Narration and visual alignment

For a narrated piece, confirm the assembled visuals actually match
`narration-manifest.md`'s **measured** durations (see `video-editing`'s
narration-first assembly), not the shot list's planned ones:

- Does each visual segment's actual length match its narration segment's
  measured length, not an estimate that was superseded?
- Is there a beat where narration and visual reference different content --
  narration describing an action the corresponding shot does not show?

## Where this fits

- Run `video-quality` first, in full -- this skill adds to that pass, it
  does not replace any part of it.
- Read `video-cinematography` for what each character/consistency/motion
  rule was trying to achieve, so a finding here can be traced to the
  production decision that caused it.
- Read `video-editing` for the stitching and grading mechanics whose
  results this skill verifies.
- A failing check here is fixed the same way any QA failure is fixed:
  the smallest responsible layer re-generates or re-cuts, not the whole
  piece.

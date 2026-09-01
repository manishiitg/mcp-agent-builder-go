---
name: video-stitching
description: Plan, generate, and verify seam-by-seam cinematic continuity across follow-up clips and the final stitched film. Use when planning or creating a shot that follows another shot, and before presenting a final video.
---

# Stitch one coherent film

Stitching is a distinct editorial decision, not a final export detail. Read
`video-editing` for deterministic media mechanics and
`longform-cinematic-video` when it applies; this skill owns the seam-by-seam
handoff between approved clips and the final cut.

For a sequence that still needs clips generated, read
`multi-clip-cinematic-generation` first. It owns pre-generation transition
grammar and the reference manifest; this skill owns the editorial plan and
final seam verification once approved clips exist.

## Generate for the next seam

Plan every follow-up shot before its generation request. The prior accepted
clip is evidence, not merely something to repair later. Read its full usable
range, then select its last **usable stable frame** at the intended edit point
(not automatically the literal final decoded frame, which may be blurred,
mid-blink, or a generation artifact). Record that selected frame and the
outgoing subject, prop, camera, screen-direction, action, lighting, and audio
state in the continuity ledger.

Create one successor at a time, preserve the ledger, and append its actual
boundary evidence. Use the user's current direction only to choose or refine
that target shot; it does not authorise the remaining shot list. If no accepted
predecessor is recorded, stop and name the missing anchor or predecessor
instead of generating a visually unrelated continuation.

For Video Studio, use the H3 routes in `minimax-h3-video`:

1. one 5–15-second H3 take when it covers the beat;
2. `minimax/h3-max/reference-to-video` with the accepted predecessor as Video 1
   for every normal continuation, including a deliberate camera-angle change;
3. an intentional discontinuity such as a time/location jump or montage;
4. `minimax/h3-max/image-to-video` only as a separately approved first/last-frame
   bridge after a direct-cut seam proof visibly fails.

Never use image-to-video as the normal continuation route. A new camera angle
is not a continuity failure when it is planned: preserve the same scene state
and choose a cut point that motivates the change. Record the new lens,
framing, camera position/vector, matching first action, screen direction, and
audio handoff; then use the accepted predecessor as Video 1. When H3 cannot
preserve the needed state, change the shot design or regenerate the prior
source; do not create two unrelated clips and expect a dissolve to solve it.

## Vary the film language deliberately

Continuity does **not** mean holding one camera angle, lens, framing, or
lighting setup indefinitely. Shape a sequence with the visual changes that
serve its story: wide, medium, close, detail, over-the-shoulder, POV,
reaction, high/low angle, static or moving camera, lens and depth-of-field
changes, motivated lighting shifts, weather or time progression, and a new
location when the story moves there. A new angle, scale, or camera move is a
positive editorial choice when its entry frame, action, eyeline, geography,
and sound bridge make it legible.

For each deliberate change, state what changes and what remains continuous:
the transition type, outgoing and incoming camera grammar, subject and prop
state, screen direction, audio handoff, and any new approved reference. Use a
match cut, cut on action, reaction, insert/cutaway, or a clear scene-reset
marker. Do not preserve a flat visual setup merely to avoid a seam; equally,
do not change several visual variables accidentally and call it cinematic.

## Prove the newest seam before advancing

After a successor clip is accepted, do not wait until final assembly to learn
whether it works. Render a short preview containing exactly the predecessor,
the recorded seam treatment, and the successor. Show it with `show_video` and
write a seam-proof record with the source versions, selected boundary frames,
in/out points, transition, audio bridge, review outcome, and pass/fail verdict.

Check identity, wardrobe, props, geography, screen direction, eyeline, action,
lighting/color, motion, dialogue or voiceover, ambience, and music. If the
seam fails, regenerate or redesign the affected pair before a third clip is
made. A final stitch plan may only use joins with passing seam proofs.

## Plan before rendering

Read the current stage contract, the shot list, measured narration, look and
sound direction, generation record, and continuity ledger. Use only accepted,
individually reviewed clips recorded at their exact paths. Do not generate,
substitute, or search for an unrecorded clip during stitching.

Inspect the full source clips and their boundary frames before selecting trims.
For every seam, record the preceding and following source version, usable
in/out ranges, chosen in/out points, timeline position, narration beat, visual
handoff, audio lead or trail, transition, grade, and seam risk. Check identity,
wardrobe, props, geography, screen direction, eyeline, action, lighting,
color, motion, native dialogue or voiceover, ambience, and music continuity.

Write the stage's required stitch-plan JSON before rendering. A seam that is
not represented in that plan is not approved for the timeline.

## Present meaningful options

Before executing a non-trivial stitch, show the user the resulting stitch plan
with `show_document` in plain language. Present at most three meaningful
editorial choices only where a choice changes the film: for example, a hard
cut versus a motivated J-cut, a cutaway versus regenerating an incompatible
clip, or a continuous score bridge versus an intentional chapter break.

Recommend one option and explain the visible continuity, pacing, and audio
tradeoff. Do not ask the user to choose codec, command, or other implementation
detail. If every seam has one clearly safe treatment, state the plan and
proceed under the user's current approved production direction rather than
inventing a choice.

## Stitch and mix

Normalize differing media specifications before concatenation, then use the
approved plan exactly. Prefer hard cuts between independently generated clips;
use cuts on action, match cuts, and J/L audio bridges when their recorded
continuity supports them. A dissolve cannot repair a character reset,
geography reversal, incompatible lighting, or contradictory action state.
Return such a seam to regeneration or a planned cutaway.

Maintain a continuous sound world. Bridge room tone, ambience, music, and
voice naturally across the edit; do not restart a cue at each AI-generated
clip. Keep captions and exact text in deterministic layers.

Render an early representative sequence when a production has repeated seam
patterns, inspect it, then render the full film. Preserve the edit decision
list so a revision rebuilds only the affected sequence.

## Prove the finished seams

Before presenting the final MP4, inspect every boundary in the rendered film,
not merely the source clips. Record boundary-frame evidence and a verdict in
the required seam report. A failed seam means the film is not ready to present:
repair the source or edit decision, rebuild the affected sequence, and check
its adjacent seams again. Present individual clips with `show_video` as they
are approved, and present the completed film only after its final QA passes.

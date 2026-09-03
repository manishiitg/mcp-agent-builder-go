---
name: multi-clip-cinematic-generation
description: Design a coherent MiniMax H3 sequence using a reference manifest and explicit camera-transition grammar. Use before planning or creating an anchor clip or any follow-up clip in a cinematic sequence.
---

# Generate one cinematic sequence, not unrelated clips

This skill owns the decisions that must happen **before** an AI-generated
follow-up clip exists. Read `minimax-h3-video`, `video-provider-capabilities`,
and `fal-ai` before a paid call; endpoint controls are not interchangeable.
Read `video-stitching` after clips are approved to plan and verify the edit.

## Choose the sequence topology first

When the user has not set a per-clip duration, shot count, or beat boundary,
use the smallest number of H3 generation boundaries that preserves the
intended action. Those explicit user constraints win over cost and seam-count
optimisation. Select and record one route for each seam:

1. `minimax/h3-max/text-to-video` for a prompt-only standalone opening shot
   with no reference or continuity obligation;
2. `minimax/h3-max/image-to-video` when an approved start image must control
   the opening composition;
3. `minimax/h3-max/reference-to-video` from the accepted predecessor for every
   normal continuation, including a motivated camera-angle change;
4. a motivated editorial cut with the stable approved reference pack;
5. an intentional discontinuity such as a time jump, location change, or
   montage beat.

Never call separately generated clips a continuous take merely because a
dissolve can join them. Reference-to-Video conditions a new successor; it does
not append the predecessor or guarantee a frame-, mouth-, or audio-exact seam.
Keep uninterrupted on-camera dialogue and one continuous action in one H3 Max
take whenever it fits 5–15 seconds. When a new request is unavoidable, the
shot list must name an editorial boundary—a completed thought/pause, reaction,
insert, or motivated angle change—not a mid-word continuation. Do not rewrite
user-approved dialogue to fit that boundary without explicit approval. Do not
create a third bridge clip when a direct review fails; regenerate or redesign
the affected H3 successor through Reference-to-Video.

## Make an approved reference pack, then a reference manifest

Before the anchor, create **real visual-development evidence** and a
sequence-level manifest. A Markdown description such as “warm coffee shop” is
not an approved reference and is not enough to condition a follow-up shot.
Generate and present the images the sequence actually needs for review:

- one location/background reference for every returning place, including its
  return geography, time of day, weather, and practical-light motivation;
- any recurring wardrobe, hero prop, or vehicle reference that will be visible
  at a seam;
- a start reference for each continuity sequence and a planned exit/end-state
  reference for every sequence that has a successor;
- selected stable boundary frames for direct-cut review and successor prompt
  handoff; they are not a request to make an intervening bridge clip.

Call `show_reference` for each generated reference and obtain approval before
footage consumes it. Keep the assets under `references/` and record exact
paths and semantic roles. The manifest is the contract each later prompt and
request must reuse:

- delivery aspect ratio and orientation;
- approved character/identity, wardrobe, prop, location, and lighting
  references, with their roles;
- palette, time of day, weather, and texture;
- composition: screen side, headroom, subject size, gaze/eyeline, and walking
  or object direction;
- camera language: lens family, camera height, distance, side of the action,
  movement vector, speed, and focus behavior;
- sound world and whether the beat uses native dialogue, off-camera voiceover,
  or no speech;
- the accepted predecessor path, its selected stable boundary frame, and the
  exact endpoint-supported reference inputs when a follow-up is required.

Keep a normal film in one orientation and aspect ratio. Do not mix 16:9 and
9:16 generated footage unless the shot list explicitly calls for an in-world
phone, screen, or archival insert; crop, frame, or composite that insert in
editing rather than silently changing the film's delivery contract.

When a planned HyperFrames insert appears inside a cinematic sequence, treat
it as an intentional boundary, not as a generated continuation: define its
entry and exit frame, sound bridge, and return to the same live-action
orientation and visual world in the shot list.

## Write the handoff before generating the next clip

Record both the outgoing state of clip A and the entry state of clip B:

- subject pose, position, gaze, gesture, movement direction, and object state;
- location geometry, camera side of the action, screen direction, eyeline,
  lens family, framing, camera vector, lighting, and audio environment;
- the exact overlapping action or visual match at the cut point;
- transition type and its motivation;
- the route and reference inputs actually supported by the selected endpoint.

Use the prior clip's **last usable stable frame**, not automatically its final
decoded frame. Do not chain from blur, a blink, a half-gesture, a malformed
face, or an unstable generated frame.

## Use deliberate camera-transition grammar

Keep a continuity cut continuous: preserve the 180-degree side of action,
screen direction, eyeline, geography, subject placement, and motion direction.
Retain the lens/framing family unless a visible motivation justifies a change.
A new angle must be a new editorial shot, not a vague request to continue.

Choose one specific handoff for every new angle:

- **Cut on action:** start clip B on the same turn, reach, sit, door movement,
  or other action clip A exits on.
- **Match cut:** match the subject's pose, a shape, motion direction, framing,
  or object position across the cut.
- **Reaction:** cut from an action to an observer whose eyeline and location
  make the reaction legible.
- **Insert/cutaway:** show a meaningful hand, object, environment, or detail
  while protecting a difficult identity or geography transition.
- **Wide-to-medium-to-close progression:** change scale deliberately while
  keeping the action axis and scene state stable.
- **Reset/jump:** declare a deliberate new time, place, or visual world and
  give the edit an audible or visual separator; never imply continuity.

When changing angle, write the new lens, framing, camera position/vector, and
the matching first action explicitly. Preserve a subject crossing screen left
to right unless the shot uses an intentional, readable reversal. Do not flip
orientation, camera side, or gaze direction by accident.

## Generate, receipt-check, then advance

Create exactly one clip per generation recipe. Reuse the manifest, approved
references, and planned handoff in its request. The normal successor uses H3
Max Reference-to-Video with the accepted predecessor as Video 1; H3 owns the
continuity, including a deliberate change of angle. Show and inspect the MP4
before accepting it. On acceptance, record the actual output and handoff state;
on rejection, record the reason and regenerate or redesign that H3 successor.

For each preview, perform only a clip receipt: `ffprobe` the downloaded asset
and inspect its stable opening and ending frames. For a successor, compare its
opening against the predecessor's ending closely enough to catch an obvious
break. Do not render a per-shot FFmpeg seam preview, set default trims, or
write a seam-proof document. If the boundary is visibly wrong, revise the H3
prompt/reference set and regenerate the successor; never use a bridge clip,
crossfade, blend, zoom, or reframe to conceal it. Full delivery QA happens once
on the final direct-concatenated export.

---
name: multi-clip-cinematic-generation
description: Design and generate a coherent sequence of AI video clips using a reference manifest and explicit camera-transition grammar. Use before planning or creating an anchor clip or any follow-up clip in a cinematic sequence.
---

# Generate one cinematic sequence, not unrelated clips

This skill owns the decisions that must happen **before** an AI-generated
follow-up clip exists. Read `video-provider-capabilities` and the selected
provider skill before a paid call; endpoint controls are not interchangeable.
Read `video-stitching` after clips are approved to plan and verify the edit.

## Choose the sequence topology first

Use the smallest number of generation boundaries that preserves the intended
action. Select and record one route for each seam, in this order when it is
supported by the chosen endpoint:

1. one uninterrupted take;
2. one native structured multi-shot generation;
3. extension or video-reference continuation of the accepted prior clip;
4. a boundary-frame chain from the prior clip's selected last usable stable
   frame, optionally toward a designed end frame;
5. a motivated editorial cut with a stable reference pack;
6. an intentional discontinuity such as a time jump, location change, or
   montage beat.

Never call separately generated clips a continuous take merely because a
dissolve can join them. If a model lacks the required input control, redesign
the seam or select a compatible, user-approved model instead of inventing an
API field.

## Make a reference manifest

Before the anchor, create a sequence-level manifest. It is the contract each
later prompt and request must reuse:

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

## Generate, review, then advance

Create exactly one clip per generation recipe. Reuse the manifest, approved
references, and planned handoff in its request. Show and inspect the MP4
before accepting it. On acceptance, update the continuity ledger with actual
boundary frames and state; on rejection, record the reason and regenerate or
redesign the seam before making a successor. Do not batch later clips or let
an unreviewed clip become continuity evidence.

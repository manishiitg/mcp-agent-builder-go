# Continuity planning

Plan scenes before prompts. A scene is continuous when the viewer expects the
same time, place, subjects, wardrobe, objects, lighting, and causal action to
persist. Script beats inside that scene do not justify separate generation.

## Strategy order

Choose the first supported strategy that fits:

1. **One take:** one request for uninterrupted action within the endpoint's
   duration limit.
2. **Native multi-shot:** one request with the endpoint's structured shot or
   prompt sequence for intentional cuts inside one coherent scene.
3. **Video extension/edit:** continue the accepted source clip while retaining
   its motion and scene state.
4. **Boundary-frame chain:** extract the accepted clip's final clean frame and
   use it as the next start frame; optionally provide a designed end frame.
5. **Reference-driven continuation:** reuse the accepted video plus approved
   character/location/object/audio references.
6. **Independent generation:** only for a deliberate discontinuity approved in
   the plan.

Minimize both paid calls and seams. Do not choose five ten-second clips for a
thirty-second continuous scene when a fifteen-second endpoint plus one
extension or boundary-frame continuation can cover it.

## State ledger

For every scene, record:

- stable state: characters, face/identity reference, wardrobe, props, location,
  time, weather, palette, lens/camera language, audio environment;
- evolving state: subject position, gaze, gesture, object state, camera
  position, dialogue/narration point, and action at start/end;
- model controls: seed, first/last frame, reference arrays, video reference,
  structured multi-shot prompts, extension source, native/reference audio;
- cut intent: continuous, motivated hard cut, montage, time/location jump, or
  faceless illustrative beat.

Reuse exact approved identity descriptors and reference files. For a real
person, require a user-provided or explicitly approved reference and do not
train or synthesize a reusable biometric identity without consent. A style
reference may guide medium, palette, texture, framing, and energy; it must not
silently import another person's identity, trademarks, or exact composition.

## Prompt rules

- For text-to-video, describe subject, action, environment, camera, lighting,
  timing, and sound with one coherent action path.
- For image-to-video, treat the image as the static contract and prompt the
  motion, camera change, temporal action, and end state. Do not waste the prompt
  redescribing every visible pixel.
- For multi-shot, use the endpoint's exact structured fields and make each
  internal shot advance the same scene. Do not concatenate numbered prose if
  the API expects an array.
- At a chained seam, repeat an overlapping action: end clip A during an action
  and start clip B from the same action and state.
- Use positive visual direction where the endpoint lacks a negative-prompt
  field. Never invent a negative-prompt parameter.

## Explainers and montage

Independent clips are acceptable for intentionally non-continuous faceless
explainers, diagrams, examples, or montages. Keep one approved style key and a
stable rendering descriptor across them, align each clip to measured narration,
and still present every clip for review before assembly. A fixed block duration
is a production choice, not a universal model rule; derive block boundaries
from narration and the live endpoint's duration contract.

## Seam record

For every boundary include:

```json
{
  "from_clip": "clips/scene-01-a.mp4",
  "boundary_frame": "frames/scene-01-a-final.png",
  "to_request": "requests/scene-01-b.json",
  "overlap_action": "the driver continues turning toward the window",
  "expected_transition": "same pose, wardrobe, lighting, location, and camera side",
  "reason_for_new_call": "endpoint duration limit",
  "review_status": "pending_user_review"
}
```

---
name: veo-video
description: Plan and generate Google Veo video effectively through current fal.ai, Gemini API, or Vertex AI contracts. Use when a production selects or considers Veo for text-to-video, image-to-video, first/last-frame interpolation, reference-image identity or product guidance, Veo video extension, portrait video, high resolution, native audio, or long-running generation operations. Read with video-provider-capabilities and the selected provider skill before any paid Veo call.
---

# Use Veo by mode, not by name alone

Read `video-provider-capabilities`, `video-cinematography`, and exactly one
provider skill: `fal-ai` for `fal-ai/veo...` routes or `google-ai` for Gemini
API/Vertex. Then read the official docs for the exact API surface:

- fal Veo 3.1 Lite image-to-video:
  `https://fal.ai/models/fal-ai/veo3.1/lite/image-to-video/api`
- fal Veo 3.1 image-to-video:
  `https://fal.ai/models/fal-ai/veo3.1/image-to-video/api`
- Gemini API: `https://ai.google.dev/gemini-api/docs/video`
- Vertex first/last frames:
  `https://cloud.google.com/vertex-ai/generative-ai/docs/video/generate-videos-from-first-and-last-frames`
- Vertex prompt guide:
  `https://cloud.google.com/vertex-ai/generative-ai/docs/video/video-gen-prompt-guide`

Do not combine fal, Gemini, or Vertex model IDs and request shapes. Resolve the
current model variant, availability, mode, limits, field nesting, and price on
the surface actually configured for the production.

## Select one generation mode

- Use text-to-video for an unanchored new shot.
- Use an initial image for approved opening composition and image animation.
- Use first plus last frame for a controlled interpolation whose end state is
  designed in advance. A last frame is not a general style reference.
- Use reference images when the live Veo variant supports them and preserving a
  person, character, product, or visual asset matters. Label every reference.
- Use extension only for a source accepted by the current extension contract.
  The Gemini API currently restricts extension to previously generated Veo
  footage; do not promise arbitrary uploaded-video continuation.

Choose the mode before writing the request. Do not combine initial/last-frame,
reference-image, and extension controls unless the live schema explicitly
permits that combination.

## Respect mode-dependent constraints

Veo limits change with model and mode. Record the matrix rather than a single
family-wide value:

- accepted duration values and modes that force a specific duration;
- landscape/portrait support and modes restricted to one ratio;
- resolution choices and duration coupling for 1080p or 4K;
- number/type of reference images;
- person-generation policy values by input mode and region;
- extension source, resolution, length, retention, and chain limits;
- native-audio behavior, seed behavior, and videos per request.

Current Gemini documentation, for example, requires eight seconds for several
advanced Veo 3.1 modes and limits extensions to 720p Veo footage. These are
schema-discovery hints, not permission to reuse the values without checking.

As checked on 2026-08-21, fal Veo 3.1 Lite image-to-video accepts only `4s`,
`6s`, or `8s`; 8 seconds is its maximum, not 10. It exposes 720p or 1080p,
`auto`/16:9/9:16, one starting image, and optional native audio. The non-Lite
fal image-to-video route has the same duration enum but also exposes 4K. Treat
these as dated discovery hints: copy the exact selected route's live enum into
the capability record before building or costing a shot list.

## Prompt for picture and sound

Build prompts in this order:

1. subject and stable appearance;
2. action and temporal progression;
3. setting and atmosphere;
4. shot size, angle, lens, camera movement, and focus behavior;
5. lighting and visual style;
6. a separate audio sentence for dialogue, ambience, effects, and music.

Quote dialogue exactly and name its speaker. Keep audio cues concrete. For an
initial image, describe motion and the destination state instead of restating
the frame. For first/last interpolation, describe the plausible action that
connects the supplied states. For extension, continue the final action and
camera state of the accepted source; do not restart the scene.

Reference images preserve content/style but do not define a timeline. Use
first/last frames for boundary composition and reference images for recurring
appearance, according to the selected mode's documented compatibility.

## Continue and execute durably

For a longer continuous Veo scene, prefer supported extension over independent
generation. Preserve the generated video object/URI required for the next
extension and act before provider retention expires. Save each long-running
operation ID immediately, poll the same operation, and never create a duplicate
paid call because a local wait timed out.

After download, verify streams, exact duration/resolution, native audio,
identity/product fidelity, prompt adherence, interpolation or extension seam,
motion, and artifacts. Present every individual result through `show_video`
and wait for approval before further extension or assembly. Record SynthID or
other provider disclosure requirements when relevant to delivery.

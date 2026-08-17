---
name: video-provider-capabilities
description: Resolve an AI-video model's live official API schema and turn it into a recorded generation and continuity plan before any paid video call. Use for Kling, Seedance, Veo, or any other video model when selecting a model, preparing a video request, chaining clips, using references/start or end frames, extending footage, or deciding whether multiple clips are necessary.
---

# Resolve provider capabilities before generation

Do not create a paid video request from a generic template. Before the first
video call for a production, read the selected model's current official API
reference. For fal-hosted models use that model's fal API page; for direct
Google models use the current Google Gemini/Veo documentation. Do not use a
search-result summary, a blog post, or another provider's schema as the
source of truth.

## Required capability record

Write `capabilities/<provider>-<model-slug>.json` under the production folder
in direct chat, or inside the current workflow-stage folder. This is a
required input to every paid video request. It must contain:

```json
{
  "schema_version": 1,
  "provider": "fal-ai or google-ai",
  "model_id": "exact resolved model id",
  "checked_at": "RFC3339 timestamp",
  "official_docs": ["official API URL"],
  "input_contract": {
    "required": [],
    "optional": [],
    "media_roles": {"start_frame": 0, "end_frame": 0, "image_references": 0, "video_references": 0, "audio_references": 0},
    "duration_seconds": {"min": 0, "max": 0},
    "multi_shot": false,
    "extension": false,
    "seed": false,
    "native_audio": false,
    "aspect_ratios": [],
    "resolutions": []
  },
  "request_shape": {"exact field names": "copied from the official schema"},
  "continuity_plan": {"strategy": "...", "why": "..."}
}
```

Record only capabilities confirmed by the current official schema. Omit an
unsupported feature rather than guessing a field name. Preserve the exact
request sent, including media-role assignments, in `production.json` next to
the output path.

## Choose a continuity plan before calling the model

Treat one continuous scene as a continuity problem, not a list of storyboard
bullets. Minimize independent generations and stitch points.

1. Prefer one generation when its supported duration covers the continuous
   action.
2. If the model supports a multi-shot request, use it for intentional internal
   cuts within its duration limit instead of generating each beat separately.
3. If more than one generation is unavoidable, use the previous accepted
   clip's final frame as the next clip's start frame when the schema supports
   it. Supply a planned end frame when supported and useful.
4. Reuse the same approved character/object references and provider/model for
   the whole continuous arc. Use all supported reference roles deliberately;
   do not reduce an array-capable model to one generic image input.
5. Use extension or reference-video input when the resolved model supports it
   and it preserves the required scene better than a fresh generation.

For every seam, record the preceding output, the next input frame/reference,
the overlapping action in both prompts, expected transition, and why a new
generation is necessary. A 30-second continuous interaction is not five
independent clips merely because its script has five beats.

## Review and presentation gate

Show every newly generated clip with `show_video` as a Preview immediately
after it is verified playable. Do not assemble a clip marked
`pending_user_review`, and do not present a stitched cut as the user's first
view of its component clips. Record the presentation ID and review outcome in
`production.json` before the clip can become an assembly input.

## Request construction

Build requests from the capability record's exact `request_shape`, not a
lowest-common-denominator helper. Check all local input files, upload or
encode them using the selected provider skill's documented mechanism, and
validate duration, media-role limits, aspect ratio, resolution, and mutually
exclusive fields before submitting. If the request cannot meet the continuity
plan within the model's documented limits, stop and explain the constraint;
do not silently fall back to unrelated independent clips.

After a response, verify the local video with `ffprobe`, present it for the
required review, and save the returned output plus the exact capability record
used. A successful provider response is not evidence that continuity passed.

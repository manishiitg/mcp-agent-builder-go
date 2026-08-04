---
name: video-quality
description: Validate a candidate video technically, visually, and editorially before presenting it as complete. Use after rendering a video or when diagnosing playback, aspect ratio, duration, audio, captions, black frames, freezes, identity drift, or other quality problems.
---

# Validate video quality

Do not call a video complete because rendering succeeded. Validate the exact output file the user will receive.

If the render report says the candidate was assembled from `placeholder: true` assets, apply only the **Deterministic checks** below — a solid-color or drawtext placeholder has no face, hand, or lip movement to check, and grading it against creative criteria it was never meant to satisfy is meaningless. Skip visual and content review, and record the verdict as a placeholder-pipeline pass or fail, not a creative `PASS` — the distinction matters because a placeholder passing means the *pipeline* works, not that the video is finished.

## Deterministic checks

Use `ffprobe` and `ffmpeg` where available to verify:

- the output exists, is non-empty, decodes, and has the expected container and codecs;
- width, height, aspect ratio, frame rate, duration, and audio streams match the request;
- dialogue is audible and the mix is not silent or clipping;
- no unintended black segments or frozen sections appear;
- the final frame and audio are not truncated.

Useful detectors include `blackdetect`, `freezedetect`, and `volumedetect`. Treat missing audio, wrong dimensions, decode errors, major black/frozen sections, and materially wrong duration as failures.

## Visual review

Create a contact sheet containing the opening, each edit boundary, representative middle frames, all text cards, and the final frame. Inspect it for:

- consistent people, products, wardrobe, sets, and color treatment;
- natural faces, hands, lip movement, and object geometry;
- legible text with correct spelling and safe margins;
- no generated gibberish, watermark, stretched media, accidental crop, or transition ghosting;
- a clean hook, coherent visual progression, and a deliberate ending.

For a single recurring presenter, compare early and late face crops. For multi-person video, compare each character separately; a single aggregate face score is misleading.

## Audio and content review

- Listen to the opening, every cut, and the ending.
- Check speech intelligibility, sync, music ducking, room-tone jumps, and abrupt fades.
- When a script matters, transcribe the final export and compare key claims, names, prices, and the call to action.
- Confirm that captions follow the final speech timing rather than a pre-trim timeline.

## Record the result

Write a concise machine-readable report containing:

- output path and inspected specifications;
- pass/fail checks and warnings;
- sampled frame times;
- unresolved items and the final verdict.

**Running as a workflow stage:** the report is delivery.md inside your own step folder under `runs/<iteration>/<group>/execution/<stage>/`, and it must name the exact candidate file you validated. **Working directly in chat:** write it under `work/qa/<output-name>.json`.

Only mark the candidate `PASS` when all required checks succeed. Fix failures and rerender the smallest affected layer. If a check cannot run locally, label it unverified and tell the user exactly what remains unchecked.

## Quality gate (binding)

Never mark a video complete because a render step returned without error. `PASS` means you personally opened the exact file the user will receive and every required check above passed against it — not that a prior stage reported success.

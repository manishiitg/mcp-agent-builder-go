---
name: video-quality
description: Validate a candidate video technically, visually, and editorially before presenting it as complete. Use after rendering a video or when diagnosing playback, aspect ratio, duration, audio, captions, black frames, freezes, identity drift, or other quality problems.
---

# Validate video quality

Do not call a video complete because rendering succeeded. Validate the exact output file the user will receive.

For a candidate assembled from `fal-ai`/`google-ai` clips, this skill is the baseline, not the whole review -- add `generated-video-quality`'s checks (identity drift, generation artifacts, temporal discontinuity at stitch points, motion plausibility, lip-sync, prompt adherence) to the same pass. Generated footage fails in ways a technical decode check and a general visual-coherence pass both miss, because nothing about them fails to decode and nothing about them is a black frame.

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

Create a contact sheet containing the opening, each edit boundary, representative middle frames, all text cards, and the final frame.

For a long-form piece assembled from dozens of clips, one sheet of every edit boundary is unreadable and stops being a review. Produce one contact sheet per chapter instead, and keep every boundary covered across the set rather than sampling a subset — a dropped boundary is exactly where a mismatch hides. Where a recurring character appears, put its reference image (see `video-cinematography`) beside its shots so consistency is checked against the reference rather than from memory.

Inspect for:

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
- a project-relative `contact_sheet_path` and at least four sampled frame times and paths;
- unresolved items and the final verdict.

Use this shared contract for every path:

```json
{
  "schema_version": 1,
  "candidate_path": "outputs/example.mp4",
  "contact_sheet_path": "work/qa/example/qa-contact-sheet.jpg",
  "verdict": "pass",
  "ready_to_present": true,
  "checks": {
    "technical": {"status": "pass", "evidence": ["..."]},
    "visual": {"status": "pass", "evidence": ["..."]},
    "audio": {"status": "not_applicable", "evidence": ["No audio was requested"]},
    "content": {"status": "pass", "evidence": ["..."]},
    "captions": {"status": "not_applicable", "evidence": ["No speech or captions were requested"]},
    "promise": {"status": "pass", "evidence": ["..."]}
  },
  "sampled_frames": [
    {"timestamp_seconds": 0.1, "path": "work/qa/example/frame-01.jpg"}
  ],
  "issues": [],
  "recommended_action": "present"
}
```

All paths are project-relative. Every required check must be `pass` or genuinely `not_applicable`; never use `not_applicable` to hide a check that could not be run. `sampled_frames` must contain at least four inspected frames including the opening and ending — four is a floor for a short piece, not a target: scale the count with duration and cut count, at least one frame per chapter and per edit boundary for a long-form assembly. A placeholder candidate uses `verdict: "placeholder-pass"` and must be described as a placeholder when presented.

**Running as a workflow stage:** write the human report named by the stage plus `quality-report.json` and `qa-contact-sheet.jpg` inside your own step folder under `runs/<iteration>/<group>/execution/<stage>/`; all three are required outputs. The JSON must name the exact project-relative candidate file you validated. **Working directly in chat:** write `quality-report.json`, `qa-contact-sheet.jpg`, and the sampled frames under `work/qa/<output-name>/`.

Only mark the candidate `PASS` when all required checks succeed. Fix failures and rerender the smallest affected layer. If a check cannot run locally, label it unverified, set `ready_to_present` to false, and tell the user exactly what remains unchecked.

## Quality gate (binding)

Never mark a video complete because a render step returned without error. `PASS` means you personally opened the exact file the user will receive and every required check above passed against it — not that a prior stage reported success. The `show_video` presentation gate validates this report and refuses a missing, non-passing, mismatched, or evidence-free report.

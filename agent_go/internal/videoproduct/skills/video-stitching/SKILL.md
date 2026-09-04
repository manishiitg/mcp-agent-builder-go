---
name: video-stitching
description: Assemble an H3 Max continuity chain with a plain deterministic concat. Use for final assembly, or only when an H3 continuation has a visibly broken boundary.
---

# Assemble one coherent H3 Max film

H3 Max Reference-to-Video owns normal visual and audio continuity. This skill
does **not** use FFmpeg to manufacture it. Read
`multi-clip-cinematic-generation` before generating a sequence and
`video-editing` for the mechanical media operation.

## Default continuity path

For every normal follow-up, use
`minimax/h3-max/reference-to-video` and send the accepted immediate predecessor
as Video 1. Include the approved character, location, wardrobe/prop, and audio
references that matter, then state the intended next camera angle, subject
state, action, eyeline, screen direction, light, ambience, and dialogue in the
H3 prompt. A changed angle is an intentional cut when the prompt preserves the
scene state; it is not a reason to create an FFmpeg repair.

H3 returns a new bounded MP4; it does not append bytes to the predecessor.
Keep the accepted clips in their recorded order. For each candidate, record its
exact path, endpoint, prompt, references, and intended handoff.

## Lightweight clip receipt

After a clip downloads, confirm it is usable: inspect its duration and media
stream with `ffprobe`, and inspect its opening and ending stable frames. For a
successor, compare the predecessor's end and successor's beginning just enough
to catch an obvious visual discontinuity. Show the candidate as a preview.

Do not create a per-shot FFmpeg seam-preview, hand-pick default trim points,
produce a seam-proof document, or run the full final QA suite for each preview.
Those are not normal H3 generation requirements.

If the boundary is visibly wrong, regenerate or redesign the H3 successor with
the predecessor as Video 1 and a more precise handoff prompt/reference set.
Do not use a bridge clip, crossfade, optical blend, zoom, reframe, speed change,
audio patch, or other creative FFmpeg repair to hide it. Escalate a manual
editorial treatment only when the user explicitly requests one.

## Deterministic assembly

Build the assembly manifest from accepted clips in their intended order. Use a
plain direct concat. Before accepting that output, run the timestamp-boundary
receipt below. Normalize when the sources are technically incompatible **or**
when the receipt measures a timestamp discontinuity; do not alter timing,
frames, composition, picture, or sound to "repair" a normal H3 continuation.
Preserve captions or separately approved deterministic layers as their own
intentional work.

## Timestamp-boundary receipt

For an assembly of two or more clips, calculate every expected boundary from
the accepted source durations. At each boundary, inspect the **output video
stream** (`v:0`) frame timestamps immediately before and after the cut. The
observed interval must stay at the output frame duration, within a small
rounding tolerance. For example, a 24 fps delivery should have approximately
`0.041667` seconds between adjacent frames; a `0.062333` interval at a cut is a
technical gap even though both inputs decode and share a codec.

Also inspect audio packets around each boundary (`a:0`) for an unintended gap
or overlap. Keep video and audio checks separate: audio timestamps do not prove
the video cadence. Record the expected boundaries, measured maximum video-frame
interval at each one, audio result, and pass/fail evidence in the final quality
report.

If the direct concat fails this receipt, reassemble with deterministic timestamp
normalization (reset each input's video/audio PTS, then concatenate and encode
to the approved delivery settings). Re-run the same receipt against the new
file. This corrects mux timing only; it must never be described as fixing a
pose, camera, expression, lip-sync, or other visual discontinuity. If there is
no measured timing defect, do not re-encode merely to hide a visible H3 seam.

The output is a new final MP4 containing the source clips in sequence. It is
not a generated continuation and no source clip is overwritten.

## Final delivery gate

Run one full inspection only after the assembled delivery MP4 exists. Read
`video-quality`, with `generated-video-quality` as its H3-specific extension,
and produce the single required quality report for that exact final file. That
gate checks the delivered film as a whole, including its real joins, picture,
audio, and technical integrity. Present the final with `show_video` and its
passing report; previews do not need a final report.

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
plain direct concat. Normalize only when media specifications are technically
incompatible; do not alter timing, frames, composition, picture, or sound to
"repair" a normal H3 continuation. Preserve captions or separately approved
deterministic layers as their own intentional work.

The output is a new final MP4 containing the source clips in sequence. It is
not a generated continuation and no source clip is overwritten.

## Final delivery gate

Run one full inspection only after the assembled delivery MP4 exists. Read
`video-quality`, with `generated-video-quality` as its H3-specific extension,
and produce the single required quality report for that exact final file. That
gate checks the delivered film as a whole, including its real joins, picture,
audio, and technical integrity. Present the final with `show_video` and its
passing report; previews do not need a final report.

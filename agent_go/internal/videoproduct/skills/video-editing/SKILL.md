---
name: video-editing
description: Edit and assemble video clips with deterministic local tools, including trimming, normalization, hard cuts, captions, branded overlays, narration, music ducking, and versioned exports. Use when combining clips or uploaded media, adding captions or audio, resizing for a platform, or revising an existing edit.
---

# Edit and assemble video

Keep the edit reproducible. Prefer scripts and manifests under `work/` over one-off manual commands.

## Normalize source media

- Inspect every input with `ffprobe` before editing.
- Match the requested canvas without stretching. Scale to fit or fill intentionally, then pad or crop with the subject kept in the safe area.
- Normalize assembled segments to a common frame rate, pixel format, resolution, audio sample rate, and channel layout before concatenation.
- Use H.264 with `yuv420p` and AAC for broadly playable MP4 exports unless the request requires another format.

## Cut for pace and continuity

- Trim generated presenter clips to useful speech, including a small natural handle before and after the spoken words.
- Remove frozen openings, generation glitches, dead air, repeated phrases, and accidental endings.
- Use hard cuts between generated talking-head clips. Crossfades commonly produce a visible double face.
- Use dissolves only where both adjacent shots are visually compatible and the transition serves the story.
- Keep a frame-based timeline manifest so captions, cards, and overlays remain aligned after revisions.

## Captions and exact graphics

- Derive caption timing from word-level timestamps when available.
- Keep short-form caption chunks to roughly three words, split at punctuation, and prevent single-frame flashes.
- Render exact text, logos, prices, code, UI screenshots, and calls to action in a deterministic overlay layer.
- Respect title-safe margins and phone UI overlays. Check line wrapping at the target resolution.
- For programmatic animation, use a prop-driven template and stable composition identifiers containing letters, digits, or hyphens.

## Audio

- Preserve good native presenter audio when it is already lip-synced.
- Use TTS only for voiceover over silent or muted footage.
- Normalize dialogue first. Loop and trim music to the video duration, then duck it under speech with side-chain compression.
- Use short fades at the beginning and end of music, avoid abrupt cutoffs, and prevent clipping.
- Never add copyrighted music unless the user supplied it or its permitted source is clear.

## Outputs and revisions

1. Save intermediate normalized segments, captions, audio, and manifests under `work/`.
2. Export a preview without discarding its sources.
3. Run the `video-quality` skill.
4. Write the accepted candidate to `outputs/<slug>-vNN.mp4` without overwriting an older version.
5. Update `work/production.json` with the output path and its source inputs.

When the user requests a revision, change the smallest responsible layer. Reuse approved shots and audio instead of regenerating the whole video.

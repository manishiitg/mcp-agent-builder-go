---
name: video-editing
description: Edit and assemble video clips with deterministic local tools, including trimming, normalization, hard cuts, captions, branded overlays, narration, music ducking, and versioned exports. Use when combining clips or uploaded media, adding captions or audio, resizing for a platform, or revising an existing edit.
---

# Edit and assemble video

Keep the edit reproducible. Prefer scripts and manifests over one-off manual commands.

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

## Where your files go

Two contexts, and they do not share a folder:

- **Running as a workflow stage:** resolve every input from the exact paths the asset manifest records, and keep intermediates and rendered candidates inside your own step folder under `runs/<iteration>/<group>/execution/<stage>/`. Do not search `work/` or `outputs/` for inputs — a stage's media is not there, and treating those folders as the source of truth reports that nothing was produced when the files exist.
- **Working directly in chat:** intermediates under `work/`, finished videos at `outputs/<slug>-vNN.mp4`, never overwriting an older version.

## Outputs and revisions

1. Verify every input path exists and is readable before building the timeline. If an input is genuinely missing, stop and say which one rather than substituting your own placeholder or an unrelated file. An asset the manifest already marks `placeholder: true` is not missing — it is expected input right now, and the render assembled from it should be reported as a placeholder assembly, not treated as a failure.
2. Save intermediate normalized segments, captions, audio, and manifests beside the render.
3. Export a preview without discarding its sources.
4. Run the `video-quality` skill.
5. Record the rendered path with the inputs and commands that produced it, so the render can be reproduced or explained.

When the user requests a revision, change the smallest responsible layer. Reuse approved shots and audio instead of regenerating the whole video.

## Quality gate

- Every input resolved to a real file; nothing silently skipped.
- Output duration matches the plan, with no gap or overlap between segments.
- Speech is intelligible over music, and nothing clips.
- The report names the exact rendered file, its inputs, and the commands used.

## Common pitfalls

- Reporting a render that was never produced, or describing a file that is not on disk.
- Filling a genuinely missing asset with your own improvised placeholder or a reused clip instead of stopping — different from consuming an asset the manifest already marks `placeholder: true`, which is expected right now.
- Re-deriving generation prompts at edit time; by this point the media already exists or the stage should fail.
- Overwriting the previous accepted version instead of adding a new one.

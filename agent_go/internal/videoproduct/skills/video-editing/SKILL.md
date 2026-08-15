---
name: video-editing
description: Edit and assemble video clips with deterministic local tools, including trimming, normalization, hard cuts, captions, branded overlays, narration, music ducking, and versioned exports. Use when combining clips or uploaded media, adding captions or audio, resizing for a platform, or revising an existing edit -- including stitching multiple independently AI-generated clips (fal-ai, google-ai) into one long-form piece when each clip's duration doesn't match the storyboard beat.
---

# Edit and assemble video

Keep the edit reproducible. Prefer scripts and manifests over one-off manual commands.

## Own the HyperFrames runtime

When the approved production plan selects HyperFrames, install and run the current CLI through `npx --yes hyperframes@latest`. Use that full prefix for every HyperFrames command; do not depend on a global `hyperframes` binary and do not ask the user to install it. The first invocation downloads the latest published package automatically and later invocations reuse npm's cache.

Before authoring or rendering, verify the managed runtime:

```bash
npx --yes hyperframes@latest doctor --json
```

`doctor --json` always exits zero. Inspect its checks rather than using the top-level `.ok` as a universal render gate: current releases set `.ok: false` when optional transcription, local TTS, or local music fallbacks are absent. For ordinary local rendering, require Version, Node.js, FFmpeg, FFprobe, and Chrome; require Docker only for `render --docker`, and require optional media helpers only when the approved plan actually uses them. If npm cannot retrieve the package or a capability required by the selected operation is missing, record the exact failure instead of silently switching render runtimes. Never install HyperFrames globally and never change an approved render runtime merely because the global command is absent.

## Normalize source media

- Inspect every input with `ffprobe` before editing.
- Match the requested canvas without stretching. Scale to fit or fill intentionally, then pad or crop with the subject kept in the safe area.
- Normalize assembled segments to a common frame rate, pixel format, resolution, audio sample rate, and channel layout before concatenation.
- Use H.264 with `yuv420p` and AAC for broadly playable MP4 exports unless the request requires another format.

## Stitching independently AI-generated clips

A long-form piece assembled from `fal-ai`/`google-ai` generations is a
harder assembly problem than editing one continuous shoot: each clip was
generated independently, often by different models, with no natural
continuity between them, and generation models frequently have a fixed or
minimum output duration that will not exactly match what the storyboard
asked for.

- **Duration mismatch is normal, not an error.** If a beat needs 5 seconds
  and the model's minimum output is 8 or 15 seconds, do not assume the first
  N seconds are the right ones to keep. Inspect the full generated clip with
  sampled frames first, then trim to the strongest continuous span that
  serves the beat -- often the middle or end of the generated clip reads
  better than its opening, which frequently carries generation warm-up
  artifacts. If no span of the generated clip actually serves the beat at
  the needed length, that is a real constraint to report (shorten the beat,
  regenerate with a model whose native duration fits, or accept the closest
  available length) -- not something to force by stretching or padding.
- **Technical normalization is necessary but not sufficient.** Match frame
  rate, resolution, pixel format, and audio sample rate across every clip
  before concatenation (see "Normalize source media" above) -- different
  models frequently default to different specs. This does not fix a visual
  or color mismatch between clips from different models or providers, which
  is a separate, harder problem: two technically-identical clips from
  different generators can still look like they belong to different videos.
  Either keep a scene's shots on one model/provider when visual consistency
  matters most, or add a light, consistent color-grade pass across every
  clip in the assembly (matched black level, white balance, and saturation)
  rather than trusting raw generation output to match on its own.
- **Concatenate with ffmpeg's concat demuxer** once every segment is
  normalized to identical specs -- it is lossless and reliable for clips
  that already match:

  ```bash
  # segments.txt: one line per clip, in order
  #   file 'shot-01-hook.mp4'
  #   file 'shot-02-claim1.mp4'
  ffmpeg -f concat -safe 0 -i segments.txt -c copy final.mp4
  ```

  Use the concat *filter* (`-filter_complex concat=n=N:v=1:a=1`) instead
  when segments still need a per-clip filter (scale, fps conversion, a
  crossfade) applied as part of the same command, since the demuxer above
  assumes segments are already stream-compatible and only concatenates.
- **Transitions between independently-generated shots.** With no natural
  continuity to preserve, a hard cut is the safe default -- it does not ask
  the viewer to reconcile two generations' subtle differences the way a
  crossfade does. Pick one transition grammar for the whole piece (e.g.
  "hard cuts throughout" or "a specific dissolve style at scene boundaries
  only") and apply it consistently, the same way `video-cinematography`
  asks for consistent camera/lighting choices -- an inconsistent mix of
  transition styles reads as unintentional even when each individual cut is
  fine.
- Record which model/provider produced each segment and the exact trim
  points used in `production.json` (see `video-model-selection`), so a
  revision to one shot re-cuts only that segment instead of re-deriving the
  whole assembly.

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

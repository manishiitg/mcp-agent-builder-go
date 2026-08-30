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
- CRF around 23 is a reasonable default for intermediate/working renders; tighten to roughly 18-20 for the final deliverable export (matches the re-encode fallback in "Stitching at true long-form scale" below).

## Stitching independently AI-generated clips

For a multi-scene or long-form cinematic production, read
`longform-cinematic-video` first and treat its sequence plan, continuity
ledger, edit-decision list, and seam report as the governing assembly
contract. This skill supplies the deterministic editing mechanics; it does
not replace the production-level continuity decisions.

**Before stitching anything, check that each clip going into the assembly
was already shown to the user individually** (see `video-creation`'s
generation checkpoints). Combining clips into a preview is not a substitute
for that -- it is one more clip nobody has approved yet, and if the
combination is wrong there is no way to tell which of the unapproved clips
caused it. A stitched preview is something to build from approved parts,
not a way to get feedback on parts that were never shown.

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
  Keeping shots on one model and provider is what actually prevents this,
  and for any recurring character or subject that is already a hard rule
  decided upstream, not an option at edit time (see "Keep the whole
  character arc on one model and provider" in `video-cinematography`). At
  this stage the remaining lever is a light, consistent color-grade pass
  across every clip in the assembly (matched black level, white balance,
  and saturation) rather than trusting raw generation output to match on
  its own -- that narrows a residual mismatch, but it does not rescue a
  character generated on two different models, which needs regeneration
  upstream.
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
- **Transitions between independently-generated shots** are chosen by the
  rules in "Choosing a transition at each cut" below, which account for
  these clips having no natural continuity to preserve.
- Record which model/provider produced each segment and the exact trim
  points used in `production.json` (see `video-model-selection`), so a
  revision to one shot re-cuts only that segment instead of re-deriving the
  whole assembly.
- **Chain adjoining generated clips with an overlapping prompt, not just a
  shared reference image.** When two independently-generated clips are
  meant to flow together, write clip N's prompt so its last described
  moment matches clip N+1's prompt's first described moment (e.g. clip N
  ends "...he reaches the doorway", clip N+1 opens "he steps through the
  doorway..."). This is a `video-cinematography`-time decision, not an
  edit-time fix, but it directly determines how much of a visible seam
  remains for this skill to hide at the cut point -- flag it back to
  cinematography when a stitch keeps failing rather than trying to paper
  over a seam with an ever-longer crossfade.

## Narration drives the timeline, not the other way around

For a narrated long-form piece, the voiceover is the spine and the visuals
are cut to it. Build in this order -- reversing it is the most expensive
mistake available here, because it forces either re-generated clips or
narration rewritten to fit an arbitrary length:

1. **Lock the script first** (see `video-storytelling` for its shape).
2. **Generate the narration before generating any visuals for those
   beats** -- via a TTS model through `fal-ai`, or preserved native
   dialogue where a video model produced lip-synced speech (see
   `video-model-selection`'s native-audio criterion).
3. **Measure what you actually got**, per beat, with `ffprobe` -- do not
   plan against an estimate from the word count. Delivered TTS timing
   routinely differs from the estimate, and every downstream duration
   depends on it:

   ```bash
   ffprobe -v error -show_entries format=duration -of csv=p=0 vo-ch2-beat3.wav
   ```
4. **Derive the shot plan from those measured durations.** A chapter whose
   narration measures 90 seconds needs 90 seconds of visuals -- how many
   clips that is falls out of the chosen model's per-call duration (see
   `video-model-selection`'s shot-count arithmetic), rather than being
   picked first and forcing the narration to match.
5. **Then generate and trim visuals** to those durations, per the stitching
   guidance above.

**Generate narration in per-beat or per-chapter segments, not one long
pass.** A single 12-minute render cannot be fixed in one place without
re-timing everything after it; segmented VO lets one beat be regenerated
while every other beat's timing survives untouched. Name each segment for
its beat and record it in `production.json`.

Keep narration and the visual it describes simultaneous -- `video-storytelling`'s
temporal-contiguity rule is enforced here, at assembly, and it is the main
reason the visuals are cut to measured narration rather than estimated
narration.

## Choosing a transition at each cut

Pick the transition by what's actually changing at that cut, not by habit
or by what looks nicest in isolation. Decide the grammar once, at the start
of the assembly, and apply the same rule to every cut of the same kind --
the same way `video-cinematography` asks for consistent camera and lighting
choices. An inconsistent mix of transition styles reads as unintentional
even when each individual cut is fine.

| At this cut | Use | Typical duration |
|---|---|---|
| Same scene, same subject, action continues | Hard cut | 0 (instant) |
| Topic or beat changes, same chapter | Crossfade/dissolve | 0.5-1.0s |
| Major section or chapter boundary | Fade through black (or through the brand color) | 0.5-1.5s, longer for a bigger tonal break |
| Dialogue-driven scene, need to keep a reaction or audio bridging a cut | L-cut / J-cut (audio leads or trails the video cut) | audio offset 0.3-1.0s |
| Fast-paced montage or pattern-interrupt beat (see `video-storytelling`) | Hard cut, shorter and more frequent | 0 |

Match transition duration to content pace: quick, energetic sections read
better with faster transitions (or none); slower, reflective sections can
sustain a longer dissolve without feeling sluggish.

**Two rules override the table.** Both exist because a dissolve asks the
viewer to look at two images at once, which is exactly when
independently-generated footage gives itself away:

1. **Between two independently-generated clips, bias toward the hard cut.**
   Where the table's choice is close, take the cut -- it does not ask the
   viewer to reconcile two generations' subtle differences the way a
   crossfade does. Reserve the dissolve and fade-through-black rows for
   boundaries that genuinely earn them (a chapter break, a deliberate tonal
   shift), not for routine beat changes.
2. **Never crossfade between two generated talking-head shots** -- it
   produces a visible double face. Use a hard cut, or cut away to B-roll
   and back.

## Stitching at true long-form scale (dozens of clips)

A true long-form (8-15+ minute) piece assembled from many short generated
clips is a materially bigger assembly problem than stitching a handful of
shots -- normalization and transition mistakes that are barely visible once
compound into an obviously inconsistent piece across dozens of cuts. A few
failure modes are common enough at this scale to check for explicitly
rather than discovering after a full render:

- **Codec/spec mismatch across a large batch.** Check every clip with
  `ffprobe`, not just a sample -- one clip generated by a different model or
  a re-generated single shot silently reintroduces a spec mismatch that
  breaks `-c copy` concatenation for the whole batch:

  ```bash
  ffprobe -v error -select_streams v:0 \
    -show_entries stream=codec_name,pix_fmt,r_frame_rate,width,height \
    -of csv=p=0 shot-*.mp4
  ```

  Re-encode only the minority that doesn't match the majority spec, rather
  than re-encoding everything (slower, and a second lossy pass on clips that
  didn't need it):

  ```bash
  ffmpeg -i mismatched.mp4 -c:v libx264 -crf 18 -preset medium \
    -pix_fmt yuv420p -r 30 -c:a aac -b:a 192k normalized.mp4
  ```
- **Audio drift over a long stitch.** Small per-clip timestamp rounding
  errors accumulate across dozens of concatenated segments and can leave
  audio audibly out of sync by the end of a long piece even when each clip
  looked fine individually. Force constant frame rate and audio sync on any
  source that might be variable frame rate before concatenating:

  ```bash
  ffmpeg -i vfr_input.mp4 -vsync cfr -r 30 -async 1 cfr_output.mp4
  ```
- **Aspect ratio mixing.** Never stretch a mismatched clip to fit -- pad
  (letterbox/pillarbox) or crop with the subject kept in the safe area,
  same as "Normalize source media" above, applied per-clip before the batch
  concat, not corrected after.
- **Stream-copy concat failure.** If `-c copy` concatenation errors or
  produces corrupt output despite specs looking identical, fall back to a
  full re-encode of the final assembly rather than debugging the exact spec
  mismatch further when time-boxed:

  ```bash
  ffmpeg -f concat -safe 0 -i segments.txt \
    -c:v libx264 -crf 18 -preset medium -c:a aac -b:a 192k final.mp4
  ```
- **Sustained audio consistency across the whole runtime**, not just
  clip-to-clip: keep one continuous music bed under the whole piece rather
  than restarting a music cue per chapter (restarting reads as several
  short videos stitched together, not one long one). Keep tempo within
  roughly ±10 BPM across the entire video -- a tempo jump between chapters
  is as noticeable as a visual jump cut -- and keep per-chapter loudness
  variation under about 2 LUFS so no chapter reads as noticeably louder or
  quieter than its neighbors.

### Audio level targets

These are three different measurements, and conflating them is the usual
cause of a mix that meets its target number and still sounds wrong. Set the
delivery target first, then place the stems inside it:

**The finished mix** -- measured across the whole program, this is the
delivery target: **-14 LUFS integrated**, with **-1 dBTP** true peak
headroom to leave margin against inter-sample clipping on export.

**Each stem, measured in isolation** -- how the elements sit inside that
mix. Narration is the anchor; everything else is placed relative to it:

| Stem | Target | Notes |
|---|---|---|
| Narration/dialogue | -16 LUFS | the intelligibility anchor -- set this first, then place everything else against it |
| Music under narration | -28 to -24 LUFS | roughly 8-12 dB below the narration |
| Music in a passage with no narration | around -18 to -16 LUFS | comes up so the moment isn't a hole, without overshooting the program target |
| SFX | -20 LUFS | push above this only for a deliberate accent, and only briefly |

**Duck depth** -- how far the music drops when narration is present, which
is a *gain reduction*, not a level: **8-12 dB**. Stating a duck as an
absolute LUFS value is ambiguous and usually wrong; a music bed sitting
around -16 LUFS solo, ducked 8-12 dB, lands in the -28 to -24 LUFS band the
table above gives for music under narration -- which is the check that
these numbers agree.

## Cut for pace and continuity

- Trim generated presenter clips to useful speech, including a small natural handle before and after the spoken words.
- Remove frozen openings, generation glitches, dead air, repeated phrases, and accidental endings.
- Choose each transition by the rules in "Choosing a transition at each cut" above, including its two overrides.
- Keep a frame-based timeline manifest so captions, cards, and overlays remain aligned after revisions.

### What to cut, and what to deliberately leave in

This applies to speech you did not author -- an uploaded recording, or a
generated clip with native dialogue. Narration you generated yourself from
a script needs no cleanup pass: you already have the exact words, and TTS
does not produce filler or false starts.

Where a cleanup pass is needed, `silencedetect` (below) locates candidate
silences precisely; word boundaries otherwise come from listening, since no
transcription capability is established here. Which category a moment falls
into is an editorial judgment call either way, not something timings
answer on their own:

| Cut | Leave in |
|---|---|
| Filler words at word boundaries ("um", "uh", "like" as a verbal tic) | Breath pauses (roughly 0.3-0.8s) -- removing these makes speech sound unnaturally clipped |
| False starts (a restarted sentence) | Emphasis pauses -- a deliberate beat before or after a key point |
| Dead air/silence beyond a natural pause | Verbal bridges ("So...", "Now...", "Okay,") -- these carry pacing and signal a topic shift, cutting them makes adjacent beats feel abrupt |
| Off-topic tangents that don't serve the beat | |
| Redundant retakes of the same line | |

For dead air specifically: silence beyond roughly 1.5 seconds reads as a
mistake, not a pause -- trim it down to about 0.5s rather than removing it
entirely (a hard zero-length cut there reads as unnatural). If detecting
silence programmatically (e.g. ffmpeg's `silencedetect` filter), a
threshold around -35dB with a 0.5s minimum duration is a reasonable
starting point, with roughly 0.08s of padding kept on each side of a cut so
a trimmed word's onset/decay isn't clipped -- treat these as a starting
point to tune per source, not a fixed rule.

### Cut types

| Type | What it is | Typical use |
|---|---|---|
| Hard cut | Instant transition, no overlap | Default between independently generated shots (see above); also the default within one continuous take |
| J-cut | Audio from the next shot starts before its video does | Let a reaction or ambient sound bridge into the upcoming shot |
| L-cut | Audio from the current shot continues after its video ends | Keep a speaker's voice over a cutaway/B-roll shot |

A J/L-cut's audio typically leads or trails the video cut by roughly
0.3-1.0s -- tune to what the beat actually needs rather than a fixed offset.

### Pacing by duration

Match cut frequency and shot-holding time to the piece's overall duration
and its momentary pace (see `video-storytelling`'s pattern-interrupt
cadence) -- a short-form cut, a fast-paced long-form pattern interrupt, and
a reflective long-form chapter beat call for different rhythms:

- **Short-form / fast-paced sections**: aggressive cutting, short shot
  holds, frequent visual change -- momentum is the point.
- **Long-form, most of the runtime**: let scenes breathe -- a shot doesn't
  need to change every couple of seconds just because it can; over-cutting
  a reflective or explanatory beat undercuts it rather than energizing it.
  Reserve the faster cadence for pattern-interrupt beats specifically, not
  as the piece's constant default.

### Speed adjustments

When a clip needs to play faster or slower (e.g. speeding through a trimmed
silence instead of cutting it, or a deliberate slow-motion beat), change
video and audio together so they stay in sync: `setpts=<1/factor>*PTS` for
video, and ffmpeg's `atempo` filter for audio, chained multiple times for a
factor outside `atempo`'s single-call 0.5-2.0 range (e.g. `atempo=2.0,
atempo=1.5` for a 3x speed-up) -- a single out-of-range `atempo` call
errors rather than clamping silently.

### Choosing a cut point inside a generated clip

When trimming a generated clip to the strongest continuous span (see
"Duration mismatch is normal" above), extract candidate frames near likely
cut points with ffmpeg and actually look at them -- a cut landing on a
blurry, blown-out, or motion-smeared frame reads as a mistake rather than a
stylistic choice:

```bash
# one frame at a candidate cut point
ffmpeg -ss 00:00:03.5 -i shot-07.mp4 -frames:v 1 -q:v 2 frame-3.5s.jpg
```

What to reject a candidate frame for: visible blur or motion smear,
crushed/blown exposure, a flat low-contrast image, a subject caught
mid-blink or mid-gesture, or a generation artifact. Inspecting the frame
is the check -- there is no scoring tool here beyond your own eye, so do
not report a numeric quality score as if one had been computed.

ffmpeg can narrow *where* to look before you inspect: `blackdetect` finds
black frames worth avoiding as cut points, and `freezedetect` finds frozen
spans, which frequently mark a generation's warm-up or stall.

## Captions and exact graphics

- Derive caption timing from word-level timestamps when available.
- Keep short-form caption chunks to roughly three words, split at punctuation, and prevent single-frame flashes.
- Render exact text, logos, prices, code, UI screenshots, and calls to action in a deterministic overlay layer.
- Respect title-safe margins and phone UI overlays. Check line wrapping at the target resolution.
- For programmatic animation, use a prop-driven template and stable composition identifiers containing letters, digits, or hyphens.

### Typography and safe-zone specs

Numeric starting points -- confirm against the actual target platform/
resolution rather than treating these as universal:

- **Title-safe margin**: keep essential text within the inner 80% of the
  frame (roughly 192px margin at 1080p). **Action-safe margin**: keep
  meaningful motion/subject placement within the inner 90% (roughly 96px
  margin at 1080p) -- title-safe is the tighter of the two.
- **Caption sizing**: 42px or larger at 1080p, roughly 32-42 characters per
  line, sized proportionally at other resolutions.
- **Caption pacing**: a reading speed around 21 characters/second is a
  reasonable target for how long a caption should stay on screen; keep
  individual caption duration in roughly a 1-6 second range, and leave a
  small gap (a couple of frames) between consecutive captions rather than
  a hard back-to-back cut, which reads as a flicker.
- **Legibility over busy footage**: a semi-transparent background box,
  a text stroke/outline, a drop shadow, a darkened gradient region behind
  the text, or (as a last resort) a full-opacity overlay panel -- pick
  whichever is least visually intrusive for the shot. Target at least a
  4.5:1 contrast ratio between text and its background (7:1 for anything
  meant to be read quickly or by a wider range of viewers).
- **Font pairing**: no more than two font families in one piece (a display
  face for titles, a clean face for body/captions), and keep at least a
  ~50% size difference between a title and body text sharing the frame so
  the hierarchy reads instantly.
- **Motion easing**: use an eased curve for text/graphic entrances and
  exits, not linear motion -- an ease-out curve (fast start, settling to a
  stop) reads well for an element entering frame, an ease-in curve
  (accelerating away) for an element exiting. Linear motion on UI-style
  elements reads as mechanical rather than intentional.

## Audio

- Preserve good native presenter audio when it is already lip-synced.
- Use TTS only for voiceover over silent or muted footage. For a narrated long-form piece, generate it before the visuals it narrates and measure it -- see "Narration drives the timeline, not the other way around".
- Normalize dialogue first. Loop and trim music to the video duration, then duck it under speech with side-chain compression, targeting the 8-12 dB duck depth in "Audio level targets" -- ffmpeg's `sidechaincompress` filter with a threshold around 0.02, ratio around 9, attack around 200ms, and release around 500ms is a reasonable starting point to tune per mix.
- Use short fades at the beginning and end of music, avoid abrupt cutoffs, and prevent clipping.
- Never add copyrighted music unless the user supplied it or its permitted source is clear.
- Hit the numbers in "Audio level targets" above -- the -14 LUFS / -1 dBTP
  delivery target for the finished mix, and the per-stem levels inside it.
  For anything longer than a couple of minutes, also apply "Sustained audio
  consistency across the whole runtime" so the mix doesn't drift chapter to
  chapter.

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
- For a multi-chapter long-form assembly: no audible tempo/loudness jump
  between chapters, one continuous music bed rather than restarted cues,
  and every `ffprobe` spec check passed before the final concat (not just
  assumed from matching source models).

## Common pitfalls

- Reporting a render that was never produced, or describing a file that is not on disk.
- Filling a genuinely missing asset with your own improvised placeholder or a reused clip instead of stopping — different from consuming an asset the manifest already marks `placeholder: true`, which is expected right now.
- Re-deriving generation prompts at edit time; by this point the media already exists or the stage should fail.
- Overwriting the previous accepted version instead of adding a new one.

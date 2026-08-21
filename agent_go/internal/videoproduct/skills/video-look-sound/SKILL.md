---
name: video-look-sound
description: Lock the visual world and complete audio direction before narration or footage generation. Use when deciding locations, backgrounds, wardrobe, lighting, palette, narrator or dialogue voice, music, ambience, sound effects, captions, and whether speech should be separate TTS or native model audio.
---

# Direct the look and sound

Create one production bible that later narration, shot planning, generation,
assembly, and QA stages can execute without reinterpreting taste or audio intent.
Do this after the script and recurring-character references are settled, but
before generating narration or visual clips.

## Lock the visual world

Record decisions that must remain stable across shots:

- locations and backgrounds per beat, including time of day and practical set anchors;
- recurring wardrobe, hair, props, product markings, and other identity anchors;
- lighting direction, contrast, color temperature, palette, texture, and realism level;
- camera grammar: framing range, lens feel, movement, depth of field, and transitions;
- aspect ratio, composition safe areas, and caption-safe regions.

Separate invariants from intentional changes. A character may move to a new
location, but their approved identity and wardrobe do not drift unless the
script explicitly calls for that change.

## Lock the sound world

Record all audible and readable layers:

- speech mode: separate narration, native dialogue, or explicitly no speech;
- voice provider/model or recorded source, voice ID, language, accent,
  warmth, energy, pacing, pronunciation notes, and performance references;
- music role, genre, tempo range, emotional arc, instrumentation, and where it yields;
- ambience and room tone per location;
- motivated sound effects and transition sounds;
- caption wording source, style, placement, line length, and timing policy.

Do not leave a generic instruction such as "warm voice" or "cinematic music."
Write enough specific direction that a replacement segment still sounds like
the same production.

## Choose speech deliberately

For instructional, tutorial, and explainer videos with spoken guidance, use
separate TTS narration by default. Exact wording, a consistent voice,
replaceable corrections, and reliable timing matter more than the convenience
of model-native speech. Only use native model dialogue when visible lip-synced
performance or natural interaction is part of the creative requirement and
the selected endpoint can reliably support it.

Native model audio remains useful for natural dialogue, ambience, Foley, and
location sound. It does not make required narration optional. Never turn a
spoken instructional brief into a silent video because a generated visual clip
arrived without an audio stream.

If the user has not chosen a material direction, offer at most three viable
choices, explain the tone and cost consequence, recommend one, and wait for the
choice before paid generation. Do not ask again about a choice already settled
in the conversation.

## Build around real narration timing

When separate narration is selected:

1. Lock the exact script.
2. Generate narration in beat-level segments, not one monolithic file.
3. Measure every segment with `ffprobe`; never substitute word-count estimates.
4. Derive the shot list and visual duration from those measured values.
5. Generate and trim visual clips to that audio timeline.
6. Assemble narration, music, ambience, effects, and captions with the visuals.

If speech is explicitly not applicable, record the approved timing source—such
as a music edit, action beats, or native dialogue—so the shot list still has a
concrete clock.

## Keep previews honest

Show every generated visual clip before assembly. If a separate narration and
mix will be added later, label it clearly as a silent visual preview. Never
present that preview as the finished video or imply that missing audio will fix
itself.

## Enforce the final promise

At assembly and QA, compare the actual export with this direction document.
An instructional video that promised narration fails when narration is missing,
silent, unintelligible, materially different from the approved script, or
misaligned with the visuals. It may not mark audio or captions
`not_applicable`. Also verify voice consistency, pronunciation, music ducking,
ambience continuity, motivated effects, caption accuracy and timing, and the
visual invariants above.

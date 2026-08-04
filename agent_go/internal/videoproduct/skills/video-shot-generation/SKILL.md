---
name: video-shot-generation
description: Produce the shots a scene plan calls for — today with local ffmpeg synthesis, and with an AI provider once one is configured. Use when the user needs new footage for a scene, a placeholder for a shot that has no source material, or revisions to a generated shot.
---

# Produce video shots

Produce short, controllable shots and assemble them later. Do not ask one generation to produce a complete multi-scene edited video.

## Where your files go

Two contexts, and they do not share a folder:

- **Running as a workflow stage:** write every file inside your own step folder under `runs/<iteration>/<group>/execution/<stage>/`, and record each path in the stage's manifest. `work/` and `outputs/` are not yours and are normally empty.
- **Working directly in chat:** use `work/` for intermediates and `outputs/` for finished videos.

## No provider is configured — synthesize locally with ffmpeg

There are no provider API keys wired into this product yet, so this skill's job today is producing a **clearly-marked placeholder** for every shot the scene plan calls for, using `execute_shell_command` and `ffmpeg` only. Never call a provider, never inspect environment variables or credentials to look for one, and never claim a shot came from AI generation when it did not.

What a placeholder can be, matched to the scene plan's duration, aspect ratio, and resolution:

- a solid or gradient background (`color`, `gradients` filters) standing in for a shot's mood and palette;
- a drawtext card standing in for a beat that needs on-screen text, a title, or a name;
- a slow `zoompan` or crop pan over a still standing in for simple camera motion;
- a `sine` tone or `anullsrc` silence standing in for a music bed or missing narration.

Before producing a full set, make **one** shot and check it with `ffprobe` — dimensions, duration, frame rate — against the scene plan. A wrong aspect ratio or duration caught on shot one is a two-second fix; caught after all of them, it's a full redo.

Mark every one `placeholder: true` in the manifest. A placeholder is there so the pipeline can be tested end to end — timing, transitions, mux, QA — without it ever being mistaken for finished creative material.

### Quality gate (placeholder mode)

- every shot the plan asks for is either saved on disk or recorded as `failed` with the command's error;
- every recorded path resolves to a real, non-empty file;
- each file's actual duration, resolution, and aspect ratio match what the scene plan asked for, verified with `ffprobe` — not assumed from the command that made it;
- every entry is marked `placeholder: true`.

### Common pitfalls (placeholder mode)

- Recording a shot as produced because the `ffmpeg` command exited 0, without checking the file it wrote.
- Letting a placeholder's duration drift from the scene plan because it was copy-pasted from an earlier shot.
- Omitting `placeholder: true`, so a later stage or the user mistakes it for real creative material.

## Once a provider is configured

The rest of this skill is for when real AI shot generation is available. Do not follow it while producing placeholders above; it describes a different production path, not an enhancement to the placeholder one.

### Lock appearance with references

- Create or select a neutral reference image for each recurring person, product, and important set.
- Use the image for appearance and the prompt for performance. Keep anchor faces calm, front-facing, evenly lit, and free of baked-in emotion or action.
- Ensure the written description agrees with every reference. Conflicting wardrobe, color, age, or set details cause drift.
- Prefer asset/reference-image conditioning when the provider supports it. Use a start frame only when the exact opening pose or camera composition matters.
- For multiple people, reference both people or use a clear two-shot reference. Review continuity for each person separately.

### Write one prompt per shot

Use this order:

`STYLE + CHARACTER/PRODUCT + SHOT AND FRAMING + ACTION + DIALOGUE/DELIVERY + AMBIENCE + EXCLUSIONS + FORMAT`

Repeat the style and identity blocks verbatim across related shots. Change only the framing, action, and dialogue needed for that beat.

Always specify:

- one subject action and one camera behavior;
- setting, lighting, mood, and intended realism;
- aspect ratio and centered/safe composition;
- either a deliberate camera movement or a static camera;
- `No captions, subtitles, text, logos, or watermarks` when text will be added later.

For dialogue, put spoken words in double quotes and put delivery direction immediately before them, for example: `She says, (warm and assured) "..."`. Do not use separate TTS when the generated presenter already has good native lip-synced speech.

Avoid vague instructions, conflicting styles, multiple scene changes, tiny precise gestures, generated UI, and readable on-screen text.

### Preserve continuity

- Prefer separate referenced shots joined with hard cuts for talking heads and dialogue.
- Do not use last-frame chaining for fast mouth motion; identity drift compounds across hops.
- Use frame chaining only for slow continuous movement where motion continuity is more important than perfect identity.
- For shot/reverse-shot dialogue, lock both characters and cut between views.
- Anchor recurring set pieces as well as faces when the background must remain stable.

### Run generations safely

1. Generate by calling the registered tools — `image_gen`, `image_edit`, `generate_video`, `text_to_speech`, `generate_music`. Call them as tools. Never invoke a provider through a shell command or raw HTTP, and never read environment variables or credentials to reach one: the tools are already authenticated, and a shell that appears to lack a token is not evidence that generation is unavailable.
2. Write down the prompt and settings for each shot before generating, so a regeneration is reproducible rather than improvised.
3. Generate the minimum number of shots at draft quality first. Reserve higher-cost quality for selected finals.
4. Save each successful result immediately, then verify the file exists and is non-empty before treating the shot as done.
5. Retry a transient empty response or provider error at most once unless the user asks to keep trying.
6. Never regenerate a usable clip merely to obtain metadata; inspect the saved file locally.

Before committing to a full set, generate **one** shot and check it. A single sample catches a wrong aspect ratio, a drifting face, or a misread prompt for the price of one generation instead of the whole batch.

If a generation fails, record it as failed with the provider's error. Never describe a shot as generated unless you created the file and confirmed it on disk — a manifest entry for a file that does not exist fails the compose stage later, further from the cause.

### Check each shot immediately

Before generating the next expensive variant, sample the start, middle, and end frames and verify:

- identity, product, wardrobe, and set continuity;
- natural hands, faces, mouth motion, and camera motion;
- correct speech and usable audio;
- no garbled text, watermark, black frame, or frozen opening;
- enough clean handles for trimming.

Record acceptance, or the exact reason for regenerating, next to the shot — in the stage manifest when running as a stage, in `work/production.json` in direct chat.

### Quality gate (provider mode)

Do not report the shot set as done until each of these is true:

- every shot the plan asks for is either saved on disk or recorded as `failed` with its error;
- every recorded path resolves to a real, non-empty file;
- recurring faces, products, and sets look like the same subject across shots;
- no shot carries garbled text, a watermark, a black frame, or a frozen opening;
- the manifest names the provider/model, duration, resolution, and aspect ratio for each generated file.

### Common pitfalls (provider mode)

- Asking one generation for a whole multi-scene edit instead of separate shots.
- Letting the prompt and the reference image disagree about wardrobe, age, or colour, then blaming the model for drift.
- Chaining last frames through fast mouth motion, so identity degrades a little at every hop.
- Generating the full set before checking one, and paying for the same mistake N times.
- Recording an asset as generated because the call returned, without opening the file.

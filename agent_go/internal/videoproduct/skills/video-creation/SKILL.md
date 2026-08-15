---
name: video-creation
description: Direct a video project from a conversational brief through a reproducible production. Use when creating a new video, deciding the production approach, turning uploaded assets into a video, scripting or storyboarding, or revising an existing video.
---

# Direct video creation

Treat the conversation as the creative brief. Do not force the user through a formal plan or expose implementation details unless asked.

## Work inside the project

- Treat `uploads/` as immutable user-owned source material.
- **Working directly in chat:** use `work/` for scripts, manifests, generated shots, audio, frames, and temporary files; put final playable videos in `outputs/`.
- **Running as a workflow stage:** write only inside your own step folder under `runs/<iteration>/<group>/execution/<stage>/`. `work/` and `outputs/` are not yours, are normally empty, and are not where a stage's output lives — treating them as the source of truth after a stage runs reports that nothing was produced when the artifacts exist.
- Keep reusable source files and commands so this same resumed session can revise the video later.
- Never publish, share, or upload a result.

## Understand the request

Infer as much as possible from the chat and uploaded assets. Establish only the details that materially affect the result:

- intended audience and outcome;
- platform, aspect ratio, resolution, and approximate duration;
- core message, tone, visual references, and call to action;
- narration, presenter, music, captions, and brand assets;
- whether to edit supplied footage, generate new shots, or combine both.

Ask one concise question only when a missing choice would substantially change the result or cost. Otherwise make a reasonable choice and state it briefly.

## Inspect before producing

1. List the files in `uploads/` and inspect their type, dimensions, duration, frame rate, audio streams, and orientation with local tools such as `file`, `ffprobe`, and sampled frames.
2. Never infer asset contents from filenames alone.
3. Preserve logos, faces, product appearance, and brand colors from supplied references. Do not invent exact brand claims.
4. Prefer editing existing assets and deterministic composition before paying to regenerate usable material.

## Shape the story

- Give every video one clear job and one primary audience.
- For short-form video, prefer `hook -> value/proof -> action`.
- Keep each shot responsible for one idea. Match the visual change to the spoken beat.
- Put exact wording, logos, UI, prices, and captions into the editing/overlay layer, never into an AI-generated shot.
- For presenter reels, a reliable pattern is host hook, screenshot-worthy value card while narration continues, host close/CTA, then a short end card.

## Make the work resumable

In direct chat, create or update `work/production.json` before substantial media work. Record:

- target specifications and creative direction;
- source assets and their roles;
- script or beat list;
- shot status and generated filenames;
- music, caption, and overlay decisions;
- final output versions and QA status.

Use stable, descriptive filenames. Never overwrite an approved output; write `outputs/<slug>-v01.mp4`, then increment the version.

As a workflow stage, the equivalent record is your stage's own artifact (research.md, proposal.md, script.md, scene-plan.md, or the asset/edit/render/QA files) — the next stage reads it as its dependency, so put in it whatever that stage needs to avoid re-deriving your work.

## Choose the production path

- Use local editing for trims, crops, concatenation, audio, captions, and supplied footage.
- Use programmatic overlays for exact text, branded cards, product UI, or repeatable templates.
- For product-led explainers, feature breakdowns, and short-form pieces, build from uploaded assets and deterministic HTML/CSS composition (`product-infographic` / HyperFrames) rather than generating footage -- exact wording, UI, and prices belong in that layer, not an AI-generated shot.
- For narrative long-form video where the brief genuinely calls for AI-generated footage, reference imagery, voice, or music: use `video-model-selection` to choose between `fal-ai` (third-party hosted models) and `google-ai` (Google's own Gemini image models, Veo) per shot, `video-cinematography` to turn each beat into camera/lighting/consistency direction, then the chosen provider skill to generate. This is the exception, not the default: prefer uploaded assets and deterministic composition whenever they can carry the brief.
- Use the `video-editing` skill for assembly, captions, audio, and exports.
- Use the `video-quality` skill before presenting a version as complete.

Avoid paid multi-variant generation unless the user requests alternatives. Cache every successful generation and never repeat a paid call merely to inspect its response.

## Finish the turn

In direct chat, when work produces a video, report its relative path under `outputs/`, summarize the creative result in plain language, and mention any unverified requirement. Do not include provider or command details unless asked.

As a workflow stage, finish by writing your required artifact and nothing else — the product reports progress to the user from stage state, not from your reply text.

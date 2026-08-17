---
name: product-infographic
description: Create or revise product infographic and explainer videos in Video Studio. Use for product launches, feature explanations, product tours, facts, statistics, comparisons, pricing, processes, or short product-led motion graphics. Own the user brief and route the confirmed production into the appropriate installed HyperFrames skill.
---

# Product infographic director

Own the outcome while HyperFrames owns its composition craft. Do not duplicate or improvise HyperFrames technical rules: read the installed specialist skill for the selected route, then the technical skills it directs you to use.

## Start with the current production

Treat a new product, brand, subject, or ground-up concept as a new production. Keep each production at:

```text
work/productions/<stable-slug>/
```

Use a new versioned slug only for a genuine ground-up alternative. A revision to copy, timing, layout, motion, sound, aspect ratio, or assets stays in the existing production and produces a new render version.

Before asking anything, read the conversation and relevant uploads. Do not ask for information already provided. Ask one compact group of only the unresolved, consequential questions:

1. What single message should the viewer remember?
2. Who is it for and where will it be watched?
3. Which product URL, UI, uploaded assets, claims, prices, numbers, names, or wording are authoritative?
4. What duration and aspect ratio are required?
5. Is there a required brand/reference direction?
6. Should it use narration, music, captions, or remain visual-only?
7. Does the user want a storyboard review, or should the agent build immediately?

Duration defaults to a concise piece appropriate to the request; aspect ratio defaults to 16:9. Never invent a factual claim because an answer is missing. If the user says to just build, state the important defaults briefly, write them to the brief, and proceed.

The seven HyperFrames prompting levels are a capability ladder, not seven mandatory interview rounds or workflow stages. Apply Levels 1–2 to every production; use motion, substance, sound, scale, and capstone techniques only when the brief benefits from them.

Write the confirmed decisions to `BRIEF.md` inside the production directory. It is the durable source for future turns and must include the chosen route, message, audience, format, exact claims/sources, visual direction, sound direction, review mode, exclusions, and unresolved blockers.

## Choose exactly one primary HyperFrames route

Read `skills/hyperframes/SKILL.md` first, then select and read the primary route:

- `skills/product-launch-video/SKILL.md` when real product screens, a product URL, UI states, or a product tour provide the proof.
- `skills/faceless-explainer/SKILL.md` when notes, documentation, facts, or a difficult idea should become invented typography, diagrams, or data visualisation.
- `skills/motion-graphics/SKILL.md` for one short logo, statistic, quote, feature, or visual beat.
- `skills/general-video/SKILL.md` only when the request is mixed or no specialist route fits.

Do not run a second generic interview after `BRIEF.md` is confirmed. Pass its decisions into the chosen route and ask again only when the source contradicts the brief or a consequential requirement remains genuinely unknown.

## Build one editable HyperFrames project

The source project, preview, checks, Studio, and render must all use the same files:

```text
BRIEF.md
STORYBOARD.md
SCRIPT.md             # only when narration or locked words need it
frame.md              # visual and motion system when useful
index.html
hyperframes.json
compositions/
assets/
snapshots/
renders/
```

Copy or derive working assets into the production's `assets/`; never modify `uploads/`. Preserve sources for factual claims in the brief or storyboard. Use variables for approved content that should change across versions—titles, logos, colors, prices, customer names—without rebuilding the layout.

Before authoring, read the relevant installed technical skills:

- `hyperframes-core` for composition structure and timing.
- `hyperframes-creative` for the design specification, beats, typography, palettes, and narration.
- `hyperframes-animation` for seek-safe motion and transitions.
- `media-use` when voice, sound, captions, images, or supplied footage are involved.
- `hyperframes-registry` before discovering or installing a catalog block/component.
- `hyperframes-keyframes` for keyframe authoring or diagnostics.
- `hyperframes-cli` before init, preview, lint, check, snapshots, diagnostics, or render.

Read only the skills the production needs. The presence of a capability is not a reason to add it.

## Preserve product-infographic quality

- Open with a question, useful tension, visible product proof, or a meaningful result—not a generic logo reveal.
- Land the main idea by the second scene.
- Let each scene advance one mechanism, feature, step, example, proof point, or implication.
- Use a chart to prove a claim, a diagram to reveal a mechanism, and motion to show what changes.
- Keep one coherent visual language across scenes.
- Make exact product UI and important text readable at playback size.
- Trace every number and factual claim to an authoritative source.
- End on the one sentence or action the viewer should remember.

## Validate before presentation

Use the CLI skill's current commands. At minimum, `lint` and `check` must pass before rendering. Inspect representative snapshots and key moments, fix defects, and keep project-local render candidates in `renders/`. Show each reviewable candidate with `show_video` as a Preview so the creator can play it without navigating Files. For direct-chat delivery, copy the chosen candidate without re-encoding to a new versioned path at `outputs/<slug>-vNN.mp4`, then run the separate `video-quality` skill against that exact output path. Its report must name the same output passed to `show_video` when updating that Preview to an approved final. A successful render is not a quality pass.

For revisions, change the smallest responsible layer and preserve the previous render. Do not restart the full workflow unless the user asks for it or the production is being reconceived from the ground up.

---
name: longform-cinematic-video
description: Direct a coherent cinematic video of any duration from story architecture through deterministic or generated sequences, continuity-controlled clips, editorial stitching, sound, and final seam review. Use as the default director for every new Video Studio production so scenes feel like one continuous authored film rather than separate generations.
---

# Direct one film, not a collection of clips

Own the production-level decisions that cross model generation and editing.
Read `video-storytelling`, `video-cinematography`,
`video-provider-capabilities`, `video-editing`, and the selected model-family
skill for their specialist rules. Keep those responsibilities coordinated by
one sequence plan and one editorial grammar.

## Define the cinematic contract

Before scripting, lock the film's dramatic question, audience, runtime,
format, emotional arc, point of view, visual language, camera grammar, color
journey, sound world, and continuity priorities. State what must remain exact
across the film: character identity, wardrobe, props, geography, time of day,
screen direction, eyelines, lighting motivation, and recurring audio motifs.

Choose a small visual vocabulary and repeat it intentionally. A cinematic
film does not need a new style, lens, camera move, transition, or model for
every beat. Treat a model or provider change inside one continuous arc as a
continuity risk requiring an explicit reason.

## Plan sequences before shots

Organize the film as chapters, sequences, scenes, and only then shots. Give
each sequence an entrance state, dramatic turn, exit state, location,
characters, time, lighting, sound, and continuity bridge into the next
sequence. A script beat is not automatically a new generated clip.

Create `longform-sequence-plan.json` before paid video generation. Include:

- chapter, sequence, scene, and shot identifiers;
- measured narration or dialogue duration covered by each sequence;
- model route and why it fits the sequence;
- references and their exact semantic roles;
- incoming and outgoing character, prop, geography, motion, camera, lighting,
  and audio state;
- generation topology: single take, structured multi-shot, extension,
  reference-video continuation, boundary-frame chain, or intentional hard cut;
- planned cut point, transition grammar, handles, and expected seam risk;
- the reason every independent generation is unavoidable.

Minimize generations and seams. Prefer one supported longer take or an
in-model multi-shot sequence when the action is continuous. When a new call is
necessary, prefer extension or reference-video continuation; otherwise carry
the accepted outgoing frame and state into the next request. Use independent
clips freely only for motivated discontinuity such as a montage, time jump,
location change, or deliberate cutaway.

## Generate for the edit

Generate sequence by sequence, not as an unrelated batch. Preserve the same
approved model route, reference order, seed when supported, character spec,
wardrobe, geography, lighting, motion vector, and audio bed across adjoining
clips. Describe overlapping action at a seam so the editor can cut on motion.
Ask for clean head and tail handles without filling them with new actions.

Show every new clip with `show_video` and obtain its review outcome before it
becomes an assembly input. Record accepted and rejected versions in
`longform-continuity-ledger.json`; never overwrite the last accepted clip.
For each accepted clip record its source request, actual duration, usable trim
range, incoming/outgoing state, boundary frames, audio state, and the next clip
it can legally join.

Reject a clip before assembly when identity, spatial continuity, screen
direction, motion, lighting, or action state contradicts its neighbors. An
editor cannot repair a fundamentally incompatible generation with a dissolve.

## Stitch with cinematic grammar

Normalize technical media properties first, then make editorial decisions.
Create `longform-edit-decision-list.json` with source version, in/out time,
timeline time, cut type, audio lead/trail, transition, grade, speed change,
and linked narration beat for every segment.

Use motivated cuts:

- cut on action when movement continues across the boundary;
- use eyeline and match cuts when composition carries the idea forward;
- use J- and L-cuts to bridge dialogue, ambience, or reactions;
- use hard cuts between independent generations by default;
- reserve dissolves and fades for real temporal, spatial, or tonal changes;
- use cutaways to conceal unavoidable discontinuity, not arbitrary effects.

Maintain one sound world across the edit. Bridge seams with room tone,
ambience, score, and J/L cuts; do not restart music or ambience with every
generated clip. Grade black level, white balance, contrast, saturation, and
grain consistently without pretending a grade can fix identity or geometry.

Build and review a short assembled sequence early, before rendering the full
film. This checkpoint tests the continuity strategy and editorial grammar,
which individual clip approval cannot prove.

## Prove every seam

Create `longform-seam-report.json` for the final candidate. Check every edit
boundary, not a sample. Record preceding and following clip/version, last and
first boundary-frame evidence, cut type, visual continuity, action match,
screen direction, identity, lighting/color, audio continuity, and verdict.

Fail the candidate if an unmotivated seam exposes a character reset, wardrobe
or prop drift, geography reversal, frozen or duplicated motion, lighting jump,
audio restart, double face, or generation artifact. Fix the source clip or edit
decision, rebuild only the affected sequence, and recheck its adjacent seams.
Approve the full film only when individual clips, every seam, narrative timing,
sound continuity, and final technical QA all pass.

---
name: hyperframes-quality
description: Review and gate an editable HyperFrames composition or its rendered video before delivery. Use after building, revising, diagnosing, or rendering a HyperFrames project when layout, contrast, deterministic timing, transitions, animation choreography, rendered-frame fidelity, or high-quality acceptance needs verification. Use alongside video-quality; this skill owns composition-specific checks, while video-quality owns final media, editorial, audio, claims, and presentation QA.
---

# HyperFrames quality gate

Review the exact source project and exact candidate render. Do not treat a successful render command as quality evidence. Read `hyperframes-cli` first; then read `hyperframes-core`, `hyperframes-creative`, `hyperframes-animation`, or `hyperframes-keyframes` only when their area is in scope.

## Run the gate

1. **Validate the source.** Run `hyperframes check`. Resolve structural, deterministic-timeline, layout, and contrast findings before continuing. Use `hyperframes inspect --json` for dense compositions or targeted timestamps; inspect every major text/data moment, scene transition, and the ending.
2. **Review rendered evidence.** Render a draft when no candidate render exists. Extract a contact sheet and representative frames from that rendered MP4, including opening, scene midpoints, dense text/data moments, transitions, and ending. For multi-composition projects, prefer rendered-MP4 frame extraction over snapshot-only evidence; it is the delivery truth.
3. **Critique craft.** Compare the evidence with `BRIEF.md`, `STORYBOARD.md`, `SCRIPT.md`, and `frame.md` when present. Check:
   - message clarity by scene two and evidence/claim fidelity;
   - safe areas, typography, contrast, hierarchy, and readable playback-speed copy;
   - transitions, pacing, continuity, and whether motion explains rather than decorates;
   - seek-safe timing, no stale frames, clipping, blank states, or unintended repeated/frozen frames;
   - design-system adherence and an intentional opening and ending.
4. **Repair, then re-check.** Fix the editable HyperFrames source, repeat only the affected checks, render again, and re-inspect. Use no more than two repair passes in a workflow gate. If a material issue cannot be proven fixed, return `revise` or `blocked`; never pass it onward.

## Record a decision

Write a concise scorecard alongside the stage report. Include `schema_version: 1`, the exact source/archive and candidate path, inspected timestamps, checks with evidence, repair passes, final verdict (`pass`, `revise`, or `blocked`), and a concrete recommended action. A `pass` requires evidence for every applicable composition check.

This is not the final delivery gate. After a composition passes here, run `video-quality` against the exact final MP4. Only its passing report may be used with `show_video`.

## Optional reference comparison

Use VMAF only when there is an approved reference render with matching timing and dimensions. It measures difference between a candidate and a reference; it does not measure creative quality and must not substitute for this review.

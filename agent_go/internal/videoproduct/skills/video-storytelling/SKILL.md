---
name: video-storytelling
description: Structure a video's narrative arc and pacing, scaled to its actual duration -- from a short explainer's single arc to a true long-form (8+ minute) piece's chapters, retention curve, and pattern interrupts. Use once the brief's message and audience are confirmed and before writing a shot-by-shot storyboard. Read alongside video-cinematography, which owns per-shot camera/lighting direction once a beat is chosen.
---

# Narrative structure and pacing

This skill owns the shape of the whole video -- what happens when, and why a
viewer keeps watching. `video-cinematography` owns what one shot looks like
once a beat is decided. Get the structure right first; a beautifully shot
sequence in the wrong place in the arc still loses viewers.

The guidance here scales with duration -- rules for a 2-minute explainer and
a 12-minute video differ in scope but not in kind. Apply the arc template for
short pieces, add chapters and retention management for anything genuinely
long-form.

## The explainer arc

For a single-arc piece (roughly under 5 minutes), structure the narrative in
this order, scaling section lengths proportionally to total duration:

1. **Hook** -- a pattern interrupt or counterintuitive claim, one to two
   sentences. The opening visual must create curiosity, not just look nice.
2. **Tension / information gap** -- state what the viewer probably believes
   and why it's incomplete or wrong. Establish why this matters to them.
3. **Concept beats** -- one idea per beat, each building on the last. End
   each beat with a "but" or "therefore" transition into the next, never
   "and then" (see "But-therefore, not and-then" below).
4. **Palate cleanser** -- a brief pause, a visual beat, or a "let that sink
   in" moment between concept beats, giving the viewer a moment to
   consolidate before the next idea lands.
5. **Key insight** -- the "aha" moment the whole piece has been building to.
   Follow it with one to three seconds of deliberate silence -- let it land
   before moving on.
6. **Proof / example** -- a concrete demonstration of the insight working in
   a specific case, not just an assertion that it's true.
7. **Implications** -- connect back to the viewer: "this means that..."
8. **Reframe and close** -- callback to the hook, restate the core insight
   in one sentence.

## Hook types

Pick one deliberately rather than defaulting to the same shape every time:

- **Contrarian**: "Everything you've been told about X is wrong." Best for
  myth-busting or correcting a common misconception.
- **Outcome**: "By the end of this, you'll understand X." Best for
  concept-driven, systematic explanations.
- **Mystery**: "In [year], something unexpected happened..." Best for
  story-driven pieces with a real narrative to unfold.
- **Stakes**: "This one mistake costs people X." Best for practical,
  how-to-shaped content.

## The 30-second rule

The large majority of viewer drop-off on typical platforms happens in the
first 30 seconds. The hook and the tension/stakes setup must be fully landed
by then -- do not spend the opening 30 seconds on a slow build. A piece that
survives this window at a healthy retention rate tends to hold a
substantially higher fraction of viewers through the rest of its length than
one that doesn't.

## But-therefore, not and-then

Never connect narrative beats with "and then" -- it reads as a list, not a
story. Connect with "but" (something complicates or contradicts what came
before) or "therefore" (something follows as a consequence):

> Bad: "Atoms have electrons, and then those electrons have energy levels,
> and then..."
>
> Good: "Atoms have electrons, **but** they don't behave like tiny planets,
> **therefore** we need a different model -- **but** that model creates a
> new puzzle, **therefore** the actual answer is..."

## Misconception-first

Presenting what the audience probably already (incorrectly) believes before
revealing the correct picture produces measurably better comprehension and
engagement than presenting the correct information cold -- this is a
well-replicated finding in educational-video research (Derek Muller's PhD
work on physics-misconception videos is the foundational study). For any
explainer where the audience likely holds an intuitive-but-wrong model,
consider opening with that wrong model before correcting it, rather than
starting from the correct answer.

## Guided discovery

Rather than stating the answer, reconstruct the reasoning path so the viewer
arrives at it themselves (the method long-form science-explainer channels use
to keep dense material engaging):

1. Pose a specific, concrete question.
2. Show the obvious/naive approach -- let it partially work, then break.
3. Introduce one new idea -- the key insight -- and pause on it (one to
   three seconds of silence after a reveal is more effective than rushing
   past it).
4. Apply the insight step by step, so each step feels like it follows
   inevitably from the last.
5. Generalize: "this pattern holds beyond this one example."

Reveal visuals progressively, layer by layer, timed to arrive exactly when
the narration references them -- never show the full picture before the
narration has earned it.

## Pacing rules, grounded in cognitive-science research

These come from Mayer's Multimedia Learning research (Cambridge University
Press) on how people actually process narrated video, not just convention:

- **Segmenting**: at most one genuinely new concept every 30-45 seconds.
  Cramming concepts together measurably reduces comprehension.
- **Signaling**: verbal signposts every 30-45 seconds ("here's where it gets
  interesting") help the viewer track structure.
- **Temporal contiguity**: narration and the visual it describes must be
  simultaneous. Even a few seconds of offset between them measurably hurts
  comprehension.
- **Coherence**: cut interesting-but-irrelevant material. Content that's
  engaging but doesn't serve the point measurably *reduces* learning on
  what actually matters -- entertaining tangents have a real cost, not just
  an opportunity cost.
- **Modality**: prefer narration (spoken) over on-screen text for
  explanatory content paired with visuals -- spoken words plus pictures
  outperforms written words plus the same pictures for comprehension. Use
  on-screen text for what needs to be read exactly (a stat, a quote, a
  claim), not as a substitute for narration.
- **New visual element roughly every 3-5 seconds** keeps attention. This is
  not a cut every 3-5 seconds -- a held shot satisfies it whenever
  something in frame changes: subject motion, a camera move, a graphic
  appearing, a reveal. Cutting that often would over-cut a reflective beat
  (see `video-editing`'s pacing guidance, which governs cut frequency);
  what this rule asks for is visual *change*, of which a cut is only one
  kind.

## Scaling to true long-form (8+ minutes)

Everything above still applies -- long-form is the same narrative discipline
under more time pressure to sustain attention, not a different discipline.
What changes at length:

### Chapter structure

Break the piece into 2-4 minute chapters (max 5-6 chapters for a 10-15
minute video -- more fragments the narrative). Add explicit chapter
markers/timestamps as metadata for the finished piece.

**The arc applies at both levels, and they are not the same arc.** The
eight-step explainer arc above runs once across the whole piece: one hook
at the top, one key insight the entire video builds toward, one reframe at
the end. Each chapter then carries a smaller version of the same shape --
a mini-hook or re-engagement line at its start, one idea, a payoff before
its end -- feeding the larger arc rather than restating it. A chapter that
delivers its own full "aha" and closes it off leaves the viewer with no
reason to continue; a chapter that ends on a payoff *and* an unanswered
question does.

### The retention curve has known danger zones

- **First 30 seconds**: the largest single drop-off point on typical
  platforms (see "The 30-second rule" above) -- this matters even more at
  length, since a viewer who leaves in the first 30 seconds of a 12-minute
  piece never sees the other 11.5 minutes of work.
- **Around the 2-3 minute mark**: a second common drop-off point once the
  opening curiosity is spent and a real payoff hasn't landed yet. Counter it
  by delivering the first major payoff before 2:00, and placing a pattern
  interrupt (a visual style change, a B-roll burst, a music shift) around
  1:45-2:00 to bridge across it.
- **A secondary drop-off point roughly 55-65% through** a longer piece --
  counter with a burst of quicker cuts and resolving an open loop planted
  earlier.
- **Re-hook every 3-4 minutes** after the 2-minute mark with a verbal
  signpost ("but that's not even the interesting part...", "here's where it
  gets surprising...") to give the viewer a reason to stay through the next
  chapter.

### Pattern interrupts

Deploy a *major* interrupt (visual style change, section transition, music
energy shift, a quick burst of several rapid cuts) every 60-90 seconds, and
a *minor* interrupt (a B-roll cut, an on-screen graphic, a direct-address
line to camera) every 20-30 seconds within a chapter. A viewer going 15+
seconds without any visual or audio change is a real risk point regardless
of how good the content is -- attention needs something to track.

### Open loops

Plant a question or a "the interesting part is coming" tease in the first 60
seconds, and hold the answer for a real payoff later -- an unresolved open
loop is one of the more reliable ways to carry a viewer through a chapter
they might otherwise not need to finish.

### End screen

Reserve the last ~20 seconds for a close and call-to-action -- do not place
essential content there, since platform end-screen elements typically
overlay that region. Do not "say goodbye"; tease what's next instead.

## Where this fits

- Use `longform-cinematic-video` to plan the shot list and own the overall brief.
- Use this skill to structure the narrative arc and pacing before writing a
  shot-by-shot storyboard.
- Use `video-cinematography` to turn each beat into camera/lighting/consistency
  direction for MiniMax H3 once the arc from this skill is in place.
- Use `video-editing` to assemble the results according to the chapter/beat
  structure planned here, `video-quality` before presenting any version as
  complete.

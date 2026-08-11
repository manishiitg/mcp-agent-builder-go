---
name: create-study-material
description: Create clear, child-ready study material (notes, worked examples, a revision sheet) for a topic, matched to the child's level and their own uploaded materials, packaged as one activity.
---

# Create study material

1. **Read the progress report FIRST — this is what makes the material hers.**
   `reports/progress.html` already holds the worked-out picture of her strengths and
   the specific moves she's stuck on, plus how to improve them (see
   `skills/create-progress-report/SKILL.md`). Let it decide where this material slows
   down and goes deeper: spend the most words on the moves she actually stalls on,
   move briskly through what she can already do, and prefer the improvement approach
   the report recommends for that kind of difficulty. Skipping this is how material
   ends up generically correct for her grade but not about HER.
   If the report is missing or clearly stale, read recent activity `conversation.json`
   files and `attempts/` directly instead, and say so to the parent.

2. **Know the child and the material.** `memory/child-profile.json` for name, grade,
   and board (ask the parent if any is missing). The relevant
   `materials/<subject>/<topic>/*.meta.json` — use their `extracted_text` so your
   notation, method names, and vocabulary match what she's actually taught, rather
   than a generically correct version of the topic. Also check `memory/interests.md`
   if it exists — a genuine interest can make one or two of the worked examples
   below feel like they're actually about her, not a generic textbook. Use it where
   it fits naturally, never in every example, and never at the cost of the maths
   itself being clear and correct.

3. **Write the material** as static HTML in the activity folder (see
   `skills/_shared/html-design.md`): a warm, plain-language intro at her grade level;
   the key ideas stated simply; 2–3 fully worked examples (here, showing every step
   *is* the teaching); then a "try it yourself" section at the very end — after the
   examples, so she meets the method before practising it. Close with a line of
   encouragement addressed to her.

   Pictures: draw it yourself when the point is a relationship, a process or a
   comparison — it stays crisp, needs no extra file, and can use the page's own
   colours. For geometry, graphs and number lines (an angle, a circle, a labelled
   triangle, a bar chart), read `skills/_shared/diagrams.md` and declare the
   figure with JSXGraph — never hand-write SVG coordinates, which gets the y-axis
   direction and arc/label placement wrong. Reach for `find_image` when seeing the
   real thing IS the teaching: an actual landform, a world map, a labeled
   anatomical diagram, a historical photograph. Print the credit it returns, and
   don't force a picture in where it adds nothing. See
   `skills/_shared/html-design.md` → "Real pictures", and check any figure you
   drew before finishing (→ "Check a figure before you finish").

4. **Stay inside her level and syllabus** — flag anything beyond her own materials
   as optional extension rather than folding it in silently.

5. **Finalize** the activity with a `goal` describing what working through it looks
   like — e.g. "read every section and try the practice questions at the end". Say in
   it that this is for learning rather than testing, so the tutor knows to teach
   openly here instead of holding answers back.

6. **Tell the parent** what you made.

7. **Then end the turn with `suggest_actions`** — "Tell the parent" above is not the last step.

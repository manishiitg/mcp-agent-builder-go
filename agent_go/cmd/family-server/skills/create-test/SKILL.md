---
name: create-test
description: Create a practice test for the child from their materials and progress — child-facing questions with a separate parent-only answer key, packaged as one activity.
---

# Create a practice test

1. **Read the progress report FIRST — this is what makes the test hers.**
   `reports/progress.html` already holds the worked-out picture of her strengths and
   the specific moves she's stuck on (see `skills/create-progress-report/SKILL.md`).
   Read it before anything else and let its "Where she's stuck" bullets decide what
   this test actually probes. Skipping this is how a test ends up generically correct
   for her grade but not about HER — the failure worth avoiding here.
   If the report is missing or clearly stale, fall back to reading recent activity
   `conversation.json` files and `attempts/` directly, and say so to the parent.

2. **Know the child and the material.** `memory/child-profile.json` for name, grade,
   and board (ask the parent if any is missing). The relevant
   `materials/<subject>/<topic>/*.meta.json` for what she's actually being taught —
   their `extracted_text` already holds the full content, so her notation and method
   names match what her teacher used. Also check `memory/interests.md` if it exists.

3. **Write the test** as static HTML in the activity folder (see
   `skills/_shared/html-design.md`). It should read like a real test paper: a clear
   header with her name, grade/board, subject and topic, then numbered questions,
   marks shown as a `.badge`, and genuine space to work under each.
   A question that needs a figure — an angle, a circle, a labelled triangle, a
   graph to read values off — declares it with JSXGraph per
   `skills/_shared/diagrams.md`, never as hand-written SVG coordinates; check any
   figure renders correctly before you finish (html-design.md → "Check a figure
   before you finish"). When she is meant to DRAW the graph herself, give her the
   empty grid instead, and don't state the scale.
   Cover the methods that appear in her own materials, and make **most of the
   questions target the specific weak moves from step 1** — a couple she can already
   do (so it isn't demoralizing), the rest on the moves that actually need work,
   approached from a different angle than last time rather than the same items again.
   No answers, and no hints that give them away.

   **Difficulty must genuinely escalate, not just reword the same problem.** A test
   that swaps the cover story every question while the operation count stays flat
   (ten word problems that are all "add three decimals," just about a different
   thing each time) is NOT increasingly difficult, even numbered 1 to 10 — it just
   *looks* like a ladder. Structure each section into real tiers, and make every
   tier genuinely harder than the last in at least one of: number of steps, number
   of concepts combined, or reasoning demanded (recall < apply < combine <
   justify/critique):
   - **Warm-up** (1–2 questions): single concept, single step — she should get
     these without a hitch; this is what keeps it from being demoralizing.
   - **Core practice** (the bulk): the actual weak moves from step 1, one to two
     steps, varied cover stories — this is where most marks live.
   - **Stretch** (a handful): multi-step, or combines two concepts from this test
     (e.g. a decimal calculation whose result then has to be read off a
     pictograph), or asks her to justify a claim rather than just compute one.
   - **Challenge** (1–2 questions, clearly marked, at the very end): genuinely
     hard for her grade — non-routine, synthesizes more than one idea, or asks her
     to find/explain an error in someone else's reasoning. It is fine if she
     cannot finish this one; that is what makes it a real challenge question, not
     a demoralizing one (there are only one or two, and they come after she has
     already succeeded on everything else).
   Vary the **question format** across the set too, not just the numbers — mix
   direct computation, word problems, "find the error" / critique-a-claim
   questions, and (where the topic supports it) a short design/justify question —
   real diversity is different *kinds* of thinking, not just different nouns in
   the same template.

   **Never reuse the same numbers or scenario across two places in the same
   file.** A worked example, a "closed" hidden example, and a graded question must
   each be a genuinely different problem — reusing a scenario (even with a
   different label) hands her the answer to a real question the moment she
   glances at an example, or makes two questions read as the same one repeated.
   Before moving on, scan your OWN draft question-by-question and confirm no two
   entries (examples included) share the same numbers, story, or answer, and that
   each question asks for exactly one clear, unambiguous deliverable.

   A test still stays a real test paper first: if a genuine interest from
   `interests.md` fits ONE word problem's cover story naturally (a fraction
   question set at a Quidditch match, say), use it — but never at the cost of
   realism, clarity, or exam register, never in every question, and never under a
   `strict` teaching_mode where she needs to recognise the question in its real
   exam form. Skip it entirely rather than force it.

4. **Write the answer key** as plain Markdown at `<name>-KEY.md` in that SAME folder
   — full worked solutions, plus a note on which questions target which weakness so
   the parent knows what to watch for. Never list it in `items`: it stays out of the
   child's activity view entirely, and what the tutor may reveal from it during her
   session is governed solely by `teaching_mode`.

5. **Finalize** the activity with `goal` = "answer all N questions" (N = the real
   count). A test is usually `strict` (hints only, no reveal) or `graduated` — ask
   rather than assume if the parent hasn't said.

6. **Tell the parent** what you made and why those particular questions.

7. **Then end the turn with `suggest_actions`** — "Tell the parent" above is not the last step.

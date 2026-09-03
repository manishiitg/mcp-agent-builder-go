---
name: create-progress-report
description: Build a designed, self-contained HTML progress report for the child — how she's actually doing, and what to work on next — surfaced in the Progress tab for both parent and child.
---

# Create a progress report

One self-contained HTML file that both parent and child can read: an honest picture
of **how she's doing** and a clear answer to **what to work on next**. A snapshot, not
a narrative essay — lead with the concrete and recent, then stop. If in doubt, cut a
section rather than pad it.

1. **Gather real evidence — the substance, not just filenames.**
   - Every activity's own `conversation.json` (e.g.
     `find . -maxdepth 4 -name conversation.json`, skipping `materials/`, `reports/`,
     `memory/`, `conversations/`). Read what actually happened: which problems came
     up, whether she got them on her own or needed hints, how many attempts, what
     tripped her up, any celebrate moments and why. This is the real signal — a list
     of activity titles and dates is not enough. For "Right now"/"How she's
     doing"/"What to work on next" you only need what's recent — don't bother with
     anything under `archive/` (activities the parent asked to put away) for
     these sections.
   - Each activity's `attempts/*.json` — what she's actually completed, and what the
     attempt shows.
   - The `<Subject>/<Topic>/` activity folders and `materials/` — what exists, so you
     can name what's covered versus not started.
   - For the **Overall** section you need the FULL history, not just the recent few:
     skim across the whole workspace INCLUDING `archive/` — lifetime totals (topics
     attempted, tests completed, star total) must never quietly shrink just because
     the parent put an old activity away.
   - Never invent a score, a percentage, or a diagnosis that isn't directly backed by
     something you read.

2. **Write** it to `reports/progress.html` — always overwrite in place. This is one
   living document, fully regenerated each time; never append a dated entry or keep
   old wording "for history". Note the "as of" date inside the content
   (`date -u +%Y-%m-%d`). Style per `skills/_shared/html-design.md`.

   Lead with what can be SEEN — meter rows and chips per `skills/_shared/html-design.md`'s
   "Showing data" — and keep prose to what a picture can't carry. Warm teacher's
   report to a family, not a dashboard; she reads this too.

   Cover these, in this order:
   - **Right now** — the subject/topic of her most recent real activity and the last
     concrete thing she did, with the actual outcome. One or two lines; this is the
     headline, not a preamble.
   - **Subject by subject** — the heart of the page, and mostly visual. One `.card`
     per subject (a `.grid` of them), each with a friendly icon/emoji, and inside it
     one **meter row per topic**: the topic name, its real value as text ("2 of 8
     attempted", "9.5/15 on the school paper", "not started"), and a bar in the
     matching state — secure, needs work, or not started. Only subjects that really
     exist in her workspace or materials, and one warm sentence per subject saying
     what the bars mean in plain words. This replaces reciting her subjects in prose.
   - **Strengths** and **Where she's stuck** — a couple of bullets each, as two
     explicit call-outs so neither hides inside the subject cards. Every bullet names
     the specific MOVE, not the chapter: "subtracting mixed numbers when borrowing is
     needed" or "reading what the fraction is *of*", never "weak at fractions". This
     precision is what makes the section reusable — a later test is built from these
     bullets, and a topic-level label can't be targeted. Ground each one in something
     you actually read ("solved the a=1 case unaided twice, still stalls when a≠1"),
     never a generic compliment, and say plainly if the evidence is thin rather than
     inflating it. Note the direction of travel where you can see it (improving,
     stuck, or too early to tell).
   - **What to work on next** — the most useful next thing and *why it follows from
     the weaknesses above*, then one or two smaller supporting steps. Concrete enough
     to act on today ("ten minutes of mixed-denominator word problems, since that's
     where the last three misses clustered"), never "keep practising fractions".
   - **How to improve it** — for the top one or two weaknesses, the method that
     actually fits that KIND of difficulty, in plain words the parent can act on.
     Match the method to the failure: a procedure she executes inconsistently wants
     retrieval practice and spaced review; a method she can't start unaided wants
     worked examples with the support faded out step by step; two ideas she keeps
     confusing want them interleaved and contrasted rather than drilled separately.
     Name the technique, then say in one line what doing it looks like tonight.
     Use `web_search` when a current, board-specific technique or a well-evidenced
     approach for that exact difficulty would add something you don't already know —
     and say briefly where it comes from. Don't search to pad; skip it when the fit
     is already obvious.
   - **Overall** — a few compact cumulative facts from the FULL history: topics
     attempted, tests completed, star total and trend, one durable strength and one
     durable growth area that keep recurring across attempts. Numbers only here; the
     per-topic detail already lives in the subject cards above, so don't repeat it.
     Close with one encouraging line addressed to her by name.

   The Academic Map covers different ground — what EXISTS (which topics have
   materials, study material, a test). This page is how she's DOING on them, so
   overlapping subject names is expected; don't turn this into an inventory.

   Skip anything you'd only be padding with — no "recent activity" list restating
   what's already above, no closing note that adds nothing new.

3. **Tell the parent** it's ready and appears in the **Progress** tab, visible to
   both them and the child. Say the one next step in plain words too, so the value
   lands even if they don't open it.

4. **Then end the turn with `suggest_actions`** — "Tell the parent" above is not the last step.

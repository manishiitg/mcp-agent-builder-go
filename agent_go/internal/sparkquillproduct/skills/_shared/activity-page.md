# The activity page in SparkQuill

An activity page is an HTML fragment saved as `<name>.sq.html` in the activity
folder. `create_learning_activity` finishes it into `<name>.html`: it numbers the
questions and gives each an answer space, injects the print hook and the
`SQ.choose` button script, and nothing else. The page is yours: a fragment or
a whole document, your own title, `<style>`, animations, scripts, inline
`<svg>`, JSXGraph boards, tables and pictures pass through as written.

Four conventions, because the tutor relies on them:

1. `<section data-role="learn|practice|check|explore">` with an `<h2>`.
   learn: explain freely, confirm answers. practice: hints first, then the
   answer. check: a real test, hints only, no right/wrong until she is done.
   explore: open-ended, follow her curiosity.
2. `<div class="q">` around each question (`data-marks="2"` on a test).
   Ids `q1, q2…` are assigned; the tutor scrolls to them with `open_file`'s
   `focus` and records what she answered inside them.
3. `<button data-choose="text sent to the tutor">…</button>` for a real choice.
4. Removed on render: form controls, `<details>` click-to-reveal, links, and
   anything loaded from the internet.

A `<figure>` gets an id (`fig1…`) so the tutor can point at it.

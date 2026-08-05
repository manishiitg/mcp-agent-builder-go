## Workflow log conventions

Canonical conventions shared by every `/review-*`/`/improve-*` skill, the post-run monitor, and any chat-driven fix touching the log. Load once; skills point here instead of restating it.

### Reviewer/writer boundary

Specialists investigate; the active Workshop parent writes for standalone
review/improve commands, while the dedicated Dashboard stage is the only HTML
writer during scheduled Pulse. The Pulse Fixer records semantic results and
workflow changes in durable state but writes no HTML. This boundary applies to
Bug Review, Artifact Review, Stores Health, Eval Health, Report Health,
Ops Review, Strategy Auditor, plan review, and both Goal
Advisor reviewers.

- A specialist is strictly read-only. It must not edit workflow files, update
  `builder/improve.html`, create or consume human-input requests, mark module
  state, or launch another maintenance reviewer.
- A specialist may read only the relevant semantic regions of
  `builder/improve.html`: matching open findings, recent entries for its module,
  current verdicts, answered outcomes, and an active Advisor experiment when
  relevant. It must not inspect CSS, restyle the page, migrate markup, load
  `review-improve-log-skeleton` or `html-output`, or spend review time formatting
  cards.
- Every specialist returns a compact, non-HTML review packet to the parent:
  `module`, `verdict` (`clean|findings|blocked|insufficient_evidence`), and
  ordered `findings`. Each finding carries a stable `finding_id`, `target_key`,
  `severity`, plain-language `summary`, exact `evidence`, bounded
  `recommended_fix`, `verification`, and `user_judgment_required` with a reason.
  Include `next_check` for the module. Return every evidence-backed finding;
  keep prose compact without imposing a semantic character or finding limit.
- After all selected specialists return, the parent validates evidence,
  deduplicates by `finding_id` and `target_key`, resolves conflicts, and applies
  only permitted bounded fixes. In scheduled Pulse it records terminal module
  results for Dashboard to render once; in standalone review/improve chat it
  updates `builder/improve.html` once. Preserve one attributed card per reviewed
  module even though the physical HTML patch is consolidated.
- A standalone `/review-*` command uses the current Workshop turn as that parent
  coordinator. Its underlying tool or generic reviewer remains read-only; the
  current turn performs the single bounded log update after the review result is
  complete.
- Interactive vs unattended: a standalone `/review-*` command runs in a live chat
  with the user present, so its coordinator turn MAY ask the user a direct question
  in that same chat and wait for the reply -- this is how a bounded fix gets a quick
  yes/no. Scheduled Pulse module review/fix stages run with nobody watching
  chat; there, the parent Fixer must NEVER ask a direct question -- a question asked
  into an unattended chat is never answered and silently stalls the run. It must use
  `create_human_input_request` instead, which is durable and surfaces to the user
  later as a Needs your decision card. When in doubt about which context this turn is
  running in, prefer `create_human_input_request`: it still reaches an attentive user
  and never goes unanswered forever.

### One log: `builder/improve.html`

`builder/improve.html` is the short executive journal and shareable user view;
SQLite/runtime is authoritative. Preserve legacy HTML recovery/Advisor state;
add no new machine semantics. SQLite owns full reviews and
finding/fix/verification state; this dashboard is a compact projection, never a
second issue ledger. Visible dashboard-level content:

- **applied or proposed changes** (what Pulse Fixer, Goal Advisor, `/improve-*` reviews, and chat fixes did, and why),
- **material run notes** (only outcomes worth retaining in the published journal),
- **material monitor transitions** (new regressions, recoveries, or ownership changes — never standing issue state),
- **answered human decisions** (the question, the user's answer, and the resulting outcome),
- **material product decisions or experiments** (one compact proposal/applied/
  measured transition when it changes workflow direction).

Do not create a review file. Store full reviews as SQLite TEXT and render their
concise narrative here. Pending questions use `create_human_input_request` in
`db/db.sqlite`; Runloop renders them above the HTML.

It is a **self-contained, human-readable HTML document — not Markdown, not a data dump.** This is the published executive journal, while the Pulse popup is the operational product surface. Only the parent writer calls `read_skill(skills=[{"name":"builder-reference","path":"references/html-output.md"}])` for the style baseline. When creating or upgrading the file, the parent also loads `read_skill(skills=[{"name":"builder-reference","path":"references/review-improve-log-skeleton.md"}])` and uses that **Starter HTML skeleton** for the exact structure and polish; specialists never load either presentation reference. Copy the structure and CSS, but omit instructional comments and example cards from the saved HTML. Keep only the stable `<!-- LOG ENTRIES: newest first -->` anchor. Runloop renders pending `report_human_inputs` above the HTML. The visible HTML reads: **two verdicts → one status headline → Latest Pulse (Outcome/Goal movement/Next) → at most six material Activity transitions → archive**. A hidden recovery marker may follow the timeline, but it is never a visible section. Do not add reviewer coverage, assumptions panels, Current work counts, issue queues, technical/cost tiles, filters, or reviewer evidence already available in Pulse.

### Format compliance is a standing part of every write, not a rare event

Every time the parent writer touches `builder/improve.html` — not just when creating it or hitting an obviously old-format file — it verifies the current structure still matches the skeleton *and* updates content in the same pass. Don't treat structural correctness as checked only when legacy markers are detected (that check still exists below for the rare full-rewrite case) — check it every time, like a linter running continuously rather than only when something looks badly broken. Drift caught immediately never accumulates into a full rewrite later.

### Activity: grouped by date, workflow and Pulse together

The timeline is organized by date, not as two separate lists. Each date is one `.daygroup` — **a plain visual wrapper, never itself a `.run`/`.entry`/`.pulse-record`** — holding at most one compact workflow `.run` row and only material Pulse transitions as sibling `.entry` cards: a consequential fix, verified recovery, newly important regression, applied user decision, or strategy change. Gate dispatch and clean reviewer results are not published history; Pulse activity already shows them. Never create two `.daygroup` blocks for the same date — edit the existing one in place.

This structure supports attribution, archiving, and publishing. The Pulse popup
does **not** parse these cards; it renders review and fix state from SQLite. Keep
material issue and Fixer transitions independently tagged so the journal remains
useful on its own.

A day with a workflow run but no material Pulse transition keeps only the compact
run row. Gate/reviewer freshness remains in the database-native Pulse popup.

The Pulse log is opened in a narrow right panel by default. Design it **mobile-first**:
the base CSS must work at 360-480px with stacked rows, no overlapping metadata, no
desktop-only tables, and long workflow names/ids allowed to wrap. Add desktop/tablet
enhancements with `@media (min-width: ...)`; do not make desktop the default and patch
mobile as an afterthought.

Standing work, counts, ownership, evidence, and verification state belong only
to the Pulse popup. Do not render `.worksummary`, `.workstat`, or standing
`.entry.open` cards. Preserve old standing cards in monthly archives; the active
journal records only material transitions.

### Four-part Pulse model

Every visible record has one owner in the Japanese-inspired review cycle:

- **Goal:** purpose and success criteria. `soul/soul.md` is the source of truth and Runloop renders it directly. Do not copy a Goal/Profile card into `builder/improve.html`.
- **Issues and reviews:** evidence-backed observations from read-only reviewers. State what was found, not what was fixed. Use the stable compatibility attribute `data-pulse-section="signals"` and that reviewer's canonical module id.
- **Decisions and analysis:** what the run means, cadence reasoning, assumptions challenged, and general Pulse/run question-and-answer outcomes. Use the stable compatibility attribute `data-pulse-section="reflection"`.
- **Fixes and improvements:** verified bounded fixes from Pulse Fixer plus Goal Advisor proposals or decisions. Link each improvement to its review evidence and verification. Use `data-pulse-section="improvements"` with `pulse_fixer` or `goal_advisor`.

`builder/improve.html` remains one newest-first chronological journal. The Pulse popup reads its operational issue and review state from SQLite, not from these cards. Do not duplicate current issue detail in Activity.

### The status headline (the 1-second read)

Directly under the verdicts, one `.status` banner carries a **single plain sentence** — the workflow's one-sentence verdict headline (there is no separate verdict file — this banner is the source of truth) — so a user knows "am I OK?" without parsing pills or scrolling. Its `ok|warn|bad` class tracks the **worse** of the two verdicts, and its `.when` shows the run + how long ago. Keep it honest both ways: on a clean, on-target run say so plainly ("Healthy and on-target."); on a regression lead with what's wrong ("Goal drifting — eval 0.78, under 0.90 target for 3 runs."). Never manufacture concern to fill it.

### Freshness — every status says which run it's "as of"

A verdict, a goal-criterion status, or a tile can silently go stale if no recent run measured it. So **stamp the run each status reflects**: the verdict pills carry a small `run #N`, each goal-criterion `.m` line ends with `· run #N`, and the status banner's `.when` shows the run + age. A 4-runs-old "Met" must read as 4-runs-old, not as current truth — this is how the reader tells a live verdict from a stale one.

Different sections may use different evidence dates. The overall status headline reflects the latest run, while Goal movement may correctly retain the last trustworthy measurement from an older run. In that case show `not measured this run · last measured run #N / YYYY-MM-DD` in the relevant `.briefitem`. Every important brief cell needs visible freshness; do not rely on nearby sections or buried evidence paths to imply the date. Never replace a known older value with `—` merely because the latest route did not measure it, and never present that older value as current.

Reviewer coverage, cadence, run/skip reasons, and costs belong to **Pulse
activity** in the popup. Never render `.coverage` or `.covitem` elements in the
published journal.

### Needs your decision — always first when present

Pending decisions are the most actionable content, so the Runloop Pulse view renders only the currently unanswered `report_human_inputs` in the section owned by their source, above that section's `builder/improve.html` history. Goal Advisor questions appear in Improvements; general Pulse questions appear in Reflection. Use `create_human_input_request`; never build custom form controls inside the static HTML. Title this surface **Needs your decision**. Ask only for a real user/business decision, credential, explicit durable constraint, or material strategy approval — never for a deterministic bug fix, stale path/receipt cleanup, schema wiring, or routine implementation choice.

Do not add a second active-question card to `builder/improve.html`. On the first Pulse after an answer, add one short historical card containing the actual question, the user's selected option and/or free-form answer, and whether the answer is waiting, applied, rejected, or superseded. Keep it with the component that asked: Goal Advisor uses **Improvements** / `goal_advisor`; a known reviewer uses **Signals** / that reviewer's canonical module; a general Pulse or run-level question uses **Reflection** / `run_summary`. When consumed, include the concrete outcome from `mark_human_input_consumed`. Never move every answer into Reflection merely because it is historical.

Assumptions and product recommendations belong to the Product work area in the
Pulse popup. If an assumption produces a material approved decision or measured
strategy change, record only that transition in Activity; never render a
standing `.assumptions` panel.

### Latest Pulse

Below the status headline, include one `.brief` section with exactly three short
cells: **Outcome**, **Goal movement**, and **Next**. Each cell is one or two plain
sentences and carries freshness where needed. Outcome summarizes what the run and
Pulse pass accomplished; Goal movement states whether evidence moved toward or
away from success; Next names the single most important next action or evidence
boundary. Do not enumerate fixes or open issues here—Activity records only
material transitions, and the Pulse popup owns the complete list.

### Plain-language card contract

The user reads this page to understand what happened without opening raw logs. Every visible card must be readable in 10 seconds:

- The always-visible layer contains only: a short status label, a title of at
  most 12 words, `<p class="takeaway"><b>What happened:</b> ...</p>`, one
  optional `<p class="impact"><b>Why it matters:</b> ...</p>`, and one short
  `<p class="meta"><b>Next:</b> ...</p>` when action or later confirmation is
  required. The three fields together must stay under 70 words.
- Use only these human status labels: `Checked`, `Needs attention`, `Fixed`,
  `Waiting to confirm`, `Blocked`, `Review incomplete`, `Skipped`, or
  `User decision`. Do not invent packet, manifest, recovery, or state-machine
  labels. A reviewer/infrastructure failure is always `Review incomplete`, not
  `Bug`, `Failed`, or `Needs attention`.
- Translate internal states into user language. Use these exact visible labels:
  - `no_terminal_packet` -> "The review did not finish."
  - `retry_due` -> "Pulse will retry."
  - `approved_awaiting_evidence` -> "Approved; waiting to confirm results."
  Also say "Fixed; waiting to confirm in a real run." instead of
  `changed_unverified`, and "Too little data to conclude." instead of `low_N`.
  Raw state values may remain in internal reviewer files, database rows, and
  invisible `data-*` attributes, but must never appear in visible Pulse text.
- Do not copy evidence paths, run/session ids, finding ids, hashes, manifests,
  SQL/table names, model ids, raw counters, recovery ledgers, step/route ids,
  or tool/function names into any visible part of `builder/improve.html` —
  including `.run .note` (same contract as entry cards, no exemption). Say
  what changed in user-visible terms ("stops the run if the wrong account is
  signed in", not "step-0-cdp-test hard-fails to browser_failed"). Exact audit
  evidence stays in SQLite-backed reviewer results and structured Pulse state. Completed
  fixer changes, verification, and
  before/after references are stored by `record_pulse_result` in the
  workflow-local `pulse_module_audit` table; do not create another audit file.
  The hidden `#pulse-agent-handoff` marker is only for minimal in-progress
  cross-module recovery state. Cards may carry stable invisible
  `data-*` attributes needed by Runloop, but must not show a visible
  **Evidence**, **Manifest**, **Finding ID**, **Files touched**, or raw-path row.
- A clean/skipped card says what was checked, why no action was needed, and when
  it will be checked again. A fixed card says what user-visible problem was
  repaired and whether the fix is proven. A blocked/failed card says the user
  impact, that no unsupported change was made, and the retry/unblock condition.
- Reviewer or infrastructure failure is not a Bug, Goal finding, Advisor idea,
  or workflow outcome. Label it `Review incomplete`; never make the user infer
  business meaning from internal reviewer-service state.
- Avoid compressed ledger language: no long semicolon chains, no unexplained abbreviations, and no dense strings like `regression(high)+low_signal`.
- Prefer user words: "The workflow did not send the email" is better than "delivery notify scope mismatch"; "Goal is short for the third clean run" is better than "low_signal persists".
- If a card cannot be understood without opening raw files, rewrite it before saving.
- Before saving, scan all visible text. If it
  contains an unexplained internal token, path, hash, packet/service phrase, or
  state code, the Pulse report is not complete.

### Repeated outcomes

Standing issue state belongs only in the Pulse popup. Repeated reviewer sightings
increment SQLite lifecycle history but do not create another visible card. Add
a new Activity event only when the issue is filed for the first time or its
state, evidence conclusion, ownership, or required next action materially
changes. Preserve older duplicate cards in the monthly archive rather than
keeping near-identical records in the active timeline.

### Activity metadata

Every run row and timeline entry must remain machine-attributable for archives and published history. Add:

- `data-date="YYYY-MM-DD"` — the actual event/run date in local workflow time when known; if only a run folder date exists, use that date.
- `data-kind="run|gate|monitor|maintenance|artifact|decision|advisor|cos|input|user|note"` — the primary activity type. `gate` remains readable for archived history but is not written for new clean/dispatch-only passes.

Do not render a filter bar or ship a filter script. The active journal is capped at 6 material cards; SQLite-backed Pulse provides full search and filtering.

### Two verdicts: Bug and Goal

Every workflow is judged on two independent axes, and the header shows **both** as separate pills — never collapse them into a single "health":

- **Bug** — did it *run correctly*? Errors, skipped steps, missing/empty artifacts, regressions vs the last run. A bug is investigated by **Bug Review** and fixed by the **Pulse Fixer**. Operational, roughly binary.
- **Goal** — is it *achieving its success criteria*? Eval scores and run evidence vs `soul.md`, trending over runs. A goal gap is addressed through **Goal Advisor strategy work**. Continuous.

They are orthogonal: a run can be **Bug: broken** while **Goal: on-target**, or **Bug: clean** while **Goal: short**. Pulse may therefore select Bug Review and Goal Advisor together when operational Bugs and a reliable Goal trend coexist.

Tag each entry with a **Bug** or **Goal** chip when applicable so the fix path is obvious (Bug → Bug Review/Fixer, Goal → Goal Advisor). Also add an action label chip when work was done, proposed, or needs input. Goal Advisor decisions must be visually distinct from Pulse bug-fix notes.

The header verdicts are stable current-state elements, not timeline entries. Update `#pulse-bug-verdict` and `#pulse-goal-verdict` in place on every Pulse; never append another verdict block. If an otherwise current-format `builder/improve.html` is missing either stable verdict element, insert the two-element `.verdicts` block beside the workflow title as a targeted repair. Do not rewrite the whole document or discard its timeline merely to repair missing verdict markup.

### Goal source and progress

`soul/soul.md` is the only durable Goal definition. Runloop's Goal tab renders it directly, so do not duplicate its objective or success criteria as a Goal/Profile card in `builder/improve.html`. The HTML may show time-bound **Goal movement** in Latest Pulse, verdicts, and material analysis entries, each stamped with its evidence run.

### Operational detail stays in Pulse

Do not render signal tiles, cost/time/model/tool breakdowns, Maintenance Radar,
reviewer field dumps, evidence paths, or a collapsed Technical details section in
`builder/improve.html`. Those are structured operational records and belong in the
Pulse popup and SQLite. A material cost or latency regression may appear once as a
plain-language Activity transition, but the detailed breakdown remains in Pulse.

### Hidden agent handoff projection at the bottom

The user-facing sections explain outcomes. Keep one `#pulse-agent-handoff` element near the bottom, immediately before Archive, with the HTML `hidden` attribute. It is **minimal current recovery state, not a second report and not a user surface**. Raw scheduler, session, execution, reviewer-task, and Pulse run identifiers are backend bookkeeping: never render them as visible text anywhere in `builder/improve.html`. When an internal identifier is required for matching, keep it only in a `data-*` attribute.

This element is a read-only projection of SQLite/runtime truth. It may help an
agent orient itself, but it never authorizes routing, recovery, approval, or a
mutation and never overrides contradictory runtime state.

It may contain only:

- current workflow contract version and plain-language execution state,
- one row per module with last result and next-check condition,
- Artifact Sync Cursor and other durable cursor/anchor ids,
- unresolved findings and pending human inputs, using readable labels while identifiers remain in `data-*` attributes,
- SQLite review ids only when required to resume an interrupted fix.

Update this hidden block in place on each Pulse. Never append historical copies. Do not repeat Latest Pulse, goal narrative, issue explanations, decisions, or full reviewer evidence. Do not paste raw logs, SQL results, model conversations, report content, or large JSON. Historical narrative stays in the timeline/archive; reviewer proof stays in SQLite-backed review records; machine scheduler state stays in SQLite; this marker is only the minimal interrupted-fix bridge.

### Newest on top — always

New entries go at the **top** of the timeline, not appended at the bottom. The file carries a stable anchor comment `<!-- LOG ENTRIES: newest first -->` directly below the header/tiles; insert each new entry immediately after it with `diff_patch_workspace_file`. Never reorder or rewrite existing entries except to close out an open finding (below). **Always read the existing file first** so you continue its style and don't duplicate entries. For a new date, insert the whole new `.daygroup` (its `.run` row plus every entry for that date) as one unit right after the anchor; for a date whose `.daygroup` already exists, insert new entries into it instead of creating a second `.daygroup` for the same date.

Every dated `.run`, `.entry`, or `.pulse-record` needs `data-date`,
`data-pulse-section`, and `data-module`. New entries use only `run_summary`,
`workflow_review`, `strategy_auditor`, `goal_advisor`, or `pulse_fixer`.
Workflow Review covers correctness, artifacts, reports/evals, learnings/KB/DB,
cost, time, tool/runtime operations, model routing, setup, and plan-design
hygiene. Retain older focused-module cards as history, but never write a retired
module id for a new result.
Use `signals` for material reviewer findings (including Strategy Auditor),
`reflection` for run/general Pulse records, and `improvements` for Goal Advisor
and Pulse Fixer decisions. Clean module results stay in SQLite-backed Pulse activity;
write a card only for a material lifecycle transition.

### Entry kinds

Each entry is a small card: a date, a kind tag, optional classification chips, a one-line title, and the short **What happened / Why it matters / Next** body defined above. The first body line must be a `<p class="takeaway"><b>What happened:</b> ...</p>` that a non-technical operator can understand immediately. Keep raw evidence and changelog references in SQLite-backed reviewer results, not in the card. Use these kinds:

- **Run** — a one-line row, the top of that date's `.daygroup`: date/time, status, key numbers (tests, eval, cost/tokens, wall time), the **backup result** (`backed up`, `unchanged — already backed up`, or `backup failed: <plain-language reason>`), and a short note only when something stands out. The note follows the same Plain-language card contract as any entry card (see below) — no step/route ids or tool names, user-visible behavior only. Routine runs stay terse; flag a run only when it regressed, the backup failed, cost/time evidence is missing, or one step/agent dominates spend/time.
- **Gate** — archived compatibility only. Do not create a new Gate/dispatch card; Pulse activity shows selected and skipped perspectives with their reasons.
- **Monitor** — a material post-run observation or issue transition: what changed in the output and why it matters. Clean reviewer results do not get Monitor cards. Keep exact evidence, classifications, recommendations, cost buckets, and tool output in SQLite; never render `.modfields` field dumps.
- **Maintenance** — only a consequential maintenance outcome or changed next-check decision. Routine depth/skipped-module bookkeeping stays in Pulse and does not get a card.
- **Artifact Review** — only when plan drift caused a material user-visible correction or blocked outcome. Clean/no-pending reviews and cursor bookkeeping stay in Pulse activity.
- **Decision** — a material change applied or proposed, with the one-line rationale. Goal Advisor decisions use `<div class="entry decision">` with tag text `Decision - Goal Advisor - Applied` or `Decision - Goal Advisor - Proposed`; use `<div class="entry decision major">` for material plan changes, report/eval changes that alter user-facing success measurement, cadence/scope changes, or any change the user should notice.
- **Advisor opportunity** — a proposal-only Goal Advisor entry for an out-of-plan idea the current workflow has not considered but an expert operator would raise because it could materially advance the goal. It should be grounded in `soul.md`, run/eval/report evidence, market/process reasoning, or a clearly stated assumption; never present speculation as fact. Record it as `Decision - Goal Advisor - Proposed` with the `Goal` chip and `Advisor idea` work label, and include why it is outside the current plan, what evidence/assumption supports it, the expected upside, and the risk/next decision. Do not auto-apply it from the advisor scan alone.
- **Advisor experiment** — one compact material transition for a strategy proposal, approval, measured result, adoption, or retirement. Keep stable `data-*` identity/status attributes for compatibility, but visible copy follows the 70-word card limit: thesis, expected goal effect, checkpoint, and current outcome only. Full baseline, guardrails, evidence, and lifecycle stay in SQLite-backed Pulse records. Update the same card in place while it is among the six active material transitions; archive terminal history normally.
- **Legacy Chief of Staff recommendation** — historical only. Preserve old cards for audit history but do not create, route, update, or act on them.
- **Human answer** — after a structured question is answered and materially changes the workflow, add one compact card containing the question, selected answer, and outcome. Full evidence stays in SQLite. The unanswered request itself stays only in `report_human_inputs` and Runloop's **Needs your decision** surface. When a later pass uses the answer, call `mark_human_input_consumed` and update that same attributed card instead of editing SQLite directly.
- **User rule** — a constraint the user stated. Mark it clearly as authoritative ("USER RULE — authoritative") so future agents treat it as a hard constraint, never silently override it. This replaces the old `source: "user"` field — say it in words.
- **Note** — a freeform observation or watchpoint that explains weird runs ("staging UI is mid-redesign, expect selector churn through ~June 20 — not a workflow bug").
- **Legacy open finding** — preserve old cards only in monthly archives. A new or materially reopened issue may create one short Monitor transition with an invisible `data-issue-id`; standing state remains in Pulse.

### Classification chips

Use two different chip families so the user can scan what happened:

- **Verdict chip**: `<span class="kind bug">Bug</span>` or `<span class="kind goal">Goal</span>` answers which verdict lane the entry belongs to. Monitor, Open finding, Decision, and Artifact Review entries should carry one when applicable.
- **Action label chip**: `<span class="worklabel ...">...</span>` answers what kind of work/fix this was. Use at most two per entry, immediately after the verdict chip.

Canonical action labels:

- `worklabel bugfix` → `Bug fix`: Pulse Fixer changed prompts/config/guards/validation/code shape to make the workflow run correctly.
- `worklabel improvement` → `Improvement`: Goal Advisor or a builder change improved strategy, plan quality, success criteria alignment, cadence, or user-facing usefulness.
- `worklabel advisor` → `Advisor idea`: Goal Advisor proposed an out-of-plan expert recommendation or unconventional opportunity that could help the goal but needs user choice or stronger evidence before changing the plan.
- `worklabel artifact` → `Artifact drift`: Artifact Review found or resolved plan-change drift in reports, evals, KB/learnings, saved code, db wiring, or generated HTML.
- `worklabel report` → `Report fix`: report dashboard/query/HTML/data-binding repair.
- `worklabel eval` → `Eval fix`: evaluation rubric, eval wiring, route scoping, or score evidence repair.
- `worklabel cost` → `Cost/time`: LLM/model/cost/time telemetry observation or repair.
- `worklabel maintenance` → `Maintenance`: Pulse hygiene/radar decision, especially when it intentionally skipped optional work on high-frequency schedules or escalated to a deeper check.
- `worklabel backup` → `Backup/publish`: backup or publish repair/status issue.
- `worklabel input` → `Needs input`: the workflow is waiting for a user answer or decision.
- `worklabel manual` → `Manual`: user-requested/manual builder action.

Do not over-label routine clean runs. If a change spans categories, choose the primary action label and add one secondary label only when it helps scanning (for example `Bug fix` + `Report fix`, or `Advisor idea` + `Improvement` when a proposal is both out-of-plan and directly tied to strategy).

### Recording an issue transition

When an issue changes materially, add one short Activity event for the
transition. Carry the stable issue identity only in
`data-issue-id`; visible copy says what changed and whether it is fixed,
waiting for verification, reopened, blocked, or externally owned. Do not edit a
standing HTML finding card because no such active card should exist.

### Decision cards (clear action and why)

A Decision card is the visual proof that an agent took or proposed an action. It should never read like a routine note. Use:

- `<div class="entry decision">` for normal applied/proposed Goal Advisor decisions.
- `<div class="entry decision major">` when the decision changes plan strategy, report/eval measurement, workflow cadence/scope, user-facing dashboard interpretation, or materially affects cost/quality/risk.
- `.tag.decision` with one of these exact labels: `Decision - Goal Advisor - Applied`, `Decision - Goal Advisor - Proposed`, `Decision - Pulse fix`, or `Decision - Manual`.
- A verdict chip plus an action label chip. Examples: a Pulse fix uses `<span class="kind bug">Bug</span><span class="worklabel bugfix">Bug fix</span>`; a Goal Advisor plan change uses `<span class="kind goal">Goal</span><span class="worklabel improvement">Improvement</span>`.
- A `<p class="takeaway">...</p>` before the grid that says the decision in one user-readable sentence.
- `.decisiongrid` rows for the fixed human fields: **Why now**, **Change**, **Expected impact**, **How we will confirm**, and **Risk / gap**. Omit a row only when it truly does not apply; do not bury these fields in prose. Each field is one short plain-language sentence. Raw paths, ids, hashes, file lists, and schemas belong only in the SQLite-backed reviewer result.

Example:

```html
<div class="entry decision major">
  <div class="ehead">
    <span class="tag decision">Decision - Goal Advisor - Applied</span>
    <span class="kind goal">Goal</span>
    <span class="worklabel improvement">Improvement</span>
    <span class="etitle">Replanned lead-scoring around verified replies</span>
    <span class="when">2026-07-02 · Goal Advisor</span>
  </div>
  <p class="takeaway">We changed the plan because verified replies stayed below target for three clean runs.</p>
  <div class="decisiongrid">
    <div><b>Why now</b><span>Reply rate stayed below the 8% target for three clean runs.</span></div>
    <div><b>How we will confirm</b><span>Compare qualified replies after the next two runs.</span></div>
    <div><b>Change</b><span>Reordered enrichment before outreach and added a verified-reply gate.</span></div>
    <div><b>Expected impact</b><span>Raise reply-rate evidence toward the success criterion without increasing send volume.</span></div>
    <div><b>How we will confirm</b><span>Measure the target outcome over the next two eligible runs.</span></div>
    <div><b>Risk / gap</b><span>Needs two more clean runs before confirming impact.</span></div>
  </div>
</div>
```

### Confirming a decision's outcome (did the change actually work?)

A Decision card records what a Pulse fix or Goal Advisor plan change applied and *why* — but a journal that only ever says "applied X" never proves the system is working. So a Decision that changed behaviour stays **unconfirmed** until a later run measures its effect, and then it gets **one** outcome stamp added in place (never a second one, never a new card):

```
<p class="outcome ok">Confirmed by run #43 — login-skip gone, eval 0.72 → 0.81 over 2 runs.</p>
<p class="outcome bad">No effect by run #44 — reopened as Goal finding of-2026-06-12-eval.</p>
<p class="outcome flat">Inconclusive — run #44 didn't exercise the changed path; still pending.</p>
```

- **ok** — the expected number moved the right way (cite before → after and the run).
- **bad** — it didn't help or regressed; say so plainly and open (or reopen) a finding for it. A change that quietly failed is worse than no change, so never hide it.
- **flat** — the run that fired didn't exercise the changed path (routing), so the decision is still pending; leave it unconfirmed.

So a Decision is checkable, **state the expected effect when you write it** ("expect login-skip to stop and eval to recover toward 0.85") — that's the bar the later run is judged against. The per-run monitor owns applying these stamps (below); don't stamp a decision on the same run that made it.

### Keep the active file small

The log must not grow without bound. Before Gate, the scheduler conditionally sends a dedicated archive turn when `builder/improve.html` has **more than 6 material Activity cards** and at least one older resolved entry is safe to move. Byte size, line count, and token count do not trigger archiving. That turn decides semantically what can move; normal Gate/module turns should not improvise a second archive pass.

`builder/improve.html` remains the **current executive** Pulse view. Keep its verdict/status header, Latest Pulse summary, unresolved material decision transition when useful, and the newest **6** material Activity cards. Move legacy standing finding cards, old Gate/clean-review cards, older resolved transitions, superseded decisions, and routine old run rows into self-contained monthly archives at `builder/improve-archive/YYYY-MM.html`.

Archive safely:

1. Read the active file and any existing target-month archive in full.
2. Stage complete active and archive HTML documents in temporary files under `builder/`; archives are never bare card fragments.
3. Verify every moved card/run appears exactly once across active plus archive, both documents are non-empty and contain `html`, `head`, and `body`, and no protected/open item moved.
4. Only after validation, replace the monthly archive and then the active file. Never truncate the original first.
5. Add or update one compact Archive Index link using `href="improve-archive/YYYY-MM.html"`, with date range and moved-item count. Merge into an existing month without duplicates and keep entries newest first.

If the file crossed a mechanical threshold but has no safely archivable history, leave it unchanged and report that plainly. The active file must always answer "what is the workflow's state now, and what still needs attention?"

### Upgrading an old-format log (one-time, REQUIRED before appending)

An existing `builder/improve.html` may be **old-format**, in which case it must be
upgraded before anything is appended to it. That migration is a one-time contract
and is not needed on a steady-state write, so it lives in its own reference doc:
`read_skill(skills=[{"name":"builder-reference","path":"references/review-improve-log-migration.md"}])`.

Load it only when the log is old-format. Everything below applies to every write.

### Starter HTML skeleton

The full copy-paste HTML skeleton lives in `read_skill(skills=[{"name":"builder-reference","path":"references/review-improve-log-skeleton.md"}])`. Load it when creating `builder/improve.html` or doing the required one-time old-format upgrade. Keep this reference doc focused on log semantics; the skeleton reference keeps the CSS, filter script, and card examples in one place without bloating every normal review/improve prompt.

# Pulse: current architecture and decision map

> **Status (2026-08-05): canonical current design and rationale.** Read
> **Current architecture** and **Current decisions** for the system that should
> exist now. The sections under **Historical migration record** preserve why the
> system changed; they are not implementation instructions. Executable agent
> guidance remains in
> `agent_go/cmd/server/guidance/templates/system/post-run-monitor.md`, while Go
> registries and lifecycle validation remain the final runtime authority.

## Documentation ownership

Keep Pulse documentation split by purpose, not by competing versions:

| Question | Authoritative place |
|---|---|
| What is the current Pulse design, and why? | This document |
| What must a running agent do? | `post-run-monitor.md` and the focused skills it loads |
| What does Go actually permit and persist? | The Pulse module registry, scheduler, worklist, finding lifecycle, and impact-ledger code |
| What does the user see? | [`workflow_monitoring.md`](./workflow_monitoring.md) |
| What experiments, objections, and measurements led here? | [`pulse_v2_proof_carrying_architecture.md`](./pulse_v2_proof_carrying_architecture.md) |
| What was the reliability-first v2.1 experiment? | [`pulse_v2_1_experiment_proposal.md`](./pulse_v2_1_experiment_proposal.md), retained as history |
| What did one real workflow review cost and produce? | [`linkedin_pulse_review_audit_2026-08-02.md`](./linkedin_pulse_review_audit_2026-08-02.md) and later cross-workflow measurements |

Do not add another general Pulse architecture document. Put a shipped design
change and its reason in **Current decisions** below; put detailed measurements,
debate, and rejected alternatives in the proof-carrying architecture record;
put a concrete defect in `docs/bugs/`.

## Current architecture (2026-08)

Pulse is a **dynamic post-run steward**, not a fixed checklist. After each
scheduled run it runs a small sequence with one mandatory intelligence turn:

1. **Gate / Worklist.** One turn reads the run evidence (run summary, `CONCERNS:`
   markers, changelog, eval/report/DB/KB/learnings state, human inputs, Chief of
   Staff recommendations, cost/tier signals, and the store freshness ledgers) and
   calls `record_pulse_worklist` exactly once with one `due|skipped` decision for
   each of three agents: `workflow_review`, `strategy_auditor`, and
   `goal_advisor`. Go enforces the complete-worklist rule, every skip carries a
   next-check condition, and Gate mutates nothing. Historical operational IDs
   remain accepted as aliases and are projected into `workflow_review`; current
   passes never schedule them separately.
2. **Two independent read-only product agents.** `strategy_auditor` asks what is
   missing or ineffective inside the selected strategy.
   `goal_advisor` separately performs the less-frequent blank-sheet search for a
   materially different approach. None consumes another reviewer's conclusion.
   When both are due they run concurrently, and the scheduler waits for them
   before starting operational mutation.
3. **One operational review-and-fix sequence.** `workflow_review` is one
   continuous agent context with ordered checkpoints for correctness/QA, plan
   and artifact drift, reports/evals, stores, and cost/model/tool/runtime
   operations. It loads shared plan, run, goal, lifecycle, and cost evidence
   once, semantically deduplicates findings, persists that consolidated review,
   and then continues into one bounded Fixer turn in the same conversation and
   tool session. The final turn resolves conflicts, applies bounded safe fixes
   sequentially, and verifies each against the single `fix-verification` contract
   (a successful write is never proof; a fix stays `changed_unverified` until a
   real run/eval/report confirms it). A residual `pulse_fixer` agent runs only
   when Strategy/Goal outputs or a failed operational sequence leave a due
   module non-terminal.

   Advisor routing is explicit and machine-backed. An actionable
   `strategy_auditor` or `goal_advisor` recommendation creates a linked pending
   approve/reject/defer decision. `proposal_only` is accepted for those modules
   only with a concrete `next_check` evidence boundary. A truth-preserving
   technical prerequisite goes to the Fixer rather than becoming a strategy
   question. Material strategy or goal changes are never applied without the
   exact recorded approval.
   SQLite is authoritative for the per-finding lifecycle: `run_concerns` files
   and deduplicates findings, `pulse_fix_attempts` plus
   `pulse_fix_attempt_findings` record the mutation boundary,
   `pulse_fix_verifications` records passed/failed/inconclusive proof, and
   `pulse_finding_events` records filed/fixing/closed/reopened history. Complete
   reviewer Markdown is stored as human-readable SQLite TEXT in
   `pulse_review_log`; it is evidence, not the close/reopen state. The Pulse popup
   is one database-native, outcome-first workspace: Goal/Success/Constraints,
   latest retained run outcome, pending decisions, Engineering/Operations/Product
   work areas, one issue and follow-through queue, Pulse run/skip/Fixer activity,
   longitudinal goal impact, and finalization status. Raw Markdown and individual
   reviewer mechanics are secondary evidence views. Retired reviewer labels are
   normalized into Engineering or Operations rather than exposed as primary UI.
   The popup does not extract or display fragments from `builder/improve.html`.
   Every reviewer and consolidated Fixer also gets one automatic
   `pulse_agent_metrics` row keyed by its runtime execution ID. It separates
   queue delay from agent duration and snapshots exact LLM calls, input/output,
   cache read/write, reasoning tokens, model breakdown, cost, and measurement
   coverage from the central cost ledger. Review agents do not self-report this
   data.
4. **Longitudinal impact ledger.** The Fixer records interventions, immutable
   per-run success-criterion observations, and append-only before/after impact
   assessments in SQLite. Reliability, measurement, and presentation work are
   explicitly separated from direct goal impact. The Pulse popup projects the
   latest criterion values and improved/regressed/inconclusive/awaiting counts;
   later passes add evidence instead of overwriting earlier conclusions.
5. **One ordered finalizer.** dashboard → backup → publish → notify, each recording
   its own live/final status. The scheduler marks anything left running as failed.

The dedicated Dashboard stage still owns `builder/improve.html`. That file is
the lightweight published executive journal and archive-linked material history,
while the Pulse popup owns the complete operational tracker. Moving reviewer
evidence and lifecycle state into SQLite does not retire the publishable artifact.

Workflow contract v1.0.17 non-destructively imports recognized historical
`pulse/reviews/**/*.md` into `pulse_review_log` and keeps the source files during
the compatibility window. New Pulse reviewers write their complete Markdown
directly to SQLite and create no review file. The popup falls back to retained
legacy files only when a workflow has no matching database review yet.

Standalone Pulse-module slash commands use the same path. `/bug-review`,
`/ops-review`, `/strategy-auditor`, and `/review-artifact-drift` pass their
canonical module to `call_generic_agent`; Go generates manual run identities,
stores the full Markdown in `pulse_review_log`, and indexes `CONCERNS:` into the
finding lifecycle before the parent updates `builder/improve.html`.
`/engineering-review` is the only user-facing mutation command: it launches the
same Engineering → LLM/Ops → consolidation → Fixer sequence used by scheduled
Pulse. The former standalone `/pulse-fixer` command is retired so manual and
scheduled maintenance cannot drift into two Fixer workflows.
`/goal-advisor` calls the native Advisor → Critic → Finalizer pipeline directly;
that pipeline persists its complete result and remaining concerns the same way.

Goal Advisor is a Pulse-selected module (not a separate schedule). Recovery,
timeout, and concurrency are hardened in Go: a trusted-session registry binds each
logical Pulse run (recovery sessions included) so only the owning session can write
its module/worklist/final-command state, agent writes are compare-and-set against
the recorded run and never overwrite a scheduler-recorded terminal state, and a
boot sweep reconciles final commands stranded by a crash.

**Store freshness (2026-07).** A code-owned ledger (`learnings/_global/_freshness.json`,
`knowledgebase/_freshness.json`) records when each store — and each reference
file / topic note — was last confirmed by a run, stamped by the runtime at the
learnings/KB contribution turns (not LLM-maintained, so it can't desync). Gate
marks Engineering Review due with the store-integrity evidence pack on a
confirmation-recency signal. That pass re-verifies → refreshes / demotes /
retires aging knowledge (never deletes on age alone). This adds a *time/decay*
axis to what was previously only contradiction-driven staleness.

## Current decisions

### Four perspectives, one shared engineering/operations reviewer, two independent product reviewers (2026-08-05)

Gate independently decides whether four perspectives are due:
`workflow_review` (Engineering Review), `llm_ops_review`, `strategy_auditor`,
and `goal_advisor`. Artifact names are evidence packs, not reviewer identities.
Engineering Review conditionally loads execution, report/eval implementation,
plan-change/artifact-consistency, and DB/knowledgebase/learnings evidence.
LLM/Ops owns cost, latency, model choice, tools, retries, timeouts, and runtime
efficiency. When either is due, the backend places only those selected
perspectives into one native ordered reviewer conversation and adds one
consolidation turn. Skipped perspectives consume no turn.

For a two-perspective shared pass, every `CONCERNS:` item must carry exactly one
selected owner. Go rejects missing, duplicate, or skipped ownership and files
the concern only under Engineering or Ops. Historical `bug_review`,
`artifact_review`, `report_health`, `eval_health`, and `stores_health` records
normalize into Engineering Review; `cost_llm_time` normalizes into LLM/Ops.

`strategy_auditor` remains a separate product/business agent because it asks
whether the correctly implemented current strategy and its report/eval system
are useful and capable of achieving the goal. It emits a user-facing
in-strategy proposal and, for a material behavior change, a linked decision
whose source is `strategy_auditor`. `goal_advisor` remains a separate,
less-frequent blank-sheet agent because it searches outside the selected
strategy; it normally emits an approve/reject/defer decision whose source is
`goal_advisor`. Neither depends on Engineering, Ops, or the other. All due
reviewers may run concurrently; the one Fixer starts only after their barrier.
Engineering normally creates issues and repairs, not product proposals.

The scheduler passes compact `review_lanes` identities; the backend owns their
ordered Engineering/Ops messages, persisted consolidation checkpoint, and
bounded Fixer turn for `role=fixer, module=workflow_review`. Pulse reviewer launcher sessions carry
an explicit `parent_session_id` and `session_kind=pulse_reviewer`. Refresh
recovery and the global activity monitor keep those internal children out of
the top-level chat list, remove child tabs persisted by older frontends without
stopping their runtimes, and retain the Pulse parent as the selected session.

### One sequenced operational writer and one finding lifecycle (2026-08-01 to 2026-08-05)

Engineering/LLM-Ops review and bounded fixing share one agent conversation:
selected review turns, a persisted pre-mutation consolidation checkpoint, then
the Fixer turn. Strategy Auditor and Goal Advisor remain independent read-only
agents; a residual Fixer runs only for their non-terminal lifecycle work or
operational recovery. The writer reconciles semantically duplicate findings,
applies bounded changes sequentially, and records attempt-scoped proof. SQLite is authoritative
for findings, attempts, verification, review artifacts, module outcomes, and
final-command status. `builder/improve.html` is still required, but it is a
generated user-facing dashboard and publishable time-series artifact, not the
database for closing or reopening findings. The Pulse popup reads structured
SQLite projections and exposes raw reviewer Markdown only as supporting detail.

The same rule applies to manual maintenance. `/engineering-review` runs the
combined sequence; `/pulse-fixer` no longer exists. The internal residual
`pulse_fixer` stage identity remains a recovery mechanism only and is not a
second normal Fixer or a slash command.

The Fixer is a full Workflow Builder writer: its tool allow-list is derived
directly from the canonical Workshop profile rather than copied into a smaller
Pulse-specific subset, and its folder guard uses the same Workshop write paths.
This includes plan/route add-delete, schedules, skills, LLM configuration,
execution/debugging, reports/evals, secrets, and managed DB/store mutation.
Pulse-run identity, finding lifecycle, external-side-effect rules, and explicit
strategy/goal approvals still govern when those tools may be used.

Direct shell/file writes to protected plan artifacts remain denied. In
particular, `update_evaluation_plan` receives a scoped in-process capability for
exactly `evaluation/evaluation_plan.json` (plus the existing managed
planning/changelog capability); it does not unlock sibling evaluation files or
leak authority back into the session.

### Verification has two valid timings (2026-08-01)

A deterministic data or logic repair can be verified immediately by replay,
re-read, or an exercised consumer path. A behavior change that affects a future
producer remains `changed_unverified` until a later workflow run exercises it.
On later passes, the relevant review lens checks eligible pending attempts; it
does not create generic verification markers for unrelated retained findings.

### One backup at finalization (2026-08-03)

The separate agent-driven pre-backup stage was removed. It duplicated the
ordered finalizer backup, spent an additional agent turn whenever any review was
due, and could prevent read-only reviews from running when the preliminary
backup failed. Pulse now performs backup once, in the finalizer after reviews,
fixes, and dashboard projection, so the durable backup captures the completed
post-Pulse state. The legacy five-step schedule detector still recognizes old
`pre backup` message queues only so they can be migrated; it does not schedule
or execute a preliminary backup.

### Measure interventions across later runs (2026-08-03)

Pulse impact is measured at the intervention level, not attributed directly to
one reviewer. The ledger links reviews and findings to one coherent repair or
approved experiment, records comparable success-criterion observations on
later runs, and appends improved/regressed/inconclusive/awaiting assessments.
Reliability, measurement quality, presentation maintenance, and direct goal
impact remain separate categories so closing many bugs cannot masquerade as
goal progress.

### Measure every Pulse agent by execution identity (2026-08-03)

Reviewer and Fixer efficiency is now durable rather than reconstructed from
scheduler logs. The backend assigns each child one execution ID, the central
cost observer attributes immutable LLM events to that ID, and completion
snapshots the matching rows into workflow-local `pulse_agent_metrics`. Parallel
reviewers therefore cannot be combined under `todo_task:0`, and a Fixer cannot
be mistaken for a reviewer. A missing ledger or missing attribution is stored
as `usage_status=unavailable` with a reason; it is never rendered as zero usage.

`GET /api/workflow/pulse-agent-metrics` supports run, module, and role filters.
`GET /api/workflow/pulse-reviews` joins the matching reviewer measurement. The
Pulse popup shows the latest pass wall time, summed agent time, calls, tokens,
cache, and cost, and each reviewer evidence view shows its own numbers. Wall
time and summed agent time are deliberately separate because concurrent agents
can make the sum larger than elapsed clock time.

### Rejected completed reviews are preserved (2026-08-03)

**Implemented and verified.** Before launching a reviewer, the backend now
derives an exact verification allowlist from owned `changed_unverified` attempts
and appends it to the trusted reviewer instructions. A marker outside that list,
or any malformed marker, cannot reach lifecycle disposition handling. The full
review is instead retained as a non-actionable `contract_failed` artifact with
the exact contract error, while valid sibling reviews remain available to the
Fixer. A stopped Fixer is still not equivalent to a successfully terminal Pulse
pass; durable module state remains the deciding evidence.

---

## Historical migration record (2026-06)

Status at the time: **Implemented** (2026-06-21) — Phase 1 (rename) + Phase 2 (backup-always +
Pulse-does-low-risk-fixes). Auto-fix was intentionally scoped to low-risk reversible
harden; bigger `replan` changes stayed with the scheduled auto-improve loop. (Both
the fixed 4-step pipeline and the separate auto-improve loop below were later
replaced by the dynamic Gate/worklist model in **Current architecture**.)

## Pulse vs the auto-improve loop (division of labor)

These are two tiers over the same Pulse log / Bug-Goal vocabulary — they compose,
they don't overlap:

- **Pulse** — runs after **every** run (when enabled). Cheap + immediate: **back up →
  triage → apply a low-risk reversible Bug harden → record a Goal replan *proposal* →
  notify.** Never runs `replan` or a full improvement pass itself.
- **Auto-improve loop** — a **separate schedule** (`/auto-improve`, optimizer mode).
  Owns the **bigger changes**: batched harden and the **`replan`** tool for structural
  plan rewrites, acting on the proposals Pulse recorded. Pulse is skipped after these
  optimizer-mode runs (`scheduler.go`: `WorkshopMode != "optimizer"`), so the two never
  fight over the same run.

## Problem

After a run there are **four disconnected mechanisms**, with different triggers,
gating, and reliability:

1. **Post-run monitor** (`runPostRunMonitor`, `scheduler.go:1164`) — opt-in
   (`post_run_monitor`), a dedicated agent pass that writes the Pulse log
   (`builder/improve.html` time-stamped Signal / Reflection / Improvement history; Goal / Ikigai comes from `soul/soul.md`). Its final notify/summary step
   writes the `builder/card.health.html` dashboard card after harden, artifact review,
   cost/time, backup, and publish are known. Auto-improve **cadence #1**.
2. **Scheduled harden** — auto-improve **cadence #2**, applies low-risk Bug fixes on
   its own schedule.
3. **Scheduled replan-proposal** — auto-improve **cadence #3**, proposes Goal changes.
4. **Backup** (`workflowRunBackupDirective`, `background_agents.go:1607`) — a directive
   appended to the builder's AUTO-NOTIFICATION after a `run_full_workflow` completion.
   Best-effort steering, **not** a guaranteed step; scheduled runs don't back up
   themselves (`scheduler.go:1268`).

Naming reality: the monitor reference doc already calls `builder/improve.html`
"**the Pulse log**". So "monitor" (the pass) and "Pulse" (the log) are two halves of
one feature that was never unified in the UI.

## Decisions (user, 2026-06-21)

1. **Pulse everywhere.** The feature/toggle is named **Pulse**. The right-panel tab is
   **Pulse** (reverts the Phase-3 "History" rename). Internally the monitor pass becomes
   the "Pulse pass."
2. **Full auto.** Enabling **Pulse** runs the complete post-run loop **every run**:
   **back up → triage → apply fixes (harden for Bug, replan for Goal)**. The four
   mechanisms above collapse into this one toggle.

## Target model

```
Pulse (one toggle) → after every run:
  1. Back up        — always, guaranteed (local-git default = zero-config). Skipped only
                      when source_hash is unchanged (no empty commits).
  2. Triage         — the current monitor: Bug + Goal verdicts, Pulse log, verdict signal.
  3. Fix            — Bug → harden (low-risk reliability/contract fixes);
                      Goal → replan (now applied, was propose-only).
  4. Notify         — one transition notification (unchanged policy).
Pulse tab (UI) → the durable record: Timeline (improve.html) + Plan edits (changelog).
```

Backup is no longer a separate steering directive; it's step 1 of the Pulse pass and
runs in the scheduler post-run block where the monitor already lives.

## Changes required

### UI (rename — safe, mechanical)
- `WorkflowCanvas.tsx`: tab label + titles "History" → **"Pulse"** (revert Phase 3).
- `HistoryView.tsx` → `PulseView.tsx` (component rename; keeps Timeline + Plan edits
  sub-tabs). `PlanChangelogFeed.tsx` unchanged.
- `WorkflowToolbar.tsx`: the monitor toggle (`monitorOn`) relabels to **"Pulse"**.

### Behavior (the real change — needs care)
- `runPostRunMonitor` → `runPulse` (or keep name, expand prompt). Add **backup as step 1**
  (guaranteed, source-hash gated) and **fix as step 3** (harden + replan).
- Rewrite `guidance/templates/system/post-run-monitor.md`: it is no longer read-only —
  it backs up, then triages, then applies the safe fixes. Reconcile with the existing
  harden / replan reference docs so there's one fix contract, not three.
- `scheduler.go`: backup runs here for scheduled runs (always). Remove / demote the
  `workflowRunBackupDirective` steering path so backup isn't double-driven.
- `PostRunMonitor` manifest flag stays the gate but is surfaced as "Pulse enabled".

### Risk / open
- **Auto-fix every run changes the safety model.** Cadences #1–#3 were deliberately
  separated (cheap read-only triage vs riskier fixes on a slower schedule). Folding them
  means a fix can land on every run. Mitigation: keep harden's "low-risk only" contract;
  keep replan as the heavier action and decide whether it truly applies or still proposes
  for high-risk plan rewrites. **To confirm before the behavioral rewrite ships.**
- Cost: every run now does backup + triage + (sometimes) fix instead of a cheap triage.
  Acceptable per the "full auto" decision, but worth a source-hash / no-op fast path.

## Backup visibility (2026-06-21)
Backup surfaces in three places now: the toolbar status dot + the Backup popup
(existing), and — added here — the **Pulse log Run row**. Since Pulse owns the backup,
its step-3 Run row records the backup result (`backed up ✓ <commit>` /
`unchanged — already backed up` / `backup ✗ <reason>`). Doc-only change to
`post-run-monitor.md` (step 3) and `review-improve-log.md` (Run kind) — agent-driven,
no Go.

## Phasing
1. **Rename to Pulse** ✅ Done (2026-06-21, UI only). `WorkflowCanvas` tab + titles
   "History" → "Pulse"; `HistoryView.tsx` → `PulseView.tsx`; toolbar "Monitor" button +
   help popup → "Pulse" (internal vars `monitorOn`/`post_run_monitor` unchanged).
   Verified: tsc 0, lints clean.
2. **Backup always + Pulse does low-risk fixes** ✅ Done (2026-06-21). Rewrote the
   Pulse pass prompt in `scheduler.go` (`runPostRunMonitor`) to the 4-step contract
   (back up → triage → low-risk harden / replan-proposal → notify) and rewrote
   `guidance/templates/system/post-run-monitor.md` to match (new "0. Back up first" +
   "3b. Apply the fix" sections; dropped the strict read-only framing; kept step 5 =
   Notify so the prompt's reference still resolves). Backend build + vet OK.
   Safety rails kept: **low-risk reversible fixes only** (bigger work → auto-improve
   loop), and **source-hash gate** so steady runs skip the push.

3. **Unify the backup directive** ✅ Done (2026-06-21). `workflowRunBackupDirective`
   (the interactive arm) now shares **one backup contract** with Pulse's step 1: same
   zero-config local-git default, same source-hash skip. So the two arms (Pulse =
   scheduled, directive = interactive + Pulse-off fallback) can't double-push — whichever
   runs second sees the source already backed up and skips. Stale `scheduler.go:1268`
   comment corrected. Build + vet OK; no test pinned the old wording.

   **Non-goal (decided 2026-06-21):** do NOT move the source-hash skip into deterministic
   Go code. The whole post-run/backup loop stays agent-driven — the agent reads
   `backup/status.json` and decides. No Go-side gating coupling.

## Phase 4 — Publish folded into the Pulse loop (2026-06-24)

Publish (the public-URL twin of Backup — see `docs/workflow/publish_design.md`) is now a
step of the Pulse pass, so a workflow's public dashboard stays current automatically:

```
Pulse → after every run:
  1. Back up    — guaranteed, source-hash gated (unchanged).
  2. Triage     — Bug + Goal verdicts, Pulse log (unchanged).
  3. Fix        — low-risk harden / replan proposal (unchanged).
  4. Re-publish — ONLY if publish is configured + enabled; rebuilds from source and
                  redeploys both artifacts. Skipped when publish is off.
  5. Notify     — one transition notification (unchanged).
```

`post-run-monitor.md` gained a "### 4b. Re-publish (only if publish is on)" section. Like
backup, publish is **agent-driven and read-only in the UI**: setup/run/restore/publish all
happen in the builder chat via the **`/backup`** and **`/publish`** slash commands; the
toolbar popups are status-only. The dead write endpoints (`/workflow/{backup,publish}/{config,run}`)
were removed.

### Publish output contract (what `/publish` ships)
- **Both artifacts, always** — the baked report **dashboard** (`dashboard.html`) AND the
  **Pulse log** (`pulse.html`, from `builder/improve.html`), joined by a top-nav
  `index.html` wrapper (Dashboard | Pulse tabs). Publishing only one is a bug.
- **Dark only, matching the app** — the published pages must set **both** theming hooks on
  `<html>`: `class="dark"` (the app's Tailwind mechanism, `ThemeContext`) **and**
  `data-theme="dark"` (what report widgets `HtmlWidgetFrame` and the Pulse-log skeleton
  key on). Setting only `data-theme` left the dashboard light, because report widgets key
  primarily off the `.dark` class. No toggle, no `prefers-color-scheme` (that follows the
  viewer's OS). See `[[project_published_page_theme_contract]]`.
- **Stage outside the workspace** — deploy CLIs (`netlify`, `vercel`, …) write state
  (`.netlify/`, `.vercel/`) to their CWD. The workflow folder is writable EXCEPT
  `planning/`, but the CLI's CWD is often the docs root (above the folder, outside the
  write allow-list), so it gets rejected. Copy the finished static files to
  `/tmp/publish-<workflow>/` and run the deploy from there.

### Plan-edits consolidation (2026-06-24)
The toolbar **Plan edits** popup (the granular `planning/changelog/*.json` audit feed) got a
**Consolidate** control — drop edits older than 7/30/90/180 days — backed by
`POST /workflow/plan-changelog/prune`. The server prunes (deletes whole `changelog-*.json`
files older than the cutoff) because `planning/` is shell-guarded from the agent.

## Workflow-specific advisor specialization (2026-08-06)

Strategy Auditor and Goal Advisor keep stable product-wide contracts, but can now
receive separate owner-approved workflow lenses. `/specialize-advisors` proposes
exactly two reusable Markdown texts and records them in one durable Pulse decision.
Its guidance loads both canonical reviewer contracts first and enforces a
delta-only filter: generic review procedure already present in the base prompts is
removed, leaving only stable domain evidence semantics, failure traps, invariants,
tradeoffs, and opportunity spaces that a general reviewer would otherwise miss.
Nothing changes until the owner selects **Activate** and the agent calls
`update_workflow_config(advisor_specialization_approval_input_id="...")`.
An answered specialization decision is routed by the next Pulse Gate to the
writer-capable consolidated Fixer; a manual Workshop chat may activate it
immediately after the answer with the same tool.

The config tool resolves the exact approved decision text rather than accepting
replacement prose, writes both lenses together, versions the result, and stores it
under `workflow.json` `pulse.advisor_specialization`. Strategy Auditor receives only
the current-strategy lens; Goal Advisor receives only the blank-sheet opportunity
lens. Engineering and Ops never receive either. Canonical reviewer contracts and
the current owner-approved soul/plan always override a conflicting specialization.

Use this as the read-only audit checklist for artifact drift after plan or configuration changes. It checks whether step config, the schedules that drive the workflow, learnings, saved code, knowledge-base notes, database contracts, reports, evaluation, and recent run evidence still match the current workflow. It also flags missing or stale eval coverage with an `Eval fix` owner label.{{if .Focus}} Focus especially on: {{.Focus}}.{{end}}

## Execution model

- In Pulse, the parent may include this checklist in the normal Engineering
  background executor when artifact-drift evidence is selected.
- Outside Pulse, launch one `run_in_background` executor with this checklist as
  its read-only instructions.
- If you are already that background reviewer, perform the audit directly. Never launch another reviewer, background tool, or nested maintenance agent.
- The call returns an `execution_id` immediately. End the current turn and wait
  for the automatic completion notification; do not poll, sleep, or repeatedly
  call `query_step`.
- The reviewer is strictly read-only. It must not edit files, mutate the plan/config,
  mark changelog entries, or mark Pulse module state.
- Artifact drift is a technical-review focus, not a third Pulse module. Read only
  matching typed state through `get_pulse_state(view="review")` with
  `module=technical_review`, using the managed SQLite-backed tools and structured finding lifecycle; do not query SQLite directly or inspect/create Pulse
  presentation artifacts.

For a suspected drift that recurs or a repaired dependency awaiting proof,
compare the current artifact/run evidence with up to three comparable retained
runs (same route/group and materially equivalent configuration). Read compact
receipts first; open raw traces only for the changed or suspicious step/attempt.
State an evidence limitation when fewer comparable runs remain.

Load `read_skill(skills=[{"name":"builder-reference","path":"references/assumption-audit.md"}])`. While tracing changed surfaces, identify dependent artifacts that preserved an old architecture, tactic, schema, metric, or execution assumption after the plan evolved. Keep consequential unresolved restrictions under Pulse's Assumptions challenged.

## Audit checklist

1. List `planning/changelog/changelog-*.json` in filename order and select entries where `artifact_review.done` is not true.
   - Changelog markers are the only artifact-review cursor. If a marker is absent,
     inspect the exact entry; do not rely on a separate presentation cursor.
   - If no cursor exists and more than 100 entries are unreviewed, inspect only the latest 100 and report that the older entries remain unreviewed.
   - Never advance the proposed cursor past an entry that was not fully inspected or safely cursor-backfilled.
   - **The changelog only records plan mutations** (`update_scripted_step` and
     the other typed plan-mod tools) — `planning/` is never a granted write
     path for any other tool, so `plan.json` itself is always fully and
     truthfully captured here. A direct edit to a step's own code
     (`learnings/<step-id>/main.py`) or a shared doc (`db/README.md`) made via
     `diff_patch_workspace_file`/`update_workspace_file` is a different kind
     of change and does not produce a changelog entry, even when it happens
     in the same turn as a plan-tool call — the entry that turn produces
     describes only the plan-tool's own field change. When a changelog entry
     does flag a step as affected, step 3 below already directs inspecting
     that step's `main.py` directly, so the code change is still caught. The
     residual gap is narrower: an isolated code/doc edit with **no**
     accompanying plan-tool call in the same turn produces no changelog
     entry at all, so nothing routes that step into this checklist's
     cursor-selection step in the first place — this is a scope boundary of
     the changelog cursor, not evidence the checklist itself missed
     something once a step is in scope.
3. For each affected step, inspect only relevant current artifacts:
   - `planning/plan.json` and `planning/step_config.json`
   - the workflow's schedules in `workflow.json` — for each schedule, its cron,
     timezone, and the `messages` queue. The queue is what the scheduler
     actually sends, so it is a first-class contract with the plan, not
     configuration noise: read every message and resolve each to the plan step
     it drives.
   - `learnings/<step-id>/main.py`, script metadata, per-step learning metadata, and relevant `learnings/_global/` guidance
   - relevant `knowledgebase/notes/` content and KB access/contribution settings; treat `knowledgebase/context/` as read-only user-owned context
   - `db/README.md`, named DB tables/assets/contracts, and their writers/consumers
   - `db/reports/index.html`, its internal views, SQL, and data contracts; flag a remaining `reports/report_plan.json` as an incomplete version migration
   - `evaluation/evaluation_plan.json`, `evaluation/step_config.json`, and matching goal/success-criteria coverage
   - one representative recent run for changed runtime behavior when evidence exists
   - for any changed status, strategy, feature flag, guard, routing rule, or
     other control value, trace the exact changed record to the current runtime
     reader and one resulting decision/output. If similarly named tables/files
     carry the same logical IDs, compare them and identify the canonical owner
     plus the required mirror rule. A clean changelog/file diff is not enough
     when the runtime reads a different store.
4. Record a finding only when evidence shows drift, including:
   - code, paths, fields, selectors, tool/API usage, or validation still implement an old contract
   - stale code/learning locks after a material change without review evidence
   - learnings or KB preserve obsolete behavior or agent-inferred policy
   - DB writers, report consumers, or eval consumers disagree on schema or semantics
   - a change updated a plausible but non-canonical store, or duplicate control
     stores disagree so the allocator/router/executor cannot observe the repair
   - report/eval checks use stale artifacts, fields, thresholds, or run identity
   - a changed success criterion lacks eval coverage, or an eval is orphaned/duplicative
   - deleted steps still have live references, or new steps lack required dependent wiring
   - a schedule-local procedure that duplicates durable plan behavior, lacks
     `direct_messages_reason`, or claims canonical step-level learning,
     validation/retry, repair, or Pulse attribution that it does not receive.
     Direct schedule conversation is a supported execution model, so do not
     report it merely because it is long. Compare reuse, inputs/outputs,
     side effects, approval and failure boundaries; report the missing route
     only when those facts show the work belongs in the canonical plan.
   - a schedule message that drives no plan step, or a plan step no schedule message reaches and no other step invokes — dead work, or a queue that was
     never updated after the step was added
   - execution order or grouping that exists only in the message queue while
     `plan.json` leaves order/groups unset. The plan is then not runnable on its
     own, and the queue silently owns the sequence
   - a message queue that only restates steps the plan already orders, which is
     duplication that will drift the next time either side changes
   - schedule cron/timezone that contradicts what `soul.md` or the plan claims
     about cadence or the window the work is valid for
   - a schedule message that should invoke canonical work but bypasses
     `run_full_workflow`/`execute_step`; use `validate_plan_change` when a repair
     changes plan references or dependency wiring
5. Include clean checks briefly. Do not manufacture drift merely because an artifact exists.

## Reviewer result

Return one compact review package containing:

- `module=technical_review`, `verdict`, and `next_check`
- cursor before and proposed cursor after
- changelog files and zero-based entry indexes fully inspected
- affected steps inspected
- findings ordered by severity; each includes no invented identifier, a plain-
  language root-cause summary, exact evidence, bounded `recommended_fix`,
  verification, recommended owner, and `user_judgment_required` with reason
- clean checks
- exact proposed marks grouped as `clean`, `findings`, or `cursor-backfill`
- for every proposed non-backfill mark, `surface_reviews` covering
  `downstream_steps`, `validation`, `evaluation`, `reporting`, `database`, and
  `learnings_and_knowledge`, each with one disposition (`updated`,
  `already_compatible`, `not_applicable`, `blocked`, or `broken`) and concrete
  evidence; `blocked` and `broken` also require durable Pulse `issue_ids`
- any blocked entry that prevented further cursor advancement

The parent Pulse Fixer/workshop agent validates this package, applies only bounded approved fixes, records typed Artifact Review findings/dispositions, and calls `mark_changelog_artifact_reviewed` for only the exact verified entries with all required surface reviews. Do not edit or delete changelog JSON directly and do not create a second cursor or state file.

A finding marked `user_judgment_required` needs the user's call before it is applied (never before). If this command is running as a live chat turn with the user present (a standalone `/review-artifact-drift`), ask directly in this chat and wait for the reply. If this is the unattended scheduled Pulse pass, never ask a direct question -- nobody is watching that chat and it would stall unanswered -- use `create_human_input_request` instead, which surfaces as a Needs your decision card the user answers later.

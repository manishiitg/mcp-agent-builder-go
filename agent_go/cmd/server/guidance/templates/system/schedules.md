## Schedule Management (Workshop mode)

For the operational cheat sheet on creating / editing / deleting schedules
(cron syntax and workshop run payload shape), see this section.

- **Tools**: `list_schedules`, `create_schedule`, `create_calendar_schedule`, `update_schedule`, `delete_schedule`, `trigger_schedule`, `get_schedule_runs`.
- To view existing schedules, call `list_schedules`; it includes schedule IDs, type, mode, workshop mode, cron/calendar shape, timezone, enabled state, groups, and recent runtime state. `get_workflow_config` also includes a Schedules section when you are already inspecting broader workflow settings.
- **Entry shape**:
  ```
  { "id": "...", "name": "...", "description": "...",
    "cron_expression": "0 9 * * 1-5", "timezone": "UTC",
    "enabled": true, "trigger_payload": {},
    "group_names": ["confida-prod"],
    "mode": "workshop", "workshop_mode": "run" }
  ```
  Fields: `id` (auto-assigned), `name` (display label), `description` (optional), `cron_expression` (standard 5-field cron), `timezone` (IANA tz e.g. `America/New_York`), `enabled` (bool), `trigger_payload` (arbitrary JSON passed to the run), `group_names` (required array of one or more explicit group names from `variables/variables.json`), `mode` (`workshop` for workflow schedules), `workshop_mode` (`run` for normal recurring workflow runs).
- Schedule management is available in **Workshop mode**. If the user asks in Run mode, tell them to switch.

### Two schedule types: cron vs calendar

Every schedule in `workflow.json` has a `schedule_type` — `"cron"` (default) or `"calendar"`. They are stored side by side under the same `schedules` key; the difference is *when* they fire.

- **`cron`** — a repeating pattern that fires forever on a cadence (`create_schedule`, `cron_expression`). Use for "every weekday at 9 AM", "every 30 minutes", "first of the month". This is the default; *Writing messages for scheduled runs* below applies to cron schedules.
- **`calendar`** — a fixed list of specific dated runs, each firing exactly once (`create_calendar_schedule`, `calendar_items`). Use when the user gives concrete dates/times instead of a recurring rhythm — e.g. a full-month Instagram content calendar, a launch sequence, a one-off batch on three specific days. There is no `cron_expression`; the scheduler registers **one job per future `calendar_item`** and each item fires once at its date+time, then is done.

**Choosing:** if the user describes a *rhythm* ("every…", "daily", "weekly") use cron; if they enumerate *dates* ("on the 3rd, 7th, and 12th", "post these on these days") use calendar. When in doubt, ask whether the runs repeat or are a fixed set of dates.

**`create_calendar_schedule` payload:**

```
{ "name": "March content calendar", "timezone": "Asia/Kolkata",
  "group_names": ["group-1"], "mode": "workshop", "workshop_mode": "run",
  "calendar_items": [
    { "date": "2026-03-03", "time": "09:00", "description": "Optional note" },
    { "date": "2026-03-07", "time": "18:30" }
  ] }
```

- `calendar_items` (required): each needs `date` (`YYYY-MM-DD`) and `time` (`HH:MM`), both interpreted in the schedule's `timezone`. `description` is an optional per-item note; `messages` is an optional per-item message queue.
- `timezone` (required, IANA — e.g. `Asia/Kolkata`, not `IST`) and `group_names` (required) work exactly as for cron schedules.
- **Mode is the same as cron**: workflow schedules use `mode="workshop"`. Supply per-item `messages` or a top-level default `messages` array when the default full-workflow run instruction is not specific enough.
- Past-dated items are skipped — only future items get registered. To change a calendar schedule, update its `calendar_items` (add/remove dates); editing tools (`update_schedule`, `delete_schedule`, `trigger_schedule`, `get_schedule_runs`) work on calendar schedules too.

### How workflow schedules execute

Workflow schedules always use the workshop builder execution path. Do not create direct `mode="workflow"` schedules; legacy manifests with that value are normalized to workshop execution.

- **Run** (`mode=workshop`, `workshop_mode=run`) — LLM-driven execution. Prefer an empty queue plus `group_names`/`route_selections` for durable workflow behavior: canonical steps receive their normal learning, validation/retry, repair, and Pulse attribution lifecycle. Direct messages remain valid for genuinely schedule-specific conversation, but require `direct_messages_reason` and do not automatically gain that step-level lifecycle.

**Default mode rule:** create workflow schedules with `mode="workshop"`. New schedules should never use `mode="workflow"`.

**Recurring Pulse**: the single source of truth is one enabled `pulse_review_only` schedule — there is no workflow-level Pulse toggle or mode. The workflow toolbar/Pulse popup's on/off toggle creates and enables this schedule directly; it never needs a chat command. If a user asks in chat to enable or configure recurring Pulse, create or update that schedule yourself with `create_schedule(pulse_review_only=true, cron_expression=..., ...)` — no `group_names`/`route_selections`/`messages` on it, since it never runs the workflow. If the workflow also has no recurring execution schedule and the user wants regular runs, create that separately (`mode="workshop", workshop_mode="run"`, select configured groups/routes as data, leave messages empty) — the two are independent schedules, not a bundled setup. Do not create a separate optimizer Goal Advisor schedule; Pulse Gate decides when the Goal Advisor module is due. `/goal-advisor` is a one-off strategy review and must not change schedules.

**Pulse never runs inline with a normal scheduled run.** A normal scheduled run does backup, execution-report publish, and run-summary notification only; Gate/Review/Fix/Finalize run separately, on the review schedule's own cadence, over whatever run backlog has accumulated. This applies regardless of how often the workflow runs: long Pulse-adjacent sessions reused across runs have caused real reliability problems independent of run frequency.

Read this workflow's intended run frequency (or, for an existing workflow, its actual `get_schedule_runs` history) to choose the review schedule's INTERVAL — that remains a judgment call, balancing review latency against genuinely batched evidence; when in doubt, prefer more frequent over less, since Gate's own backlog reasoning already handles reviewing several accumulated runs in one pass cheaply. Check `run_retention_count` (workflow.json, default 3) against the interval chosen: if the workflow could produce more runs between reviews than retention preserves, raise it, since a rotated run folder beyond `run_retention_count` is permanently deleted. The lightweight per-run pass itself keeps re-checking this cadence on an ongoing basis as actual run volume changes — this is the one-time setup version of that same responsibility, not the only time it happens.

### Back up scheduled workflows

Scheduled runs execute unattended and accumulate state (`workflow.json`, `planning/`, `knowledgebase/`, `learnings/`, `db/`, reports) that otherwise lives only on local disk. **Whenever you set up a recurring schedule, also arrange a backup** so each run persists its output off-box. Load `read_skill(skills=[{"name":"builder-reference","path":"references/backup-strategy.md"}])`, follow it once to initialise the workflow's backup destination, and persist the result in `workflow.json.backup`.

- Set `workflow.json.backup.enabled=true`, `mode="agent"`, `triggers.after_scheduled_run=true`, and a `destinations` entry for each backup target (git/github for config, R2/S3/B2/HuggingFace for large artifacts as needed).
- After each backup attempt, write `backup/status.json` with the destination results, timestamps, summary, and errors. Do not put changing backup status in `workflow.json`.
- If you rely on the default full-workflow message, the auto-notification after `run_full_workflow` will still ask the builder to honor `workflow.json.backup` and write `backup/status.json`.

Confirm with the user before skipping backup on a recurring schedule.

### Writing messages for scheduled runs

`messages` is an ordered queue of strings sent to the workshop LLM one-by-one as user turns. The LLM completes all tool calls triggered by message N before message N+1 is sent.

- Write each message as a plain instruction, like you would type in chat: `"Run the full workflow"`, `"Generate the final report"`.
- **Route-backed mode (default)**: select planned work through `group_names` plus optional `route_selections`, and keep `messages` empty. Use this for durable workflow behavior so one canonical plan owns its lifecycle.
- **Direct-sequence mode (supported exception)**: use one or more messages when the conversation itself is genuinely schedule-specific and should not become reusable plan behavior. Set `direct_messages_reason` with the concrete tradeoff. These turns run in the workshop but are not canonical steps, so step learnings, validation/retry, repair, and Pulse attribution are weaker or unavailable unless the sequence explicitly invokes planned work.
- Never choose solely from message length. Compare behavior, inputs/outputs, external side effects, approval boundaries, failure behavior, and expected reuse.
- Read `variables/variables.json` for available group names and include them explicitly in the message if needed.

**CRITICAL — schedules run unattended; messages must never require human input:**

- Explicitly tell the agent to make all decisions autonomously: `"Do not ask for confirmation, proceed automatically"`.
- Provide all required parameters upfront in the message (group names, run folders, step IDs) so the agent never needs to ask.
- Tell the agent to skip or use defaults for anything unclear rather than pausing to ask.
- Never include open-ended questions or `"let me know"` style instructions.
- **Bad**: `"Run the workflow and ask me which steps to optimize"`.
- **Good**: `"Review runs/iteration-0 for group-1, collect read-only reliability evidence, then let the parent fixer choose a bounded repair, an approved plan change, a Goal Advisor proposal, or no action."`

Pulse module cadence is not encoded in schedule JSON. Pulse Gate stores module state in `db/db.sqlite` and decides which modules are due after each normal run.

Do not create new `workshop_mode="optimizer"` schedules. Existing saved legacy
values are handled by migration/backend compatibility; new continuous
improvement uses normal Run mode plus Pulse.

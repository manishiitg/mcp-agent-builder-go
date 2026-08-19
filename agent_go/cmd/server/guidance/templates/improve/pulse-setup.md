Set up recurring workflow runs with dynamic Pulse. Goal Advisor remains a
Pulse-selected module; this command does not run Goal Advisor itself.

Pulse is SQLite-backed and shown in the Pulse popup. Do not create, read, or
update a separate Pulse HTML journal.{{if .Focus}} Focus especially on: {{.Focus}}.{{end}}

## Discovery

1. Read `workflow.json`, schedules, `soul/soul.md`, valid variable groups,
   planning changelog, and typed Pulse module/finding/review/human-input state.
2. Confirm that the Goal has success criteria. If not, run `/define-success`
   before scheduling.
3. Identify an existing normal execution schedule, stale optimizer schedules,
   and any current Pulse state that affects safe cadence.

## Setup

1. Create or update one normal workshop Run-mode schedule for recurring
   workflow execution. Do not create a separate optimizer or Goal Advisor
   schedule.
2. Create or update one enabled `pulse_review_only` schedule for recurring
   Pulse. That schedule is the single source of truth for Pulse enablement and
   cadence; never add a second workflow-level toggle.
3. If an old optimizer schedule duplicates Pulse, disable it (do not delete it
   unless requested) and record the reason as a typed Pulse outcome.
4. Let Gate select review modules from durable typed state, current run
   evidence, and review history. `pulse_module_state` is the canonical
   scheduler/UI state; do not add cadence fields to schedule JSON.

## Close-out

Report both schedules, disabled duplicate schedules, and
what Gate will decide on future runs. Persist findings or decisions through the
typed tools only.

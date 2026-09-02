[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-269 — `todo_task` ran on its own duplicated turn loop, carried an unused scripted fast path, and was named for a todo list that no longer existed

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `implemented; live e2e reverify pending` |
| Last synchronized | `2026-09-02` |

- **Priority:** P2 maintainability with a P1 side effect: every fix to the
  conversational turn loop had to be made twice. PLAT-189 (proactive
  validation-schema surfacing) and PLAT-255 (pre-validation notifications)
  were each patched separately in `controller_message_sequence.go` and
  `controller_todo_task.go`.
- **Owner:** `pkg/orchestrator/agents/workflow/step_based_workflow/`
  (`controller_todo_task.go`, `controller_message_sequence.go`,
  `sub_agent_async.go`), `cmd/server/virtual-tools/sub_agent_tools.go`,
  guidance `todo-task.md` / `optimize-playbook.md`.

## What was wrong

**Two executors for one shape.** A `todo_task` step is a conversational agent
plus a delegation capability. `controller_todo_task.go` (2019 lines)
re-implemented the message_sequence executor's turn loop, folder guard,
per-turn logging, prevalidation, and learnings/KB contribution turns. The
`messages` field on a `todo_task` even reused `MessageSequenceItem`, then ran
it through a private copy of the item loop (`runTodoTaskMessageSequence`).

**A scripted fast path nothing used.** A `todo_task` could declare
`declared_execution_mode="scripted"` and run `learnings/{step-id}/main.py`
instead of the LLM orchestrator. Its eligibility rule — a fixed set of route
calls branching only on success/failure — is exactly the case PLAT-068 says
must not be an orchestrator. A scan of every `Workflow/*/planning/plan.json`
found 12 live `todo_task` steps; none declared scripted mode and none had a
`main.py`. The path also lacked the regular scripted step's repair loop and
save-back, and a failure fell back to a fresh LLM run that re-ran every child.

**A dead todo list.** `todo_tools.go` (`create_todo`, `update_todo`,
`complete_todo`, `list_todos`) was never registered anywhere; the three
`todo_task_item_*` events were never emitted. Yet `call_sub_agent` and
`call_generic_agent` required a `todo_id` described as an "existing durable
todo ID", and PLAT-157's identifier-boundary text justified it as a link to
that todo item.

## What changed

1. **Scripted orchestrator path removed.** `executeTodoTaskStep` no longer
   consults `declared_execution_mode`; `update_step_config` rejects `scripted`
   on a `todo_task` with a message pointing at a regular scripted step that
   calls routes. `controller_scripted.go` is untouched and still serves regular
   scripted steps. The guidance sections describing the mode are deleted; the
   PLAT-068 render test now asserts they are gone.
2. **Dead todo list deleted; `todo_id` → `task_id`.** The tools, the three
   event types, their event-store allowlist entries, and the frontend render
   paths that could never fire are removed. The sub-agent tool argument is now
   `task_id`, a plain label for tracking and artifact naming. The
   route-selected and step-completed events keep their field names for wire
   compatibility. `todo_id_to_execute` on the orchestrator decision record is
   a different field and is untouched.
3. **Orchestrator runs on the message_sequence executor.**
   `messageSequenceCallOptions.Delegation` is a new seam holding the
   `SubAgentExecutionContext`, the narrower orchestrator folder guard, an agent
   factory (the orchestrator agent type, so PLAT-027's progress bridge keeps
   working), the orchestrator's template variables, and the `todo_task`
   execution-log writer (so `ExecutionLogsPopup`'s route-selection view and
   `stepLogs.todo_task` keep rendering). The executor:
   - builds the agent once and reuses it for every item;
   - after every conversational turn, waits for the children that turn
     launched and feeds one completion batch back (`reconcileAsyncSubAgentCalls`)
     before the next item — on the error path too;
   - refuses non-conversational items for an orchestrator even if plan
     validation was bypassed;
   - guarantees `cancelOutstandingAndWait` on every exit path;
   - skips the global learnings mutex on the reflection turn, because a parent
     that waits for children inside the turn would deadlock against a child's
     own learnings turn (the orchestrator never held that lock before either).
   `executeTodoTaskStep` is now a ~150-line wrapper. Deleted:
   `runTodoTaskMessageSequence`, `runTodoTaskPreValidation`,
   `runTodoTaskContributionTurns`, `persistCompletedTodoTaskSummary`,
   `executeTodoTaskOrchestratorAgent`, and the restart-the-step retry loop.
4. **Retry semantics changed deliberately.** The orchestrator used to restart
   the whole step up to three times with validation feedback, and an exhausted
   retry returned success=false without an error, which the caller then marked
   completed. It now uses the sequence model: per-item `prevalidation` with
   in-place repair turns, a synthetic final validation gate with the same
   repair loop, and a hard error when the gate is exhausted. Repairs keep the
   conversation and its sub-agent results instead of re-running every child.

## Not changed (by decision)

- The plan type name `todo_task`, the planning tools, the drift-review check
  IDs, the event wire names, and the frontend compound-node graph. Same
  pattern PLAT-259 used for branch vs routing: one executor, distinct type. A
  rename of the type string to `orchestrator` (with a parse-time alias and a
  contract-version migration) is a separate follow-up.
- The PLAT-068 eligibility invariant. Reworded from "use todo_task only when"
  to the same rule about giving a step routes; the render test still asserts
  it in all four guides.

## Verification

- `go build ./...`, `go vet`, `go test` for `step_based_workflow`,
  `cmd/server`, `cmd/server/virtual-tools`, `cmd/server/guidance`,
  `internal/terminals`, `internal/events`: green. Frontend `tsc -b && vite
  build`: green.
- **Live e2e still pending** (unit tests count as zero coverage for agent
  code): run a real `todo_task` with routes (`testing` /
  `execution-regression-router`, or `linkedin` / `step-p2-multi-drafter`)
  via Workshop `execute_step` and confirm children launch async, the
  completion batch lands, the step completes only after children settle,
  `stepLogs.todo_task` is populated, the route-selection view renders, and the
  folder guard excludes the workflow root; run one `todo_task` with a
  `messages` list; run a plain `message_sequence` and confirm no sub-agent
  tools appear; set `declared_execution_mode=scripted` on a `todo_task` and
  confirm the rejection.

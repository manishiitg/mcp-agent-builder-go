# Incident archive

Each file is one investigated defect: symptom, evidence, root cause, and what was
changed. They are written to be re-read by someone who was not there.

Cross-workflow Pulse findings that require shared runtime work are consolidated
in [pulse_platform_issue_register.md](pulse_platform_issue_register.md). It is a
triage register, not a replacement for the individual incident reports below.

## The agent-facing contract (2026-08-01 → 02)

Nine of these were written in two days and describe one subsystem: **what an
agent is told it can do, versus what the runtime actually permits.** They are
listed together because each was found by pulling on the previous one, and
reading any of them alone understates the pattern.

| Document | What it establishes |
|---|---|
| [custom_tool_category_as_agent_addressing.md](custom_tool_category_as_agent_addressing.md) | `CustomTool.Category` served as both an authorization key and an agent-visible address. `get_api_spec` demanded a category the prompt said was not an address — 46 failed calls in one day, the largest single source of failed tool calls in the system. Also covers the tool-inventory delivery gap and the `read_skill` migration. |
| [pulse_fixer_sqlite_readonly_wal_and_schema_guessing.md](pulse_fixer_sqlite_readonly_wal_and_schema_guessing.md) | A WAL database cannot be opened read-only without an existing `-shm`. The failure looked environment-dependent and was not; the original sandbox diagnosis is recorded and disproved. Includes the audit of every read-only open site. |
| [codex_structured_turn_completion_hang.md](codex_structured_turn_completion_hang.md) | Process teardown was reachable only from a stdout terminal event. A stage agent wrote its complete result, then held its caller for 65 minutes. Fixed with an independent completion signal and a bounded reap. |
| [stage_agents_cannot_read_skills_or_query_db.md](stage_agents_cannot_read_skills_or_query_db.md) | The missing builder allow-list entry was a false diagnosis: mcpagent injects intrinsic `read_skill` per turn. The stage still had a real bug—no reference skills were attached—now fixed by attaching the complete reference surface. Also records the remaining plan-review DB gap. |
| [scripted_steps_invisible_to_pulse_review.md](scripted_steps_invisible_to_pulse_review.md) | A scripted step that exits 0 with schema-valid output is reported as success forever, because `bug_review`'s trace contract named only agentic artifacts. The run log existed all along at `logs/<step>/execution/scripted_fast_path.json`. Split: `bug_review` owns correctness defects, `strategy_auditor` owns drift. **Prompt changes unverified — mcpagent tree does not compile.** |
| [steps_never_learn_from_their_own_validation_failures.md](steps_never_learn_from_their_own_validation_failures.md) | A step is given its validation schema but never told its last output failed it, so a contract mismatch recurs forever — `deliver-briefing` at `seen_count 3` reading an identical prompt each run. Fixed for agentic steps. **Two gaps stay open**: `bug_review` saw these and worked on other things, and a *succeeding* scripted step produces no trace for review to read. |
| [tool_failures_invisible_in_backend_logs.md](tool_failures_invisible_in_backend_logs.md) | A rollout scan found 78 bridge failure-envelope occurrences rendered as green checks and unfindable in logs because the outer shell transport returned `exit_code: 0`. Fixed with a `[TOOL_ERROR]` marker and two narrow UI detectors; one post-rebuild runtime marker check remains. |
| [workflow_step_shell_working_directory.md](workflow_step_shell_working_directory.md) | A workflow step has two unrelated working directories—the CLI's isolated temp dir and the server-side shell's. Dedicated child sessions lost the intended shell cwd and fell back to workspace root. Fixed by assigning the run execution cwd directly and rejecting missing workflow-step cwd. |
| [diff_patch_unbounded_subprocess_hang.md](diff_patch_unbounded_subprocess_hang.md) | Found *using* the `[TOOL_ERROR]` markers above. `DiffPatchDocument` shells to `patch(1)` with `exec.Command` — no context, no timeout, no stdin — so a stuck subprocess is uninterruptible: 4 of 10 requests never completed, against a 7–12 ms norm. The agent saw `context canceled` (step teardown, not the cause), retried three times, then routed around the tool with `printf >`. **Root-cause location established, trigger unconfirmed. Not fixed.** |
| [../refactor/mcpagent_public_api_simplification.md](../refactor/mcpagent_public_api_simplification.md) | The refactor these motivated: 70 exported `Agent` methods let callers sequence internal lifecycle themselves, so the same fact lived in several independently stale copies. |

### The shared shape

Every one is the same defect in a different place: **one fact, two sources, and
nothing checking they agree.**

- the prompt says categories are not addresses; `get_api_spec` demanded one
- the prompt says `db/db.sqlite` is writable; the folder guard denies it
- the prompt lists `get_prompt`/`get_resource`; they are not registered
- the prompt names the run folder; the guard grants only its `execution/` child
- the tool index is materialized into prompt text that a later overwrite discards

Two second-order lessons worth carrying:

**Recovery text is an instruction, not documentation.** An agent that gets a bad
error message does not stop — it acts on the hint. Three separate bugs turned one
failed call into several because the correction named something that did not
exist. Error text should be held to the same standard of truth as a tool result.

**Absence is the case that needs handling.** Several of these were written as
"update if present, else silently skip", which is indistinguishable from
"correctly had nothing to do". When a component's job is to supply something,
skipping is the bug.

### Why these were invisible

A tool that fails behind the HTTP bridge returns its error as stdout with
`exit_code: 0`, and shell denials exit 0 as well. Every one of these rendered in
the UI as a green check until `frontend/src/utils/toolCallFormatting.ts` learned
to read the harness envelope and stderr. Before changing anything in this area,
confirm the failure is actually visible — otherwise a fix cannot be verified.

## Earlier incidents

- [session_tool_registry_lifecycle_leak.md](session_tool_registry_lifecycle_leak.md)
- [auto_unlock_loop_orchestration.md](auto_unlock_loop_orchestration.md)
- [dependency_update_failure.md](dependency_update_failure.md)
- [parallel_tool_lock_contention.md](parallel_tool_lock_contention.md)
- [mcp_startup_retry_and_double_construction.md](mcp_startup_retry_and_double_construction.md)
- [uvx_cache_bloat_latest_versions.md](uvx_cache_bloat_latest_versions.md)
- [workspace_docs_path_inside_repo.md](workspace_docs_path_inside_repo.md)

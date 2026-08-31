## Workshop & Workflow Tools — Full Reference

The workshop chat agent has access to a broad set of workflow-management
tools. The inline system prompt now carries only a one-line-per-category
cheat sheet; this skill is the deep reference with full signatures,
parameters, when-to-use rules, and gotchas.

If you need to confirm an exact parameter shape that isn't documented here
and you are a coding CLI reaching tools through the api-bridge (code
execution / agentic mode — see the "Coding-CLI bridge routing" section
below), call `get_api_spec(tool_name="...")` — that returns the live JSON
schema for the tool. `get_api_spec` is registered only in that mode; a
native tool-calling session already receives every tool's full schema
declared directly to the model and must not call it — the runtime has
nothing to answer with, and the call fails as "not registered for session".

### Coding-CLI bridge routing

The function-style names in this catalog are logical workflow tool names;
they do not mean every tool is exposed natively by `api-bridge`. In a coding
CLI, the native bridge exposes only `execute_shell_command`,
`diff_patch_workspace_file`, `agent_browser`, and `get_api_spec`. Never try
`api-bridge.list_executions`, `api-bridge.query_step`, or another catalog name
as a native bridge call. For every non-native tool, call
`get_api_spec(tool_name="<name>")` first, then use
`execute_shell_command` to invoke the endpoint it returns with the supplied
`$MCP_MCP`/`$MCP_CUSTOM` route and `$MCP_AUTH`. Do not guess or hardcode an
HTTP URL.

## Step Execution & Inspection

- **`execute_step(step_id, group_name, instructions?, human_input?, tier?, message_sequence_restart?)`** — Start a single step in the background; returns `execution_id`. In Workshop mode this is the primary way to test one step after adding or editing it. Execution uses `iteration-0`. A standalone `message_sequence` always starts its configured queue from the beginning; `human_input` adds opening context, not durable resume. Only a message-sequence route inside an active todo-task run has in-memory re-entry. For `human_input` steps, `human_input` is used as the response. For other executable steps, `human_input` is high-priority custom context.
- **`execute_step(step_id, group_name, fast_path_only=true)`** *(Workshop mode only)* — Run the step's saved Python `learnings/{step-id}/main.py` directly with the same workflow env, args, output folder, and validation behavior as a real workflow run. Never falls back to LLM. Used to quickly test `scripted` main.py patches.
- **`query_step(step_id, tool_call_id?)`** — One-off live status check for a running single step. Resolves the latest execution for that step and shows execution status plus structured MCP tool calls and tool-call details captured so far. For coding-CLI providers, terminal/TUI progress does not appear as MCP tool calls — but when the step runs in tmux, query_step **inlines the latest lines of the live terminal pane** and also returns the tmux session name + a `tmux capture-pane -pt <session> -S -200` command for deeper history (same session as the UI terminal). Do **not** combine `sleep`, `list_executions`, and `query_step` into a polling loop. After launching background work, use at most one immediate status check, then yield; `[AUTO-NOTIFICATION]` will resume the conversation when the work completes.
- **`send_step_message(execution_id, message)`** — Send a correction or follow-up to the exact active child-agent turn inside a running step or full-workflow execution. Use the ID returned by `execute_step` / `run_full_workflow`; never guess from the step ID. Coding CLIs receive native live input, while non-coding agents queue the message for the next safe turn boundary. This does not start or resume work. `no_active_agent` means the execution is between agent turns or running validation/script work; wait instead of retrying in a loop.
- **`debug_step(step_id, iteration, group_name)`** — Rich insights: learning status, validation result, log paths.
- **`list_executions(status_filter?)`** — List all background executions. Use it to find an execution id or debug ambiguity, not as a repeated progress poll.
- **`stop_step(execution_id)` / `stop_all_executions()`** — Cancel running steps.
- **`run_in_background(name, instruction, message_sequence?)`** — Spawn an independent background agent with the same tools. `message_sequence` is optional. Use it only when one executor needs ordered follow-up turns in the same conversation; each item needs a non-empty `message`, e.g. `[ {"message":"Review the evidence."}, {"message":"Apply and verify safe fixes."} ]`. Optional labels are generated automatically.
- **`run_full_workflow(group_name, human_inputs?, route_selections?, disable_eval?)`** — Execute the complete workflow (all steps) for a single variable group in background. Always uses `iteration-0` and starts from the beginning. If the selected path has `human_input` steps, provide `human_inputs` (object mapping step_id to response string). For deterministic routers, pass `route_selections` keyed by routing or branch step ID, with each value as a `route_id` or unique `next_step_id`. Pass `disable_eval=true` only when the user explicitly wants to skip the automatic evaluation pass. Returns `execution_id`. In scheduled/run-mode flows, call it once for the intended group, then stop instead of polling substeps; auto-notifications report progress/completion.

## Step Config & Analysis (Workshop mode)

- **`update_step_config(step_id, ...)`** — Update servers, tools, skills, learning settings, execution mode, LLMs, locks, review notes, description review state, and read-only `additional_read_paths` grants for declared workflow-relative inputs outside the standard run/db/KB/learnings surfaces. For eval steps this writes to `evaluation/step_config.json`.
- **Pulse Review → Fix** — Scheduled Pulse stays in one main-agent conversation: Gate → Review+Fix → Finalize. Technical maintenance uses one retained executor that reviews, applies only a bounded safe repair when warranted, proportionally verifies it, then persists typed findings, repair outcomes, terminal result, and its `technical_review` receipt before ending. The receipt is durable evidence, not a permission switch or a separate turn. Strategic Review remains a separate read-only task and persists its own result. The parent validates typed state instead of recreating findings from notification prose. Manual `/pulse-review` and every focused `/pulse-review-*` alias use the same retained Review+Fix task; `/pulse-fixer` remains an explicit repair-only recovery command for already reviewed issues.
- **Objective + success criteria** — Edit `soul/soul.md` directly via shell (fill in the `## Objective` and `## Success Criteria` sections). `soul.md` is stable intent, not architecture: never add step design, provider/tool choices, implementation decisions, inferred assumptions, or a decision log. Optional constraints belong there only when the user explicitly approved them as durable boundaries. `plan.json` owns the current implementation attempt. No dedicated tool — use `diff_patch_workspace_file` or a shell heredoc.
- **Notification preferences** — Keep execution reporting separate from Pulse reporting. `update_workflow_config(run_notification_instructions="...")` controls the **Run outcome** section: outputs, failures, goal movement, and metrics from the workflow execution. `update_workflow_config(pulse_notification_instructions="...")` controls the **What Pulse did** section: reviews, fixes, recommendations, decisions, backup/publish state, and next actions. If the user explicitly says a preference applies to every notification, save it in both fields. `workflow.json` `capabilities.notifications.run_summary_instructions` and `pulse_summary_instructions` are authoritative and supplied to Workflow Builder and Pulse finalization. Never put notification preferences in `soul/soul.md`; that file owns goal and user-approved outcome constraints, not presentation or delivery behavior. Missing preferences use Pulse defaults.
- **Strategic Review proposals and experiments** — Material strategy/path changes use the existing report interaction flow, not a separate plan-change workflow. Scheduled Pulse runs one retained `strategic_review` sequence: verify prior strategic work and audit hidden mechanisms, conditionally discover materially different approaches, run a critic conclusion, then persist typed state. An experiment is optional; the conclusion may instead keep, improve, propose an alternative, or wait for named evidence. Every running/measuring experiment records its causal interference domains. Independent experiments may run concurrently, while experiments that share a control surface, scarce resource, cohort, metric stream, or contaminating observation are queued or separated. Strategic Review may create or refresh `create_human_input_request(source="strategic_review", input_id="strategy-proposal-...", options=[approve,reject,defer], context="<proposal + exact intended edits + rationale + expected impact + risk + evidence>")`. A later Pulse run may apply an approved proposal with normal plan modification/config/eval/report tools, call `mark_human_input_consumed`, and advance the same typed intervention or experiment. Manual `/goal-advisor` remains an invocation alias, not a second scheduled Pulse module.
- **`review_workflow_timing(iteration?, group_name?, focus?)`** — Read-only latency review: finds the slowest groups/steps/tools/LLM calls and recommends faster descriptions, fewer handoffs, safer step merges, or plan changes.
- **`review_workflow_costs(iteration?, group_name?, focus?)`** — Read-only cost review: finds the biggest cost drivers and recommends cheaper models, fewer retries/handoffs, better descriptions, or plan changes without sacrificing success criteria.
- **`get_cost_summary(run_folder?)`** — Token usage and cost breakdown.

## Read-Only Info

- **`get_step_prompts(step_id, attempt?, iteration?)`** — System prompt and user message for a step.
- **`get_workflow_config`** — Inspect the workflow's current MCP servers, selected skills, available secrets, notification content instructions and one-way destinations, LLM config, and schedules. Use this instead of `cat workflow.json` when you need the full workflow config. For the global installed skill catalog, use `list_skills`.
- **`get_llm_config`** — Per-step LLM overrides.
- **`get_workflow_command_guidance(kind="review-artifact-drift", focus?)`** *(Workshop only)* — Canonical read-only artifact drift audit after material plan/config changes. In Pulse it is separate from Bug Review; the parent Pulse Fixer applies verified repairs and marks reviewed changelog entries.

## Plan Modification (Workshop mode)

- **Create steps**: `create_plan`, `add_scripted_step`, `add_message_sequence_step`, `add_human_input_step`, `add_todo_task_step`, `add_routing_step`, `add_branch_step`, `delete_plan_steps`, `cleanup_orphan_step_configs`.
- **Update steps**: `update_scripted_step`, `update_message_sequence_step`, `update_human_input_step`, `update_routing_step`, `update_branch_step`, `update_todo_task_step`. For a saved legacy `regular` step whose `declared_execution_mode` is absent or non-scripted, call `update_message_sequence_step`; the harness atomically upgrades it to its effective `message_sequence` runtime type while applying the edit. Never use `update_scripted_step` for that compatibility case.
- **Reclassify routing/branch**: `convert_routing_branch_step_type(existing_step_id, target_type)` atomically relabels an existing `routing` step as `branch` or vice versa — same id, same routes, only the question-field name changes. Never use `add_branch_step`/`add_routing_step` + `delete_plan_steps` to do this by hand: deleting the old step prunes its `step_config.json` row (`drift_review`, `execution_tier`, etc.) before the id can be reused, so it does not actually preserve history.
- **Todo task routes**: `add_todo_task_route`, `update_todo_task_route`, `delete_todo_task_route`. For todo_task routes, choose one pattern per route: inline `sub_agent_step` for a route-specific agent, or `orphan_step_ref` to reuse a shared orphan step already allowlisted via `shared_with.orchestrator_ids`. Do not set both.
- **Validation**: `update_validation_schema`.
- **Graph integrity is atomic**: every plan mutation validates all routing/branch routes, todo/message-sequence `next_step_id` fields, and human-input branches before saving. `PLAN_GRAPH_INVALID` means nothing was saved. Repair every reference listed in the error with the appropriate `update_*_step` tool (target an existing step ID or `end`), then retry the original mutation. In particular, reroute inbound references before deleting their target step.

## Variables & Config

- **`update_variable(action, name?, value?, description?)`** — Add, update, or delete a variable.
- **`add_group` / `update_group` / `delete_group`** — Manage variable groups.
- **MCP servers workflow**:
  1. `get_workflow_config` to inspect which servers are currently selected.
  2. `update_workflow_config(add_servers=["server-name"])` selects an **already-registered** server into the workflow. **Do NOT edit `workflow.json` manually.**
     - To **register a new server first** (so it can be selected), use `add_mcp_server(name, protocol="stdio"|"sse"|"http", ...)`: for a stdio server give `command` + `args` (+ optional `env`, `working_dir`) — e.g. an npx-launched server is `command="npx", args=["-y","<package>"]`; for SSE/HTTP give `url`. It registers a user-defined server and triggers discovery; then select it with `add_servers`.
  3. Optional workflow-level allowlist: `update_workflow_config(add_tools=["server:*"])` or `add_tools=["server:tool_name"]`. Tool entries must reference selected workflow servers.
  4. `update_step_config(step_id, servers=["server-name"], tools=["server:tool_name"])` to scope specific servers/tools to a step.
- **Browser workflow**:
  1. Pick the workflow mode with `update_workflow_config(browser_mode="none"|"auto"|"headless"|"cdp")`. Prefer `auto` unless the workflow must require an authenticated visible Chrome (`cdp`) or must stay isolated in the background (`headless`). For the specialized case where one workflow needs independent login identities, set `cdp_ports=[9222,9333]` (maximum four) and launch each port with a distinct Chrome `--user-data-dir`; ordinary workflow concurrency uses one shared CDP browser.
  2. For `agent_browser` steps, enable `workspace_browser:agent_browser` via `update_step_config(enabled_custom_tools=[...])` and attach the matching runtime skill with `enabled_skills=["agent-browser"]`.
  3. For browser steps, attach `enabled_skills=["agent-browser"]`; the runtime provides the managed `agent_browser` tool.
- **Slack Incoming Webhook notifications** (one-way, per workflow):
  1. Store the full URL with `set_workflow_secret(name="SLACK_NOTIFICATION_WEBHOOK_URL", value="https://hooks.slack.com/services/...")`. This encrypts and auto-attaches it; never write the URL into `workflow.json`.
  2. Configure the safe reference with `update_workflow_config(slack_webhook_secret_name="SLACK_NOTIFICATION_WEBHOOK_URL")`.
  3. `update_workflow_config` converts that secret into a backend-only notification credential and removes it from agent-visible `selected_secrets` / `SECRET_*` injection. Current and future `notify_user` calls automatically send a rich Block Kit card to the webhook in addition to enabled account-level channels. Use `slack_title`, `slack_color`, `slack_fields`, `slack_sections`, and `slack_footer` for structured summaries; even a plain call gets the safe rich default. `human_feedback` never uses it because Incoming Webhooks cannot return an answer. Pass `slack_webhook_secret_name=""` to disable it without deleting the stored secret.
  4. Never read a webhook through `$SECRET_*`, post with `curl`, put a webhook recipe into a plan step, or disable automatic Slack delivery to avoid a duplicate. The backend owns the URL, Block Kit validation, fan-out, and delivery status; the agent calls `notify_user` exactly once.
  5. This is independent of the interactive Slack bot (Socket Mode, @mentions, threads, and replies).
- **`update_workflow_config(add_servers?, remove_servers?, add_tools?, remove_tools?, add_skills?, remove_skills?, add_secrets?, remove_secrets?, run_notification_instructions?, pulse_notification_instructions?, slack_webhook_secret_name?, browser_mode?, cdp_ports?, run_retention_count?, advisor_specialization_approval_input_id?)`** — Update workflow MCP servers, workflow-level MCP tool allowlist, skills, secrets, scoped notification content preferences, one-way Slack webhook reference, browser mode/profile ports, run/eval backup retention, or activate the exact owner-approved Strategy Auditor + Goal Advisor specialization proposal. Advisor specialization activation accepts only an answered `advisor-specialization-*` decision whose selected option is `activate`; it resolves both texts from that durable decision and writes them together to `workflow.json`.

## Shell & Discovery

- **`execute_shell_command`** — Run shell commands. Quick lookups:
  - `jq '[.steps[] | {id, title, type}]' planning/plan.json`
  - `` jq --arg sid "step-id" '.. | objects | select(.id? == $sid) | {id, title, type, description, context_dependencies, context_output}' planning/plan.json ``
  - `cat planning/step_config.json`
  - `ls runs/`
  - `cat variables/variables.json`
- **`human_feedback`** — Open a blocking AgentWorks response card and immediately alert enabled notification channels. Use only for an explicit channel test or urgent, short-lived human-only input such as CAPTCHA, OTP, or immediate approval. In Builder chat, ask ordinary questions in the normal response instead. In bridge-only coding-CLI sessions, invoke its `$MCP_CUSTOM/human_feedback` endpoint with a **foreground curl** and wait for that same call to return the answer. Never use `nohup`, append `&`, delegate/background the call, save its result to a temporary file, poll for completion, or ask the user to send another message after responding. The foreground response resumes the agent automatically; do not set the shell timeout shorter than `human_feedback.timeout_seconds`. Cursor CLI has an approximately 60-second silent MCP-call ceiling, so Cursor agents must use `timeout_seconds <= 45`; after a real expiry, retry only if the input is still required.
- **`create_human_input_request`** — Non-blocking Pulse/goal-advisor question stored in the workflow's `db/db.sqlite`. The user answers in the AgentWorks Pulse/report panel.
- **`get_human_input_request`** — Read one existing decision by exact `workspace_path` and `input_id`, including its context, selected answer, evidence, and lifecycle status. Use this before applying an answered decision; do not query the backing SQLite table directly.
- **`answer_human_input_request`** — Record the user's explicit final answer to an existing pending question using the exact decision and option IDs supplied by the UI chat context. Never infer an answer from discussion. This sets the question to `answered`; it does not apply the decision or mark it consumed.
- **`mark_human_input_consumed`** — Mark an answered report question consumed after using it and recording the outcome through typed Pulse tools. Pending questions are rendered directly from SQLite, not duplicated in a separate presentation artifact.

## Skills

Skills are reusable instruction sets injected into step agents at runtime. They live at the **workspace root** `{{"{{"}}.AbsDocsRoot{{"}}"}}/skills/{folder}/SKILL.md` — shared across all workflows. Do NOT create or reference skills inside the workflow folder (e.g. `Workflow/trading/skills/` does not exist).

**Workflow for managing skills**:

1. **Find**: `list_skills` to see installed skills, or `search_skills(query)` to search the public registry.
2. **Install**: `install_skill(source)` (e.g. `owner/repo@skill-name`) or `import_skill(github_url)` — downloads into `{{"{{"}}.AbsDocsRoot{{"}}"}}/skills/{folder}/`. If a skill folder exists but has no `SKILL.md`, reinstall it using the same method it was originally installed with — **never write `SKILL.md` content manually**.
3. **Select for workflow/builder context**: `update_workflow_config(add_skills=["folder-name"])` — makes the skill visible as a selected workflow capability for workshop/builder discovery. **Do NOT edit `workflow.json` manually.**
4. **Enable for runtime steps**: `update_step_config(step_id, enabled_skills=["skill-a"])`. Step execution only receives the skills listed in that step's `enabled_skills`; workflow-selected skills do not cascade into runtime agents.
5. **Remove from workflow**: `update_workflow_config(remove_skills=["folder-name"])`.
6. **Uninstall**: `uninstall_skill(folder_name)` — removes files from workspace entirely.

Use `get_workflow_config` to see the workflow's selected skills. Use `list_skills` to see all installed skills.

## Secrets

Secrets are credentials (API keys, tokens, passwords) injected into step agents as `$SECRET_<NAME>` environment variables at execution time. They exist in three buckets:

- **Workflow secrets** — per-user, encrypted server-side, scoped only to this workflow. Use these by default for workflow-specific credentials.
- **User secrets** — per-user, encrypted server-side, reusable across workflows.
- **Global secrets** — operator-managed via `GLOBAL_SECRET_*` env vars on the server. Read-only from chat.

**Storing a new secret is one step.** `set_workflow_secret(name="BUFFER_API_KEY", value="<plaintext>")` stores, attaches, and injects the value into the active builder shell and future workflow steps. Use `set_user_secret` only when the same credential should be reusable across workflows; it also auto-attaches in a workflow-builder session.

**Attaching an already-stored secret is a separate operation.** Call `list_secrets`, then `update_workflow_config(add_secrets=["BUFFER_API_KEY"])`. The builder can use it immediately as `$SECRET_BUFFER_API_KEY`; no restart or new chat is required. If the requested name does not exist, ask for the value and store it. Never claim that a stored secret is unusable merely because its plaintext cannot be returned—the builder should consume it through the injected environment without displaying it.

Do **not** give boilerplate advice like `"rotate this secret"` after a normal user-requested save. Recommend rotation only when there is a concrete exposure reason: the value was printed into logs/output, committed to a file, sent to the wrong channel, or the user explicitly asks for security remediation.

**Other secret ops**:

- **Inspect**: `list_secrets` returns `global`, `workflow`, and `user` buckets — values are never exposed.
- **Edit a value**: call `set_workflow_secret` or `set_user_secret` again with the same name — it upserts.
- **Delete from store**: `delete_workflow_secret(name)` or `delete_user_secret(name)`. Workflow attachments are separate — also run `update_workflow_config(remove_secrets=["NAME"])` to detach.
- **Detach only (keep value)**: `update_workflow_config(remove_secrets=["NAME"])`.

Secret VALUES are never rendered into prompts, logs, or tool outputs. Builder and step agents consume them only through `$SECRET_<NAME>` in `execute_shell_command`. Never echo, print, or hardcode a secret value in descriptions, learnings, or `main.py`.

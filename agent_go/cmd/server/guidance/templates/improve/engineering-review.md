# ENGINEERING AND OPERATIONS REVIEW WITH FIXES

Run the same operational review-and-fix sequence used by scheduled Pulse. This
is one agent conversation: Engineering review, LLM/Ops review, consolidation,
then one bounded Fixer turn. Do not launch a separate Fixer and do not run Pulse
Gate, Strategy Auditor, or Goal Advisor.{{if .Focus}}

Focus especially on: {{.Focus}}. The focus sets priority; it does not suppress
other material operational evidence found by the selected lanes.{{end}}{{if .RunFolder}}

Use `{{.RunFolder}}` as the primary retained run.{{end}}

1. Load `read_skill(skills=[{"name":"builder-reference","path":"references/post-run-monitor.md"}])`.
   Use retained workflow, run, lifecycle, cost, and store evidence. Do not run
   the workflow merely to create review evidence and do not update
   `builder/improve.html`; the Dashboard is a later projection of SQLite state.
2. Launch exactly one agent with
   `call_generic_agent(todo_id="standalone-engineering-review",
   instructions="Run the complete Engineering and LLM/Ops operational review,
   persist its consolidated findings, then apply and verify bounded safe fixes.",
   preferred_tier=3, module="workflow_review", role="fixer",
   review_lanes=["workflow_review","llm_ops_review"])`.
   Do not pass `pulse_run_id`, `review_run_id`, or `message_sequence`. The backend
   creates the manual Pulse identity and supplies the canonical ordered turns:
   Engineering → LLM/Ops → consolidation → Fixer. All turns reuse one agent,
   conversation, MCP session, and mutation authority. Do not perform any review
   or repair inline in the parent chat and do not call `begin_pulse_fixer_run`.
3. `call_generic_agent` returns an `execution_id` immediately. End the current
   turn and resume only from its automatic completion notification. Do not poll
   the child or launch another Fixer while it is running.
4. After completion, read the persisted module and backlog state with
   `get_pulse_state`. Report what the sequence reviewed, what it fixed, what it
   verified, what remains `changed_unverified`, and any exact user or external
   boundary still required. Do not manufacture a second lifecycle pass.

The command is complete only when the single sequence has persisted its review
checkpoint and its Fixer turn has recorded one terminal module result for each
selected operational lane.

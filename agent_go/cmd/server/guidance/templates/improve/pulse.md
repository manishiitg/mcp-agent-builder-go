# MANUAL ONE-OFF PULSE

Run one standalone Pulse review now against retained workflow evidence. This is
for testing and investigation; it does not participate in the scheduler's
durable worklist, cadence, timeout, or final-command state machine. Use the
Pulse toolbar Run action for the full scheduler-equivalent workflow + Pulse
pipeline. Use `/pulse-setup` when recurring Pulse needs configuration. The
Manual-run boundary below is the authoritative list of what this command must
not do.{{if .Focus}}

User focus: {{.Focus}}.{{end}}{{if .RunFolder}}

Use `{{.RunFolder}}` as the primary run folder.{{end}}

## Canonical contract

	1. Call `get_reference_doc(kind="post-run-monitor")` and
	   `get_reference_doc(kind="review-improve-log")`. Follow the same evidence
	   review and independent read-only reviewer contracts. If bounded repairs are
	   warranted, use the same one consolidated Pulse Fixer as scheduled Pulse.
2. Select the latest meaningful retained run when no run folder was supplied.
   State the selected run before reviewing it. Do not generate fresh workflow
   evidence merely to make Pulse run.
	3. Read `get_pulse_module_state` as historical context. Decide which modules
	   are useful for this standalone review. Do not call `record_pulse_worklist`
	   or `mark_pulse_final_command_result`. If fixes are warranted, call
	   `begin_pulse_fixer_run` exactly once for the selected modules; its trusted
	   run owns the lifecycle and module-result writes.
4. Run every module this standalone review selects. Process modules independently
   in registry order. Reconcile and drain each module's retained backlog before
   deciding whether fresh discovery is warranted. If it is, issue exactly one
   synchronous `call_generic_agent` reviewer call for that module; never combine
   reviewers in one shell command or use a background shell/wait group. Every
   reviewer is read-only. Goal Advisor uses a separate read-only critic after
   its strategy draft.
	5. After all selected reviews, run exactly one `call_generic_agent` with
	   `role="fixer"`, `module="pulse_fixer"`, the trusted manual `pulse_run_id`,
	   and the full selected module list. It builds one short ordered queue of
	   compatible repair bundles, applies them sequentially, verifies them, and
	   calls `mark_pulse_module_result` once per selected module. Do not fix inline.
	6. Keep review, finding, attempt, verification, and module outcomes in SQLite.
	   Do not write `builder/improve.html` from reviewers or the Fixer; the Pulse
	   Dashboard is the presentation owner and projects this state on its next render.
7. Do not run the scheduler finalizer from this standalone command. Do not
   publish or notify. Back up before any local mutation, and report backup
   status in the final summary.

## Manual-run boundary

- Do not call schedule create/update/delete/trigger tools.
- Do not change `post_run_monitor`.
- Do not call `run_full_workflow` or `execute_step`.
- Do not enable Pulse as a side effect.
- Do not turn an operational correctness repair into a user decision.
- Do not apply a new strategy, business-semantic, or LLM/provider change without
  the existing exact approval flow.

Finish with one concise summary: evidence/run reviewed, modules selected and
omitted, fixes made, unresolved decisions, backup status, and the next evidence
checkpoint.

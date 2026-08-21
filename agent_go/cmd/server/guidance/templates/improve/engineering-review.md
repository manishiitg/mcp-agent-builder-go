# TECHNICAL REVIEW

Run the operational Review contract directly in this continuing Workflow Builder conversation.
Do not modify implementation files and do not run Pulse Gate,
Strategy Auditor, Goal Advisor, Dashboard, publish, or notify.{{if .Focus}}

Focus especially on: {{.Focus}}. The focus sets priority; it does not suppress
a lightweight scan for critical technical evidence.{{end}}{{if .RunFolder}}

Use `{{.RunFolder}}` as the primary retained run.{{end}}

1. Load `read_skill(skills=[{"name":"builder-reference","path":"references/pulse-review-fixer.md"},{"name":"workflow-commands","path":"references/ops-review.md"}])`.
   Treat `ops-review.md` as the canonical operations evidence and structural
   checklist. This continuing Review command overrides only that reference's
   Standalone Operations Review dispatch and read-only return wrapper: do not launch its
   standalone wrapper. Apply its checks inside this conversation,
   persist evidence-backed findings, and leave implementation changes to the
   independent `/pulse-fixer` command.
2. Use `pulse_run_id="current"`, which resolves to this current Workflow Builder
   chat. Call `record_pulse_worklist` exactly once with `mode="backlog_drain"`
   and a concrete `mode_reason`: `technical_review` is due and
   `strategic_review` is deferred with an explicit next-check boundary. Read
   `get_pulse_review_focus_agenda(module="technical_review", route_scope=<relevant route>)`, perform a
   lightweight scan for critical regressions, matured verification, answered
   decisions, plan routes, and retained run selectors, then choose the smallest
   sufficient route-aware technical focus set using priority plus durable
   rotation history. Route size is evidence, not a mechanical quota. Then read the retained backlog, pending verification,
   `get_pulse_state(view="backlog", detail="compact")` exactly once, plus the
   latest meaningful run evidence, plan/store state, and cost/runtime evidence.
   Select relevant issue/observation IDs from that bounded index, then request
   `detail="full"` only for those exact IDs (at most 20 per call). Never reload
   the complete compact index merely to filter or confirm a small ID set.
3. Own the review yourself. Use a specialist child only when independent focused
   analysis is genuinely useful; wait for its automatic completion and
   consolidate it before persistence. For every selected workflow observation,
   link it to an existing issue, promote it with evidence, or reject it as a
   non-issue. Persist typed findings and matured verification as they are
   established. Do not create a Markdown review report. For an exceptional
   repair that genuinely requires operator judgment, create or refresh one
   `create_human_input_request(source="technical_review", input_id="technical-decision-...", options=[approve,reject,defer])`
   before filing it with `recommended_route="decision_required"`, and pass the
   returned id as `human_input_id` to `record_pulse_finding`. Never leave a
   decision-required finding without a real pending question. Normal safe
   engineering repairs use `fixer_handoff` and do not consume operator attention.
4. Deduplicate by root cause and leave one compact, ordered canonical repair
   queue. Do not apply repairs. Record the chosen focus exactly once, then call
   `complete_pulse_review(modules=["technical_review"], ...)` exactly once with
   the truthful terminal review verdict. `/pulse-fixer` owns later mutations
   and repair outcome; it must not rewrite this review receipt.
5. Finish with a concise summary of what was reviewed, promoted, linked,
   rejected, already verified, awaiting evidence, or blocked. Tell the operator
   to run `/pulse-fixer` next when at least one safe canonical issue is actionable.

# ENGINEERING AND OPERATIONS REVIEW WITH FIXES

Run the operational Review+Fix contract directly in this continuing Workflow Builder conversation.
Do not launch a separate Fixer and do not run Pulse Gate,
Strategy Auditor, Goal Advisor, Dashboard, publish, or notify.{{if .Focus}}

Focus especially on: {{.Focus}}. The focus sets priority; it does not suppress
other material Engineering or Operations evidence.{{end}}{{if .RunFolder}}

Use `{{.RunFolder}}` as the primary retained run.{{end}}

1. Load `read_skill(skills=[{"name":"builder-reference","path":"references/post-run-monitor.md"},{"name":"builder-reference","path":"references/pulse-review-fixer.md"}])`.
2. Use `pulse_run_id="current"`, which resolves to this current Workflow Builder
   chat. Call `record_pulse_worklist` exactly once: Engineering and Operations are due;
   Strategy Auditor and Goal Advisor are deferred with explicit next-check
   boundaries. Then read the complete retained backlog, pending verification,
   latest meaningful run evidence, plan/store state, and cost/runtime evidence.
3. Own the review yourself. Use a specialist child only when independent focused
   analysis is genuinely useful; wait for its automatic completion and
   consolidate it before mutation. Persist typed findings and verification as
   they are established. Do not create a Markdown review report.
4. Deduplicate by root cause, build a compact repair queue, apply safe bounded
   fixes with normal Workflow Builder tools, and prove each immediately or name
   its exact future producing-run boundary.
5. Record one terminal module result for Engineering and one for Operations.
   Finish with a concise summary of what was reviewed, fixed, verified, left
   awaiting evidence, or blocked by an exact user/external boundary.

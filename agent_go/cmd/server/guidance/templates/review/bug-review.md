# STANDALONE PULSE BUG REVIEW

Run the same read-only QA and logic-bug review used by Pulse, without running
Pulse Gate and without applying fixes.{{if .Focus}}

Focus especially on: {{.Focus}}.{{end}}{{if .RunFolder}}

Use `{{.RunFolder}}` as the primary run folder.{{end}}

1. Load `read_skill(skills=[{"name":"builder-reference","path":"references/post-run-monitor.md"}])`,
   `read_skill(skills=[{"name":"builder-reference","path":"references/pulse-bug-review.md"}])`,
   `read_skill(skills=[{"name":"builder-reference","path":"references/assumption-audit.md"}])`, and
   `read_skill(skills=[{"name":"builder-reference","path":"references/review-improve-log.md"}])`. Use the `bug_review` triggers
   from `post-run-monitor` plus the Exploratory QA, observable execution-trace,
   and control-path reachability contract in `pulse-bug-review` as the audit
   contract. These references belong to the parent. Do
   not pass HTML style, skeleton, CSS, migration, or card-formatting work to the
   reviewer.
2. Choose the latest meaningful retained run when no run folder was supplied.
   Read its compact step results first. Deep-read only suspect attempts and the
   artifacts needed to prove or reject a defect.
3. Launch exactly one reviewer with
   `call_generic_agent(todo_id="standalone-bug-review",
   instructions="READ-ONLY REVIEW ...", preferred_tier=3,
   module="bug_review")`. Do not pass `pulse_run_id` or `review_run_id`; for
   this standalone command the backend generates both identities, stores the
   complete Markdown in SQLite, and files its `CONCERNS:` lines into the
   structured finding lifecycle. The reviewer may inspect artifacts, copied
   fixtures, scratch DBs, and side-effect-free tests. It must not edit workflow
   files, send external messages, publish, trade, post, mutate production data,
   ask the user, or launch another agent. It may read only matching Bug
   Review/open-finding regions of `builder/improve.html`; it must not format or
   write the page. `call_generic_agent` returns an `execution_id` immediately;
   end the current turn and resume only from the automatic completion
   notification.
4. Require: behavioral contract, QA coverage, expected versus observed,
   findings classified as `correctness_bug`, `efficiency_or_coaching`,
   `no_issue`, or `insufficient_evidence`, exact evidence, bounded recommended
   fix, verification, confidence, and untested risk. The result must be a compact
   non-HTML packet with `module=bug_review`, `verdict`, `next_check`, and ordered
   findings. Every finding includes a stable `finding_id`, `target_key`,
   severity, plain-language summary, exact evidence, bounded
   `recommended_fix`, verification, and `user_judgment_required` with reason.
   When evidence proves the defect belongs to the shared harness/runtime/bridge
   or tool API, classify it as `issue_kind=harness_issue` and emit the exact
   `PULSE_FINDING_JSON` marker required by the injected artifact contract. Do
   not use that classification for a workflow that supplied wrong arguments,
   paths, credentials, IDs, or stale data. Include a minimal side-effect-free
   reproduction when possible; otherwise identify the precise reproduction
   limitation. Harness correctness fixes are platform-owned and do not require
   a user decision unless a genuine product-policy choice remains.
5. Read the persisted result with `get_pulse_state(view="review")` using the exact
   `review_run_id` and `module` supplied by the completion notification. Validate and
   deduplicate that complete result against `builder/improve.html`. As the
   parent, make one bounded update that appends one compact newest-first Bug
   Review entry with
   `data-pulse-section="signals" data-module="bug_review"` to that HTML. Do not
   modify the workflow, close findings, call Pulse module-state tools, or claim
   that a recommendation was fixed.

Finish with a short executive summary followed by all confirmed bugs in severity
order, what was tested, what remains untested, and which findings are ready for
`/engineering-review`. Do not truncate the findings to a Top 3.

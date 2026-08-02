# STANDALONE STRATEGY AUDITOR

Run the same read-only cross-run plan-versus-goal diagnosis used by Pulse,
without running Pulse Gate, Goal Advisor, the workflow, or any fixer.{{if .Focus}}

Focus especially on: {{.Focus}}.{{end}}{{if .RunFolder}}

Use `{{.RunFolder}}` as the newest evidence anchor, then compare it with the
smallest useful retained window.{{end}}

1. Load `read_skill(skills=[{"name":"builder-reference","path":"references/post-run-monitor.md"}])`,
   `read_skill(skills=[{"name":"builder-reference","path":"references/strategy-auditor.md"}])`,
   `read_skill(skills=[{"name":"builder-reference","path":"references/assumption-audit.md"}])`, and
   `read_skill(skills=[{"name":"builder-reference","path":"references/review-improve-log.md"}])`. The Strategy Auditor
   reference is the classification and evidence contract. These references
   belong to the parent; never give the reviewer HTML/CSS/formatting work.
2. Read the objective and success criteria from `soul/soul.md`, then inspect the
   current plan/config, planning changelog, evaluation/report contracts, prior
   Strategy Auditor results, compact retained run evidence, and bounded
   read-only aggregates/samples from existing workflow tables in
   `db/db.sqlite`.
3. Enforce the diagnostic order. Inspect the latest relevant Bug Review and
   current correctness evidence. If a current correctness bug invalidates the
   strategy window, do not make a strategy claim: classify `execution_bug` or
   `insufficient_evidence`, identify the affected path, and name the exact
   post-fix outcome-bearing checkpoint. This command does not run Bug Review or
   Goal Advisor.
4. Launch exactly one reviewer with
   `call_generic_agent(todo_id="standalone-strategy-auditor",
   instructions="READ-ONLY REVIEW ...", preferred_tier=3,
   module="strategy_auditor")`. Do not pass `pulse_run_id` or `review_run_id`;
   for this standalone command the backend generates both identities, stores
   the complete Markdown in SQLite, and files its `CONCERNS:` lines into the
   structured finding lifecycle. The reviewer must not edit files or databases,
   run producing actions, publish, notify, ask the user, create/consume
   decisions, update Pulse state, or launch another agent. SQLite access is
   read-only. `call_generic_agent` returns an `execution_id` immediately; end
   the current turn and resume only from the automatic completion notification.
5. Require one primary classification: `strategy_flaw`, `execution_bug`,
   `measurement_gap`, `insufficient_evidence`, or `no_material_problem`.
   Require the goal/causal chain, evidence window and exclusions,
   activity-versus-outcome comparison with counts and denominators, segments,
   lag, perfect-execution counterfactual, competing explanations, and exact
   next checkpoint. Return a compact non-HTML packet with
   `module=strategy_auditor`, `verdict`, `next_check`, and every evidence-backed
   ordered finding. Each finding includes stable `finding_id`, `target_key`, severity,
   claim/mechanism, exact evidence, confidence, `recommended_fix` limited to an
   evidence or module handoff rather than a plan mutation, verification, and
   `user_judgment_required` with reason.
6. Read the persisted result with `get_pulse_review_result` using the exact
   `review_run_id` and `module` supplied by the completion notification. Validate and
   deduplicate that complete result against `builder/improve.html`. As the
   parent, append one compact newest-first diagnostic entry with
   `data-pulse-section="signals" data-module="strategy_auditor"`. Do not edit
   the plan, configuration, DB, reports/evals, or Pulse module state.
   Do not launch `/goal-advisor` automatically.

Finish with a short executive summary followed by every finding in severity
order, the evidence boundary, and whether a later `/goal-advisor` run is
warranted. Do not truncate the result to a Top 3.

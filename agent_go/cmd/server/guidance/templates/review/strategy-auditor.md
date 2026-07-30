# STANDALONE STRATEGY AUDITOR

Run the same read-only cross-run plan-versus-goal diagnosis used by Pulse,
without running Pulse Gate, Goal Advisor, the workflow, or any fixer.{{if .Focus}}

Focus especially on: {{.Focus}}.{{end}}{{if .RunFolder}}

Use `{{.RunFolder}}` as the newest evidence anchor, then compare it with the
smallest useful retained window.{{end}}

1. Load `get_reference_doc(kind="post-run-monitor")`,
   `get_reference_doc(kind="strategy-auditor")`,
   `get_reference_doc(kind="assumption-audit")`, and
   `get_reference_doc(kind="review-improve-log")`. The Strategy Auditor
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
4. Launch exactly one generic reviewer with a prompt beginning
   `READ-ONLY REVIEW`. It must not edit files or databases, run producing
   actions, publish, notify, ask the user, create/consume decisions, update
   Pulse state, or launch another agent. SQLite access is read-only.
5. Require one primary classification: `strategy_flaw`, `execution_bug`,
   `measurement_gap`, `insufficient_evidence`, or `no_material_problem`.
   Require the goal/causal chain, evidence window and exclusions,
   activity-versus-outcome comparison with counts and denominators, segments,
   lag, perfect-execution counterfactual, competing explanations, and exact
   next checkpoint. Return a compact non-HTML packet with
   `module=strategy_auditor`, `verdict`, `next_check`, and at most five ordered
   findings. Each finding includes stable `finding_id`, `target_key`, severity,
   claim/mechanism, exact evidence, confidence, `recommended_fix` limited to an
   evidence or module handoff rather than a plan mutation, verification, and
   `user_judgment_required` with reason.
6. Validate and deduplicate the complete result against
   `builder/improve.html`. As the parent, append one compact newest-first
   diagnostic entry with
   `data-pulse-section="signals" data-module="strategy_auditor"`. Do not edit
   the plan, configuration, DB, reports/evals, or Pulse module state.
   Do not launch `/goal-advisor` automatically.

Finish with a short executive summary followed by every finding in severity
order, the evidence boundary, and whether a later `/goal-advisor` run is
warranted. Do not truncate the result to a Top 3.

# STANDALONE STRATEGY AUDITOR

Run the same read-only cross-run plan-versus-goal diagnosis used by Pulse,
without running Pulse Gate, Goal Advisor, the workflow, or any fixer.{{if .Focus}}

Focus especially on: {{.Focus}}.{{end}}{{if .RunFolder}}

Use `{{.RunFolder}}` as the newest evidence anchor, then compare it with the
smallest useful retained window.{{end}}

Read `workflow.json`. If `pulse.advisor_specialization.strategy_auditor` is
active, apply it as the owner-approved workflow-specific lens subordinate to
this canonical role and the current `soul.md`/plan. It may specialize what to
inspect; it may not turn this into Goal Advisor, Engineering, or Ops review.

1. Load `read_skill(skills=[{"name":"builder-reference","path":"references/strategy-auditor.md"}])`,
   `read_skill(skills=[{"name":"builder-reference","path":"references/assumption-audit.md"}])`. The Strategy Auditor
   reference is the classification and evidence contract. These references
   belong to the parent; never give the reviewer HTML/CSS/formatting work.
2. Read the objective and success criteria from `soul/soul.md`, then inspect the
   current plan/config, planning changelog, evaluation/report contracts, prior
   Strategy Auditor results, compact retained run evidence, and bounded
   read-only aggregates/samples from existing workflow tables in
   `db/db.sqlite`.
3. Judge current correctness evidence directly without waiting for or consuming
   Bug Review, Artifact Review, or Goal Advisor conclusions. If evidence is
   unreliable, classify `execution_bug` or `insufficient_evidence`, identify the
   affected path, and name the exact next outcome-bearing checkpoint.
4. Launch exactly one reviewer with
   `run_in_background(name="Standalone Strategy Audit", instruction="READ-ONLY STRATEGY AUDIT ...", agent_type="executor")`.
   The reviewer must not edit files or databases,
   run producing actions, publish, notify, ask the user, create/consume
   decisions, update Pulse state, or launch another agent. SQLite access is
   read-only. `run_in_background` returns an `execution_id` immediately; end
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
6. Read the child completion and validate its evidence against the actual
   artifacts. Do not edit the plan, configuration, DB, reports/evals, or
   Pulse module state.
   Do not launch `/goal-advisor` automatically.

Finish with a short executive summary followed by every finding, bounded
in-plan recommendation, and evidence boundary in severity order. Do not truncate
the result to a Top 3.

# STANDALONE LLM AND OPERATIONS REVIEW

Run the same unified, agentic, read-only Ops Review used by Pulse. It owns cost,
timing, LLM selection, tool calling, runtime operations, setup, and plan-design
hygiene. Do not change models, tiers, fallbacks, schedules, notification
recipients, backup, publish, or credentials in this command.{{if .Focus}}

Focus especially on: {{.Focus}}.{{end}}{{if .RunFolder}}

Use `{{.RunFolder}}` as the primary run folder.{{end}}

1. Load `get_reference_doc(kind="post-run-monitor")`,
   `get_reference_doc(kind="llm-selection")`, and
   `get_reference_doc(kind="review-improve-log")`. These references belong to
   the parent. Do not pass HTML style, skeleton, CSS, migration, or card-format
   work to the reviewer.
2. Inspect the current trustworthy Goal verdict, resolved workflow/step/eval
   LLM configuration, actual model/tier use, fallbacks, cost ledgers, token
   usage, timing summaries, representative conversation/tool traces, retained
   `efficiency_or_coaching` findings, workflow version, and current
   backup/publish/notify readiness. Sample comparable earlier runs when needed
   to establish recurrence; do not open every trace. Use retained evidence, not
   provider assumptions or generic best practices.
3. Launch exactly one reviewer with
   `call_generic_agent(todo_id="standalone-ops-review",
   instructions="READ-ONLY REVIEW ...", preferred_tier=3,
   module="llm_ops_review")`. Do not pass `pulse_run_id` or `review_run_id`;
   for this standalone command the backend generates both identities, stores
   the complete Markdown in SQLite, and files its `CONCERNS:` lines into the
   structured finding lifecycle. The reviewer must not edit files or config,
   create questions, publish, notify, run the workflow, call Pulse module-state
   tools, or launch another agent. It may read only matching
   LLM/Ops/open-finding regions of `builder/improve.html`; it must not format or
   write the page.
4. Require the reviewer to check all of the following agentically:
   event correlation; nested JSON/MCP/shell-envelope interpretation; argument
   identity; failure-status precedence; errors hidden in nominal success; HTTP
   and path/database failures; retries and duplicate calls; measured versus
   missing timing and timeout risk; serial/parallel and batching opportunities;
   payload imbalance and truncation; cost attribution without double-counting;
   missing/unpriced evidence; recurrence across runs; whether tool results were
   interpreted and used correctly; and whether the chosen tool, source,
   workspace, run, group, table, endpoint, IDs, filters, time window, and
   destination were semantically correct. A zero duration is unmeasured, not
   instant. A zero exit code containing explicit error evidence is suspicious,
   not clean. Distinguish proven failure, review candidate, and evidence gap.
   Use judgment to decide necessity, impact, and the recommendation; do not
   assume a deterministic Go detector has already classified the trace.
5. Reconcile raw ledgers before judging cost. Preserve
   `date + scope + group_folder + run_folder`. Treat `by_model` as authoritative
   and `by_step_and_model` as included attribution; never add both. Report a
   positive remainder as unattributed/orchestrator, do not double-count an
   explicit `workflow_orchestrator` row, and report overflow, missing buckets,
   and unpriced calls instead of estimating.
6. Inventory exact model pins in explicit workflow roles and
   planning/evaluation step config. Call `list_provider_models` once per pinned
   provider and compare against its catalog and `default_tier_models`; never
   infer recency from model names. Provider-profile defaults update automatically
   and are not stale pins.
7. Require a compact result grouped by `cost`, `time`,
   `tool/runtime reliability`, `quality`, and `setup`. Every recommendation
   needs current state, exact suggestion, expected benefit, risk, and evidence.
   Separate evidence gaps from true optimization opportunities. If a material goal criterion is below target,
   forbid tier/model downgrades for outcome-bearing, reasoning, diagnostic,
   recovery, eval, and verification steps. A downgrade is eligible only for a
   deterministic non-bottleneck step with representative evidence proving
   quality-equivalent output and no downstream outcome loss; label it as an
   approval-required reversible trial. Missing evidence means keep the tier.
   For an unavailable/deprecated pin or a materially useful newer provider-owned
   default, recommend either clearing the pin to inherit its tier or one exact
   supported replacement. Label it user approval required and include current
   model, affected roles/steps, capability/cost/reasoning comparison, expected
   benefit, and risk. A newer model is not automatically better.
   Return a non-HTML packet with `module=llm_ops_review`, `verdict`, `next_check`,
   and ordered findings. Every finding includes a stable `finding_id`,
   `target_key`, severity, plain-language summary, exact evidence, bounded
   `recommended_fix`, verification, and `user_judgment_required` with reason.
   A shared harness/runtime/bridge/tool-API defect must be classified as
   `issue_kind=harness_issue` and carry the exact `PULSE_FINDING_JSON` marker
   from the injected artifact contract, including expected versus observed and
   a minimal safe reproduction or an explicit limitation. Wrong workflow
   arguments, paths, credentials, IDs, and data remain workflow findings. A
   harness issue is platform-owned, not a user-decision request, unless the
   remaining question is genuinely product policy.
8. Read the persisted result with `get_pulse_review_result` using the exact
   `review_run_id` and `module` returned by `call_generic_agent`. Validate and
   deduplicate that result against `builder/improve.html`. As the parent, make
   one bounded update that refreshes one compact LLM & operations review area
   with
   `data-pulse-section="signals" data-module="llm_ops_review"` in that HTML. Do not
   apply recommendations or create approval cards in this read-only command.

Finish with a short executive summary followed by every evidence-backed
recommendation in severity order. Identify which exact changes require user
approval before `/pulse-fixer` can apply them. Do not truncate the result to a
Top 3.

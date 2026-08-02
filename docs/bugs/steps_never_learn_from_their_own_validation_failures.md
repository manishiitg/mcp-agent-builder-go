# Bug Report: A Step Is Never Told Its Own Output Failed Validation

## Status

Fixed 2026-08-02 for agentic steps in
`controller_execution.go` / `run_concerns.go` / `execution_only_agent.go`.

Two related gaps are **open and not fixed**: `bug_review` sees these findings and
does not act on them, and scripted steps that *succeed* are invisible to review
entirely. Both are described below.

## Symptom

`tectonicusadaytrading`, step `deliver-briefing`:

```text
prevalidation gate failed at delivery_receipt.json
  $.delivery_status    must exist but was not found: unknown key delivery_status
  $.actionable_count   must exist but was not found: unknown key actionable_count
  $.idea_count         Expected number, got map[string]interface{}
```

Five findings, first seen 2026-07-29, still open at `seen_count 3` on 07-30. Not
flaky — a stable contract mismatch reproduced identically on every run.

## Root cause

The step **is** given its schema (`templateVars["ValidationSchema"]`,
`controller_execution.go:1588`). But prevalidation runs *after* the step, and
nothing carried the verdict forward. So every run:

```text
step reads schema → writes wrong shape → prevalidation fails → concern filed
  → next run reads an identical prompt → writes the same wrong shape
```

The information existed, was durable, and was never shown to the only party able
to act on it.

The scripted path already solved this and the solution was not generalised:

```go
templateVars["ScriptedPriorScript"] = learnCodePriorScript
templateVars["ScriptedPriorError"]  = learnCodePriorError
```

A failed script is handed its own error and repairs itself. An agentic step had
no equivalent.

## Fix

`LoadPriorPreValidationFailures(ctx, workspacePath, stepID, limit)` reads the
step's still-open `phase='prevalidation'` concerns;
`FormatPriorPreValidationFailures` renders them; `controller_execution.go` sets
`templateVars["PriorValidationFailures"]`; the execution prompt gains a
**"Previous Validation Failures — Fix These"** block directly after the schema.

Reads `run_concerns` rather than the prevalidation log because those rows are
already durable, already deduplicated by fingerprint, and already carry
`seen_count`. Recurrence is the strongest signal that a step is not learning, and
it comes free.

The rendered text also gives the step an exit that is not "write output I know
will fail":

> if the schema itself is wrong, say so explicitly in your summary as a
> `CONCERNS:` line rather than writing output you know will fail.

Scoped to the step and to prevalidation — a step must not be handed another
step's problems, and review-phase findings are Pulse's to route. Empty renders
nothing, so a step that has never failed sees no block at all.

Three tests cover the carry-forward, the scoping, and the empty case.

## Open gap 1: `bug_review` sees these and does not act

Not a hypothesis:

```text
concerns first seen   2026-07-29 14:51
bug_review ran        2026-07-30 20:05   decision=due  result=changed
concerns last seen    2026-07-30 18:22   seen_count=3   still open
```

`bug_review` was due, ran, reported `changed` — so it fixed *something* — and
left these five. Two structural reasons, neither of them reviewer carelessness:

- **They do not look like bugs from its angle.** Its brief is reliability and
  exploratory QA: execution logs, stale artifacts, hallucinated success. A
  schema-key mismatch is a contract defect and the step "succeeded".
- **The Fixer's queue may not contain them.** They are
  `step_id=deliver-briefing`, which is not a module. Scoped to
  `module=bug_review`, `get_pulse_finding_backlog` filters them out unless they
  are adopted through a linked fix attempt — and that adoption path has **never
  been used**: 0 of 21 attempts across both workflows target a step concern.

Two 2026-08-02 changes make them reachable — the `pulse_fixer` sentinel now
resolves to the complete backlog, and the standalone Fixer prompt works the whole
backlog with no module filter. Reachable is not the same as prioritised.

## Open gap 2: a *succeeding* scripted step is invisible to review

The sharper long-term risk, and it is not covered by the fix above.

A scripted step runs `main.py`. It has no LLM turn, so no completion summary and
no `CONCERNS:` line — its only feedback channel is prevalidation. If its logic
drifts out of date while still exiting 0 and producing schema-valid output,
**nothing reports anything**. It looks like success indefinitely.

`bug_review` cannot close this gap with its current method. Its trace review is
explicitly:

> follow the post-run-monitor Observable execution-trace review contract and
> inspect only its latest applicable `*-conversation.json`
> (conversation_history, tool_calls, llm_calls), or message-sequence
> `session.json`

A scripted step produces neither. Measured on social-media's execution folder:

```text
*conversation.json   0
session.json         6      (message-sequence steps)
```

So the primary review method has nothing to read for scripted steps. What
`bug_review` can still inspect is their *output* — but output that is stale and
schema-valid is exactly the case that looks fine.

`strategy_auditor` is the better-placed module, because it reasons about
outcomes rather than traces: it reconstructs goal → action → target/source →
outcome across comparable runs and tests for saturation, diminishing returns, and
proxy optimisation. Drifting logic that still produces well-formed output shows
up there as an outcome that stops moving while activity continues.

But it is not currently aimed at this. It has no notion of "this step's code has
not changed in N runs while its inputs have", and on the workflow above it was
`skipped` and last ran 2026-07-30.

Neither module owns this today. Worth a deliberate decision rather than an
assumption that review covers it. A concrete cheap signal, if it is wanted: a
scripted step whose `main.py` is unchanged across many runs while its upstream
inputs or the world it queries have changed is a candidate for re-derivation, and
that comparison is arithmetic over data the system already keeps.

## Related

- `docs/bugs/custom_tool_category_as_agent_addressing.md` — same family: the
  party who could act is not told what the system already knows.
- `docs/workflow/learn_code_flow.md` — the scripted/`learn_code` execution path.

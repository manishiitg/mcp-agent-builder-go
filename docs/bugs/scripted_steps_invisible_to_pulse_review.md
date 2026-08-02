# Scripted Steps Were Invisible to Pulse Review

## Status

Prompt changes written 2026-08-02 in `agent_go/cmd/server/scheduler.go`
(`bug_review` and `strategy_auditor` module briefs).

**Not yet verified.** `mcpagent` is mid-refactor and its working tree does not
compile, and `agent_go` builds against it through a local `replace`, so
`go test ./cmd/server/ -run TestPostRunMonitor` — which asserts these prompt
strings — could not be run. Re-run it once that tree settles. `gofmt` is clean
and the edits are prompt text only.

Written for independent review. The measurements below are reproducible; the
design split was the operator's call and is recorded as such.

## The problem

A scripted step runs `main.py`. It has no LLM turn, so it produces no completion
summary and no `CONCERNS:` line. Its only automatic feedback is prevalidation,
which checks output *shape*, not correctness.

That leaves a specific and durable blind spot: **a script whose logic has drifted
out of date, but which still exits 0 and produces schema-valid output, is
reported as a success indefinitely.** Nothing in the system disagrees with it.

`bug_review` could not close this with its stated method. Its trace contract was:

> follow the post-run-monitor Observable execution-trace review contract and
> inspect only its latest applicable `*-conversation.json`
> (conversation_history, tool_calls, llm_calls), or message-sequence
> `session.json`

Both artifacts are agentic. Measured on one workflow's run tree:

```text
scripted steps with code/main.py     28
*-conversation.json in same tree      5
```

So the reviewer was told to look for two things a scripted step never produces,
and the absence of a trace reads as nothing to review rather than as a step
outside the method's reach.

## What was wrong with the first two diagnoses

Recorded because both were plausible and both were wrong.

**"Scripted steps leave nothing to review, so point the reviewer at
`code/main.py`."** Reading source is a weaker signal than reading a run, and it
would have made review of a *correct* script indistinguishable from review of a
drifted one.

**"Scripted steps have no run logs at all."** They do. The first `find` that
returned zero was run from the wrong working directory. Corrected:

```text
scripted_fast_path.json on disk   107
logs/ dirs under runs/            120
```

`saveScriptedFastPathLog` has been writing
`<run>/logs/<step>/execution/scripted_fast_path.json` all along:

```text
mode              scripted_fast_path
script_path       …/main.py
exit_code         0
success           true
output            [inputs] draft=…          ← the script's own stdout
error · execution_error · validation_error · failure_reason
timestamp
```

**The gap was never missing logs.** It was a review contract that named two
artifacts a scripted step cannot produce and did not mention the one it does.

## The design split

Operator decision, and it matches what each module is actually good at:

| Module | Owns |
|---|---|
| `bug_review` | Ordinary correctness defects in scripted steps |
| `strategy_auditor` | Drift — a still-correct script that no longer fits the goal |

The boundary is stated in both prompts so neither defers to the other.

### `bug_review`

Now directed at the real trace, and given the scripted forms of the failure
modes it already looks for:

> Its equivalent trace is `logs/<step>/execution/scripted_fast_path.json`, which
> records the script path, exit code, success, captured stdout, and any
> execution/validation error. Read that first, then its `code/main.py`, declared
> `validation_schema`, output files, and prevalidation results. Judge the run the
> same way you judge a trace: **`exit_code` 0 with empty or unchanged output is
> the scripted form of hallucinated success**, and `success=true` alongside a
> non-empty `validation_error` is a contradiction worth a finding.

Plus the explicit handoff: *"Whether a correct script has become outdated as the
world moved is not yours; that belongs to strategy_auditor."*

### `strategy_auditor`

Drift added to its probe list, phrased in the causal-chain terms it already uses:

> a saved `main.py` is frozen logic executing against a world that moves, and it
> keeps exiting 0 with schema-valid output long after it stopped being right, so
> nothing else in Pulse will flag it … Unchanged logic plus flat or declining
> outcomes, or unchanged logic while its inputs, sources, or upstream schema
> changed, is a drift candidate … **never infer drift from age alone.**

That last clause matters. A stable script is the normal case and usually correct;
without it the module would file drift findings against every step that has not
needed to change.

The signal is measurable today. `authenticated-known-baseline-audit`:

```text
iteration-19  2026-07-25   main.py sha eff7e9974d71
iteration-21  2026-08-02   main.py sha eff7e9974d71   ← byte-identical, 8 days apart
```

Comparing that against the outcomes those runs produced is arithmetic over data
already retained.

## For the reviewer: what to check

1. **Does the `scripted_fast_path.json` path in the prompt match reality across
   step kinds?** It was confirmed for `linkedin/step-p3b-image-gen`. Nested
   sub-agent steps and `todo_task` steps write through
   `getExecutionFolderPathForLogs` too but were not individually verified.
2. **Is `bug_review`'s brief now too long?** These modules already carry large
   prompts, and this adds a paragraph. If it displaces attention from the
   agentic path, the trade is bad.
3. **Is drift the right home for `strategy_auditor`?** It runs less often than
   `bug_review` — on `tectonicusadaytrading` it was `skipped` and last ran
   2026-07-30. Correct ownership with a cadence that never fires is not a fix.
4. **Should the fast-path log capture more?** It records stdout and exit code but
   not the script hash. A hash would make drift detection exact rather than a
   path comparison, and it is one line at the write site.

## Related

- `docs/bugs/steps_never_learn_from_their_own_validation_failures.md` — the
  agentic half of the same gap: a step told what shape to produce and never told
  its last output failed.
- `docs/workflow/learn_code_flow.md` — the scripted/`learn_code` execution path.

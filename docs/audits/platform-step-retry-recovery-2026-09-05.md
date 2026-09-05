# Individual-step retry recovery

## Finding

The workshop single-step path could successfully retry a failed step without
recording recovery against the original run metadata. The scheduler consequently
reported the workflow failure even when the workshop session ended normally.
The Upwork report PUL-9C0D14D3 also involved a plan edit between failure and retry;
that case does not establish successful completion of the original workflow.

## Local implementation

- Record required top-level step IDs and durable completion receipts for each
  execution. This is recovery evidence, not a restored plan/progress cache.
  Existing file-backed step-progress persistence is disabled.
- Capture the failed execution ID and plan revision before a targeted retry.
  Clear that step's earlier receipt before executing it.
- Promote failed to completed only with receipts for every required step,
  matching execution and revision, fresh evidence for the retried step, no
  recorded persistence errors, and an unchanged current execution contract.
- Otherwise record `step_recovered_run_unverified`, retaining failed status.
- Preserve the original failure status, completion time, and duration in
  `recovery_history`; expose recovery metadata to the run loader. The scheduler
  explains unverified recovery rather than reporting a contradictory generic
  session-success/run-failure message.
- Guard metadata mutations with bounded in-process locks and execution-ID
  checks. Recovery receipt writes reject missing/corrupt metadata rather than
  recreating it. A new execution clears old required IDs, receipts and verdict.

## Validation and limitations

Focused tests cover full and incomplete evidence, duplicate/missing IDs, foreign
execution/revision receipts, stale evidence, changed plans, reused execution
slots, persistence errors, terminal/newer failures, and workspace API receipt
round-tripping. Existing scheduler reconciliation and plan-snapshot tests pass.

This does not retroactively prove legacy runs lacking receipts, infer completion
of unexecuted routing alternatives, or rewrite historical schedule records.
Inner-step and evaluation retries do not promote whole workflow runs. Locks are
in-process, not a distributed transaction across independent server processes.

No live SQLite rows or schedule statuses were changed. PUL-9C0D14D3 should not be
closed as a verified recovered run: its changed plan still requires whole-run
verification. Changes are local; deployment and live verification remain.
